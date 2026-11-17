//go:build darwin && cgo

package capture

// ScreenCaptureKit + VideoToolbox köprüsü. desktop/spikes/capture-go/capture_darwin.m
// dosyasından taşındı (sentetik kaynak ve bellek ölçümü çıkarıldı, ekran
// kökeni eklendi). Go'ya dışa aktarılan geri çağrılar capture_darwin.go'dadır;
// //export kullanan dosyanın önsözü tanım içeremediği için kod burada durur.

/*
extern void goCaptureEncoded(void *p, int n, int key, long long pts);
extern void goCaptureError(char *msg);

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <VideoToolbox/VideoToolbox.h>
#include <stdatomic.h>


static SCStream *gStream;
static VTCompressionSessionRef gSession;
static dispatch_queue_t gQueue;
static atomic_int gForceKey;
static id gOutput; // güçlü referans

int cf_preflight(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }
int cf_request(void) { return CGRequestScreenCaptureAccess() ? 1 : 0; }

void cf_display_info(double *ox, double *oy, int *wpt, int *hpt, int *wpx, int *hpx) {
    CGDirectDisplayID d = CGMainDisplayID();
    CGRect b = CGDisplayBounds(d);
    *ox = b.origin.x;
    *oy = b.origin.y;
    *wpt = (int)b.size.width;
    *hpt = (int)b.size.height;
    CGDisplayModeRef m = CGDisplayCopyDisplayMode(d);
    if (m) {
        *wpx = (int)CGDisplayModeGetPixelWidth(m);
        *hpx = (int)CGDisplayModeGetPixelHeight(m);
        CGDisplayModeRelease(m);
    } else {
        *wpx = *wpt;
        *hpx = *hpt;
    }
}

void cf_force_keyframe(void) { atomic_store(&gForceKey, 1); }

// AVCC (4 bayt uzunluk) -> Annex-B (00 00 00 01). Anahtar karede SPS/PPS başa eklenir.
static void encodedCallback(void *refcon, void *frameRef, OSStatus status, VTEncodeInfoFlags flags,
                            CMSampleBufferRef sb) {
    if (status != noErr || sb == NULL || (flags & kVTEncodeInfo_FrameDropped)) return;
    if (!CMSampleBufferDataIsReady(sb)) return;

    int key = 1;
    CFArrayRef att = CMSampleBufferGetSampleAttachmentsArray(sb, false);
    if (att && CFArrayGetCount(att) > 0) {
        CFDictionaryRef d = CFArrayGetValueAtIndex(att, 0);
        CFBooleanRef notSync = NULL;
        if (CFDictionaryGetValueIfPresent(d, kCMSampleAttachmentKey_NotSync, (const void **)&notSync) &&
            notSync && CFBooleanGetValue(notSync)) {
            key = 0;
        }
    }

    CMBlockBufferRef bb = CMSampleBufferGetDataBuffer(sb);
    size_t total = CMBlockBufferGetDataLength(bb);
    static const uint8_t sc[4] = {0, 0, 0, 1};

    NSMutableData *out = [NSMutableData dataWithCapacity:total + 256];
    if (key) {
        CMFormatDescriptionRef fd = CMSampleBufferGetFormatDescription(sb);
        size_t count = 0;
        CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fd, 0, NULL, NULL, &count, NULL);
        for (size_t i = 0; i < count; i++) {
            const uint8_t *ps = NULL;
            size_t psLen = 0;
            if (CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fd, i, &ps, &psLen, NULL, NULL) == noErr) {
                [out appendBytes:sc length:4];
                [out appendBytes:ps length:psLen];
            }
        }
    }

    // Blok bitişik değilse kopyala.
    char *ptr = NULL;
    size_t lenAt = 0;
    NSMutableData *tmp = nil;
    if (CMBlockBufferGetDataPointer(bb, 0, &lenAt, NULL, &ptr) != kCMBlockBufferNoErr || lenAt < total) {
        tmp = [NSMutableData dataWithLength:total];
        CMBlockBufferCopyDataBytes(bb, 0, total, tmp.mutableBytes);
        ptr = tmp.mutableBytes;
    }
    size_t off = 0;
    while (off + 4 <= total) {
        uint32_t nl = 0;
        memcpy(&nl, ptr + off, 4);
        nl = CFSwapInt32BigToHost(nl);
        off += 4;
        if (off + nl > total) break;
        [out appendBytes:sc length:4];
        [out appendBytes:ptr + off length:nl];
        off += nl;
    }

    CMTime pts = CMSampleBufferGetPresentationTimeStamp(sb);
    int64_t ptsNs = (int64_t)(CMTimeGetSeconds(pts) * 1e9);
    goCaptureEncoded((void *)out.bytes, (int)out.length, key, (long long)ptsNs);
}

@interface CFOutput : NSObject <SCStreamOutput, SCStreamDelegate>
@end

@implementation CFOutput
- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sb ofType:(SCStreamOutputType)type {
    if (type != SCStreamOutputTypeScreen || !CMSampleBufferIsValid(sb)) return;
    // Yalnız "complete" kareler yeni içerik taşır; ekran durağansa SCK kare göndermez.
    CFArrayRef att = CMSampleBufferGetSampleAttachmentsArray(sb, false);
    if (!att || CFArrayGetCount(att) == 0) return;
    CFDictionaryRef d = CFArrayGetValueAtIndex(att, 0);
    CFNumberRef st = CFDictionaryGetValue(d, (__bridge CFStringRef)SCStreamFrameInfoStatus);
    int status = -1;
    if (st) CFNumberGetValue(st, kCFNumberIntType, &status);
    if (status != SCFrameStatusComplete) {
        return;
    }
    CVImageBufferRef pb = CMSampleBufferGetImageBuffer(sb);
    if (!pb) return;

    CFDictionaryRef props = NULL;
    if (atomic_exchange(&gForceKey, 0)) {
        const void *k[] = {kVTEncodeFrameOptionKey_ForceKeyFrame};
        const void *v[] = {kCFBooleanTrue};
        props = CFDictionaryCreate(NULL, k, v, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    }
    VTCompressionSessionEncodeFrame(gSession, pb, CMSampleBufferGetPresentationTimeStamp(sb),
                                    CMSampleBufferGetDuration(sb), props, NULL, NULL);
    if (props) CFRelease(props);
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
    goCaptureError((char *)error.localizedDescription.UTF8String);
}
@end

static void setProp(CFStringRef k, CFTypeRef v) { VTSessionSetProperty(gSession, k, v); }

static int makeEncoder(int w, int h, int fps, int kbps, char *errbuf, int errlen) {
    // Donanım kodlayıcı + düşük gecikmeli hız denetimi (WebRTC'nin kullandığı yol).
    NSDictionary *spec = @{
        (__bridge NSString *)kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder : @YES,
        (__bridge NSString *)kVTVideoEncoderSpecification_EnableLowLatencyRateControl : @YES,
    };
    NSDictionary *src = @{
        (__bridge NSString *)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange),
    };
    OSStatus s = VTCompressionSessionCreate(NULL, w, h, kCMVideoCodecType_H264, (__bridge CFDictionaryRef)spec,
                                            (__bridge CFDictionaryRef)src, NULL, encodedCallback, NULL, &gSession);
    if (s != noErr) {
        // Düşük gecikme modu desteklenmezse normal donanım oturumu dene.
        NSDictionary *spec2 = @{
            (__bridge NSString *)kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder : @YES,
        };
        s = VTCompressionSessionCreate(NULL, w, h, kCMVideoCodecType_H264, (__bridge CFDictionaryRef)spec2,
                                       (__bridge CFDictionaryRef)src, NULL, encodedCallback, NULL, &gSession);
        if (s != noErr) {
            snprintf(errbuf, errlen, "VTCompressionSessionCreate hata %d", (int)s);
            return -1;
        }
        fprintf(stderr, "uyarı: düşük gecikmeli hız denetimi yok, normal donanım kodlayıcı\n");
    }
    setProp(kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
    setProp(kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
    setProp(kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_ConstrainedBaseline_AutoLevel);
    setProp(kVTCompressionPropertyKey_AverageBitRate, (__bridge CFTypeRef) @(kbps * 1000));
    setProp(kVTCompressionPropertyKey_ExpectedFrameRate, (__bridge CFTypeRef) @(fps));
    setProp(kVTCompressionPropertyKey_MaxKeyFrameInterval, (__bridge CFTypeRef) @(fps * 4));
    setProp(kVTCompressionPropertyKey_MaxKeyFrameIntervalDuration, (__bridge CFTypeRef) @4);
    VTCompressionSessionPrepareToEncodeFrames(gSession);
    return 0;
}

int cf_start(int out_w, int out_h, int fps, int bitrate_kbps, char *errbuf, int errlen) {
    // Komut satırı aracında CGS bağlantısı için NSApplication gerekir.
    [NSApplication sharedApplication];

    __block SCShareableContent *content = nil;
    __block NSError *cerr = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    [SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent *c, NSError *e) {
      content = c;
      cerr = e;
      dispatch_semaphore_signal(sem);
    }];
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    if (!content) {
        snprintf(errbuf, errlen, "SCShareableContent: %s", cerr ? cerr.localizedDescription.UTF8String : "boş");
        return -1;
    }
    SCDisplay *display = nil;
    CGDirectDisplayID mainID = CGMainDisplayID();
    for (SCDisplay *d in content.displays) {
        if (d.displayID == mainID) display = d;
    }
    if (!display) display = content.displays.firstObject;
    if (!display) {
        snprintf(errbuf, errlen, "ekran bulunamadı");
        return -1;
    }

    if (makeEncoder(out_w, out_h, fps, bitrate_kbps, errbuf, errlen) != 0) return -1;

    SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
    SCStreamConfiguration *cfg = [SCStreamConfiguration new];
    cfg.width = out_w;
    cfg.height = out_h;
    cfg.minimumFrameInterval = CMTimeMake(1, fps);
    cfg.pixelFormat = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
    cfg.showsCursor = YES;
    cfg.queueDepth = 5;

    CFOutput *o = [CFOutput new];
    gOutput = o;
    gQueue = dispatch_queue_create("clickfix.capture", DISPATCH_QUEUE_SERIAL);
    gStream = [[SCStream alloc] initWithFilter:filter configuration:cfg delegate:o];
    NSError *aerr = nil;
    if (![gStream addStreamOutput:o type:SCStreamOutputTypeScreen sampleHandlerQueue:gQueue error:&aerr]) {
        snprintf(errbuf, errlen, "addStreamOutput: %s", aerr.localizedDescription.UTF8String);
        return -1;
    }
    __block NSError *serr = nil;
    [gStream startCaptureWithCompletionHandler:^(NSError *e) {
      serr = e;
      dispatch_semaphore_signal(sem);
    }];
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    if (serr) {
        snprintf(errbuf, errlen, "startCapture: %s", serr.localizedDescription.UTF8String);
        return -1;
    }
    return 0;
}

void cf_stop(void) {
    if (gStream) {
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        [gStream stopCaptureWithCompletionHandler:^(NSError *e) {
          dispatch_semaphore_signal(sem);
        }];
        dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC));
        gStream = nil;
    }
    if (gSession) {
        VTCompressionSessionCompleteFrames(gSession, kCMTimeInvalid);
        VTCompressionSessionInvalidate(gSession);
        CFRelease(gSession);
        gSession = NULL;
    }
}
*/
import "C"

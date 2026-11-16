//go:build darwin && cgo

package capture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework CoreGraphics -framework CoreMedia -framework CoreVideo -framework VideoToolbox -framework ScreenCaptureKit
#include <stdlib.h>

int cf_preflight(void);
int cf_request(void);
void cf_display_info(double *ox, double *oy, int *wpt, int *hpt, int *wpx, int *hpx);
int cf_start(int out_w, int out_h, int fps, int bitrate_kbps, char *errbuf, int errlen);
void cf_stop(void);
void cf_force_keyframe(void);
*/
import "C"

import (
	"errors"
	"sync"
	"time"
	"unsafe"
)

// cgo geri çağrıları tek bir yakalamaya bağlıdır.
var (
	mu      sync.Mutex
	running *stream
)

type stream struct {
	onFrame func(Frame)
	onError func(error)
	once    sync.Once
}

//export goCaptureEncoded
func goCaptureEncoded(p unsafe.Pointer, n C.int, key C.int, pts C.longlong) {
	mu.Lock()
	s := running
	mu.Unlock()
	if s == nil || n <= 0 {
		return
	}
	s.onFrame(Frame{Data: C.GoBytes(p, n), Key: key != 0, PTS: time.Duration(pts)})
}

//export goCaptureError
func goCaptureError(msg *C.char) {
	mu.Lock()
	s := running
	mu.Unlock()
	if s != nil && s.onError != nil {
		s.onError(errors.New("capture: " + C.GoString(msg)))
	}
}

// ScreenPermission, ekran kaydı iznini istem açmadan denetler. macOS, süreç
// açıldıktan sonra verilen izni yeniden başlatmaya kadar görmeyebilir.
func ScreenPermission() Permission {
	if C.cf_preflight() != 0 {
		return Granted
	}
	return Denied
}

// RequestScreenPermission, sistem izin istemini açar (ilk seferde) ve durumu
// döner.
func RequestScreenPermission() Permission {
	if C.cf_request() != 0 {
		return Granted
	}
	return Denied
}

// MainDisplay, birincil ekranın boyutlarıdır.
func MainDisplay() (Display, error) {
	var ox, oy C.double
	var wpt, hpt, wpx, hpx C.int
	C.cf_display_info(&ox, &oy, &wpt, &hpt, &wpx, &hpx)
	if wpt <= 0 || hpt <= 0 {
		return Display{}, errors.New("capture: ekran bulunamadı")
	}
	d := Display{OriginX: float64(ox), OriginY: float64(oy), PointsW: int(wpt), PointsH: int(hpt), PixelsW: int(wpx), PixelsH: int(hpx)}
	d.Scale = float64(d.PixelsW) / float64(d.PointsW)
	return d, nil
}

// Start, birincil ekranı yakalamaya başlar. onFrame kodlayıcının
// kuyruğundan çağrılır ve hızlı dönmelidir.
func Start(cfg Config, onFrame func(Frame), onError func(error)) (Stream, error) {
	if ScreenPermission() != Granted {
		return nil, ErrPermission
	}
	mu.Lock()
	if running != nil {
		mu.Unlock()
		return nil, ErrBusy
	}
	s := &stream{onFrame: onFrame, onError: onError}
	running = s
	mu.Unlock()

	errbuf := (*C.char)(C.malloc(512))
	defer C.free(unsafe.Pointer(errbuf))
	if rc := C.cf_start(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), errbuf, 512); rc != 0 {
		C.cf_stop()
		mu.Lock()
		running = nil
		mu.Unlock()
		return nil, errors.New("capture: " + C.GoString(errbuf))
	}
	return s, nil
}

func (s *stream) ForceKeyframe() { C.cf_force_keyframe() }

func (s *stream) Stop() {
	s.once.Do(func() {
		C.cf_stop()
		mu.Lock()
		if running == s {
			running = nil
		}
		mu.Unlock()
	})
}

// Command capture-go, CF-235 aday B'dir: Go yan süreçte yerel ekran yakalama.
// Ana ekranı ScreenCaptureKit ile yakalar, VideoToolbox ile H.264 kodlar ve
// LiveKit odasına SCREEN_SHARE izi olarak yayınlar. Yalnız keşif içindir.
package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework CoreGraphics -framework CoreMedia -framework CoreVideo -framework VideoToolbox -framework ScreenCaptureKit
#include <stdlib.h>
#include "capture.h"
*/
import "C"

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type frame struct {
	data []byte
	key  bool
	pts  int64
}

var (
	frames      = make(chan frame, 30)
	nCaptured   atomic.Int64 // yeni içerikli kare
	nIdle       atomic.Int64 // içeriksiz (durağan ekran) bildirim
	nEncoded    atomic.Int64
	nKey        atomic.Int64
	nDropped    atomic.Int64
	nBytes      atomic.Int64
	nKeyRequest atomic.Int64
	streamErr   = make(chan string, 1)
)

//export goOnCaptured
func goOnCaptured(complete C.int) {
	if complete != 0 {
		nCaptured.Add(1)
	} else {
		nIdle.Add(1)
	}
}

//export goOnEncoded
func goOnEncoded(p unsafe.Pointer, n C.int, key C.int, pts C.longlong) {
	b := C.GoBytes(p, n)
	nEncoded.Add(1)
	nBytes.Add(int64(n))
	if key != 0 {
		nKey.Add(1)
	}
	select {
	case frames <- frame{data: b, key: key != 0, pts: int64(pts)}:
	default:
		nDropped.Add(1)
	}
}

//export goOnError
func goOnError(msg *C.char) {
	select {
	case streamErr <- C.GoString(msg):
	default:
	}
}

type tokenResp struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Room  string `json:"room"`
}

func fetchToken(harness, identity string) (tokenResp, error) {
	var t tokenResp
	u := harness + "/token?identity=" + url.QueryEscape(identity) + "&publish=1"
	resp, err := http.Get(u)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return t, fmt.Errorf("harness %s", resp.Status)
	}
	return t, json.NewDecoder(resp.Body).Decode(&t)
}

// outSize, istenen yüksekliğe göre en-boy oranını koruyarak çift sayılı boyut döner.
func outSize(res string, wpx, hpx int) (int, int, error) {
	var h int
	switch res {
	case "native":
		return wpx &^ 1, hpx &^ 1, nil
	case "1080":
		h = 1080
	case "2160":
		h = 2160
	default:
		return 0, 0, fmt.Errorf("-res 1080|2160|native olmalı")
	}
	if h > hpx {
		fmt.Printf("not: %dp ekranın piksel yüksekliğinden (%d) büyük, native kullanılıyor\n", h, hpx)
		return wpx &^ 1, hpx &^ 1, nil
	}
	w := int(float64(wpx)*float64(h)/float64(hpx)+0.5) &^ 1
	return w, h, nil
}

func cpuSeconds() float64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
}

func main() {
	res := flag.String("res", "1080", "çıkış çözünürlüğü: 1080|2160|native")
	fps := flag.Int("fps", 30, "hedef kare hızı")
	kbps := flag.Int("bitrate", 0, "hedef bit hızı (kbps), 0 = çözünürlüğe göre otomatik")
	harness := flag.String("harness", "http://127.0.0.1:7890", "harness adresi")
	identity := flag.String("identity", "customer-go", "LiveKit kimliği")
	smoke := flag.Bool("smoke", false, "yakalamadan odaya bağlanıp çık (izin istemez)")
	synthetic := flag.Bool("synthetic", false, "ekran yerine sentetik desen kodla ve yayınla (izin istemez)")
	dur := flag.Duration("duration", 0, "bu süre sonra dur (0 = Ctrl+C'ye kadar)")
	flag.Parse()

	tok, err := fetchToken(*harness, *identity)
	if err != nil {
		fail("token alınamadı: %v", err)
	}

	if *smoke {
		room, err := lksdk.ConnectToRoomWithToken(tok.URL, tok.Token, &lksdk.RoomCallback{}, lksdk.WithAutoSubscribe(false))
		if err != nil {
			fail("odaya bağlanılamadı: %v", err)
		}
		fmt.Printf("smoke: %s odasına %s olarak bağlandı, uzak katılımcı: %d\n", room.Name(), *identity, len(room.GetRemoteParticipants()))
		room.Disconnect()
		return
	}

	// İzin: CGPreflight sessizdir; yoksa sistem penceresini açıp çık.
	if *synthetic {
		fmt.Println("sentetik kaynak: ekran izni denetlenmiyor")
	} else if C.cf_preflight() == 0 {
		C.cf_request()
		fmt.Fprintln(os.Stderr, "Ekran kaydı izni yok. Sistem Ayarları > Gizlilik ve Güvenlik > Ekran ve Sistem Ses Kaydı'nda bu uygulamaya (terminal) izin verin, sonra uygulamayı yeniden başlatıp tekrar çalıştırın.")
		os.Exit(2)
	} else {
		fmt.Println("ekran kaydı izni: var")
	}

	var wpt, hpt, wpx, hpx C.int
	C.cf_display_info(&wpt, &hpt, &wpx, &hpx)
	w, h, err := outSize(*res, int(wpx), int(hpx))
	if err != nil {
		fail("%v", err)
	}
	if *kbps == 0 {
		// Ekran içeriği için kaba başlangıç: piksel*fps*0.08 bit, 1-20 Mbps arası.
		*kbps = int(float64(w*h**fps) * 0.08 / 1000)
		*kbps = max(1000, min(*kbps, 20000))
	}
	fmt.Printf("ekran: %dx%d nokta, %dx%d piksel; çıkış %dx%d @%d fps, %d kbps H.264 (VideoToolbox)\n",
		wpt, hpt, wpx, hpx, w, h, *fps, *kbps)

	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}, lksdk.WithRTCPHandler(func(p rtcp.Packet) {
		switch p.(type) {
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			nKeyRequest.Add(1)
			C.cf_force_keyframe()
		}
	}))
	if err != nil {
		fail("iz oluşturulamadı: %v", err)
	}

	room, err := lksdk.ConnectToRoomWithToken(tok.URL, tok.Token, &lksdk.RoomCallback{}, lksdk.WithAutoSubscribe(false))
	if err != nil {
		fail("odaya bağlanılamadı: %v", err)
	}
	defer room.Disconnect()
	if _, err := room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name: "screen", Source: livekit.TrackSource_SCREEN_SHARE, VideoWidth: w, VideoHeight: h,
	}); err != nil {
		fail("iz yayınlanamadı: %v", err)
	}
	fmt.Printf("%s odasına %s olarak bağlandı, iz yayında\n", room.Name(), *identity)

	// Kodlanmış kareleri LiveKit'e yaz; süre gerçek pts farkından.
	go func() {
		var last int64
		def := time.Second / time.Duration(*fps)
		for f := range frames {
			d := def
			if last != 0 && f.pts > last {
				d = time.Duration(f.pts - last)
			}
			last = f.pts
			_ = track.WriteSample(media.Sample{Data: f.data, Duration: d}, nil)
		}
	}()

	errbuf := (*C.char)(C.malloc(512))
	defer C.free(unsafe.Pointer(errbuf))
	var rc C.int
	if *synthetic {
		rc = C.cf_start_synthetic(C.int(w), C.int(h), C.int(*fps), C.int(*kbps), errbuf, 512)
	} else {
		rc = C.cf_start(C.int(w), C.int(h), C.int(*fps), C.int(*kbps), errbuf, 512)
	}
	if rc != 0 {
		fail("yakalama başlatılamadı: %s", C.GoString(errbuf))
	}
	defer C.cf_stop()
	fmt.Println("yakalama başladı (Ctrl+C ile dur)")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var stop <-chan time.Time
	if *dur > 0 {
		stop = time.After(*dur)
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	lastT, lastCPU := time.Now(), cpuSeconds()
	var lc, li, le, lb int64
	for {
		select {
		case <-sig:
			fmt.Println("durduruluyor")
			return
		case <-stop:
			fmt.Println("süre doldu")
			return
		case m := <-streamErr:
			fmt.Fprintln(os.Stderr, "yakalama durdu:", m)
			return
		case now := <-tick.C:
			dt := now.Sub(lastT).Seconds()
			cpu := cpuSeconds()
			c, i, e, b := nCaptured.Load(), nIdle.Load(), nEncoded.Load(), nBytes.Load()
			var rss, foot C.uint64_t
			C.cf_mem(&rss, &foot)
			fmt.Printf("[%s] yakalanan %.1f fps (boş %.1f/s), kodlanan %.1f fps, %.0f kbps, CPU %.1f%%, RSS %.0f MB, footprint %.0f MB, anahtar %d, PLI %d, düşen %d\n",
				now.Format("15:04:05"), float64(c-lc)/dt, float64(i-li)/dt, float64(e-le)/dt,
				float64(b-lb)*8/1000/dt, (cpu-lastCPU)/dt*100,
				float64(rss)/1e6, float64(foot)/1e6, nKey.Load(), nKeyRequest.Load(), nDropped.Load())
			lastT, lastCPU, lc, li, le, lb = now, cpu, c, i, e, b
		}
	}
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", a...)
	os.Exit(1)
}

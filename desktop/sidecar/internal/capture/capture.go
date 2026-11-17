// Package capture, birincil ekranı yakalar ve H.264 (Annex-B) karelere
// kodlar (CF-235 kararı B).
//
//   - macOS: ScreenCaptureKit (SCStream, NV12) + VideoToolbox donanım
//     kodlayıcısı, düşük gecikmeli hız denetimi. Kod desktop/spikes/capture-go
//     örneğinden taşındı (capture_darwin.go).
//   - Windows: henüz yok (Windows Graphics Capture + Media Foundation H.264
//     ayrı iştir); Start ErrUnsupported döner (capture_windows.go).
//   - Diğerleri: ErrUnsupported (Destek uygulaması Linux'ta yoktur).
//
// Aynı anda tek bir yakalama çalışır.
package capture

import (
	"errors"
	"time"
)

// ErrUnsupported, bu platformda ekran yakalama olmadığını söyler.
var ErrUnsupported = errors.New("capture: bu platformda ekran yakalama desteklenmiyor")

// ErrPermission, ekran kaydı izninin olmadığını söyler.
var ErrPermission = errors.New("capture: ekran kaydı izni yok")

// ErrBusy, başka bir yakalama sürerken Start çağrıldığını söyler.
var ErrBusy = errors.New("capture: yakalama zaten çalışıyor")

// Permission, bir işletim sistemi izninin durumudur (IPC permissions.status).
type Permission string

const (
	Granted     Permission = "granted"
	Denied      Permission = "denied"
	NotRequired Permission = "not_required"
	Unsupported Permission = "unsupported"
)

// Display, birincil ekranın boyutlarıdır. Nokta (point) işletim sisteminin
// mantıksal koordinatıdır (macOS CGEvent, robotgo); piksel gerçek
// çözünürlüktür.
type Display struct {
	OriginX float64 `json:"-"`
	OriginY float64 `json:"-"`
	PointsW int     `json:"-"`
	PointsH int     `json:"-"`
	PixelsW int     `json:"width"`
	PixelsH int     `json:"height"`
	Scale   float64 `json:"scale"`
}

// Frame, kodlanmış tek bir H.264 erişim birimidir (Annex-B; anahtar karede
// SPS/PPS başta).
type Frame struct {
	Data []byte
	Key  bool
	PTS  time.Duration
}

// Config, yakalama ayarıdır.
type Config struct {
	Width, Height int // kodlanan boyut (çift)
	FPS           int
	BitrateKbps   int
}

// Varsayılanlar: 1080p üstü ekranlar 1080 satıra ölçeklenir (CF-235
// ölçümünde 25-29 fps, %3-6 CPU).
const (
	DefaultMaxHeight = 1080
	DefaultFPS       = 30
)

// OutSize, piksel boyutundan en-boy oranını koruyarak çift sayılı kodlama
// boyutunu seçer; yükseklik maxH'yi aşmaz.
func OutSize(wpx, hpx, maxH int) (int, int) {
	if wpx <= 0 || hpx <= 0 {
		return 0, 0
	}
	if maxH <= 0 || hpx <= maxH {
		return wpx &^ 1, hpx &^ 1
	}
	w := int(float64(wpx)*float64(maxH)/float64(hpx)+0.5) &^ 1
	return w, maxH &^ 1
}

// Bitrate, ekran içeriği için kaba bir başlangıç hızıdır (kbps):
// piksel × fps × 0,08 bit, 1-20 Mbps arası.
func Bitrate(w, h, fps int) int {
	k := int(float64(w*h*fps) * 0.08 / 1000)
	return max(1000, min(k, 20000))
}

// DefaultConfig, ekrana göre varsayılan ayarı döndürür.
func DefaultConfig(d Display) Config {
	w, h := OutSize(d.PixelsW, d.PixelsH, DefaultMaxHeight)
	return Config{Width: w, Height: h, FPS: DefaultFPS, BitrateKbps: Bitrate(w, h, DefaultFPS)}
}

// Stream, çalışan bir yakalamadır.
type Stream interface {
	// ForceKeyframe, bir sonraki kareyi anahtar kare yapar (PLI/FIR).
	ForceKeyframe()
	// Stop yakalamayı durdurur; birden çok kez çağrılabilir.
	Stop()
}

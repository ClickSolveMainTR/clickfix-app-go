// Package input, kontrol kapısından geçen olayları işletim sistemine uygular
// (control.Injector).
//
//   - macOS: Quartz CGEvent (inject_darwin.go). Erişilebilirlik izni gerekir.
//     CF-235 robotgo ile denendi; üretimde robotgo yerine doğrudan CGEvent
//     kullanılır: tuşlar DOM KeyboardEvent.code'dan sanal tuş koduna,
//     klavye düzeninden bağımsız eşlenir (robotgo karakterden ve etkin
//     düzenden eşler), sürükleme ve çift tıklama durumu doğru kurulur ve
//     robotgo'nun ekran görüntüsü/görüntü işleme bağımlılıkları pakete girmez.
//   - Windows ve diğerleri: ErrUnsupported.
package input

import (
	"errors"
	"math"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
)

// ErrUnsupported, bu platformda girdi enjeksiyonu olmadığını söyler.
var ErrUnsupported = errors.New("input: bu platformda girdi enjeksiyonu desteklenmiyor")

// Point, işletim sisteminin mantıksal (nokta) koordinatıdır.
type Point struct{ X, Y float64 }

// ToPixel, normalize koordinatı (0..1, paylaşılan ekranın sol üstüne göre)
// ekran pikseline çevirir: px = round(x × (genişlik − 1)). Biçim belgesindeki
// ve control.ts toScreen'deki kuralın aynısıdır; aralık dışı değer kırpılır.
func ToPixel(x, y float64, wpx, hpx int) (int, int) {
	x = math.Min(1, math.Max(0, x))
	y = math.Min(1, math.Max(0, y))
	return int(math.Round(x * float64(wpx-1))), int(math.Round(y * float64(hpx-1)))
}

// ToPoint, normalize koordinatı önce piksele, sonra HiDPI ölçeğiyle noktaya
// çevirir ve ekran kökenini ekler (çoklu ekranda birincil ekranın kökeni).
func ToPoint(x, y float64, d capture.Display) Point {
	px, py := ToPixel(x, y, d.PixelsW, d.PixelsH)
	sx, sy := 1.0, 1.0
	if d.PixelsW > 0 && d.PointsW > 0 {
		sx = float64(d.PixelsW) / float64(d.PointsW)
	}
	if d.PixelsH > 0 && d.PointsH > 0 {
		sy = float64(d.PixelsH) / float64(d.PointsH)
	}
	return Point{X: d.OriginX + float64(px)/sx, Y: d.OriginY + float64(py)/sy}
}

// Çift tıklama sınırları (macOS varsayılanına yakın).
const (
	DoubleClickInterval = 500 * time.Millisecond
	DoubleClickDistance = 4.0 // nokta
)

// ClickCounter, ardışık basmaları sayar (CGEvent kCGMouseEventClickState):
// aynı düğme, kısa süre ve yakın konumda 2, 3 ... olur.
type ClickCounter struct {
	last   time.Time
	pos    Point
	button int
	count  int
}

// Down bir basmayı kaydeder ve tıklama sayısını döner.
func (c *ClickCounter) Down(now time.Time, p Point, button int) int {
	if c.count > 0 && button == c.button && now.Sub(c.last) <= DoubleClickInterval &&
		math.Abs(p.X-c.pos.X) <= DoubleClickDistance && math.Abs(p.Y-c.pos.Y) <= DoubleClickDistance {
		c.count++
	} else {
		c.count = 1
	}
	c.last, c.pos, c.button = now, p, button
	return c.count
}

// Current, son basmanın sayısıdır (bırakma olayı aynı sayıyı taşır).
func (c *ClickCounter) Current() int {
	if c.count == 0 {
		return 1
	}
	return c.count
}

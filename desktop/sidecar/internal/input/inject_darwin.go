//go:build darwin && cgo

package input

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

// prompt=1 ise sistem izin penceresini açar (yalnız ilk seferde).
static int cf_ax_trusted(int prompt) {
	if (!prompt) return AXIsProcessTrusted() ? 1 : 0;
	const void *k[] = {kAXTrustedCheckOptionPrompt};
	const void *v[] = {kCFBooleanTrue};
	CFDictionaryRef o = CFDictionaryCreate(NULL, k, v, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int r = AXIsProcessTrustedWithOptions(o) ? 1 : 0;
	CFRelease(o);
	return r;
}

// Özel olay kaynağı: fiziksel klavyenin değiştirici durumu enjekte edilen
// olaylara karışmaz; bayraklar her olayda açıkça kurulur.
static CGEventSourceRef cf_source(void) {
	static CGEventSourceRef src = NULL;
	if (!src) src = CGEventSourceCreate(kCGEventSourceStatePrivate);
	return src;
}

static void cf_mouse(int type, double x, double y, int button, int clicks, uint64_t flags) {
	CGEventRef e = CGEventCreateMouseEvent(cf_source(), (CGEventType)type, CGPointMake(x, y), (CGMouseButton)button);
	if (!e) return;
	if (clicks > 0) CGEventSetIntegerValueField(e, kCGMouseEventClickState, clicks);
	CGEventSetFlags(e, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void cf_scroll(double x, double y, int dy, int dx, uint64_t flags) {
	CGEventRef e = CGEventCreateScrollWheelEvent(cf_source(), kCGScrollEventUnitPixel, 2, dy, dx);
	if (!e) return;
	CGEventSetLocation(e, CGPointMake(x, y));
	CGEventSetFlags(e, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void cf_key(int code, int down, int modifier, uint64_t flags) {
	CGEventRef e = CGEventCreateKeyboardEvent(cf_source(), (CGKeyCode)code, down ? true : false);
	if (!e) return;
	if (modifier) CGEventSetType(e, kCGEventFlagsChanged);
	CGEventSetFlags(e, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static void cf_main_bounds(double *x, double *y, double *w, double *h) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	*x = b.origin.x; *y = b.origin.y; *w = b.size.width; *h = b.size.height;
}
*/
import "C"

import (
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
)

// CGEventType değerleri.
const (
	evLeftDown     = 1
	evLeftUp       = 2
	evRightDown    = 3
	evRightUp      = 4
	evMoved        = 5
	evLeftDragged  = 6
	evRightDragged = 7
	evOtherDown    = 25
	evOtherUp      = 26
	evOtherDragged = 27
)

// CGEventFlags maskeleri.
const (
	flagShift   = 0x00020000
	flagControl = 0x00040000
	flagAlt     = 0x00080000
	flagCommand = 0x00100000
)

func flagsOf(m control.Mods) C.uint64_t {
	var f uint64
	if m.Shift {
		f |= flagShift
	}
	if m.Ctrl {
		f |= flagControl
	}
	if m.Alt {
		f |= flagAlt
	}
	if m.Meta {
		f |= flagCommand
	}
	return C.uint64_t(f)
}

// Accessibility, Erişilebilirlik iznini istem açmadan denetler.
func Accessibility() capture.Permission {
	if C.cf_ax_trusted(0) != 0 {
		return capture.Granted
	}
	return capture.Denied
}

// RequestAccessibility, izin istemini açar ve durumu döner.
func RequestAccessibility() capture.Permission {
	if C.cf_ax_trusted(1) != 0 {
		return capture.Granted
	}
	return capture.Denied
}

// Supported, bu derlemede girdi enjeksiyonu var mı.
const Supported = true

// Injector, macOS CGEvent ile çalışan control.Injector'dır.
type Injector struct {
	mu      sync.Mutex
	display func() capture.Display
	mods    modState
	held    [3]bool // DOM düğmeleri
	clicks  ClickCounter
	pos     Point
}

// NewInjector, ekran boyutlarını display'den okuyan bir enjektör döner
// (çözünürlük oturum sırasında değişebilir). display nil ise birincil ekran
// her olayda sorulur.
func NewInjector(display func() capture.Display) *Injector {
	if display == nil {
		display = func() capture.Display {
			d, err := capture.MainDisplay()
			if err != nil {
				var x, y, w, h C.double
				C.cf_main_bounds(&x, &y, &w, &h)
				return capture.Display{OriginX: float64(x), OriginY: float64(y), PointsW: int(w), PointsH: int(h), PixelsW: int(w), PixelsH: int(h), Scale: 1}
			}
			return d
		}
	}
	return &Injector{display: display}
}

func (in *Injector) point(x, y float64) Point { return ToPoint(x, y, in.display()) }

// Move imleci taşır; basılı düğme varsa sürükleme olayı gönderir.
func (in *Injector) Move(x, y float64) {
	in.mu.Lock()
	defer in.mu.Unlock()
	p := in.point(x, y)
	in.pos = p
	typ, btn := evMoved, 0
	switch {
	case in.held[0]:
		typ, btn = evLeftDragged, 0
	case in.held[2]:
		typ, btn = evRightDragged, 1
	case in.held[1]:
		typ, btn = evOtherDragged, 2
	}
	C.cf_mouse(C.int(typ), C.double(p.X), C.double(p.Y), C.int(btn), 0, flagsOf(in.mods.current()))
}

// Button bir fare düğmesini basar ya da bırakır (DOM düğme numarası).
func (in *Injector) Button(x, y float64, button int, down bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if button < 0 || button > 2 {
		return
	}
	p := in.point(x, y)
	in.pos = p
	var typ int
	switch button {
	case 0:
		typ = evLeftUp
		if down {
			typ = evLeftDown
		}
	case 2:
		typ = evRightUp
		if down {
			typ = evRightDown
		}
	default:
		typ = evOtherUp
		if down {
			typ = evOtherDown
		}
	}
	clicks := in.clicks.Current()
	if down {
		clicks = in.clicks.Down(time.Now(), p, button)
	}
	in.held[button] = down
	C.cf_mouse(C.int(typ), C.double(p.X), C.double(p.Y), C.int(MacButton(button)), C.int(clicks), flagsOf(in.mods.current()))
}

// Scroll piksel cinsinden kaydırır. DOM'da pozitif dy aşağı kaydırmadır,
// Quartz'da yukarı; işaret çevrilir.
func (in *Injector) Scroll(x, y float64, dx, dy int) {
	in.mu.Lock()
	defer in.mu.Unlock()
	p := in.point(x, y)
	if p != in.pos {
		in.pos = p
		C.cf_mouse(evMoved, C.double(p.X), C.double(p.Y), 0, 0, flagsOf(in.mods.current()))
	}
	C.cf_scroll(C.double(p.X), C.double(p.Y), C.int(-dy), C.int(-dx), flagsOf(in.mods.current()))
}

// Key fiziksel bir tuşu basar ya da bırakır. Eşlenmeyen kod atılır.
func (in *Injector) Key(code string, down bool, mods control.Mods, _ bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	kc, ok := MacKeyCodes[code]
	if !ok {
		return
	}
	m, modifier := in.mods.key(code, down, mods)
	C.cf_key(C.int(kc), btoi(down), btoi(modifier), flagsOf(m))
}

func btoi(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

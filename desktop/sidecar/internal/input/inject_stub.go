//go:build !(darwin && cgo)

package input

import (
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
)

// Windows'ta girdi enjeksiyonu (SendInput) ekran yakalamayla birlikte ayrı
// işte gelir; Linux'ta Destek uygulaması yoktur. Kapı bu platformlarda hiç
// açılmaz (Accessibility Unsupported döner).

// Accessibility desteklenmiyor.
func Accessibility() capture.Permission { return capture.Unsupported }

// RequestAccessibility desteklenmiyor.
func RequestAccessibility() capture.Permission { return capture.Unsupported }

// Supported, bu derlemede girdi enjeksiyonu var mı.
const Supported = false

// Injector hiçbir şey yapmaz.
type Injector struct{}

// NewInjector boş bir enjektör döner.
func NewInjector(func() capture.Display) *Injector { return &Injector{} }

func (*Injector) Move(float64, float64)                {}
func (*Injector) Button(float64, float64, int, bool)   {}
func (*Injector) Scroll(float64, float64, int, int)    {}
func (*Injector) Key(string, bool, control.Mods, bool) {}

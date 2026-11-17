package input

import (
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
)

func TestToPixelMatchesProtocol(t *testing.T) {
	// control.test.mjs toScreen örnekleri.
	cases := []struct {
		x, y   float64
		w, h   int
		px, py int
	}{
		{0, 0, 2560, 1440, 0, 0},
		{1, 1, 2560, 1440, 2559, 1439},
		{2, -1, 100, 100, 99, 0},
		{0.5, 0.5, 1920, 1080, 960, 540},
	}
	for _, c := range cases {
		if px, py := ToPixel(c.x, c.y, c.w, c.h); px != c.px || py != c.py {
			t.Errorf("ToPixel(%v,%v,%d,%d) = %d,%d", c.x, c.y, c.w, c.h, px, py)
		}
	}
}

func TestToPointHiDPI(t *testing.T) {
	// MacBook Air M2: 1470x956 nokta, 2940x1912 piksel.
	d := capture.Display{PointsW: 1470, PointsH: 956, PixelsW: 2940, PixelsH: 1912}
	if p := ToPoint(1, 1, d); p.X != 1469.5 || p.Y != 955.5 {
		t.Errorf("sağ alt = %+v", p)
	}
	if p := ToPoint(0.5, 0.5, d); p.X != 735 || p.Y != 478 {
		t.Errorf("orta = %+v", p)
	}
	// Ölçek 1 ve kaydırılmış köken.
	d = capture.Display{OriginX: -1920, OriginY: 0, PointsW: 1920, PointsH: 1080, PixelsW: 1920, PixelsH: 1080}
	if p := ToPoint(0, 1, d); p.X != -1920 || p.Y != 1079 {
		t.Errorf("köken = %+v", p)
	}
}

func TestClickCounter(t *testing.T) {
	var c ClickCounter
	t0 := time.Unix(0, 0)
	p := Point{100, 100}
	if n := c.Down(t0, p, 0); n != 1 {
		t.Errorf("ilk = %d", n)
	}
	if n := c.Down(t0.Add(200*time.Millisecond), Point{102, 101}, 0); n != 2 {
		t.Errorf("çift = %d", n)
	}
	if n := c.Down(t0.Add(400*time.Millisecond), p, 0); n != 3 {
		t.Errorf("üçlü = %d", n)
	}
	if n := c.Down(t0.Add(1500*time.Millisecond), p, 0); n != 1 {
		t.Errorf("geç = %d", n)
	}
	if n := c.Down(t0.Add(1600*time.Millisecond), Point{200, 100}, 0); n != 1 {
		t.Errorf("uzak = %d", n)
	}
	if n := c.Down(t0.Add(1700*time.Millisecond), Point{200, 100}, 2); n != 1 {
		t.Errorf("başka düğme = %d", n)
	}
}

func TestModState(t *testing.T) {
	var m modState
	if mods, mod := m.key("ShiftLeft", true, control.Mods{}); !mod || !mods.Shift {
		t.Errorf("shift basılı: %+v %v", mods, mod)
	}
	m.key("ShiftRight", true, control.Mods{})
	if mods, _ := m.key("ShiftLeft", false, control.Mods{}); !mods.Shift {
		t.Errorf("sağ shift hâlâ basılı olmalı: %+v", mods)
	}
	if mods, _ := m.key("ShiftRight", false, control.Mods{}); mods.Shift {
		t.Errorf("shift bırakıldı: %+v", mods)
	}
	m.key("MetaLeft", true, control.Mods{})
	if mods, mod := m.key("KeyC", true, control.Mods{}); mod || !mods.Meta {
		t.Errorf("Cmd+C: %+v %v", mods, mod)
	}
	// Olay mods'u da eklenir.
	if mods, _ := m.key("KeyV", true, control.Mods{Ctrl: true}); !mods.Meta || !mods.Ctrl {
		t.Errorf("birleşik: %+v", mods)
	}
}

func TestKeyMapCoverage(t *testing.T) {
	for _, code := range []string{"KeyA", "KeyZ", "Digit0", "Enter", "Backspace", "Delete", "Escape", "Tab", "Space",
		"ArrowUp", "ArrowLeft", "Home", "End", "PageUp", "F1", "F12", "ShiftLeft", "MetaRight", "Semicolon", "Quote", "Backquote", "IntlBackslash", "Numpad5"} {
		if _, ok := MacKeyCodes[code]; !ok {
			t.Errorf("%s eşlenmemiş", code)
		}
	}
	seen := map[uint16]string{}
	aliases := map[string]bool{"OSLeft": true, "OSRight": true, "Help": true}
	for code, kc := range MacKeyCodes {
		if aliases[code] {
			continue
		}
		if prev, ok := seen[kc]; ok {
			t.Errorf("%s ve %s aynı kodda (%#x)", code, prev, kc)
		}
		seen[kc] = code
	}
	if MacButton(0) != 0 || MacButton(1) != 2 || MacButton(2) != 1 {
		t.Error("düğme eşlemesi")
	}
}

var _ control.Injector = (*Injector)(nil)

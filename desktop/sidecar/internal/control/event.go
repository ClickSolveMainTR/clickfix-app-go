// Package control, temsilci konsolundan gelen kontrol olaylarını (sürüm 1,
// CF-237) çözer ve kontrol kapısından geçirerek bir Injector'a uygular.
//
// Biçim docs/design/masaustu.md "Kontrol olayı biçimi" bölümündedir; başvuru
// kodu frontend/src/features/remote-support/lib/control.ts'dir. Decode, oradaki
// decodeEvent ile aynı doğrulamayı yapar; üçü birlikte değişir.
package control

import (
	"encoding/json"
	"math"
	"unicode/utf16"
)

// Protokol sabitleri (control.ts ile aynı).
const (
	Version  = 1
	Topic    = "clickfix.rs.control.v1"
	MaxBytes = 1024
	WheelMax = 1200
	// RateLimit, saniyede uygulanan en çok olay sayısıdır.
	RateLimit = 200
)

// Olay türleri.
const (
	PointerMove = "pointer.move"
	PointerDown = "pointer.down"
	PointerUp   = "pointer.up"
	Wheel       = "wheel"
	KeyDown     = "key.down"
	KeyUp       = "key.up"
	Release     = "release"
)

// Mods, olayın taşıdığı değiştirici tuşlardır.
type Mods struct {
	Alt   bool `json:"alt"`
	Ctrl  bool `json:"ctrl"`
	Meta  bool `json:"meta"`
	Shift bool `json:"shift"`
}

// Or iki kümeyi birleştirir.
func (m Mods) Or(o Mods) Mods {
	return Mods{Alt: m.Alt || o.Alt, Ctrl: m.Ctrl || o.Ctrl, Meta: m.Meta || o.Meta, Shift: m.Shift || o.Shift}
}

// Event çözülmüş ve doğrulanmış bir kontrol olayıdır. Türe göre yalnız ilgili
// alanlar anlamlıdır.
type Event struct {
	V      int
	Seq    int64
	T      int64
	Type   string
	X, Y   float64 // pointer.*, wheel: 0..1
	Button int     // pointer.down/up: 0 sol, 1 orta, 2 sağ
	DX, DY int     // wheel: piksel
	Code   string  // key.*: DOM KeyboardEvent.code
	Key    string  // key.*: bilgi amaçlı
	Mods   Mods    // key.*
	Repeat bool    // key.down
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func isUnit(v any) (float64, bool) {
	f, ok := num(v)
	return f, ok && f >= 0 && f <= 1
}

// isInt, JS Number.isInteger ile aynıdır (1.0 tamsayıdır).
func isInt(v any) (float64, bool) {
	f, ok := num(v)
	return f, ok && f == math.Trunc(f)
}

// jsLen, JS string.length gibi UTF-16 birim sayısıdır.
func jsLen(s string) int { return len(utf16.Encode([]rune(s))) }

// Decode, bir veri kanalı yükünü alıcının yapması gerektiği gibi çözer:
// bilinmeyen sürüm ve tür, aralık dışı değer, engelli kısayol ve 1024 baytı
// aşan yük atılır (ok=false). Bilinmeyen ek alanlar yok sayılır.
func Decode(data []byte) (Event, bool) {
	if len(data) == 0 || len(data) > MaxBytes {
		return Event{}, false
	}
	var e map[string]any
	if err := json.Unmarshal(data, &e); err != nil || e == nil {
		return Event{}, false
	}
	if v, ok := num(e["v"]); !ok || v != Version {
		return Event{}, false
	}
	seq, ok := isInt(e["seq"])
	if !ok || seq < 1 {
		return Event{}, false
	}
	t, ok := isInt(e["t"])
	if !ok {
		return Event{}, false
	}
	typ, _ := e["type"].(string)
	ev := Event{V: Version, Seq: int64(seq), T: int64(t), Type: typ}

	unitXY := func() bool {
		x, okx := isUnit(e["x"])
		y, oky := isUnit(e["y"])
		ev.X, ev.Y = x, y
		return okx && oky
	}
	switch typ {
	case PointerMove:
		if !unitXY() {
			return Event{}, false
		}
	case PointerDown, PointerUp:
		if !unitXY() {
			return Event{}, false
		}
		b, ok := isInt(e["button"])
		if !ok || (b != 0 && b != 1 && b != 2) {
			return Event{}, false
		}
		ev.Button = int(b)
	case Wheel:
		if !unitXY() {
			return Event{}, false
		}
		dx, okx := isInt(e["dx"])
		dy, oky := isInt(e["dy"])
		if !okx || !oky || math.Abs(dx) > WheelMax || math.Abs(dy) > WheelMax {
			return Event{}, false
		}
		ev.DX, ev.DY = int(dx), int(dy)
	case KeyDown, KeyUp:
		code, okc := e["code"].(string)
		key, okk := e["key"].(string)
		if !okc || jsLen(code) == 0 || jsLen(code) > 32 || !okk || jsLen(key) > 32 {
			return Event{}, false
		}
		m, ok := e["mods"].(map[string]any)
		if !ok {
			return Event{}, false
		}
		var mods Mods
		for name, dst := range map[string]*bool{"alt": &mods.Alt, "ctrl": &mods.Ctrl, "meta": &mods.Meta, "shift": &mods.Shift} {
			b, ok := m[name].(bool)
			if !ok {
				return Event{}, false
			}
			*dst = b
		}
		if IsBlockedCombo(code, mods) {
			return Event{}, false
		}
		ev.Code, ev.Key, ev.Mods = code, key, mods
		ev.Repeat = e["repeat"] == true
	case Release:
	default:
		return Event{}, false
	}
	return ev, true
}

// IsBlockedCombo, hiçbir zaman uygulanmayan sistem kısayollarıdır: güvenli
// dikkat, kilit, görev yöneticisi, oturum kapatma, zorla çıkış. control.ts
// isBlockedCombo ile aynıdır.
func IsBlockedCombo(code string, m Mods) bool {
	switch {
	case m.Ctrl && m.Alt && (code == "Delete" || code == "Backspace"):
		return true
	case m.Ctrl && m.Shift && code == "Escape":
		return true
	case m.Meta && code == "KeyL":
		return true
	case m.Meta && m.Ctrl && code == "KeyQ":
		return true
	case m.Meta && m.Alt && code == "Escape":
		return true
	case m.Meta && m.Shift && code == "KeyQ":
		return true
	}
	return false
}

// modifierOf, bir değiştirici tuşun fiziksel kodunu Mods'a çevirir.
func modifierOf(code string) (Mods, bool) {
	switch code {
	case "ControlLeft", "ControlRight":
		return Mods{Ctrl: true}, true
	case "AltLeft", "AltRight":
		return Mods{Alt: true}, true
	case "MetaLeft", "MetaRight", "OSLeft", "OSRight":
		return Mods{Meta: true}, true
	case "ShiftLeft", "ShiftRight":
		return Mods{Shift: true}, true
	}
	return Mods{}, false
}

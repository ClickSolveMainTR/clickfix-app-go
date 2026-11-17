package input

import "github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"

// MacKeyCodes, DOM KeyboardEvent.code → macOS sanal tuş kodu (Carbon
// kVK_*, HIToolbox/Events.h). Fiziksel konuma göredir: temsilcinin düzeni ne
// olursa olsun müşteri bilgisayarındaki aynı konumdaki tuş basılır ve
// karakteri müşterinin düzeni belirler (ör. Türkçe Q klavyede "Semicolon"
// konumu "ş" yazar). Listede olmayan kod atılır.
var MacKeyCodes = map[string]uint16{
	"KeyA": 0x00, "KeyS": 0x01, "KeyD": 0x02, "KeyF": 0x03, "KeyH": 0x04, "KeyG": 0x05,
	"KeyZ": 0x06, "KeyX": 0x07, "KeyC": 0x08, "KeyV": 0x09, "IntlBackslash": 0x0A, "KeyB": 0x0B,
	"KeyQ": 0x0C, "KeyW": 0x0D, "KeyE": 0x0E, "KeyR": 0x0F, "KeyY": 0x10, "KeyT": 0x11,
	"Digit1": 0x12, "Digit2": 0x13, "Digit3": 0x14, "Digit4": 0x15, "Digit6": 0x16, "Digit5": 0x17,
	"Equal": 0x18, "Digit9": 0x19, "Digit7": 0x1A, "Minus": 0x1B, "Digit8": 0x1C, "Digit0": 0x1D,
	"BracketRight": 0x1E, "KeyO": 0x1F, "KeyU": 0x20, "BracketLeft": 0x21, "KeyI": 0x22, "KeyP": 0x23,
	"Enter": 0x24, "KeyL": 0x25, "KeyJ": 0x26, "Quote": 0x27, "KeyK": 0x28, "Semicolon": 0x29,
	"Backslash": 0x2A, "Comma": 0x2B, "Slash": 0x2C, "KeyN": 0x2D, "KeyM": 0x2E, "Period": 0x2F,
	"Tab": 0x30, "Space": 0x31, "Backquote": 0x32, "Backspace": 0x33, "Escape": 0x35,
	"MetaRight": 0x36, "MetaLeft": 0x37, "OSRight": 0x36, "OSLeft": 0x37,
	"ShiftLeft": 0x38, "CapsLock": 0x39, "AltLeft": 0x3A, "ControlLeft": 0x3B,
	"ShiftRight": 0x3C, "AltRight": 0x3D, "ControlRight": 0x3E,
	"F17": 0x40, "NumpadDecimal": 0x41, "NumpadMultiply": 0x43, "NumpadAdd": 0x45, "NumLock": 0x47,
	"NumpadDivide": 0x4B, "NumpadEnter": 0x4C, "NumpadSubtract": 0x4E, "F18": 0x4F, "F19": 0x50,
	"NumpadEqual": 0x51, "Numpad0": 0x52, "Numpad1": 0x53, "Numpad2": 0x54, "Numpad3": 0x55,
	"Numpad4": 0x56, "Numpad5": 0x57, "Numpad6": 0x58, "Numpad7": 0x59, "F20": 0x5A,
	"Numpad8": 0x5B, "Numpad9": 0x5C, "IntlYen": 0x5D, "IntlRo": 0x5E, "NumpadComma": 0x5F,
	"F5": 0x60, "F6": 0x61, "F7": 0x62, "F3": 0x63, "F8": 0x64, "F9": 0x65, "Lang2": 0x66,
	"F11": 0x67, "Lang1": 0x68, "F13": 0x69, "F16": 0x6A, "F14": 0x6B, "F10": 0x6D,
	"ContextMenu": 0x6E, "F12": 0x6F, "F15": 0x71, "Insert": 0x72, "Help": 0x72, "Home": 0x73,
	"PageUp": 0x74, "Delete": 0x75, "F4": 0x76, "End": 0x77, "F2": 0x78, "PageDown": 0x79,
	"F1": 0x7A, "ArrowLeft": 0x7B, "ArrowRight": 0x7C, "ArrowDown": 0x7D, "ArrowUp": 0x7E,
}

// modifierBit, değiştirici tuşların ayrı ayrı izlenmesi içindir (sol ve sağ
// Shift ayrı tuşlardır; biri bırakılınca diğeri basılı kalabilir).
func modifierOf(code string) (control.Mods, bool) {
	switch code {
	case "ControlLeft", "ControlRight":
		return control.Mods{Ctrl: true}, true
	case "AltLeft", "AltRight":
		return control.Mods{Alt: true}, true
	case "MetaLeft", "MetaRight", "OSLeft", "OSRight":
		return control.Mods{Meta: true}, true
	case "ShiftLeft", "ShiftRight":
		return control.Mods{Shift: true}, true
	}
	return control.Mods{}, false
}

// modState, enjekte edilen değiştirici tuşların durumudur. Olaylara konan
// bayraklar buradan hesaplanır; böylece Cmd+C gibi kısayollar ve
// değiştirici bırakma (flagsChanged) doğru bayrakları taşır.
type modState struct{ held map[string]bool }

// key, bir tuş olayını kaydeder ve olaya konacak değiştiricileri döner.
// Değiştirici tuşun kendisi basılınca bayrağa girer, bırakılınca çıkar.
func (m *modState) key(code string, down bool, evMods control.Mods) (mods control.Mods, isModifier bool) {
	if m.held == nil {
		m.held = map[string]bool{}
	}
	_, isModifier = modifierOf(code)
	if isModifier {
		if down {
			m.held[code] = true
		} else {
			delete(m.held, code)
		}
		// Değiştirici olaylarında yalnız gerçekten basılı olanlar sayılır.
		return m.current(), true
	}
	return m.current().Or(evMods), false
}

func (m *modState) current() control.Mods {
	var out control.Mods
	for code := range m.held {
		if mm, ok := modifierOf(code); ok {
			out = out.Or(mm)
		}
	}
	return out
}

// MacButton, DOM düğmesini (0 sol, 1 orta, 2 sağ) CGMouseButton'a çevirir
// (0 sol, 1 sağ, 2 orta).
func MacButton(dom int) int {
	switch dom {
	case 1:
		return 2
	case 2:
		return 1
	}
	return 0
}

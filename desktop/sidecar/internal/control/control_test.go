package control

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	agent    = "rs:agent:6f1c"
	customer = "rs:customer:6f1c"
)

type call struct {
	Kind   string
	X, Y   float64
	Button int
	Down   bool
	DX, DY int
	Code   string
	Mods   Mods
	Repeat bool
}

type fakeInjector struct{ calls []call }

func (f *fakeInjector) Move(x, y float64) { f.calls = append(f.calls, call{Kind: "move", X: x, Y: y}) }
func (f *fakeInjector) Button(x, y float64, b int, down bool) {
	f.calls = append(f.calls, call{Kind: "button", X: x, Y: y, Button: b, Down: down})
}
func (f *fakeInjector) Scroll(x, y float64, dx, dy int) {
	f.calls = append(f.calls, call{Kind: "scroll", X: x, Y: y, DX: dx, DY: dy})
}
func (f *fakeInjector) Key(code string, down bool, m Mods, repeat bool) {
	f.calls = append(f.calls, call{Kind: "key", Code: code, Down: down, Mods: m, Repeat: repeat})
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Unix(1795339200, 0)} }

var seq int64

func ev(fields map[string]any) []byte {
	seq++
	m := map[string]any{"v": 1, "seq": seq, "t": 1795339200000}
	for k, v := range fields {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func move(x, y float64) []byte { return ev(map[string]any{"type": PointerMove, "x": x, "y": y}) }
func down(b int) []byte {
	return ev(map[string]any{"type": PointerDown, "x": 0.5, "y": 0.5, "button": b})
}
func up(b int) []byte { return ev(map[string]any{"type": PointerUp, "x": 0.5, "y": 0.5, "button": b}) }
func key(t, code string, m Mods) []byte {
	return ev(map[string]any{"type": t, "code": code, "key": "x", "mods": m, "repeat": false})
}
func release() []byte { return ev(map[string]any{"type": Release}) }

// openGate, bütün koşulları sağlanmış bir kapı döndürür.
func openGate(t *testing.T) (*Gate, *fakeInjector, *clock, *[]Notice) {
	t.Helper()
	inj := &fakeInjector{}
	c := newClock()
	var notices []Notice
	g := NewGate(inj, c.now, func(n Notice) { notices = append(notices, n) })
	g.SetSession(true, agent)
	g.SetControl(true)
	g.SetPermitted(true)
	if !g.Open() {
		t.Fatal("kapı açık olmalı")
	}
	return g, inj, c, &notices
}

func TestDecodeRoundTripAndValidation(t *testing.T) {
	ok := map[string][]byte{
		"move":     []byte(`{"v":1,"seq":1,"t":1,"type":"pointer.move","x":0.5123,"y":0.2871}`),
		"down":     []byte(`{"v":1,"seq":42,"t":1795339200000,"type":"pointer.down","x":0.5123,"y":0.2871,"button":0}`),
		"key":      []byte(`{"v":1,"seq":43,"t":1795339200016,"type":"key.down","code":"KeyC","key":"c","mods":{"alt":false,"ctrl":true,"meta":false,"shift":false},"repeat":false}`),
		"wheel":    []byte(`{"v":1,"seq":2,"t":1,"type":"wheel","x":0,"y":1,"dx":-1200,"dy":48}`),
		"release":  []byte(`{"v":1,"seq":3,"t":1,"type":"release","extra":true}`),
		"intf":     []byte(`{"v":1.0,"seq":4.0,"t":1,"type":"release"}`),
		"emptyKey": []byte(`{"v":1,"seq":5,"t":1,"type":"key.up","code":"Enter","key":"","mods":{"alt":false,"ctrl":false,"meta":false,"shift":false}}`),
	}
	for name, b := range ok {
		if _, good := Decode(b); !good {
			t.Errorf("%s reddedildi: %s", name, b)
		}
	}
	e, _ := Decode(ok["key"])
	want := Event{V: 1, Seq: 43, T: 1795339200016, Type: KeyDown, Code: "KeyC", Key: "c", Mods: Mods{Ctrl: true}}
	if !reflect.DeepEqual(e, want) {
		t.Errorf("Decode = %+v, beklenen %+v", e, want)
	}

	bad := map[string]string{
		"sürüm 2":        `{"v":2,"seq":1,"t":1,"type":"release"}`,
		"sürüm yok":      `{"seq":1,"t":1,"type":"release"}`,
		"bilinmeyen tür": `{"v":1,"seq":1,"t":1,"type":"clipboard"}`,
		"x > 1":          `{"v":1,"seq":1,"t":1,"type":"pointer.move","x":1.2,"y":0}`,
		"y < 0":          `{"v":1,"seq":1,"t":1,"type":"pointer.move","x":0,"y":-0.1}`,
		"x metin":        `{"v":1,"seq":1,"t":1,"type":"pointer.move","x":"0.5","y":0}`,
		"düğme 3":        `{"v":1,"seq":1,"t":1,"type":"pointer.down","x":0,"y":0,"button":3}`,
		"seq 0":          `{"v":1,"seq":0,"t":1,"type":"release"}`,
		"seq kesirli":    `{"v":1,"seq":1.5,"t":1,"type":"release"}`,
		"t yok":          `{"v":1,"seq":1,"type":"release"}`,
		"wheel büyük":    `{"v":1,"seq":1,"t":1,"type":"wheel","x":0,"y":0,"dx":0,"dy":1201}`,
		"wheel kesirli":  `{"v":1,"seq":1,"t":1,"type":"wheel","x":0,"y":0,"dx":0.5,"dy":1}`,
		"mods eksik":     `{"v":1,"seq":1,"t":1,"type":"key.down","code":"KeyA","key":"a","mods":{"alt":false}}`,
		"mods yok":       `{"v":1,"seq":1,"t":1,"type":"key.down","code":"KeyA","key":"a"}`,
		"code boş":       `{"v":1,"seq":1,"t":1,"type":"key.down","code":"","key":"a","mods":{"alt":false,"ctrl":false,"meta":false,"shift":false}}`,
		"code uzun":      `{"v":1,"seq":1,"t":1,"type":"key.down","code":"` + strings.Repeat("A", 33) + `","key":"a","mods":{"alt":false,"ctrl":false,"meta":false,"shift":false}}`,
		"key uzun":       `{"v":1,"seq":1,"t":1,"type":"key.down","code":"KeyA","key":"` + strings.Repeat("ğ", 33) + `","mods":{"alt":false,"ctrl":false,"meta":false,"shift":false}}`,
		"ctrl+alt+del":   `{"v":1,"seq":1,"t":1,"type":"key.down","code":"Delete","key":"Delete","mods":{"alt":true,"ctrl":true,"meta":false,"shift":false}}`,
		"json değil":     `not json`,
		"dizi":           `[1,2]`,
		"null":           `null`,
		"boş":            ``,
	}
	for name, s := range bad {
		if _, good := Decode([]byte(s)); good {
			t.Errorf("%s kabul edildi: %s", name, s)
		}
	}
	big := fmt.Sprintf(`{"v":1,"seq":1,"t":1,"type":"release","pad":"%s"}`, strings.Repeat("x", MaxBytes))
	if _, good := Decode([]byte(big)); good {
		t.Error("1024 baytı aşan yük kabul edildi")
	}
}

func TestBlockedCombos(t *testing.T) {
	cases := []struct {
		code string
		m    Mods
		want bool
	}{
		{"Delete", Mods{Ctrl: true, Alt: true}, true},
		{"Backspace", Mods{Ctrl: true, Alt: true}, true},
		{"Escape", Mods{Ctrl: true, Shift: true}, true},
		{"KeyL", Mods{Meta: true}, true},
		{"KeyQ", Mods{Meta: true, Ctrl: true}, true},
		{"Escape", Mods{Meta: true, Alt: true}, true},
		{"KeyQ", Mods{Meta: true, Shift: true}, true},
		{"KeyC", Mods{Ctrl: true}, false},
		{"KeyQ", Mods{Meta: true}, false},
		{"Delete", Mods{Ctrl: true}, false},
		{"Escape", Mods{}, false},
	}
	for _, c := range cases {
		if got := IsBlockedCombo(c.code, c.m); got != c.want {
			t.Errorf("IsBlockedCombo(%s, %+v) = %v", c.code, c.m, got)
		}
	}
}

func TestGateRejectsWithoutConsent(t *testing.T) {
	inj := &fakeInjector{}
	g := NewGate(inj, nil, nil)

	// Oturum yok.
	if r := g.Handle(agent, move(0.1, 0.1)); r != DropClosed {
		t.Errorf("oturumsuz: %v", r)
	}
	// Oturum var, kontrol onayı yok (yalnız izleme).
	g.SetSession(true, agent)
	g.SetPermitted(true)
	if r := g.Handle(agent, down(0)); r != DropClosed {
		t.Errorf("onaysız: %v", r)
	}
	if r := g.Handle(agent, key(KeyDown, "KeyA", Mods{})); r != DropClosed {
		t.Errorf("onaysız tuş: %v", r)
	}
	// Onay var ama oturum etkin değil.
	g.SetControl(true)
	g.SetSession(false, "")
	if r := g.Handle(agent, move(0.1, 0.1)); r != DropClosed {
		t.Errorf("oturum bitti: %v", r)
	}
	if len(inj.calls) != 0 {
		t.Errorf("hiçbir olay uygulanmamalıydı: %+v", inj.calls)
	}
}

func TestGateRejectsAfterRevoke(t *testing.T) {
	g, inj, _, _ := openGate(t)
	if r := g.Handle(agent, move(0.25, 0.75)); r != Applied {
		t.Fatalf("açık kapı uygulamalıydı: %v", r)
	}
	g.Handle(agent, down(0))
	g.Handle(agent, key(KeyDown, "ShiftLeft", Mods{Shift: true}))
	g.Handle(agent, key(KeyDown, "KeyA", Mods{Shift: true}))
	inj.calls = nil

	g.SetControl(false)
	if g.Open() {
		t.Fatal("geri almadan sonra kapı kapalı olmalı")
	}
	// Geri alma basılı her şeyi bırakır: önce normal tuş, sonra
	// değiştirici, sonra düğme.
	want := []call{
		{Kind: "key", Code: "KeyA", Down: false},
		{Kind: "key", Code: "ShiftLeft", Down: false},
		{Kind: "button", X: 0.5, Y: 0.5, Button: 0, Down: false},
	}
	if !reflect.DeepEqual(inj.calls, want) {
		t.Errorf("bırakma = %+v\nbeklenen %+v", inj.calls, want)
	}
	inj.calls = nil
	for _, p := range [][]byte{move(0.1, 0.1), down(0), up(0), key(KeyDown, "KeyB", Mods{}), release()} {
		if r := g.Handle(agent, p); r != DropClosed {
			t.Errorf("geri almadan sonra %s: %v", p, r)
		}
	}
	if len(inj.calls) != 0 {
		t.Errorf("geri almadan sonra uygulandı: %+v", inj.calls)
	}
	// Onay yeniden verilse de basılı durum geri gelmez; up'lar atılır.
	g.SetControl(true)
	if r := g.Handle(agent, key(KeyUp, "KeyA", Mods{})); r != DropNotPressed {
		t.Errorf("yeniden onay sonrası eski key.up: %v", r)
	}
}

func TestGateRejectsSpoofedSender(t *testing.T) {
	g, inj, _, _ := openGate(t)
	for _, sender := range []string{customer, "rs:agent:baska-oturum", "", "agent", "RS:AGENT:6f1c", agent + " "} {
		if r := g.Handle(sender, move(0.5, 0.5)); r != DropSender {
			t.Errorf("gönderen %q: %v", sender, r)
		}
	}
	if len(inj.calls) != 0 {
		t.Errorf("sahte gönderen uygulandı: %+v", inj.calls)
	}
	// Kimlik değişince (ör. yeni oturum) eskisinin basılıları bırakılır.
	g.Handle(agent, down(2))
	inj.calls = nil
	g.SetSession(true, "rs:agent:yeni")
	if len(inj.calls) != 1 || inj.calls[0].Kind != "button" || inj.calls[0].Down {
		t.Errorf("kimlik değişiminde bırakma yok: %+v", inj.calls)
	}
	if r := g.Handle(agent, move(0.5, 0.5)); r != DropSender {
		t.Errorf("eski temsilci kimliği: %v", r)
	}
}

func TestGateRateLimit(t *testing.T) {
	g, inj, c, notices := openGate(t)
	applied := 0
	for range RateLimit + 50 {
		if g.Handle(agent, move(0.5, 0.5)) == Applied {
			applied++
		}
	}
	if applied != RateLimit {
		t.Errorf("aynı anda %d olay uygulandı, sınır %d", applied, RateLimit)
	}
	if r := g.Handle(agent, move(0.5, 0.5)); r != DropRate {
		t.Errorf("sınır aşımında: %v", r)
	}
	// release sınırdan muaf.
	if r := g.Handle(agent, release()); r != Applied {
		t.Errorf("release hız sınırına takıldı: %v", r)
	}
	// Bildirim oturumda bir kez.
	n := 0
	for _, x := range *notices {
		if x == NoticeRateLimited {
			n++
		}
	}
	if n != 1 {
		t.Errorf("rate_limited bildirimi %d kez", n)
	}
	// 100 ms sonra 20 olaylık yer açılır.
	c.advance(100 * time.Millisecond)
	applied = 0
	for range 30 {
		if g.Handle(agent, move(0.5, 0.5)) == Applied {
			applied++
		}
	}
	if applied != 20 {
		t.Errorf("100 ms sonra %d olay uygulandı, beklenen 20", applied)
	}
	// Bir saniye boşluk kovayı doldurur ama sınırı aşmaz.
	c.advance(10 * time.Second)
	applied = 0
	for range RateLimit + 1 {
		if g.Handle(agent, move(0.5, 0.5)) == Applied {
			applied++
		}
	}
	if applied != RateLimit {
		t.Errorf("dolu kova %d olay geçirdi", applied)
	}
	st := g.Stats()
	if st.Dropped["rate"] == 0 || st.Applied == 0 {
		t.Errorf("sayaçlar: %+v", st)
	}
	_ = inj
}

func TestGateBlocksShortcuts(t *testing.T) {
	g, inj, c, notices := openGate(t)
	// Olayın kendi mods'u ile.
	if r := g.Handle(agent, key(KeyDown, "Delete", Mods{Ctrl: true, Alt: true})); r != DropBlocked {
		t.Errorf("ctrl+alt+del: %v", r)
	}
	// Ayrı key.down'larla kurulan kombinasyon (mods alanı yalan söylüyor).
	g.Handle(agent, key(KeyDown, "MetaLeft", Mods{}))
	if r := g.Handle(agent, key(KeyDown, "KeyL", Mods{})); r != DropBlocked {
		t.Errorf("Meta basılıyken KeyL: %v", r)
	}
	if r := g.Handle(agent, key(KeyUp, "KeyL", Mods{})); r != DropNotPressed {
		t.Errorf("engellenen tuşun up'ı: %v", r)
	}
	for _, x := range inj.calls {
		if x.Code == "KeyL" || x.Code == "Delete" {
			t.Errorf("engelli tuş uygulandı: %+v", x)
		}
	}
	// İzin verilen kısayol uygulanır ve basılı Meta mods'a eklenir.
	if r := g.Handle(agent, key(KeyDown, "KeyC", Mods{})); r != Applied {
		t.Errorf("Meta+C: %v", r)
	}
	last := inj.calls[len(inj.calls)-1]
	if !last.Mods.Meta {
		t.Errorf("basılı Meta mods'a eklenmedi: %+v", last)
	}
	// Bildirim 5 sn'de en çok bir kez.
	if len(*notices) != 1 || (*notices)[0] != NoticeBlockedShortcut {
		t.Errorf("bildirimler: %v", *notices)
	}
	c.advance(6 * time.Second)
	g.Handle(agent, key(KeyDown, "KeyQ", Mods{Shift: true}))
	if len(*notices) != 2 {
		t.Errorf("5 sn sonra yeni bildirim bekleniyordu: %v", *notices)
	}
}

func TestGatePermission(t *testing.T) {
	g, inj, _, notices := openGate(t)
	g.Handle(agent, key(KeyDown, "KeyA", Mods{}))
	inj.calls = nil
	g.SetPermitted(false)
	if len(inj.calls) != 1 || inj.calls[0].Code != "KeyA" || inj.calls[0].Down {
		t.Errorf("izin kalkınca bırakma yok: %+v", inj.calls)
	}
	for range 3 {
		if r := g.Handle(agent, move(0.1, 0.1)); r != DropPermission {
			t.Errorf("izinsiz: %v", r)
		}
	}
	if len(*notices) != 1 || (*notices)[0] != NoticeAccessibilityMissing {
		t.Errorf("izin bildirimi: %v", *notices)
	}
}

func TestGateCloseAndRelease(t *testing.T) {
	g, inj, _, _ := openGate(t)
	g.Handle(agent, move(0.2, 0.3))
	g.Handle(agent, down(0))
	g.Handle(agent, ev(map[string]any{"type": PointerDown, "x": 0.4, "y": 0.6, "button": 2}))
	g.Handle(agent, key(KeyDown, "ControlLeft", Mods{Ctrl: true}))
	inj.calls = nil
	if r := g.Handle(agent, release()); r != Applied {
		t.Fatalf("release: %v", r)
	}
	want := []call{
		{Kind: "key", Code: "ControlLeft"},
		{Kind: "button", X: 0.4, Y: 0.6, Button: 0},
		{Kind: "button", X: 0.4, Y: 0.6, Button: 2},
	}
	if !reflect.DeepEqual(inj.calls, want) {
		t.Errorf("release = %+v\nbeklenen %+v", inj.calls, want)
	}
	// İkinci release boştur.
	inj.calls = nil
	g.Handle(agent, release())
	if len(inj.calls) != 0 {
		t.Errorf("boş release uyguladı: %+v", inj.calls)
	}
	// Bağlantı kopması (Close) basılıları bırakır ve kapıyı kapatır.
	g.Handle(agent, key(KeyDown, "KeyZ", Mods{}))
	inj.calls = nil
	g.Close()
	if len(inj.calls) != 1 || inj.calls[0].Code != "KeyZ" {
		t.Errorf("Close bırakmadı: %+v", inj.calls)
	}
	if r := g.Handle(agent, move(0.1, 0.1)); r != DropClosed {
		t.Errorf("Close sonrası: %v", r)
	}
}

func TestGateAppliesEvents(t *testing.T) {
	g, inj, _, _ := openGate(t)
	g.Handle(agent, ev(map[string]any{"type": Wheel, "x": 0.1, "y": 0.9, "dx": 0, "dy": -48}))
	g.Handle(agent, ev(map[string]any{"type": KeyDown, "code": "KeyA", "key": "a", "mods": Mods{}, "repeat": true}))
	g.Handle(agent, key(KeyUp, "KeyA", Mods{}))
	want := []call{
		{Kind: "scroll", X: 0.1, Y: 0.9, DY: -48},
		{Kind: "key", Code: "KeyA", Down: true, Repeat: true},
		{Kind: "key", Code: "KeyA", Down: false},
	}
	if !reflect.DeepEqual(inj.calls, want) {
		t.Errorf("uygulanan = %+v\nbeklenen %+v", inj.calls, want)
	}
	if r := g.Handle(agent, []byte(`{"v":1}`)); r != DropInvalid {
		t.Errorf("geçersiz: %v", r)
	}
	if r := g.Handle(agent, up(1)); r != DropNotPressed {
		t.Errorf("basılmamış düğme up: %v", r)
	}
}

package control

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// Injector, kapıdan geçen olayları işletim sistemine uygular. Koordinatlar
// paylaşılan ekrana göre 0..1 normalizedir; piksele çevirmek Injector'ın
// işidir. Kapı yöntemleri tek bir kilit altında, sırayla çağırır.
type Injector interface {
	Move(x, y float64)
	Button(x, y float64, button int, down bool)
	Scroll(x, y float64, dx, dy int)
	Key(code string, down bool, mods Mods, repeat bool)
}

// Result, Handle'ın bir olay için verdiği karardır.
type Result int

const (
	Applied        Result = iota
	DropClosed            // oturum etkin değil ya da kontrol onayı yok
	DropSender            // gönderen temsilci değil
	DropInvalid           // çözülemedi, bilinmeyen sürüm/tür, aralık dışı
	DropBlocked           // engelli sistem kısayolu
	DropRate              // hız sınırı aşıldı
	DropPermission        // onay var ama işletim sistemi izni (Erişilebilirlik) yok
	DropNotPressed        // basılı olmayan tuşun/düğmenin bırakılması
)

func (r Result) String() string {
	return [...]string{"applied", "closed", "sender", "invalid", "blocked", "rate", "permission", "not_pressed"}[r]
}

// Notice, kapının arayüze bildirmek istediği olaydır (IPC control olayı).
type Notice string

const (
	NoticeBlockedShortcut      Notice = "blocked_shortcut"
	NoticeRateLimited          Notice = "rate_limited"
	NoticeAccessibilityMissing Notice = "accessibility_missing"
)

// Stats, oturum boyunca sayılan kararlardır.
type Stats struct {
	Applied int64            `json:"applied"`
	Dropped map[string]int64 `json:"dropped"`
}

// Gate, tasarım 5. bölümdeki kontrol kapısıdır. Bir girdi olayı yalnız şu
// koşulların hepsi sağlanınca uygulanır: oturum etkin, kontrol onayı verilmiş
// ve geri alınmamış, olay temsilcinin LiveKit kimliğinden gelmiş, hız sınırı
// aşılmamış. Kapı kapanınca basılı her tuş ve düğme bırakılır.
type Gate struct {
	mu  sync.Mutex
	inj Injector
	now func() time.Time

	active    bool
	agentID   string
	consent   bool
	permitted bool

	keys    map[string]Mods // basılı tuşlar
	buttons [3]bool
	lastX   float64
	lastY   float64

	tokens   float64
	refilled time.Time

	onNotice    func(Notice)
	rateNoticed bool
	permNoticed bool
	lastBlocked time.Time
	applied     int64
	dropped     map[Result]int64
}

// NewGate kapalı bir kapı döndürür. onNotice nil olabilir; kilit altında
// çağrılır, bu yüzden geri dönüp kapıyı çağırmamalıdır.
func NewGate(inj Injector, now func() time.Time, onNotice func(Notice)) *Gate {
	if now == nil {
		now = time.Now
	}
	return &Gate{inj: inj, now: now, onNotice: onNotice, keys: map[string]Mods{}, dropped: map[Result]int64{}, tokens: RateLimit}
}

// Open, kapının şu an olay uygulayıp uygulamayacağıdır.
func (g *Gate) Open() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.open()
}

func (g *Gate) open() bool { return g.active && g.consent && g.permitted && g.agentID != "" }

// SetSession, oturumun etkin olup olmadığını ve kabul edilen tek gönderen
// kimliğini (rs:agent:<oturum>) ayarlar. Etkin değilse kapı kapanır.
func (g *Gate) SetSession(active bool, agentIdentity string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !active || agentIdentity != g.agentID {
		g.releaseAll()
	}
	g.active, g.agentID = active, agentIdentity
	if !active {
		g.agentID = ""
		g.rateNoticed, g.permNoticed = false, false
	}
}

// SetControl, müşterinin kontrol onayını ayarlar. false (geri alma) kapıyı
// hemen kapatır ve basılı her şeyi bırakır.
func (g *Gate) SetControl(consent bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !consent {
		g.releaseAll()
	}
	g.consent = consent
}

// SetPermitted, işletim sisteminin girdi enjeksiyonuna izin verip
// vermediğidir (macOS Erişilebilirlik).
func (g *Gate) SetPermitted(ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !ok {
		g.releaseAll()
	}
	g.permitted = ok
}

// Close kapıyı her koşulda kapatır (bağlantı koptu, oturum bitti).
func (g *Gate) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseAll()
	g.active, g.agentID = false, ""
}

// Stats, sayaçların kopyasını döndürür.
func (g *Gate) Stats() Stats {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := Stats{Applied: g.applied, Dropped: map[string]int64{}}
	for r, n := range g.dropped {
		s.Dropped[r.String()] = n
	}
	return s
}

// Handle, sender kimliğinden gelen bir veri kanalı yükünü değerlendirir ve
// kapı açıksa uygular.
func (g *Gate) Handle(sender string, payload []byte) Result {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.handle(sender, payload)
	if r == Applied {
		g.applied++
	} else {
		g.dropped[r]++
	}
	return r
}

func (g *Gate) handle(sender string, payload []byte) Result {
	if !g.active || g.agentID == "" {
		return DropClosed
	}
	if sender != g.agentID {
		return DropSender
	}
	if !g.consent {
		return DropClosed
	}
	if !g.permitted {
		if !g.permNoticed {
			g.permNoticed = true
			g.notice(NoticeAccessibilityMissing)
		}
		return DropPermission
	}
	ev, ok := Decode(payload)
	if !ok {
		if blockedPayload(payload) {
			g.noticeBlocked()
			return DropBlocked
		}
		return DropInvalid
	}
	// release her zaman uygulanır: hız sınırı bırakmayı engellememeli.
	if ev.Type == Release {
		g.releaseAll()
		return Applied
	}
	if !g.take() {
		if !g.rateNoticed {
			g.rateNoticed = true
			g.notice(NoticeRateLimited)
		}
		return DropRate
	}
	return g.apply(ev)
}

func (g *Gate) apply(ev Event) Result {
	switch ev.Type {
	case PointerMove:
		g.lastX, g.lastY = ev.X, ev.Y
		g.inj.Move(ev.X, ev.Y)
	case PointerDown, PointerUp:
		down := ev.Type == PointerDown
		if !down && !g.buttons[ev.Button] {
			return DropNotPressed
		}
		g.lastX, g.lastY = ev.X, ev.Y
		g.buttons[ev.Button] = down
		g.inj.Button(ev.X, ev.Y, ev.Button, down)
	case Wheel:
		g.lastX, g.lastY = ev.X, ev.Y
		g.inj.Scroll(ev.X, ev.Y, ev.DX, ev.DY)
	case KeyDown:
		// Olayın kendi mods alanı ile sidecar'ın bildiği basılı
		// değiştiriciler birleşir: ayrı key.down'larla kurulan bir
		// kombinasyon da (ör. MetaLeft, sonra KeyL mods=false) engellenir.
		mods := ev.Mods.Or(g.heldMods())
		if IsBlockedCombo(ev.Code, mods) {
			g.noticeBlocked()
			return DropBlocked
		}
		g.keys[ev.Code] = mods
		g.inj.Key(ev.Code, true, mods, ev.Repeat)
	case KeyUp:
		mods, ok := g.keys[ev.Code]
		if !ok {
			return DropNotPressed
		}
		delete(g.keys, ev.Code)
		g.inj.Key(ev.Code, false, mods, false)
	}
	return Applied
}

func (g *Gate) heldMods() Mods {
	var m Mods
	for code := range g.keys {
		if mm, ok := modifierOf(code); ok {
			m = m.Or(mm)
		}
	}
	return m
}

// releaseAll basılı tuşları (değiştiriciler en son) ve düğmeleri bırakır.
func (g *Gate) releaseAll() {
	if len(g.keys) > 0 {
		codes := make([]string, 0, len(g.keys))
		for c := range g.keys {
			codes = append(codes, c)
		}
		sort.Slice(codes, func(i, j int) bool {
			_, mi := modifierOf(codes[i])
			_, mj := modifierOf(codes[j])
			if mi != mj {
				return !mi
			}
			return codes[i] < codes[j]
		})
		for _, c := range codes {
			g.inj.Key(c, false, Mods{}, false)
		}
		clear(g.keys)
	}
	for b, down := range g.buttons {
		if down {
			g.inj.Button(g.lastX, g.lastY, b, false)
			g.buttons[b] = false
		}
	}
}

// take, saniyede RateLimit olaylık jeton kovasından bir jeton alır.
func (g *Gate) take() bool {
	now := g.now()
	if !g.refilled.IsZero() {
		g.tokens += now.Sub(g.refilled).Seconds() * RateLimit
		if g.tokens > RateLimit {
			g.tokens = RateLimit
		}
	}
	g.refilled = now
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	return true
}

func (g *Gate) notice(n Notice) {
	if g.onNotice != nil {
		g.onNotice(n)
	}
}

func (g *Gate) noticeBlocked() {
	now := g.now()
	if g.lastBlocked.IsZero() || now.Sub(g.lastBlocked) >= 5*time.Second {
		g.lastBlocked = now
		g.notice(NoticeBlockedShortcut)
	}
}

// blockedPayload, Decode'un reddettiği bir yükün engelli kısayol yüzünden
// reddedilip reddedilmediğini söyler (yalnız bildirim için).
func blockedPayload(p []byte) bool {
	var raw struct {
		Type string `json:"type"`
		Code string `json:"code"`
		Mods *Mods  `json:"mods"`
	}
	if len(p) > MaxBytes || json.Unmarshal(p, &raw) != nil || raw.Mods == nil {
		return false
	}
	return (raw.Type == KeyDown || raw.Type == KeyUp) && IsBlockedCombo(raw.Code, *raw.Mods)
}

package support

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/remote"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/sysinfo"
)

const (
	sessionUUID = "6f1c0000-0000-4000-8000-000000000001"
	secret      = "cs_SUPERSECRET_value"
	agent       = "rs:agent:" + sessionUUID
)

// --- sahteler ---

type fakeAPI struct {
	mu          sync.Mutex
	resolveErr  error
	consentErr  error
	endErr      error
	endReason   string
	mode        string
	recording   bool
	consents    []api.ConsentInput
	resolved    []api.ResolveInput
	ends        int
	reports     []string
	secretsSeen []string
}

func (f *fakeAPI) Resolve(_ context.Context, in api.ResolveInput) (api.Resolved, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, in)
	if f.resolveErr != nil {
		return api.Resolved{}, f.resolveErr
	}
	mode := f.mode
	if mode == "" {
		mode = "control"
	}
	return api.Resolved{
		SessionUUID: sessionUUID, ClientSecret: secret,
		Organization:  api.Organization{UUID: "o", Name: "Atlas Teknik Servis", Verified: true},
		Agent:         api.Person{UUID: "a", Name: "Elif Kaya"},
		ModeRequested: mode, RecordingRequested: f.recording, MaxDurationMinutes: 60,
		ConsentDeadline: time.Now().Add(time.Hour), Locale: "tr",
	}, nil
}

func (f *fakeAPI) Consent(_ context.Context, _, sec string, in api.ConsentInput) (api.CustomerView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secretsSeen = append(f.secretsSeen, sec)
	f.consents = append(f.consents, in)
	if f.consentErr != nil {
		return api.CustomerView{}, f.consentErr
	}
	return api.CustomerView{SessionUUID: sessionUUID}, nil
}

func (f *fakeAPI) Token(context.Context, string, string) (api.Grant, error) {
	return api.Grant{ServerURL: "wss://lk", Token: "lk-token"}, nil
}

func (f *fakeAPI) End(context.Context, string, string) (api.CustomerView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ends++
	if f.endErr != nil {
		return api.CustomerView{}, f.endErr
	}
	r := f.endReason
	if r == "" {
		r = "customer"
	}
	return api.CustomerView{EndedReason: r}, nil
}

func (f *fakeAPI) Report(_ context.Context, _, _, reason string) (api.CustomerView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, reason)
	return api.CustomerView{EndedReason: "reported", Reported: true}, nil
}

type fakeRemote struct {
	cfg  remote.Config
	exit chan remote.Exit
}

func (r *fakeRemote) Run(ctx context.Context) (remote.Exit, error) {
	select {
	case <-ctx.Done():
		return remote.ExitStopped, nil
	case e := <-r.exit:
		return e, errors.New("fake")
	}
}
func (r *fakeRemote) PushFrame(capture.Frame) {}

type fakeStream struct {
	mu      sync.Mutex
	stopped bool
	keys    int
}

func (s *fakeStream) ForceKeyframe() { s.mu.Lock(); s.keys++; s.mu.Unlock() }
func (s *fakeStream) Stop()          { s.mu.Lock(); s.stopped = true; s.mu.Unlock() }

type recInjector struct {
	mu     sync.Mutex
	events []string
}

func (r *recInjector) add(s string)          { r.mu.Lock(); r.events = append(r.events, s); r.mu.Unlock() }
func (r *recInjector) Move(float64, float64) { r.add("move") }
func (r *recInjector) Button(_, _ float64, b int, down bool) {
	if down {
		r.add("down")
	} else {
		r.add("up")
	}
}
func (r *recInjector) Scroll(float64, float64, int, int)    { r.add("scroll") }
func (r *recInjector) Key(string, bool, control.Mods, bool) { r.add("key") }
func (r *recInjector) count() int                           { r.mu.Lock(); defer r.mu.Unlock(); return len(r.events) }

type events struct {
	mu  sync.Mutex
	all []string // "event json"
}

func (e *events) notify(ev string, p any) {
	b, _ := json.Marshal(p)
	e.mu.Lock()
	e.all = append(e.all, ev+" "+string(b))
	e.mu.Unlock()
}

func (e *events) has(prefix string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.all {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func (e *events) dump() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.all, "\n")
}

type harness struct {
	t      *testing.T
	api    *fakeAPI
	svc    *Service
	ev     *events
	inj    *recInjector
	stream *fakeStream
	screen capture.Permission
	access capture.Permission
	pmu    sync.Mutex
	rem    chan *fakeRemote
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, api: &fakeAPI{}, ev: &events{}, inj: &recInjector{}, stream: &fakeStream{},
		screen: capture.Granted, access: capture.Granted, rem: make(chan *fakeRemote, 4)}
	perm := func(p *capture.Permission) func() capture.Permission {
		return func() capture.Permission { h.pmu.Lock(); defer h.pmu.Unlock(); return *p }
	}
	h.svc = New(Config{
		API: h.api, Version: "0.2.0", Notify: h.ev.notify, PermPoll: 10 * time.Millisecond,
		Platform: Platform{
			Name: "darwin", ScreenPermission: perm(&h.screen), Accessibility: perm(&h.access),
			RequestScreenPermission: perm(&h.screen), RequestAccessibility: perm(&h.access),
			MainDisplay: func() (capture.Display, error) {
				return capture.Display{PointsW: 1470, PointsH: 956, PixelsW: 2940, PixelsH: 1912, Scale: 2}, nil
			},
			StartCapture: func(capture.Config, func(capture.Frame), func(error)) (capture.Stream, error) {
				return h.stream, nil
			},
			Injector: h.inj,
			SysInfo: func() sysinfo.Info {
				return sysinfo.Info{OS: "macOS", OSVersion: "27.2", Arch: "arm64", AppVersion: "0.2.0"}
			},
		},
		NewRemote: func(c remote.Config) Remote {
			r := &fakeRemote{cfg: c, exit: make(chan remote.Exit, 1)}
			h.rem <- r
			return r
		},
	})
	t.Cleanup(func() { h.svc.Shutdown(context.Background()) })
	return h
}

func (h *harness) setPerm(screen, access capture.Permission) {
	h.pmu.Lock()
	h.screen, h.access = screen, access
	h.pmu.Unlock()
}

func (h *harness) call(method string, params any) (State, *ipc.Error) {
	h.t.Helper()
	raw, _ := json.Marshal(params)
	handlers := map[string]ipc.Handler{
		"session.resolve": h.svc.handleResolve, "session.consent": h.svc.handleConsent,
		"session.start": h.svc.handleStart, "session.stop": h.svc.handleStop,
		"control.revoke": h.svc.handleRevoke, "session.report": h.svc.handleReport,
	}
	res, err := handlers[method](context.Background(), raw)
	if err != nil {
		var e *ipc.Error
		if !errors.As(err, &e) {
			h.t.Fatalf("%s: ipc.Error değil: %v", method, err)
		}
		return State{}, e
	}
	return res.(StateResult).State, nil
}

func (h *harness) must(method string, params any) State {
	h.t.Helper()
	st, err := h.call(method, params)
	if err != nil {
		h.t.Fatalf("%s: %v", method, err)
	}
	return st
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("beklenen olmadı: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// activeSession, kontrol onaylı ve bağlı bir oturum kurar.
func (h *harness) activeSession() *fakeRemote {
	h.t.Helper()
	h.must("session.resolve", map[string]any{"code": "123 456-789"})
	h.must("session.consent", map[string]any{"view": true})
	st := h.must("session.start", nil)
	if st.Phase != PhaseStarting {
		h.t.Fatalf("start sonrası phase = %s", st.Phase)
	}
	r := <-h.rem
	r.cfg.OnStatus(remote.Status{Connection: remote.Connected, AgentPresent: true})
	eventually(h.t, "active", func() bool { return h.svc.State().Phase == PhaseActive })
	return r
}

func move() []byte { return []byte(`{"v":1,"seq":1,"t":1,"type":"pointer.move","x":0.5,"y":0.5}`) }

// --- testler ---

func TestFullFlowWithControlAndRevoke(t *testing.T) {
	h := newHarness(t)
	st := h.must("session.resolve", map[string]any{"code": "123 456-789", "share_system_info": true})
	if st.Phase != PhaseResolved || st.Session == nil || st.Session.Agent.Name != "Elif Kaya" || st.Control != ControlRequested {
		t.Fatalf("resolve durumu: %+v", st)
	}
	if got := h.api.resolved[0]; got.Code != "123456789" || got.ClientOS != "macOS 27.2 arm64" || got.ClientVersion != "0.2.0" {
		t.Errorf("resolve girdisi: %+v", got)
	}

	if _, e := h.call("session.start", nil); e == nil || e.Code != CodeInvalidState {
		t.Errorf("onaysız start: %v", e)
	}
	if _, e := h.call("session.consent", map[string]any{"control": true}); e == nil || e.Code != CodeInvalidParams {
		t.Errorf("izleme onayından önce kontrol: %v", e)
	}
	st = h.must("session.consent", map[string]any{"view": true})
	if st.Phase != PhaseReady || !st.Consent.View {
		t.Fatalf("view onayı: %+v", st)
	}
	h.must("session.start", nil)
	r := <-h.rem
	if r.cfg.AgentIdentity != agent {
		t.Errorf("temsilci kimliği %q", r.cfg.AgentIdentity)
	}

	// Bağlanmadan kapı kapalı.
	r.cfg.OnData(agent, move())
	if h.inj.count() != 0 {
		t.Fatal("bağlanmadan girdi uygulandı")
	}
	r.cfg.OnStatus(remote.Status{Connection: remote.Connected, AgentPresent: true})
	eventually(t, "active", func() bool { return h.svc.State().Phase == PhaseActive })
	st = h.svc.State()
	if st.StartedAt == nil || st.EndsAt == nil || !st.AgentPresent {
		t.Errorf("active durumu: %+v", st)
	}

	// Kontrol onayı yokken girdi atılır.
	r.cfg.OnData(agent, move())
	if h.inj.count() != 0 {
		t.Fatal("onaysız girdi uygulandı")
	}
	st = h.must("session.consent", map[string]any{"control": true})
	if st.Control != ControlActive {
		t.Fatalf("kontrol = %s", st.Control)
	}
	r.cfg.OnData(agent, move())
	r.cfg.OnData("rs:customer:x", move()) // sahte gönderen
	if h.inj.count() != 1 {
		t.Fatalf("girdi sayısı %d", h.inj.count())
	}
	r.cfg.OnKeyframe()
	if h.stream.keys != 1 {
		t.Error("anahtar kare isteği iletilmedi")
	}

	// Yeniden bağlanırken kapı kapanır.
	r.cfg.OnStatus(remote.Status{Connection: remote.Reconnecting})
	if st := h.svc.State(); st.Phase != PhaseReconnecting {
		t.Errorf("reconnecting: %s", st.Phase)
	}
	r.cfg.OnData(agent, move())
	if h.inj.count() != 1 {
		t.Fatal("yeniden bağlanırken girdi uygulandı")
	}
	r.cfg.OnStatus(remote.Status{Connection: remote.Connected, AgentPresent: true})

	// Geri alma: yerelde hemen kapanır, sonra API'ye gider.
	st = h.must("control.revoke", nil)
	if st.Control != ControlRevoked || st.Consent.Control {
		t.Fatalf("geri alma: %+v", st)
	}
	r.cfg.OnData(agent, move())
	if h.inj.count() != 1 {
		t.Fatal("geri almadan sonra girdi uygulandı")
	}
	last := h.api.consents[len(h.api.consents)-1]
	if last.Control == nil || *last.Control {
		t.Errorf("API'ye control:false gitmedi: %+v", last)
	}
	if !h.ev.has(`control {"state":"revoked","previous":"active","by":"customer"`) {
		t.Errorf("control olayı yok:\n%s", h.ev.dump())
	}

	st = h.must("session.stop", nil)
	if st.Phase != PhaseEnded || st.Summary == nil {
		t.Fatalf("stop: %+v", st)
	}
	sum := st.Summary
	if sum.EndedReason != "customer" || sum.EndedBy != "customer" || !sum.Control.Granted ||
		sum.Control.RevokedBy == nil || *sum.Control.RevokedBy != "customer" || sum.AgentName != "Elif Kaya" ||
		sum.OrganizationName != "Atlas Teknik Servis" || sum.StartedAt == nil {
		t.Errorf("özet: %+v", sum)
	}
	if !h.stream.stopped {
		t.Error("yakalama durmadı")
	}
	for _, s := range h.api.secretsSeen {
		if s != secret {
			t.Errorf("API'ye yanlış sır gitti: %q", s)
		}
	}
	// Sır ve token hiçbir olayda yok.
	if d := h.ev.dump(); strings.Contains(d, secret) || strings.Contains(d, "lk-token") {
		t.Errorf("olaylarda sır var:\n%s", d)
	}
}

func TestResolveValidationAndErrors(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		params any
		code   int
	}{
		{map[string]any{}, CodeInvalidParams},
		{map[string]any{"code": "12345678"}, CodeInvalidParams},
		{map[string]any{"code": "12345678a"}, CodeInvalidParams},
		{map[string]any{"code": "123456789", "link": "x"}, CodeInvalidParams},
		{map[string]any{"link": "https://evil.example/join?t=abc"}, CodeInvalidParams},
		{map[string]any{"link": "clickfix-support://join?t=a b"}, CodeInvalidParams},
	}
	for _, c := range cases {
		if _, e := h.call("session.resolve", c.params); e == nil || e.Code != c.code {
			t.Errorf("%v: %v", c.params, e)
		}
	}
	if len(h.api.resolved) != 0 {
		t.Fatal("geçersiz girdi API'ye gitti")
	}

	apiCases := []struct {
		err  error
		code int
	}{
		{&api.Error{Status: 404}, CodeCodeInvalid},
		{&api.Error{Status: 429}, CodeRateLimited},
		{&api.Error{Status: 403}, CodeUnavailable},
		{&api.Error{Status: 410}, CodeEnded},
		{&api.Error{Network: true}, CodeNetwork},
		{&api.Error{Status: 502}, CodeServer},
	}
	for _, c := range apiCases {
		h.api.resolveErr = c.err
		_, e := h.call("session.resolve", map[string]any{"code": "123456789"})
		if e == nil || e.Code != c.code || e.Data.(errorData).Kind != Kind(c.code) {
			t.Errorf("%v → %v", c.err, e)
		}
	}
	h.api.resolveErr = nil

	// Bağlantı ve ayrıntısız sistem bilgisi.
	st := h.must("session.resolve", map[string]any{"link": "clickfix-support://join?t=AbC-123_x"})
	if st.Phase != PhaseResolved {
		t.Fatal(st.Phase)
	}
	last := h.api.resolved[len(h.api.resolved)-1]
	if last.Token != "AbC-123_x" || last.ClientOS != "macos" {
		t.Errorf("bağlantı girdisi: %+v", last)
	}
	if _, e := h.call("session.resolve", map[string]any{"code": "123456789"}); e == nil || e.Code != CodeInvalidState {
		t.Errorf("ikinci oturum: %v", e)
	}
}

func TestStartRequiresScreenPermission(t *testing.T) {
	h := newHarness(t)
	h.setPerm(capture.Denied, capture.Granted)
	h.must("session.resolve", map[string]any{"code": "123456789"})
	h.must("session.consent", map[string]any{"view": true})
	_, e := h.call("session.start", nil)
	if e == nil || e.Code != CodePermissionRequired || e.Data.(errorData).Permission != "screen_recording" {
		t.Fatalf("izinsiz start: %+v", e)
	}
	h.setPerm(capture.Unsupported, capture.Unsupported)
	if _, e := h.call("session.start", nil); e == nil || e.Code != CodeUnsupported {
		t.Fatalf("desteklenmeyen platform: %+v", e)
	}
}

func TestPendingPermissionBecomesActive(t *testing.T) {
	h := newHarness(t)
	h.setPerm(capture.Granted, capture.Denied)
	r := h.activeSession()
	st := h.must("session.consent", map[string]any{"control": true})
	if st.Control != ControlPendingPermission {
		t.Fatalf("kontrol = %s", st.Control)
	}
	r.cfg.OnData(agent, move())
	if h.inj.count() != 0 {
		t.Fatal("izinsiz girdi uygulandı")
	}
	eventually(t, "accessibility_missing", func() bool {
		return h.ev.has(`control {"state":"pending_permission","previous":"pending_permission","notice":"accessibility_missing"`)
	})

	h.setPerm(capture.Granted, capture.Granted) // kullanıcı Sistem Ayarları'nda izin verdi
	eventually(t, "active", func() bool { return h.svc.State().Control == ControlActive })
	r.cfg.OnData(agent, move())
	if h.inj.count() != 1 {
		t.Fatal("izin geldikten sonra girdi uygulanmadı")
	}
	h.setPerm(capture.Granted, capture.Denied) // izin oturumda kaldırıldı
	eventually(t, "pending", func() bool { return h.svc.State().Control == ControlPendingPermission })
	r.cfg.OnData(agent, move())
	if h.inj.count() != 1 {
		t.Fatal("izin kalktıktan sonra girdi uygulandı")
	}
}

func TestServerClosesRoom(t *testing.T) {
	h := newHarness(t)
	h.api.endReason = "agent"
	r := h.activeSession()
	h.must("session.consent", map[string]any{"control": true})
	r.exit <- remote.ExitClosed
	eventually(t, "ended", func() bool { return h.svc.State().Phase == PhaseEnded })
	st := h.svc.State()
	if st.Summary.EndedReason != "agent" || st.Summary.EndedBy != "agent" || st.Summary.Control.RevokedBy != nil {
		t.Errorf("özet: %+v", st.Summary)
	}
	r.cfg.OnData(agent, move())
	if h.inj.count() != 0 {
		t.Fatal("bitmiş oturumda girdi uygulandı")
	}
	// Yeni oturum çözülebilir.
	if st := h.must("session.resolve", map[string]any{"code": "123456789"}); st.Phase != PhaseResolved || st.Summary != nil {
		t.Errorf("yeni oturum: %+v", st)
	}
}

func TestStopWhenServerUnreachable(t *testing.T) {
	h := newHarness(t)
	h.activeSession()
	h.api.endErr = &api.Error{Network: true}
	st := h.must("session.stop", nil)
	if st.Phase != PhaseEnded || st.Summary.EndedReason != ReasonConnectionLost || st.Summary.EndedBy != "connection" {
		t.Fatalf("özet: %+v", st.Summary)
	}
	if !h.ev.has(`error {"code":-32005,"kind":"NETWORK"`) {
		t.Errorf("hata olayı yok:\n%s", h.ev.dump())
	}
}

func TestLostBeforeStartReturnsToReady(t *testing.T) {
	h := newHarness(t)
	h.must("session.resolve", map[string]any{"code": "123456789"})
	h.must("session.consent", map[string]any{"view": true})
	h.must("session.start", nil)
	r := <-h.rem
	r.exit <- remote.ExitLost
	eventually(t, "ready", func() bool { return h.svc.State().Phase == PhaseReady })
	if h.api.ends != 0 {
		t.Error("hiç başlamayan oturum sunucuda bitirildi")
	}
	if !h.ev.has(`error {"code":-32013,"kind":"LIVEKIT"`) {
		t.Errorf("hata olayı yok:\n%s", h.ev.dump())
	}
}

func TestDeclineControlAndRevokeWhenAPIFails(t *testing.T) {
	h := newHarness(t)
	r := h.activeSession()
	st := h.must("session.consent", map[string]any{"control": false})
	if st.Control != ControlViewOnly {
		t.Errorf("Yalnız izlesin: %s", st.Control)
	}
	h.must("session.consent", map[string]any{"control": true})
	h.api.consentErr = &api.Error{Network: true}
	st = h.must("control.revoke", nil)
	if st.Control != ControlRevoked {
		t.Fatalf("API hatasında geri alma: %s", st.Control)
	}
	r.cfg.OnData(agent, move())
	if h.inj.count() != 0 {
		t.Fatal("API hatası kapıyı açtı")
	}
}

func TestViewOnlyModeAndRecording(t *testing.T) {
	h := newHarness(t)
	h.api.mode, h.api.recording = "view", true
	h.must("session.resolve", map[string]any{"code": "123456789"})
	if _, e := h.call("session.consent", map[string]any{"view": true, "control": true}); e == nil || e.Code != CodeInvalidParams {
		t.Errorf("izleme modunda kontrol: %v", e)
	}
	st := h.must("session.consent", map[string]any{"view": true, "recording": true})
	if st.Control != ControlViewOnly || st.Consent.Recording == nil || !*st.Consent.Recording {
		t.Fatalf("durum: %+v", st)
	}
	h.must("session.start", nil)
	r := <-h.rem
	r.cfg.OnStatus(remote.Status{Connection: remote.Connected})
	eventually(t, "active", func() bool { return h.svc.State().Phase == PhaseActive })
	st = h.must("session.stop", nil)
	if !st.Summary.Recorded || st.Summary.Control.Granted {
		t.Errorf("özet: %+v", st.Summary)
	}
}

func TestReport(t *testing.T) {
	h := newHarness(t)
	h.activeSession()
	if _, e := h.call("session.report", map[string]any{"reason": strings.Repeat("ş", 1001)}); e == nil || e.Code != CodeInvalidParams {
		t.Errorf("uzun neden: %v", e)
	}
	st := h.must("session.report", map[string]any{"reason": "Para istedi"})
	if st.Phase != PhaseEnded || !st.Summary.Reported || st.Summary.EndedReason != "reported" || st.Summary.EndedBy != "customer" {
		t.Fatalf("bildirim: %+v", st.Summary)
	}
	if h.api.ends != 0 || len(h.api.reports) != 1 {
		t.Errorf("ends=%d reports=%v", h.api.ends, h.api.reports)
	}

	// Bittikten sonra da bildirilebilir.
	h2 := newHarness(t)
	h2.activeSession()
	h2.must("session.stop", nil)
	st = h2.must("session.report", nil)
	if !st.Summary.Reported || st.Summary.EndedReason != "customer" {
		t.Errorf("sonradan bildirim: %+v", st.Summary)
	}
}

func TestControlSecondsCounted(t *testing.T) {
	h := newHarness(t)
	var mu sync.Mutex
	now := time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)
	h.svc.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	h.activeSession()
	h.must("session.consent", map[string]any{"control": true})
	advance(90 * time.Second)
	h.must("control.revoke", nil)
	advance(30 * time.Second)
	st := h.must("session.stop", nil)
	if st.Summary.Control.Seconds != 90 || st.Summary.DurationSeconds != 120 {
		t.Errorf("süreler: %+v", st.Summary)
	}
}

func TestEndedBy(t *testing.T) {
	cases := map[string]string{
		"customer": "customer", "declined": "customer", "reported": "customer",
		"agent": "agent", "conversation_closed": "agent",
		"timeout": "time", "max_duration": "time", "code_expired": "time", "attempts": "time",
		"platform": "platform", ReasonConnectionLost: "connection",
	}
	for in, want := range cases {
		if got := EndedBy(in); got != want {
			t.Errorf("EndedBy(%q) = %q", in, got)
		}
	}
}

func TestJoinToken(t *testing.T) {
	ok := []string{"clickfix-support://join?t=abc", "abc_DEF-1", "clickfix-support:join?t=x"}
	for _, s := range ok {
		if _, good := JoinToken(s); !good {
			t.Errorf("%q reddedildi", s)
		}
	}
	bad := []string{"", "clickfix-support://other?t=abc", "clickfix-support://join", "a/b", strings.Repeat("a", MaxJoinToken+1)}
	for _, s := range bad {
		if _, good := JoinToken(s); good {
			t.Errorf("%q kabul edildi", s)
		}
	}
}

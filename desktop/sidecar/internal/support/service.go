// Package support, sidecar'ın --mode=support düzenleyicisidir: IPC
// yöntemlerini (IPC.md) API istemcisi, ekran yakalama, LiveKit oturumu ve
// kontrol kapısıyla birleştirir ve tek bir durum makinesini yönetir.
//
// Durum s.mu altında tutulur. Her değişiklik change() içinden geçer; o da
// kontrol durumunu yeniden hesaplar, kapıyı ayarlar, kontrol süresini sayar
// ve kilit bırakıldıktan sonra control/state olaylarını yazar.
package support

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/remote"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/sysinfo"
)

// API, CF-233 public uçlarıdır (*api.Client).
type API interface {
	Resolve(ctx context.Context, in api.ResolveInput) (api.Resolved, error)
	Consent(ctx context.Context, session, secret string, in api.ConsentInput) (api.CustomerView, error)
	Token(ctx context.Context, session, secret string) (api.Grant, error)
	End(ctx context.Context, session, secret string) (api.CustomerView, error)
	Report(ctx context.Context, session, secret, reason string) (api.CustomerView, error)
}

// Remote, LiveKit oturumudur (*remote.Session).
type Remote interface {
	Run(ctx context.Context) (remote.Exit, error)
	PushFrame(f capture.Frame)
}

// Platform, işletim sistemine bağlı parçalardır; testlerde sahtesi verilir.
type Platform struct {
	Name                    string
	ScreenPermission        func() capture.Permission
	RequestScreenPermission func() capture.Permission
	Accessibility           func() capture.Permission
	RequestAccessibility    func() capture.Permission
	MainDisplay             func() (capture.Display, error)
	StartCapture            func(capture.Config, func(capture.Frame), func(error)) (capture.Stream, error)
	Injector                control.Injector
	SysInfo                 func() sysinfo.Info
}

// Config, düzenleyici ayarıdır.
type Config struct {
	API        API
	Platform   Platform
	Version    string
	Logger     *slog.Logger
	Notify     func(event string, params any)
	NewRemote  func(remote.Config) Remote
	Now        func() time.Time
	PermPoll   time.Duration // oturum sürerken Erişilebilirlik denetim aralığı
	StopWait   time.Duration // durdururken bağlantı döngüsünü bekleme süresi
	ExpireWait time.Duration // onay süresi dolduktan sonra sunucuya sorma payı
}

// Service, support modunun durum makinesidir.
type Service struct {
	cfg  Config
	log  *slog.Logger
	gate *control.Gate
	now  func() time.Time

	emitMu sync.Mutex // state olaylarının sırasını korur

	mu       sync.Mutex
	phase    Phase
	resolved *api.Resolved
	secret   string

	consentView      bool
	consentControl   bool
	consentRecording *bool
	controlDeclined  bool // müşteri kontrol isteğini reddetti
	controlRevoked   bool // müşteri verilmiş onayı geri aldı
	controlGranted   bool // oturumda kontrol onayı hiç verildi mi

	control      Control
	connection   remote.Connection
	agentPresent bool
	startedAt    time.Time
	endsAt       time.Time
	perms        Permissions
	summary      *Summary
	reported     bool

	controlOn    time.Time // kontrolün yürürlüğe girdiği an (yürürlükte değilse sıfır)
	controlTotal time.Duration

	runCancel  context.CancelFunc
	runDone    chan struct{}
	stream     atomic.Pointer[streamBox]
	ended      chan struct{} // finish'te kapanır
	expire     *time.Timer
	generation int // her resolve'da artar; eski oturumun geri çağrılarını ayırır
}

type streamBox struct{ s capture.Stream }

// New bir düzenleyici kurar.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Notify == nil {
		cfg.Notify = func(string, any) {}
	}
	if cfg.NewRemote == nil {
		cfg.NewRemote = func(c remote.Config) Remote { return remote.New(c) }
	}
	if cfg.PermPoll <= 0 {
		cfg.PermPoll = 2 * time.Second
	}
	if cfg.StopWait <= 0 {
		cfg.StopWait = 5 * time.Second
	}
	if cfg.ExpireWait <= 0 {
		cfg.ExpireWait = 5 * time.Second
	}
	if cfg.Platform.Name == "" {
		cfg.Platform.Name = runtime.GOOS
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Service{cfg: cfg, log: log, now: cfg.Now, phase: PhaseIdle, control: ControlViewOnly, connection: remote.None}
	s.gate = control.NewGate(cfg.Platform.Injector, cfg.Now, func(n control.Notice) {
		// Kapının kilidi altında çağrılır; olay ayrı goroutine'de yazılır.
		go s.emitNotice(n)
	})
	s.perms = s.readPermissions()
	return s
}

// Gate, kontrol kapısıdır (testler ve istatistik için).
func (s *Service) Gate() *control.Gate { return s.gate }

// State, anlık görüntüdür.
func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Service) snapshot() State {
	st := State{
		Phase: s.phase, Control: s.control, Connection: s.connection, AgentPresent: s.agentPresent,
		StartedAt: timePtr(s.startedAt), EndsAt: timePtr(s.endsAt), Permissions: s.perms,
		Consent: Consent{View: s.consentView, Control: s.consentControl, Recording: s.consentRecording},
	}
	if s.resolved != nil {
		st.Session = sessionView(*s.resolved)
	}
	if s.summary != nil {
		sum := *s.summary
		st.Summary = &sum
	}
	return st
}

func (s *Service) result() StateResult { return StateResult{State: s.State()} }

// --- değişiklik ve olaylar ---

// controlInEffect: kontrol yürürlükte (temsilci girdi gönderebilir durumda).
func (s *Service) controlInEffect() bool {
	return s.control == ControlActive && (s.phase == PhaseActive || s.phase == PhaseReconnecting)
}

func (s *Service) computeControl() Control {
	switch {
	case s.resolved == nil || s.resolved.ModeRequested != "control":
		return ControlViewOnly
	case s.consentControl && s.perms.Accessibility != capture.Granted:
		return ControlPendingPermission
	case s.consentControl:
		return ControlActive
	case s.controlRevoked:
		return ControlRevoked
	case s.controlDeclined:
		return ControlViewOnly
	}
	return ControlRequested
}

// change, f'yi kilit altında çalıştırır, türetilmiş durumu günceller ve
// olayları yazar. by, kontrol durumu değişirse olayın by alanıdır.
func (s *Service) change(by string, f func()) {
	s.mu.Lock()
	prev := s.control
	wasOn := s.controlInEffect()
	f()
	s.control = s.computeControl()
	now := s.now()
	isOn := s.controlInEffect()
	switch {
	case wasOn && !isOn:
		s.controlTotal += now.Sub(s.controlOn)
		s.controlOn = time.Time{}
	case !wasOn && isOn:
		s.controlOn = now
	}
	s.applyGate()
	cur := s.control
	s.mu.Unlock()

	if cur != prev {
		s.cfg.Notify("control", ControlEvent{State: cur, Previous: prev, By: by, At: now.UTC()})
	}
	s.emitState()
}

// applyGate kapıyı duruma göre ayarlar (s.mu altında).
func (s *Service) applyGate() {
	agent := ""
	if s.resolved != nil {
		agent = "rs:agent:" + s.resolved.SessionUUID
	}
	active := s.phase == PhaseActive && s.connection == remote.Connected
	s.gate.SetControl(s.consentControl)
	s.gate.SetPermitted(s.perms.Accessibility == capture.Granted)
	s.gate.SetSession(active, agent)
}

func (s *Service) emitState() {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.cfg.Notify("state", s.State())
}

func (s *Service) emitError(e *ipc.Error, fatal bool) {
	s.cfg.Notify("error", errorEvent(e, fatal))
}

func (s *Service) emitNotice(n control.Notice) {
	s.mu.Lock()
	c := s.control
	s.mu.Unlock()
	s.cfg.Notify("control", ControlEvent{State: c, Previous: c, Notice: string(n), At: s.now().UTC()})
}

// --- izinler ---

func (s *Service) readPermissions() Permissions {
	p := s.cfg.Platform
	perm := func(f func() capture.Permission) capture.Permission {
		if f == nil {
			return capture.Unsupported
		}
		return f()
	}
	return Permissions{Platform: p.Name, ScreenRecording: perm(p.ScreenPermission), Accessibility: perm(p.Accessibility)}
}

// refreshPermissions izinleri yeniden okur; değiştiyse durumu yayar.
func (s *Service) refreshPermissions(by string) Permissions {
	fresh := s.readPermissions()
	s.mu.Lock()
	fresh.RestartRequired = s.perms.RestartRequired && fresh.ScreenRecording != capture.Granted
	changed := fresh != s.perms
	s.mu.Unlock()
	if changed {
		s.change(by, func() { s.perms = fresh })
	}
	return fresh
}

// --- oturum yaşam döngüsü ---

// begin, yeni çözülen oturumu kurar.
func (s *Service) begin(r api.Resolved) {
	s.change(BySystem, func() {
		s.generation++
		s.phase = PhaseResolved
		secret := r.ClientSecret
		r.ClientSecret = ""
		s.resolved, s.secret = &r, secret
		s.consentView, s.consentControl, s.consentRecording = false, false, nil
		s.controlDeclined, s.controlRevoked, s.controlGranted = false, false, false
		s.connection, s.agentPresent = remote.None, false
		s.startedAt, s.endsAt = time.Time{}, time.Time{}
		s.summary, s.reported = nil, false
		s.controlOn, s.controlTotal = time.Time{}, 0
		s.ended = make(chan struct{})
		gen := s.generation
		if s.expire != nil {
			s.expire.Stop()
		}
		if d := r.ConsentDeadline.Sub(s.now()); !r.ConsentDeadline.IsZero() {
			s.expire = time.AfterFunc(max(d, 0)+s.cfg.ExpireWait, func() { s.expireConsent(gen) })
		}
	})
}

// expireConsent: izleme onayı süresinde verilmediyse sunucu oturumu
// kapatmıştır; nedeni öğrenip yerelde bitirir.
func (s *Service) expireConsent(gen int) {
	s.mu.Lock()
	stale := gen != s.generation || s.phase != PhaseResolved
	s.mu.Unlock()
	if stale {
		return
	}
	if !s.beginEnd(gen) {
		return
	}
	s.finish(s.endOnServer(), nil)
}

// beginEnd, oturumu ending'e alır. Başka biri zaten bitiriyorsa false.
func (s *Service) beginEnd(gen int) bool {
	ok := false
	s.change(BySystem, func() {
		if gen != s.generation {
			return
		}
		switch s.phase {
		case PhaseIdle, PhaseEnding, PhaseEnded:
			return
		}
		ok = true
		s.phase = PhaseEnding
	})
	if ok {
		s.gate.Close()
	}
	return ok
}

// teardown bağlantı döngüsünü ve yakalamayı durdurur.
func (s *Service) teardown() {
	s.mu.Lock()
	cancel, done := s.runCancel, s.runDone
	s.runCancel, s.runDone = nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-time.After(s.cfg.StopWait):
			s.log.Warn("bağlantı döngüsü zamanında durmadı")
		}
	}
	if b := s.stream.Swap(nil); b != nil {
		b.s.Stop()
	}
}

// endOnServer, sunucuda oturumu bitirir ve nedeni döner. Sunucuya
// ulaşılamazsa connection_lost.
func (s *Service) endOnServer() string {
	s.mu.Lock()
	r, secret := s.resolved, s.secret
	s.mu.Unlock()
	if r == nil {
		return ReasonConnectionLost
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	v, err := s.cfg.API.End(ctx, r.SessionUUID, secret)
	if err != nil {
		s.log.Warn("oturum sunucuda bitirilemedi", "err", err)
		s.emitError(fromAPI(err), false)
		return ReasonConnectionLost
	}
	if v.Reported {
		s.mu.Lock()
		s.reported = true
		s.mu.Unlock()
	}
	if v.EndedReason == "" {
		return "customer"
	}
	return v.EndedReason
}

// finish özeti yazar ve oturumu ended'a alır.
func (s *Service) finish(reason string, revokedBy *string) {
	s.teardown()
	s.change(BySystem, func() {
		now := s.now()
		sum := &Summary{EndedAt: now.UTC(), EndedReason: reason, EndedBy: EndedBy(reason), Reported: s.reported || reason == "reported"}
		if s.resolved != nil {
			sum.AgentName, sum.OrganizationName = s.resolved.Agent.Name, s.resolved.Organization.Name
		}
		if !s.startedAt.IsZero() {
			sum.StartedAt = timePtr(s.startedAt)
			sum.DurationSeconds = int64(now.Sub(s.startedAt) / time.Second)
			sum.Recorded = s.consentRecording != nil && *s.consentRecording
		}
		sum.Control = ControlSummary{Granted: s.controlGranted, Seconds: int64(s.controlTotal / time.Second)}
		if s.controlRevoked {
			c := ByCustomer
			sum.Control.RevokedBy = &c
		} else if revokedBy != nil {
			sum.Control.RevokedBy = revokedBy
		}
		s.summary = sum
		s.phase = PhaseEnded
		s.connection, s.agentPresent = remote.None, false
		s.consentControl = false
		if s.expire != nil {
			s.expire.Stop()
			s.expire = nil
		}
		// Sır oturum bittikten sonra da report için tutulur; yeni resolve
		// onu değiştirir.
		if s.ended != nil {
			close(s.ended)
			s.ended = nil
		}
	})
	s.log.Info("oturum bitti", "reason", reason)
}

// waitEnded, başka bir yolun başlattığı bitişi bekler.
func (s *Service) waitEnded(ctx context.Context) {
	s.mu.Lock()
	ch := s.ended
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-ctx.Done():
	}
}

// --- yayın ---

// startRun, yakalamayı ve bağlantı döngüsünü arka planda başlatır.
func (s *Service) startRun(gen int) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.runCancel, s.runDone = cancel, done
	r, secret := *s.resolved, s.secret
	endsAt := s.endsAt
	s.mu.Unlock()

	go func() {
		defer close(done)
		s.run(ctx, gen, r, secret, endsAt)
	}()
}

func (s *Service) run(ctx context.Context, gen int, r api.Resolved, secret string, endsAt time.Time) {
	p := s.cfg.Platform
	var d capture.Display
	if p.MainDisplay != nil {
		if got, err := p.MainDisplay(); err == nil {
			d = got
		}
	}
	cfg := capture.DefaultConfig(d)

	rem := s.cfg.NewRemote(remote.Config{
		Grant:         func(ctx context.Context) (api.Grant, error) { return s.cfg.API.Token(ctx, r.SessionUUID, secret) },
		AgentIdentity: "rs:agent:" + r.SessionUUID,
		Capture:       cfg,
		Logger:        s.log,
		OnData:        func(sender string, payload []byte) { s.gate.Handle(sender, payload) },
		OnStatus:      func(st remote.Status) { s.onStatus(gen, st) },
		OnKeyframe: func() {
			if b := s.stream.Load(); b != nil {
				b.s.ForceKeyframe()
			}
		},
	})

	stream, err := p.StartCapture(cfg, rem.PushFrame, func(err error) {
		s.log.Error("ekran yakalama durdu", "err", err)
		s.emitError(rpcError(CodeCaptureFailed, msgCapture), true)
		go s.stopFromSystem(gen)
	})
	if err != nil {
		s.log.Error("ekran yakalama başlatılamadı", "err", err)
		e := rpcError(CodeCaptureFailed, msgCapture)
		if errors.Is(err, capture.ErrPermission) {
			e = permissionError("screen_recording", "Ekran Kaydı izni gerekli.")
		}
		s.emitError(e, false)
		s.change(BySystem, func() {
			if gen == s.generation && s.phase == PhaseStarting {
				s.phase = PhaseReady
				s.connection = remote.None
			}
		})
		return
	}
	s.stream.Store(&streamBox{stream})

	pollCtx, stopPoll := context.WithCancel(ctx)
	defer stopPoll()
	go s.pollPermissions(pollCtx)
	if !endsAt.IsZero() {
		// Sunucu süre dolunca odayı kapatır; bu yalnız güvenlik payıdır.
		t := time.AfterFunc(time.Until(endsAt)+time.Minute, func() { s.stopFromSystem(gen) })
		defer t.Stop()
	}

	exit, rerr := rem.Run(ctx)
	switch exit {
	case remote.ExitStopped:
		return // durdurma yolu kendisi bitirir
	case remote.ExitClosed:
		s.log.Info("oda kapandı", "err", rerr)
	case remote.ExitLost:
		s.log.Warn("bağlantı yeniden kurulamadı", "err", rerr)
	}

	s.mu.Lock()
	everStarted := !s.startedAt.IsZero()
	s.mu.Unlock()
	if exit == remote.ExitLost && !everStarted {
		// Hiç yayına geçilemedi: oturum bitmedi, müşteri yeniden deneyebilir.
		if b := s.stream.Swap(nil); b != nil {
			b.s.Stop()
		}
		s.emitError(rpcError(CodeLiveKit, msgLiveKit), false)
		s.change(BySystem, func() {
			if gen == s.generation && s.phase == PhaseStarting {
				s.phase = PhaseReady
				s.connection, s.agentPresent = remote.None, false
			}
		})
		s.mu.Lock()
		s.runCancel, s.runDone = nil, nil
		s.mu.Unlock()
		return
	}
	go func() {
		if !s.beginEnd(gen) {
			return
		}
		reason := s.endOnServer()
		if exit == remote.ExitLost {
			s.emitError(rpcError(CodeLiveKit, "Bağlantı koptu ve yeniden kurulamadı."), true)
		}
		s.finish(reason, nil)
	}()
}

// stopFromSystem, oturumu müşteri dışı bir nedenle bitirir.
func (s *Service) stopFromSystem(gen int) {
	if !s.beginEnd(gen) {
		return
	}
	s.finish(s.endOnServer(), nil)
}

func (s *Service) onStatus(gen int, st remote.Status) {
	s.change(BySystem, func() {
		if gen != s.generation {
			return
		}
		switch s.phase {
		case PhaseStarting, PhaseActive, PhaseReconnecting:
		default:
			return
		}
		s.connection, s.agentPresent = st.Connection, st.AgentPresent
		switch st.Connection {
		case remote.Connected:
			if s.startedAt.IsZero() {
				s.startedAt = s.now()
				if s.endsAt.IsZero() && s.resolved != nil && s.resolved.MaxDurationMinutes > 0 {
					s.endsAt = s.startedAt.Add(time.Duration(s.resolved.MaxDurationMinutes) * time.Minute)
				}
			}
			s.phase = PhaseActive
		case remote.Reconnecting:
			if s.phase == PhaseActive {
				s.phase = PhaseReconnecting
			}
		}
	})
}

// pollPermissions oturum sürerken Erişilebilirlik iznini izler: izin
// verilince pending_permission → active, kaldırılınca kapı kapanır.
func (s *Service) pollPermissions(ctx context.Context) {
	t := time.NewTicker(s.cfg.PermPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshPermissions(BySystem)
		}
	}
}

package support

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/remote"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/sysinfo"
)

// Register, support yöntemlerini IPC sunucusuna kaydeder.
func (s *Service) Register(srv *ipc.Server) {
	srv.Handle("session.resolve", s.handleResolve)
	srv.Handle("session.consent", s.handleConsent)
	srv.Handle("session.start", s.handleStart)
	srv.Handle("session.stop", s.handleStop)
	srv.Handle("control.revoke", s.handleRevoke)
	srv.Handle("permissions.status", s.handlePermissionsStatus)
	srv.Handle("permissions.request", s.handlePermissionsRequest)
	srv.Handle("session.report", s.handleReport)
	srv.Handle("sysinfo.get", s.handleSysInfo)
}

// Shutdown, süren oturumu bitirir (kabuk kapanırken, SIGTERM).
func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	phase := s.phase
	s.mu.Unlock()
	if phase == PhaseIdle || phase == PhaseEnded {
		return
	}
	_, _ = s.Stop(ctx)
}

func decodeParams(raw json.RawMessage, v any) *ipc.Error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return rpcError(CodeInvalidParams, "Parametreler okunamadı.")
	}
	return nil
}

func invalid(msg string) *ipc.Error { return rpcError(CodeInvalidParams, msg) }

// --- session.resolve ---

type resolveParams struct {
	Code            *string `json:"code"`
	Link            *string `json:"link"`
	ShareSystemInfo bool    `json:"share_system_info"`
}

// NormalizeCode, kullanıcının yazdığı kodu 9 haneye indirger (boşluk ve
// tire yok sayılır).
func NormalizeCode(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == ' ' || r == '-' || r == ' ':
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	c := b.String()
	return c, len(c) == 9
}

// MaxJoinToken, katılma bağlantısı token'ının kabul edilen en büyük boyutudur.
const MaxJoinToken = 512

// JoinToken, clickfix-support://join?t=… bağlantısından ya da yalnız t
// değerinden token'ı çıkarır.
func JoinToken(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	t := raw
	if strings.Contains(raw, ":") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "clickfix-support" {
			return "", false
		}
		where := u.Host
		if where == "" {
			where = strings.Trim(u.Opaque+u.Path, "/")
		}
		if where != "join" {
			return "", false
		}
		t = u.Query().Get("t")
	}
	if t == "" || len(t) > MaxJoinToken {
		return "", false
	}
	for _, r := range t {
		if !tokenRune(r) {
			return "", false
		}
	}
	return t, true
}

func (s *Service) handleResolve(ctx context.Context, raw json.RawMessage) (any, error) {
	var p resolveParams
	if e := decodeParams(raw, &p); e != nil {
		return nil, e
	}
	if (p.Code == nil) == (p.Link == nil) {
		return nil, invalid("Kod ya da bağlantıdan yalnız biri verilmeli.")
	}
	in := api.ResolveInput{ClientVersion: s.cfg.Version}
	if p.Code != nil {
		c, ok := NormalizeCode(*p.Code)
		if !ok {
			return nil, invalid("Kod 9 haneden oluşmalı.")
		}
		in.Code = c
	} else {
		t, ok := JoinToken(*p.Link)
		if !ok {
			return nil, invalid("Bağlantı geçersiz.")
		}
		in.Token = t
	}
	in.ClientOS = sysinfo.ShortOS()
	if p.ShareSystemInfo {
		in.ClientOS = sysinfo.ClientOS(s.sysInfo(), true)
	}

	s.mu.Lock()
	phase := s.phase
	s.mu.Unlock()
	if phase != PhaseIdle && phase != PhaseEnded {
		return nil, rpcError(CodeInvalidState, "Önceki oturum sürüyor.")
	}
	r, err := s.cfg.API.Resolve(ctx, in)
	if err != nil {
		s.log.Warn("oturum çözülemedi", "err", err)
		return nil, fromAPI(err)
	}
	// Çağrı sürerken başka bir resolve oturumu başlatmış olabilir.
	s.mu.Lock()
	phase = s.phase
	s.mu.Unlock()
	if phase != PhaseIdle && phase != PhaseEnded {
		return nil, rpcError(CodeInvalidState, "Önceki oturum sürüyor.")
	}
	s.begin(r)
	s.log.Info("oturum çözüldü", "session", r.SessionUUID, "mode", r.ModeRequested)
	return s.result(), nil
}

// --- session.consent ---

type consentParams struct {
	View      *bool `json:"view"`
	Control   *bool `json:"control"`
	Recording *bool `json:"recording"`
}

func (s *Service) handleConsent(ctx context.Context, raw json.RawMessage) (any, error) {
	var p consentParams
	if e := decodeParams(raw, &p); e != nil {
		return nil, e
	}
	if p.View == nil && p.Control == nil && p.Recording == nil {
		return nil, invalid("En az bir onay verilmeli.")
	}
	if p.View != nil && !*p.View {
		return nil, invalid("İzleme onayı geri alınamaz; oturumu durdurun.")
	}

	s.mu.Lock()
	phase, r, secret, viewed, gen := s.phase, s.resolved, s.secret, s.consentView, s.generation
	s.mu.Unlock()
	switch phase {
	case PhaseResolved, PhaseReady, PhaseStarting, PhaseActive, PhaseReconnecting:
	default:
		return nil, rpcError(CodeInvalidState, msgInvalidState)
	}
	if !viewed && p.View == nil {
		return nil, invalid("Önce izleme onayı verilmeli.")
	}
	if p.Control != nil && *p.Control && r.ModeRequested != "control" {
		return nil, invalid("Temsilci kontrol istemedi.")
	}
	if p.Recording != nil && !r.RecordingRequested {
		return nil, invalid("Bu oturumda kayıt istenmedi.")
	}

	// Geri alma her zaman önce yerelde geçerlidir.
	if p.Control != nil && !*p.Control {
		s.withdrawControl(gen)
	}

	v, err := s.cfg.API.Consent(ctx, r.SessionUUID, secret, api.ConsentInput{View: p.View, Control: p.Control, Recording: p.Recording})
	if err != nil {
		return nil, fromAPI(err)
	}
	s.change(ByCustomer, func() {
		if gen != s.generation {
			return
		}
		if p.View != nil {
			s.consentView = true
			if s.phase == PhaseResolved {
				s.phase = PhaseReady
			}
		}
		if p.Control != nil && *p.Control {
			s.consentControl, s.controlGranted = true, true
			s.controlRevoked, s.controlDeclined = false, false
		}
		if p.Recording != nil {
			rec := *p.Recording
			s.consentRecording = &rec
		}
		if v.EndsAt != nil {
			s.endsAt = *v.EndsAt
		}
	})
	return s.result(), nil
}

// withdrawControl kontrol onayını yerelde kaldırır (kapı hemen kapanır).
func (s *Service) withdrawControl(gen int) {
	s.change(ByCustomer, func() {
		if gen != s.generation {
			return
		}
		if s.consentControl {
			s.consentControl, s.controlRevoked = false, true
		} else if !s.controlRevoked {
			s.controlDeclined = true
		}
	})
}

// --- control.revoke ---

func (s *Service) handleRevoke(ctx context.Context, _ json.RawMessage) (any, error) {
	s.mu.Lock()
	on, r, secret, gen := s.consentControl, s.resolved, s.secret, s.generation
	s.mu.Unlock()
	if !on || r == nil {
		return s.result(), nil
	}
	s.withdrawControl(gen)
	no := false
	if _, err := s.cfg.API.Consent(ctx, r.SessionUUID, secret, api.ConsentInput{Control: &no}); err != nil {
		// Kapı kapalı kalır; sunucu temsilcinin veri iznini kaldıramadı.
		s.log.Warn("kontrol geri alma sunucuya iletilemedi", "err", err)
		s.emitError(fromAPI(err), false)
	}
	return s.result(), nil
}

// --- session.start ---

func (s *Service) handleStart(context.Context, json.RawMessage) (any, error) {
	p := s.cfg.Platform
	perms := s.refreshPermissions(BySystem)
	if p.StartCapture == nil || perms.ScreenRecording == capture.Unsupported {
		return nil, rpcError(CodeUnsupported, "Ekran paylaşımı bu işletim sisteminde henüz desteklenmiyor.")
	}
	ok := false
	var gen int
	var stateErr *ipc.Error
	s.change(BySystem, func() {
		switch {
		case s.phase != PhaseReady:
			stateErr = rpcError(CodeInvalidState, msgInvalidState)
		case perms.ScreenRecording != capture.Granted:
			stateErr = permissionError("screen_recording", "Ekran Kaydı izni gerekli.")
		default:
			ok, gen = true, s.generation
			s.phase, s.connection = PhaseStarting, remote.Connecting
		}
	})
	if !ok {
		return nil, stateErr
	}
	s.startRun(gen)
	return s.result(), nil
}

// --- session.stop ---

// Stop, oturumu müşteri isteğiyle bitirir.
func (s *Service) Stop(ctx context.Context) (StateResult, error) {
	s.mu.Lock()
	phase, gen := s.phase, s.generation
	s.mu.Unlock()
	switch phase {
	case PhaseIdle, PhaseEnded:
		return s.result(), nil
	}
	if !s.beginEnd(gen) {
		s.waitEnded(ctx)
		return s.result(), nil
	}
	s.teardown()
	s.finish(s.endOnServer(), nil)
	return s.result(), nil
}

func (s *Service) handleStop(ctx context.Context, _ json.RawMessage) (any, error) {
	return s.Stop(ctx)
}

// --- session.report ---

type reportParams struct {
	Reason string `json:"reason"`
}

// MaxReportReason, bildirim nedeninin en büyük uzunluğudur (karakter).
const MaxReportReason = 1000

func (s *Service) handleReport(ctx context.Context, raw json.RawMessage) (any, error) {
	var p reportParams
	if e := decodeParams(raw, &p); e != nil {
		return nil, e
	}
	p.Reason = strings.TrimSpace(p.Reason)
	if utf8.RuneCountInString(p.Reason) > MaxReportReason {
		return nil, invalid("Açıklama en çok 1000 karakter olabilir.")
	}
	s.mu.Lock()
	phase, r, secret, gen := s.phase, s.resolved, s.secret, s.generation
	s.mu.Unlock()
	if r == nil || phase == PhaseIdle {
		return nil, rpcError(CodeInvalidState, msgInvalidState)
	}

	running := phase != PhaseEnded
	if running && !s.beginEnd(gen) {
		s.waitEnded(ctx)
		running = false
	}
	if running {
		s.teardown()
	}
	v, err := s.cfg.API.Report(ctx, r.SessionUUID, secret, p.Reason)
	if err != nil {
		if running {
			s.finish(s.endOnServer(), nil)
		}
		return nil, fromAPI(err)
	}
	if running {
		s.mu.Lock()
		s.reported = true
		s.mu.Unlock()
		reason := v.EndedReason
		if reason == "" {
			reason = "reported"
		}
		s.finish(reason, nil)
		return s.result(), nil
	}
	s.change(ByCustomer, func() {
		if gen != s.generation {
			return
		}
		s.reported = true
		if s.summary != nil {
			s.summary.Reported = true
		}
	})
	return s.result(), nil
}

// --- izinler ---

func (s *Service) handlePermissionsStatus(context.Context, json.RawMessage) (any, error) {
	return s.refreshPermissions(BySystem), nil
}

type permissionParams struct {
	Kind string `json:"kind"`
}

func (s *Service) handlePermissionsRequest(_ context.Context, raw json.RawMessage) (any, error) {
	var p permissionParams
	if e := decodeParams(raw, &p); e != nil {
		return nil, e
	}
	pl := s.cfg.Platform
	switch p.Kind {
	case "screen_recording":
		if pl.RequestScreenPermission == nil {
			return s.refreshPermissions(BySystem), nil
		}
		if got := pl.RequestScreenPermission(); got == capture.Denied {
			// macOS, süreç açıkken verilen Ekran Kaydı iznini yeniden
			// açılana kadar göstermez.
			s.change(BySystem, func() { s.perms.RestartRequired = true })
		}
	case "accessibility":
		if pl.RequestAccessibility != nil {
			pl.RequestAccessibility()
		}
	default:
		return nil, invalid("kind screen_recording ya da accessibility olmalı.")
	}
	return s.refreshPermissions(BySystem), nil
}

// --- sysinfo.get ---

func (s *Service) sysInfo() sysinfo.Info {
	if s.cfg.Platform.SysInfo != nil {
		return s.cfg.Platform.SysInfo()
	}
	return sysinfo.Collect(s.cfg.Version)
}

func (s *Service) handleSysInfo(context.Context, json.RawMessage) (any, error) {
	return s.sysInfo(), nil
}

// tokenRune: URL'de kaçışsız kullanılabilen karakterler (RFC 3986 unreserved).
func tokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '-' || r == '_' || r == '.' || r == '~'
}

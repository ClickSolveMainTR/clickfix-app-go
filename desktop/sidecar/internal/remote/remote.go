// Package remote, müşterinin LiveKit oturumudur: odaya müşteri token'ıyla
// katılır, ekranı H.264 SCREEN_SHARE izi olarak yayınlar, kontrol konusundaki
// veri paketlerini çağırana iletir ve bağlantı koparsa yeni token'la yeniden
// bağlanır.
//
// LiveKit SDK'nın kendi sürdürme (resume) mekanizması kısa kopmaları
// odadan çıkmadan onarır (OnReconnecting/OnReconnected). Oda bağlantısı
// tamamen giderse Session yeni bir token alıp yeniden katılır; sunucu odayı
// sildiyse ya da katılımcıyı çıkardıysa oturum biter.
package remote

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
)

// Connection, IPC state.connection değeridir.
type Connection string

const (
	None         Connection = "none"
	Connecting   Connection = "connecting"
	Connected    Connection = "connected"
	Reconnecting Connection = "reconnecting"
)

// Status, bağlantı durumundaki her değişiklikte bildirilir.
type Status struct {
	Connection   Connection
	AgentPresent bool
}

// Exit, Run'ın neden döndüğüdür.
type Exit int

const (
	// ExitStopped: bağlam iptal edildi (müşteri durdurdu).
	ExitStopped Exit = iota
	// ExitClosed: sunucu odayı kapattı ya da katılımcıyı çıkardı (temsilci
	// bitirdi, süre doldu, platform kapattı...). Nedeni API söyler.
	ExitClosed
	// ExitLost: bağlantı yeniden kurulamadı.
	ExitLost
)

func (e Exit) String() string { return [...]string{"stopped", "closed", "lost"}[e] }

// disconnectKind, bir oda bağlantısının neden kapandığıdır.
type disconnectKind int

const (
	dropTransient disconnectKind = iota // ağ, sunucu yeniden başladı...
	dropClosed                          // oda silindi/kapandı, çıkarıldı, kimlik çakıştı
	dropLocal                           // biz kapattık
)

// link, bağlı tek bir oda bağlantısıdır.
type link interface {
	WriteFrame(f capture.Frame, d time.Duration) error
	Close()
}

// linkEvents, bir bağlantının olaylarını Session'a taşır.
type linkEvents struct {
	data         func(sender, topic string, payload []byte)
	participant  func(identity string, joined bool)
	reconnecting func()
	reconnected  func()
	disconnected func(disconnectKind)
	keyframe     func()
}

// dialFunc bir oda bağlantısı kurar ve odadaki uzak katılımcıların
// kimliklerini döner.
type dialFunc func(ctx context.Context, g api.Grant, cfg capture.Config, ev linkEvents) (link, []string, error)

// Config, oturum ayarıdır.
type Config struct {
	// Grant her bağlantı denemesinde yeni bir müşteri token'ı alır.
	Grant func(ctx context.Context) (api.Grant, error)
	// AgentIdentity, kontrol olaylarının kabul edildiği tek kimliktir
	// (rs:agent:<oturum>); yalnız varlık bildirimi için kullanılır, kapı
	// kimliği kendisi de denetler.
	AgentIdentity string
	Capture       capture.Config
	// OnData, kontrol konusundaki her veri paketi için çağrılır.
	OnData func(sender string, payload []byte)
	// OnStatus, bağlantı durumu değişince çağrılır.
	OnStatus func(Status)
	// OnKeyframe, alıcı anahtar kare isteyince (PLI/FIR) ve yeni bağlantıda.
	OnKeyframe func()
	Logger     *slog.Logger

	// ReconnectFor, kopmadan sonra yeniden bağlanmaya çalışılan en uzun süre.
	ReconnectFor time.Duration

	dial  dialFunc
	sleep func(context.Context, time.Duration) error
}

// DefaultReconnectFor, varsayılan yeniden bağlanma süresidir.
const DefaultReconnectFor = 60 * time.Second

// Session, müşterinin oda oturumudur.
type Session struct {
	cfg Config
	log *slog.Logger

	cur     atomic.Pointer[linkBox]
	frames  chan capture.Frame
	dropped atomic.Int64

	mu     sync.Mutex
	status Status
}

type linkBox struct{ l link }

// New bir oturum kurar; Run çağrılana kadar bağlanmaz.
func New(cfg Config) *Session {
	if cfg.dial == nil {
		cfg.dial = dialLiveKit
	}
	if cfg.sleep == nil {
		cfg.sleep = sleepCtx
	}
	if cfg.ReconnectFor <= 0 {
		cfg.ReconnectFor = DefaultReconnectFor
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Session{cfg: cfg, log: log, frames: make(chan capture.Frame, 30), status: Status{Connection: None}}
}

// PushFrame, yakalamadan gelen kareyi kuyruğa koyar; kuyruk doluysa ya da
// bağlantı yoksa kare atılır (yakalama hiçbir zaman beklemez).
func (s *Session) PushFrame(f capture.Frame) {
	select {
	case s.frames <- f:
	default:
		s.dropped.Add(1)
	}
}

// Dropped, atılan kare sayısıdır.
func (s *Session) Dropped() int64 { return s.dropped.Load() }

// Status, son durumdur.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Session) setStatus(f func(*Status)) {
	s.mu.Lock()
	prev := s.status
	f(&s.status)
	st := s.status
	s.mu.Unlock()
	if st != prev && s.cfg.OnStatus != nil {
		s.cfg.OnStatus(st)
	}
}

// Run bağlanır ve oturum bitene kadar bekler.
func (s *Session) Run(ctx context.Context) (Exit, error) {
	if s.cfg.Grant == nil {
		return ExitLost, ErrNoGrant
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.writer(ctx)
	defer s.setStatus(func(st *Status) { *st = Status{Connection: None} })

	var lostSince time.Time
	attempt := 0
	for {
		if ctx.Err() != nil {
			return ExitStopped, nil
		}
		if attempt == 0 {
			s.setStatus(func(st *Status) { st.Connection = Connecting })
		} else {
			s.setStatus(func(st *Status) { st.Connection, st.AgentPresent = Reconnecting, false })
		}
		kind, err := s.connectOnce(ctx)
		switch {
		case ctx.Err() != nil:
			return ExitStopped, nil
		case kind == dropClosed:
			return ExitClosed, err
		}
		// Geçici kopma ya da bağlanamama: süre bütçesi içinde yeniden dene.
		if lostSince.IsZero() || err == nil {
			lostSince = time.Now()
			attempt = 0
		}
		attempt++
		if time.Since(lostSince) >= s.cfg.ReconnectFor {
			return ExitLost, err
		}
		delay := backoff(attempt)
		s.log.Warn("livekit bağlantısı yok, yeniden denenecek", "attempt", attempt, "delay", delay, "err", err)
		if s.cfg.sleep(ctx, delay) != nil {
			return ExitStopped, nil
		}
	}
}

// backoff: 0,5 sn, 1, 2, 4, 8 sn (üst sınır).
func backoff(attempt int) time.Duration {
	d := 500 * time.Millisecond << min(attempt-1, 4)
	return min(d, 8*time.Second)
}

// connectOnce bir bağlantı kurar ve kopana kadar bekler. err, bağlanamadıysa
// nedenidir; bağlanıp sonra koptuysa nil'dir.
func (s *Session) connectOnce(ctx context.Context) (disconnectKind, error) {
	g, err := s.cfg.Grant(ctx)
	if err != nil {
		if fatalGrantError(err) {
			return dropClosed, err
		}
		return dropTransient, err
	}
	done := make(chan disconnectKind, 1)
	ev := linkEvents{
		data: func(sender, topic string, payload []byte) {
			if topic == control.Topic && s.cfg.OnData != nil {
				s.cfg.OnData(sender, payload)
			}
		},
		participant: func(identity string, joined bool) {
			if identity == s.cfg.AgentIdentity {
				s.setStatus(func(st *Status) { st.AgentPresent = joined })
			}
		},
		reconnecting: func() {
			s.setStatus(func(st *Status) { st.Connection = Reconnecting })
		},
		reconnected: func() {
			s.setStatus(func(st *Status) { st.Connection = Connected })
			s.keyframe()
		},
		disconnected: func(k disconnectKind) {
			select {
			case done <- k:
			default:
			}
		},
		keyframe: s.keyframe,
	}
	l, present, err := s.cfg.dial(ctx, g, s.cfg.Capture, ev)
	if err != nil {
		return dropTransient, err
	}
	s.cur.Store(&linkBox{l})
	agent := false
	for _, id := range present {
		agent = agent || id == s.cfg.AgentIdentity
	}
	s.setStatus(func(st *Status) { st.Connection, st.AgentPresent = Connected, agent })
	s.keyframe()
	s.log.Info("livekit odasına bağlanıldı", "room", g.Room, "agent_present", agent)

	var kind disconnectKind
	select {
	case <-ctx.Done():
		kind = dropLocal
	case kind = <-done:
	}
	s.cur.Store(nil)
	l.Close()
	if kind == dropClosed {
		s.log.Info("sunucu odayı kapattı")
	}
	return kind, nil
}

func (s *Session) keyframe() {
	if s.cfg.OnKeyframe != nil {
		s.cfg.OnKeyframe()
	}
}

// writer kuyruktaki kareleri geçerli bağlantıya yazar. Süre, ardışık
// karelerin gerçek zaman damgası farkından hesaplanır.
func (s *Session) writer(ctx context.Context) {
	fps := s.cfg.Capture.FPS
	if fps <= 0 {
		fps = capture.DefaultFPS
	}
	def := time.Second / time.Duration(fps)
	var last time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-s.frames:
			d := def
			if last != 0 && f.PTS > last && f.PTS-last < time.Second {
				d = f.PTS - last
			}
			last = f.PTS
			box := s.cur.Load()
			if box == nil {
				s.dropped.Add(1)
				continue
			}
			_ = box.l.WriteFrame(f, d)
		}
	}
}

// fatalGrantError, token alınamamasının oturumun bittiği anlamına gelip
// gelmediğidir (410 bitti, 401 sır geçersiz, 404 oturum yok, 403 kapatıldı).
func fatalGrantError(err error) bool {
	e, ok := api.AsError(err)
	if !ok || e.Network {
		return false
	}
	switch e.Status {
	case 401, 403, 404, 410:
		return true
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ErrNoGrant, Config.Grant verilmediğinde döner.
var ErrNoGrant = errors.New("remote: token kaynağı yok")

package remote

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/control"
)

const agentID = "rs:agent:6f1c"

type fakeLink struct {
	mu     sync.Mutex
	frames int
	closed bool
}

func (f *fakeLink) WriteFrame(capture.Frame, time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames++
	return nil
}
func (f *fakeLink) Close() { f.mu.Lock(); f.closed = true; f.mu.Unlock() }

type harness struct {
	mu       sync.Mutex
	dials    int
	grants   int
	ev       linkEvents
	links    []*fakeLink
	dialErr  []error
	grantErr error
	statuses []Status
	data     []string
	keys     int
	dialed   chan struct{}
}

func (h *harness) session(t *testing.T) *Session {
	h.dialed = make(chan struct{}, 10)
	return New(Config{
		AgentIdentity: agentID,
		Grant: func(context.Context) (api.Grant, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.grants++
			if h.grantErr != nil {
				return api.Grant{}, h.grantErr
			}
			return api.Grant{ServerURL: "wss://lk", Token: "tok", Room: "rs_6f1c", Identity: "rs:customer:6f1c"}, nil
		},
		OnData: func(sender string, p []byte) {
			h.mu.Lock()
			h.data = append(h.data, sender+" "+string(p))
			h.mu.Unlock()
		},
		OnStatus:     func(s Status) { h.mu.Lock(); h.statuses = append(h.statuses, s); h.mu.Unlock() },
		OnKeyframe:   func() { h.mu.Lock(); h.keys++; h.mu.Unlock() },
		ReconnectFor: time.Hour,
		sleep:        func(ctx context.Context, d time.Duration) error { return ctx.Err() },
		dial: func(ctx context.Context, g api.Grant, cfg capture.Config, ev linkEvents) (link, []string, error) {
			h.mu.Lock()
			h.dials++
			if len(h.dialErr) > 0 {
				err := h.dialErr[0]
				h.dialErr = h.dialErr[1:]
				h.mu.Unlock()
				return nil, nil, err
			}
			l := &fakeLink{}
			h.links = append(h.links, l)
			h.ev = ev
			h.mu.Unlock()
			h.dialed <- struct{}{}
			return l, []string{agentID}, nil
		},
	})
}

func (h *harness) wait(t *testing.T) {
	t.Helper()
	select {
	case <-h.dialed:
	case <-time.After(2 * time.Second):
		t.Fatal("bağlantı kurulmadı")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("koşul sağlanmadı")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestReconnectAfterTransientDrop(t *testing.T) {
	h := &harness{}
	s := h.session(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exit := make(chan Exit, 1)
	go func() { e, _ := s.Run(ctx); exit <- e }()

	h.wait(t)
	if st := s.Status(); st.Connection != Connected || !st.AgentPresent {
		t.Fatalf("bağlı değil: %+v", st)
	}
	// Veri: yalnız kontrol konusu iletilir.
	h.ev.data(agentID, control.Topic, []byte("a"))
	h.ev.data(agentID, "chat", []byte("b"))
	// Kare yazılır.
	s.PushFrame(capture.Frame{Data: []byte{1}, PTS: time.Millisecond})
	waitFor(t, func() bool { h.links[0].mu.Lock(); defer h.links[0].mu.Unlock(); return h.links[0].frames == 1 })

	// Geçici kopma: yeni token, yeni bağlantı, anahtar kare.
	h.ev.disconnected(dropTransient)
	h.wait(t)
	waitFor(t, func() bool { return s.Status().Connection == Connected })
	h.mu.Lock()
	if h.grants != 2 || h.dials != 2 || !h.links[0].closed || h.keys < 2 {
		t.Errorf("yeniden bağlanma: grants=%d dials=%d closed=%v keys=%d", h.grants, h.dials, h.links[0].closed, h.keys)
	}
	if len(h.data) != 1 || h.data[0] != agentID+" a" {
		t.Errorf("veri = %v", h.data)
	}
	sawReconnecting := false
	for _, st := range h.statuses {
		sawReconnecting = sawReconnecting || st.Connection == Reconnecting
	}
	h.mu.Unlock()
	if !sawReconnecting {
		t.Error("reconnecting durumu bildirilmedi")
	}

	// Temsilci ayrılınca varlık düşer.
	h.ev.participant(agentID, false)
	if s.Status().AgentPresent {
		t.Error("temsilci ayrıldı ama varlık sürüyor")
	}
	h.ev.participant("rs:customer:x", true)
	if s.Status().AgentPresent {
		t.Error("başka katılımcı temsilci sayıldı")
	}

	cancel()
	if e := <-exit; e != ExitStopped {
		t.Errorf("çıkış = %v", e)
	}
	if s.Status().Connection != None {
		t.Errorf("son durum %+v", s.Status())
	}
}

func TestRoomClosedEndsSession(t *testing.T) {
	h := &harness{}
	s := h.session(t)
	exit := make(chan Exit, 1)
	go func() { e, _ := s.Run(context.Background()); exit <- e }()
	h.wait(t)
	h.ev.disconnected(dropClosed)
	select {
	case e := <-exit:
		if e != ExitClosed {
			t.Errorf("çıkış = %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oda kapanınca bitmedi")
	}
	if h.dials != 1 {
		t.Errorf("kapanan odaya yeniden bağlanıldı: %d", h.dials)
	}
}

func TestEndedGrantEndsSession(t *testing.T) {
	h := &harness{grantErr: &api.Error{Status: 410, Code: "REMOTE_SUPPORT_ENDED"}}
	s := h.session(t)
	e, err := s.Run(context.Background())
	if e != ExitClosed || err == nil {
		t.Errorf("410: %v %v", e, err)
	}
}

func TestGiveUpAfterBudget(t *testing.T) {
	h := &harness{dialErr: []error{errors.New("a"), errors.New("b"), errors.New("c")}, grantErr: nil}
	s := h.session(t)
	s.cfg.ReconnectFor = time.Nanosecond
	e, err := s.Run(context.Background())
	if e != ExitLost || err == nil {
		t.Errorf("bütçe: %v %v", e, err)
	}
	// Ağ hatasıyla token alınamaması geçicidir.
	h2 := &harness{grantErr: &api.Error{Network: true}}
	s2 := h2.session(t)
	s2.cfg.ReconnectFor = time.Nanosecond
	if e, _ := s2.Run(context.Background()); e != ExitLost {
		t.Errorf("ağ hatası: %v", e)
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for i, w := range want {
		if got := backoff(i + 1); got != w {
			t.Errorf("backoff(%d) = %v", i+1, got)
		}
	}
}

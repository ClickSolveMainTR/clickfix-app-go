package mail

import (
	"context"
	"sync"
	"time"
)

// Transport states reported by Monitor.
const (
	StateOK            = "ok"
	StateDown          = "down"
	StateNotConfigured = "not_configured"
	StateUnknown       = "unknown"
)

// Status is the last known reachability of the SMTP transport.
type Status struct {
	Transport
	State     string     `json:"status"`
	Cause     string     `json:"cause,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// Monitor probes the SMTP server at most once per TTL and caches the result,
// so health pages can show whether mail egress works without sending mail
// and without hammering a blocked port.
type Monitor struct {
	sender  *SMTPSender
	ttl     time.Duration
	timeout time.Duration
	now     func() time.Time
	probe   func(context.Context) error

	mu      sync.Mutex
	last    Status
	running bool
}

// NewMonitor builds a monitor; ttl <= 0 means five minutes.
func NewMonitor(sender *SMTPSender, ttl time.Duration) *Monitor {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	m := &Monitor{sender: sender, ttl: ttl, timeout: 5 * time.Second, now: time.Now}
	if sender != nil {
		m.probe = sender.Probe
	}
	m.last = Status{Transport: sender.Transport(), State: StateUnknown}
	if !sender.Configured() {
		m.last.State, m.last.Cause = StateNotConfigured, CauseNotConfigured
	}
	return m
}

// Cached returns the last result without probing. A stale result starts a
// background refresh.
func (m *Monitor) Cached() Status {
	if m == nil {
		return Status{State: StateUnknown}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stale() && !m.running && m.probe != nil {
		m.running = true
		go m.refresh(context.Background())
	}
	return m.last
}

// Status returns a fresh result, probing when the cached one is stale.
func (m *Monitor) Status(ctx context.Context) Status {
	if m == nil {
		return Status{State: StateUnknown}
	}
	m.mu.Lock()
	if !m.stale() || m.running || m.probe == nil {
		s := m.last
		m.mu.Unlock()
		return s
	}
	m.running = true
	m.mu.Unlock()
	return m.refresh(ctx)
}

func (m *Monitor) stale() bool {
	if m.last.State == StateNotConfigured {
		return false
	}
	return m.last.CheckedAt == nil || m.now().Sub(*m.last.CheckedAt) >= m.ttl
}

func (m *Monitor) refresh(ctx context.Context) Status {
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
	err := m.probe(pctx)
	cancel()
	at := m.now().UTC()
	s := Status{Transport: m.sender.Transport(), State: StateOK, CheckedAt: &at}
	if err != nil {
		s.State, s.Cause = StateDown, Cause(err)
		if s.Cause == CauseNotConfigured {
			s.State = StateNotConfigured
		}
	}
	m.mu.Lock()
	m.last, m.running = s, false
	m.mu.Unlock()
	return s
}

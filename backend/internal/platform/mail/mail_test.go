package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
)

func TestStripHeaderBreaks(t *testing.T) {
	got := stripHeaderBreaks("Hi Ali\r\nBcc: attacker@example.com\nX")
	if got != "Hi Ali Bcc: attacker@example.com X" {
		t.Fatalf("got %q", got)
	}
}

// fakeSMTP is a minimal plain SMTP server that accepts one message per
// connection and records the DATA payloads.
type fakeSMTP struct {
	ln      net.Listener
	mu      sync.Mutex
	bodies  []string
	silence bool // accept connections but never greet
}

func newFakeSMTP(t *testing.T, silence bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, silence: silence}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if f.silence {
		_, _ = bufio.NewReader(conn).ReadString('\n')
		return
	}
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.Fields(line + " x")[0])
		switch cmd {
		case "EHLO", "HELO":
			_ = tp.PrintfLine("250 fake")
		case "MAIL", "RCPT":
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go ahead")
			body, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.bodies = append(f.bodies, string(body))
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestSMTPSenderDeliversAndProbes(t *testing.T) {
	srv := newFakeSMTP(t, false)
	s := NewSMTPSender(config.SMTPConfig{Host: "127.0.0.1", Port: srv.port(), From: "noreply@example.com", FromName: "ClickFix"})
	if err := s.Send(context.Background(), Message{To: []string{"a@example.com"}, Subject: "Hi\r\nBcc: x", Body: "hello"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	srv.mu.Lock()
	got := strings.Join(srv.bodies, "")
	srv.mu.Unlock()
	if !strings.Contains(got, "hello") || strings.Contains(got, "\r\nBcc:") {
		t.Fatalf("unexpected payload %q", got)
	}
	if err := s.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}
}

func TestSMTPSenderNotConfiguredIsPermanent(t *testing.T) {
	s := NewSMTPSender(config.SMTPConfig{Host: " ", Port: 587})
	err := s.Send(context.Background(), Message{To: []string{"a@example.com"}})
	if !errors.Is(err, ErrNotConfigured) || IsTransient(err) {
		t.Fatalf("want permanent ErrNotConfigured, got %v", err)
	}
	if err := s.Send(context.Background(), Message{}); !errors.Is(err, ErrInvalidMessage) || IsTransient(err) {
		t.Fatalf("want permanent invalid message, got %v", err)
	}
}

func TestSMTPSenderTimesOutSilentServer(t *testing.T) {
	srv := newFakeSMTP(t, true)
	s := NewSMTPSender(config.SMTPConfig{Host: "127.0.0.1", Port: srv.port(), SendTimeout: 200 * time.Millisecond})
	start := time.Now()
	err := s.Send(context.Background(), Message{To: []string{"a@example.com"}})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("want quick timeout, got %v after %s", err, time.Since(start))
	}
	if Cause(err) != CauseTimeout || !IsTransient(err) {
		t.Fatalf("want transient timeout, got cause %q (%v)", Cause(err), err)
	}
}

func TestSMTPSenderContextCancelAborts(t *testing.T) {
	srv := newFakeSMTP(t, true)
	s := NewSMTPSender(config.SMTPConfig{Host: "127.0.0.1", Port: srv.port(), SendTimeout: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Send(ctx, Message{To: []string{"a@example.com"}}); err == nil {
		t.Fatal("want error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("send ignored context: %s", time.Since(start))
	}
}

func TestSMTPSenderFallsBackOnDialFailure(t *testing.T) {
	primary, fallback := closedPort(t), closedPort(t)
	s := NewSMTPSender(config.SMTPConfig{Host: "127.0.0.1", Port: primary, FallbackPort: fallback, DialTimeout: time.Second})
	err := s.Send(context.Background(), Message{To: []string{"a@example.com"}})
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprintf(":%d", primary)) || !strings.Contains(msg, fmt.Sprintf(":%d", fallback)) {
		t.Fatalf("fallback not attempted: %v", err)
	}
	if Cause(err) != CauseRefused || !IsTransient(err) {
		t.Fatalf("want transient refused, got %q", Cause(err))
	}
}

func TestCauseClassifiesSMTPReplies(t *testing.T) {
	cases := map[string]struct {
		err       error
		cause     string
		transient bool
	}{
		"4xx":  {&SendError{Stage: "rcpt", Err: &textproto.Error{Code: 451, Msg: "try later"}}, CauseTempRejected, true},
		"5xx":  {&SendError{Stage: "rcpt", Err: &textproto.Error{Code: 550, Msg: "no such user"}}, CauseRejected, false},
		"auth": {&SendError{Stage: "auth", Err: &textproto.Error{Code: 535, Msg: "bad creds"}}, CauseAuth, false},
		"dial": {&SendError{Stage: "dial", Err: &net.OpError{Op: "dial", Err: context.DeadlineExceeded}}, CauseTimeout, true},
	}
	for name, c := range cases {
		if got := Cause(c.err); got != c.cause {
			t.Errorf("%s: cause %q, want %q", name, got, c.cause)
		}
		if got := IsTransient(c.err); got != c.transient {
			t.Errorf("%s: transient %v, want %v", name, got, c.transient)
		}
	}
}

func TestMonitorCachesProbe(t *testing.T) {
	s := NewSMTPSender(config.SMTPConfig{Host: "127.0.0.1", Port: 1})
	m := NewMonitor(s, time.Hour)
	calls := 0
	m.probe = func(context.Context) error {
		calls++
		return &SendError{Stage: "dial", Err: &net.OpError{Op: "dial", Err: context.DeadlineExceeded}}
	}
	first := m.Status(context.Background())
	second := m.Status(context.Background())
	if calls != 1 || first.State != StateDown || second.Cause != CauseTimeout {
		t.Fatalf("calls=%d first=%+v second=%+v", calls, first, second)
	}
	unset := NewMonitor(NewSMTPSender(config.SMTPConfig{}), time.Hour)
	if st := unset.Status(context.Background()); st.State != StateNotConfigured {
		t.Fatalf("want not_configured, got %+v", st)
	}
}

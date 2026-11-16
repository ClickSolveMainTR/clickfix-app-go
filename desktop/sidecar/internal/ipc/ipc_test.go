package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type line struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
}

// harness, sunucuyu bir boru çiftine bağlar.
type harness struct {
	t     *testing.T
	in    *io.PipeWriter
	out   *bufio.Scanner
	done  chan error
	srv   *Server
	close sync.Once
}

func newHarness(t *testing.T, setup func(*Server)) *harness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer(outW)
	RegisterBuiltins(srv, NewInfo("0.1.0-test", "support"))
	if setup != nil {
		setup(srv)
	}
	h := &harness{t: t, in: inW, out: bufio.NewScanner(outR), done: make(chan error, 1), srv: srv}
	go func() {
		h.done <- srv.Serve(context.Background(), inR)
		_ = outW.Close()
	}()
	t.Cleanup(func() { _ = h.shutdown() })
	return h
}

func (h *harness) send(s string) {
	h.t.Helper()
	if _, err := io.WriteString(h.in, s+"\n"); err != nil {
		h.t.Fatalf("yazılamadı: %v", err)
	}
}

func (h *harness) read() line {
	h.t.Helper()
	got := make(chan line, 1)
	go func() {
		var l line
		if h.out.Scan() {
			_ = json.Unmarshal(h.out.Bytes(), &l)
		}
		got <- l
	}()
	select {
	case l := <-got:
		return l
	case <-time.After(5 * time.Second):
		h.t.Fatal("yanıt gelmedi")
		return line{}
	}
}

func (h *harness) shutdown() error {
	var err error
	h.close.Do(func() {
		_ = h.in.Close()
		// Kalan çıktıyı boşalt ki yazıcılar takılmasın.
		go func() {
			for h.out.Scan() {
			}
		}()
		err = <-h.done
	})
	return err
}

func TestPingAndVersion(t *testing.T) {
	h := newHarness(t, nil)

	h.send(`{"id":1,"method":"ping"}`)
	l := h.read()
	if string(l.ID) != "1" || l.Error != nil || string(l.Result) != `{"pong":true}` {
		t.Fatalf("ping yanıtı beklenmedik: %+v %s", l, l.Result)
	}

	h.send(`{"id":"v","method":"version","params":{}}`)
	l = h.read()
	var info Info
	if err := json.Unmarshal(l.Result, &info); err != nil {
		t.Fatalf("version çözülemedi: %v", err)
	}
	if string(l.ID) != `"v"` || info.Version != "0.1.0-test" || info.Mode != "support" || info.Go == "" {
		t.Fatalf("version yanıtı beklenmedik: %+v", info)
	}
}

func TestMalformedLines(t *testing.T) {
	h := newHarness(t, nil)
	cases := []struct {
		in   string
		id   string
		code int
	}{
		{`{bozuk`, "null", CodeParseError},
		{`[1,2]`, "null", CodeParseError},
		{`{"method":"ping"}`, "null", CodeInvalidRequest},
		{`{"id":null,"method":"ping"}`, "null", CodeInvalidRequest},
		{`{"id":{"a":1},"method":"ping"}`, "null", CodeInvalidRequest},
		{`{"id":7}`, "7", CodeInvalidRequest},
	}
	for _, c := range cases {
		h.send(c.in)
		l := h.read()
		if l.Error == nil || l.Error.Code != c.code || string(l.ID) != c.id {
			t.Fatalf("%q: beklenen kod %d id %s, gelen %+v id %s", c.in, c.code, c.id, l.Error, l.ID)
		}
	}
	// Boş satırlar yok sayılır, sunucu çalışmaya devam eder.
	h.send("   ")
	h.send(`{"id":2,"method":"ping"}`)
	if l := h.read(); string(l.ID) != "2" || l.Error != nil {
		t.Fatalf("bozuk satırlardan sonra ping çalışmadı: %+v", l)
	}
}

func TestUnknownMethod(t *testing.T) {
	h := newHarness(t, nil)
	h.send(`{"id":3,"method":"yok"}`)
	l := h.read()
	if l.Error == nil || l.Error.Code != CodeMethodNotFound || string(l.ID) != "3" {
		t.Fatalf("bilinmeyen yöntem hatası beklenirdi: %+v", l)
	}
}

func TestHandlerErrorsAndPanics(t *testing.T) {
	h := newHarness(t, func(s *Server) {
		s.Handle("typed", func(context.Context, json.RawMessage) (any, error) {
			return nil, Errorf(CodeInvalidParams, "eksik alan")
		})
		s.Handle("plain", func(context.Context, json.RawMessage) (any, error) {
			return nil, errors.New("patladı")
		})
		s.Handle("panic", func(context.Context, json.RawMessage) (any, error) {
			panic("beklenmedik")
		})
		s.Handle("empty", func(context.Context, json.RawMessage) (any, error) {
			return nil, nil
		})
	})
	want := map[string]int{"typed": CodeInvalidParams, "plain": CodeInternal, "panic": CodeInternal}
	for m, code := range want {
		h.send(fmt.Sprintf(`{"id":%q,"method":%q}`, m, m))
		l := h.read()
		if l.Error == nil || l.Error.Code != code {
			t.Fatalf("%s: kod %d beklenirdi, gelen %+v", m, code, l.Error)
		}
	}
	h.send(`{"id":9,"method":"empty"}`)
	if l := h.read(); l.Error != nil || string(l.Result) != "{}" {
		t.Fatalf("boş sonuç {} olmalı: %+v %s", l, l.Result)
	}
}

func TestConcurrentRequests(t *testing.T) {
	const n = 200
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(n)
	h := newHarness(t, func(s *Server) {
		s.Handle("echo", func(_ context.Context, p json.RawMessage) (any, error) {
			started.Done()
			<-release // hepsi aynı anda içeride olsun
			var v struct {
				N int `json:"n"`
			}
			if err := json.Unmarshal(p, &v); err != nil {
				return nil, err
			}
			return v.N * 2, nil
		})
	})

	go func() {
		for i := range n {
			h.send(fmt.Sprintf(`{"id":%d,"method":"echo","params":{"n":%d}}`, i, i))
		}
	}()
	waitCh := make(chan struct{})
	go func() { started.Wait(); close(waitCh) }()
	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("istekler eşzamanlı işlenmiyor")
	}
	close(release)

	seen := map[string]bool{}
	for range n {
		l := h.read()
		if l.Error != nil {
			t.Fatalf("hata: %+v", l.Error)
		}
		var id, res int
		_ = json.Unmarshal(l.ID, &id)
		_ = json.Unmarshal(l.Result, &res)
		if res != id*2 {
			t.Fatalf("id %d için sonuç %d, beklenen %d", id, res, id*2)
		}
		if seen[string(l.ID)] {
			t.Fatalf("yinelenen yanıt: %s", l.ID)
		}
		seen[string(l.ID)] = true
	}
}

func TestNotify(t *testing.T) {
	h := newHarness(t, nil)
	go func() { _ = h.srv.Notify("ready", map[string]string{"mode": "agent"}) }()
	l := h.read()
	if l.Event != "ready" || string(l.Params) != `{"mode":"agent"}` || len(l.ID) != 0 {
		t.Fatalf("olay satırı beklenmedik: %+v", l)
	}
}

func TestEOFWaitsForInflight(t *testing.T) {
	finished := make(chan struct{})
	h := newHarness(t, func(s *Server) {
		s.Handle("slow", func(context.Context, json.RawMessage) (any, error) {
			time.Sleep(50 * time.Millisecond)
			close(finished)
			return "tamam", nil
		})
	})
	h.send(`{"id":1,"method":"slow"}`)
	_ = h.in.Close()
	if l := h.read(); string(l.Result) != `"tamam"` {
		t.Fatalf("süren istek yanıtlanmadan kapandı: %+v", l)
	}
	if err := h.shutdown(); err != nil {
		t.Fatalf("EOF nil dönmeli: %v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("işleyici bitmeden Serve döndü")
	}
}

func TestLineTooLong(t *testing.T) {
	h := newHarness(t, nil)
	go func() {
		_, _ = io.WriteString(h.in, `{"id":1,"method":"ping","params":"`+strings.Repeat("a", MaxLineBytes)+`"}`+"\n")
	}()
	l := h.read()
	if l.Error == nil || l.Error.Code != CodeParseError {
		t.Fatalf("uzun satır için ayrıştırma hatası beklenirdi: %+v", l)
	}
	if err := h.shutdown(); !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("ErrTooLong beklenirdi: %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	srv := NewServer(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, inR) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("context.Canceled beklenirdi: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("iptal sonrası Serve dönmedi")
	}
}

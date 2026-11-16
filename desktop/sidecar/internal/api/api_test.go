package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func noSleep(context.Context, time.Duration) error { return nil }

func newTest(t *testing.T, h http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, UserAgent("1.2.3", "macos"), append([]Option{WithSleep(noSleep)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ok(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data, "meta": map[string]any{"request_id": "r"}})
}

func fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"code": code, "message": "m"}, "meta": map[string]any{}})
}

func TestResolveRequest(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/public/remote-support/resolve" {
			t.Errorf("istek %s %s", r.Method, r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != "ClickFixSupport/1.2.3 (macos)" {
			t.Errorf("kullanıcı ajanı %q", ua)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["code"] != "123456789" || in["client_os"] != "macos" || in["token"] != nil {
			t.Errorf("gövde %v", in)
		}
		ok(w, map[string]any{
			"session_uuid": "6f1c", "client_secret": "s", "mode_requested": "control",
			"organization":        map[string]any{"uuid": "o", "name": "Atlas", "logo_url": nil, "verified": true},
			"agent":               map[string]any{"uuid": "a", "name": "Elif", "avatar_url": nil},
			"recording_requested": true, "max_duration_minutes": 60,
			"consent_deadline": "2026-11-16T14:30:00Z", "warning": "w", "locale": "tr",
		})
	})
	res, err := c.Resolve(context.Background(), ResolveInput{Code: "123456789", ClientOS: "macos"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionUUID != "6f1c" || res.Agent.Name != "Elif" || !res.Organization.Verified || res.MaxDurationMinutes != 60 {
		t.Errorf("çözüm %+v", res)
	}
}

func TestConsentBody(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/public/remote-support/6f1c/consent" {
			t.Errorf("yol %s", r.URL.Path)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["client_secret"] != "sec" || in["control"] != false || in["view"] != nil || len(in) != 2 {
			t.Errorf("gövde %v", in)
		}
		ok(w, map[string]any{"session_uuid": "6f1c", "status": "active", "mode_requested": "control", "control_active": false, "ended_reason": "", "reported": false})
	})
	f := false
	v, err := c.Consent(context.Background(), "6f1c", "sec", ConsentInput{Control: &f})
	if err != nil || v.Status != "active" {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestErrorsAndRetries(t *testing.T) {
	var n atomic.Int32
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1, 2:
			fail(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		default:
			ok(w, map[string]any{"server_url": "wss://lk", "token": "tok", "identity": "rs:customer:6f1c", "room": "rs_6f1c", "can_publish_data": false, "expires_at": "2026-11-16T15:00:00Z"})
		}
	})
	g, err := c.Token(context.Background(), "6f1c", "sec")
	if err != nil || g.Identity != "rs:customer:6f1c" || n.Load() != 3 {
		t.Fatalf("yeniden deneme: %+v %v (%d istek)", g, err, n.Load())
	}

	n.Store(0)
	c = newTest(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		fail(w, http.StatusNotFound, "REMOTE_SUPPORT_CODE_INVALID")
	})
	_, err = c.Resolve(context.Background(), ResolveInput{Code: "1"})
	e, isAPI := AsError(err)
	if !isAPI || e.Status != 404 || e.Code != "REMOTE_SUPPORT_CODE_INVALID" || n.Load() != 1 {
		t.Fatalf("404 yeniden denenmemeli: %v (%d istek)", err, n.Load())
	}

	n.Store(0)
	c = newTest(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		fail(w, http.StatusTooManyRequests, "RATE_LIMITED")
	})
	_, err = c.End(context.Background(), "6f1c", "sec")
	if e, _ := AsError(err); e == nil || e.Status != 429 || n.Load() != 1 {
		t.Fatalf("429 yeniden denenmemeli: %v (%d istek)", err, n.Load())
	}
}

func TestTimeoutIsNetworkError(t *testing.T) {
	var n atomic.Int32
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		time.Sleep(200 * time.Millisecond)
	}, WithTimeouts(50*time.Millisecond, 2*time.Second))
	_, err := c.End(context.Background(), "6f1c", "sec")
	if e, _ := AsError(err); e == nil || !e.Network {
		t.Fatalf("ağ hatası bekleniyordu: %v", err)
	}
	if n.Load() != DefaultRetries+1 {
		t.Errorf("end idempotent, %d deneme beklendi, %d oldu", DefaultRetries+1, n.Load())
	}
	// Çözme tek kullanımlık: gönderildikten sonraki zaman aşımı yeniden denenmez.
	n.Store(0)
	_, err = c.Resolve(context.Background(), ResolveInput{Code: "1"})
	if e, _ := AsError(err); e == nil || !e.Network || n.Load() != 1 {
		t.Fatalf("resolve: %v (%d istek)", err, n.Load())
	}
}

func TestParseAppURL(t *testing.T) {
	good := map[string]string{
		"https://getclickfix.com":       "https://getclickfix.com",
		"https://getclickfix.com/x?y=1": "https://getclickfix.com",
		"http://localhost:3000":         "http://localhost:3000",
		"http://127.0.0.1:3000":         "http://127.0.0.1:3000",
	}
	for in, want := range good {
		u, err := ParseAppURL(in)
		if err != nil || u.String() != want {
			t.Errorf("ParseAppURL(%q) = %v, %v", in, u, err)
		}
	}
	for _, in := range []string{"http://getclickfix.com", "ftp://x", "getclickfix.com", "", "https://u:p@getclickfix.com"} {
		if _, err := ParseAppURL(in); err == nil {
			t.Errorf("ParseAppURL(%q) kabul edildi", in)
		}
	}
}

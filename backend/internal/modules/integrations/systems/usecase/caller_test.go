package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

func TestBuildURLEscapesValuesAndKeepsHost(t *testing.T) {
	got, err := BuildURL("https://api.example.com/v1/", "/invoices/{external_id}?company={company_external_id}",
		map[string]string{"external_id": "../admin/x?y=1", "company_external_id": "a&b=c"})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://api.example.com/v1/invoices/..%2Fadmin%2Fx%3Fy=1?company=a%26b%3Dc"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	for _, tmpl := range []string{"//evil.com/x", "@evil.com", "/a/../b", "invoices", "/x/{nope", "/x\\y"} {
		if _, err := BuildURL("https://api.example.com", tmpl, nil); err == nil {
			t.Fatalf("template %q accepted", tmpl)
		}
	}
	if _, err := BuildURL("https://api.example.com", "/x/{external_id}", map[string]string{}); CodeOf(err) != CodeIdentityMissing {
		t.Fatalf("missing value: %v", err)
	}
	if _, err := BuildURL("https://u:p@api.example.com", "/x", nil); err == nil {
		t.Fatal("user info in base accepted")
	}
}

func TestMaskURLHidesSecrets(t *testing.T) {
	got := MaskURL("https://api.example.com/x/supersecret-token-1?api_key=abc123&musteri=42&token=zzz", "supersecret-token-1")
	for _, leak := range []string{"supersecret-token-1", "abc123", "zzz"} {
		if strings.Contains(got, leak) {
			t.Fatalf("leaked %q in %s", leak, got)
		}
	}
	if !strings.Contains(got, "musteri=42") {
		t.Fatalf("plain query dropped: %s", got)
	}
}

// localCaller reaches httptest servers: tests only.
func localCaller(srv *httptest.Server) *Caller {
	c := NewCaller(nil)
	c.Validate = func(string) error { return nil }
	c.HTTP = func(timeout time.Duration) *http.Client {
		cl := srv.Client()
		cl.Timeout = timeout
		return cl
	}
	return c
}

func TestCallerBlocksPrivateTargets(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	// Default rules: a loopback literal is refused before any request.
	c := NewCaller(nil)
	_, err := c.Do(context.Background(), Request{Org: 1, BaseURL: srv.URL, Path: "/x", Method: http.MethodGet})
	if CodeOf(err) != CodeSSRF {
		t.Fatalf("loopback literal: %v", err)
	}
	// When the URL check passes (e.g. a public name that resolves to a
	// private address: DNS rebinding), the netguard dialer still refuses
	// the connection.
	c.Validate = func(string) error { return nil }
	_, err = c.Do(context.Background(), Request{Org: 1, BaseURL: srv.URL, Path: "/x", Method: http.MethodGet, Timeout: 2 * time.Second})
	if CodeOf(err) != CodeSSRF {
		t.Fatalf("private address reached: %v", err)
	}
	for _, raw := range []string{"http://api.example.com", "https://10.0.0.1", "https://metadata.internal", "https://169.254.169.254"} {
		if netguard.ValidateURL(raw, false) == nil {
			t.Fatalf("%s accepted", raw)
		}
	}
}

func TestCallerTimeoutLimitAndAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/slow":
			time.Sleep(1500 * time.Millisecond)
		case "/big":
			_, _ = w.Write([]byte(`"` + strings.Repeat("a", MaxResponseBytes+10) + `"`))
			return
		case "/fail":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := localCaller(srv)
	auth := Credentials{Type: model.AuthBearer, Secret: "tok-very-secret"}
	if _, err := c.Do(context.Background(), Request{Org: 2, BaseURL: srv.URL, Path: "/slow", Method: http.MethodGet, Timeout: time.Second, Auth: auth}); CodeOf(err) != CodeTimeout {
		t.Fatalf("timeout: %v", err)
	}
	if _, err := c.Do(context.Background(), Request{Org: 2, BaseURL: srv.URL, Path: "/big", Method: http.MethodGet}); CodeOf(err) != CodeTooLarge {
		t.Fatalf("too large: %v", err)
	}
	resp, err := c.Do(context.Background(), Request{Org: 2, BaseURL: srv.URL, Path: "/fail?token={external_id}", Vars: map[string]string{"external_id": "tok-very-secret"}, Method: http.MethodGet, Auth: auth})
	if CodeOf(err) != CodeHTTP || resp.Status != http.StatusBadGateway {
		t.Fatalf("http error: %v %d", err, resp.Status)
	}
	if strings.Contains(resp.URL, "tok-very-secret") || strings.Contains(err.Error(), "tok-very-secret") {
		t.Fatalf("secret leaked: %s / %v", resp.URL, err)
	}
	if gotAuth != "Bearer tok-very-secret" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	// Rate limit: the per-organization budget is enforced before calling.
	limited := localCaller(srv)
	for i := 0; i < CallsPerMinute; i++ {
		if _, err := limited.Do(context.Background(), Request{Org: 3, BaseURL: srv.URL, Path: "/ok", Method: http.MethodGet}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := limited.Do(context.Background(), Request{Org: 3, BaseURL: srv.URL, Path: "/ok", Method: http.MethodGet}); CodeOf(err) != CodeRateLimited {
		t.Fatalf("rate limit: %v", err)
	}
	if _, err := limited.Do(context.Background(), Request{Org: 4, BaseURL: srv.URL, Path: "/ok", Method: http.MethodGet}); err != nil {
		t.Fatalf("other org limited: %v", err)
	}
}

func TestExtractPathsAndPick(t *testing.T) {
	body := map[string]any{"data": map[string]any{"invoices": []any{
		map[string]any{"no": "F-1", "status": "Hata", "error": map[string]any{"text": "SMTP refused"}, "total": 120.5},
	}}}
	if a := ArrayPaths(body); len(a) != 1 || a[0] != "data.invoices" {
		t.Fatalf("arrays: %v", a)
	}
	items := Items(body, "data.invoices")
	paths := Paths(items[0])
	if len(paths) != 4 || paths[0].Path != "error.text" {
		t.Fatalf("paths: %+v", paths)
	}
	rows := Pick(items, []model.Field{{Path: "no", Label: "Fatura"}, {Path: "error.text", Label: "Hata metni"}})
	if rows[0]["Fatura"] != "F-1" || rows[0]["Hata metni"] != "SMTP refused" || len(rows[0]) != 2 {
		t.Fatalf("pick: %+v", rows)
	}
	if model.ChannelFrom(model.WithWorkflow(context.Background())) != model.ChannelWorkflow || model.ChannelFrom(context.Background()) != model.ChannelAgent {
		t.Fatal("channel marker")
	}
}

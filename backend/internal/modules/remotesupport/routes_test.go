package remotesupport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/remotesupport"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
)

func publicMux(limit remotesupport.Limiter) *http.ServeMux {
	mux := http.NewServeMux()
	svc := &remotesupport.Service{Hasher: remotesupport.NewHasher("test")}
	remotesupport.RegisterRoutes(supportops.Router{Mux: mux}, svc, limit)
	return mux
}

func post(mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)
	return w
}

func TestPublicResolveRateLimitedFailsClosed(t *testing.T) {
	deny := func(*http.Request, string, string, int, time.Duration) bool { return false }
	if w := post(publicMux(deny), "/v1/public/remote-support/resolve", `{"code":"123456789"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d", w.Code)
	}
	// No limiter wired at all is treated as "limited", never as "unlimited".
	if w := post(publicMux(nil), "/v1/public/remote-support/resolve", `{"code":"123456789"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("nil limiter status %d", w.Code)
	}
}

func TestPublicRoutesRejectBadInput(t *testing.T) {
	var subjects []string
	allow := func(_ *http.Request, action, subject string, _ int, _ time.Duration) bool {
		subjects = append(subjects, action)
		return true
	}
	mux := publicMux(allow)
	if w := post(mux, "/v1/public/remote-support/resolve", `{"code":"12"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("short code status %d", w.Code)
	}
	if w := post(mux, "/v1/public/remote-support/resolve", `{"code":"123456789","token":"x"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("code and token status %d", w.Code)
	}
	if w := post(mux, "/v1/public/remote-support/resolve", `{"unknown":1}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status %d", w.Code)
	}
	// A malformed client secret never reaches the database.
	if w := post(mux, "/v1/public/remote-support/0b4f7c55-8f5e-4f61-9d39-5d1f4fbc2d11/token", `{"client_secret":"short"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad secret status %d", w.Code)
	}
	if w := post(mux, "/v1/public/remote-support/not-a-uuid/end", `{"client_secret":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad uuid status %d", w.Code)
	}
	if len(subjects) == 0 {
		t.Fatal("limiter not consulted")
	}
}

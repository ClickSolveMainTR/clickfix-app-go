package handler

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
)

func TestNotifyOutcome(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{nil, 200},
		{usecase.ErrUnknownOrder, 200},
		{payments.ErrProviderDisabled, 404},
		{payments.ErrInvalidSignature, 400},
		{fmt.Errorf("wrapped: %w", payments.ErrMalformed), 400},
		{usecase.ErrAmountMismatch, 400},
		{model.ErrConflict, 400},
		{&payments.ProviderError{Code: "token_not_found"}, 400},
		{&payments.ProviderError{Temporary: true}, 503},
		{errors.New("connection refused"), 503},
	} {
		if got, _ := notifyOutcome(tc.err); got != tc.status {
			t.Errorf("%v: got %d want %d", tc.err, got, tc.status)
		}
	}
}

func TestWebhookSourceAndAllowlist(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "172.18.0.5:5000"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	// Default: private-network peers (Traefik, the BFF) are proxies.
	if got := webhookSource(r); got != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("default trusted proxy %v", got)
	}
	// Untrusted peer: forwarded headers are ignored.
	t.Setenv("PAYMENTS_TRUSTED_PROXY_CIDRS", "10.99.0.0/16")
	if got := webhookSource(r); got != netip.MustParseAddr("172.18.0.5") {
		t.Fatalf("untrusted peer %v", got)
	}
	public := httptest.NewRequest(http.MethodPost, "/", nil)
	public.RemoteAddr = "198.51.100.7:443"
	public.Header.Set("X-Forwarded-For", "6.6.6.6")
	t.Setenv("PAYMENTS_TRUSTED_PROXY_CIDRS", "")
	if got := webhookSource(public); got != netip.MustParseAddr("198.51.100.7") {
		t.Fatalf("public peer trusted by default %v", got)
	}
	// Behind a trusted proxy, the right-most untrusted hop is the source;
	// a spoofed left-most entry is not.
	t.Setenv("PAYMENTS_TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	if got := webhookSource(r); got != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("trusted proxy %v", got)
	}
	// Empty allowlist (default): every source, the signature decides.
	if !webhookSourceAllowed("iyzico", netip.MustParseAddr("198.51.100.1")) {
		t.Fatal("empty list blocked")
	}
	t.Setenv("PAYMENTS_IYZICO_WEBHOOK_CIDRS", "203.0.113.0/24, bad-entry")
	if !webhookSourceAllowed("iyzico", netip.MustParseAddr("203.0.113.9")) || webhookSourceAllowed("iyzico", netip.MustParseAddr("198.51.100.1")) {
		t.Fatal("allowlist not enforced")
	}
	if webhookSourceAllowed("iyzico", netip.Addr{}) {
		t.Fatal("unknown source allowed with a list set")
	}
}

func TestNotifyBodyLimitAndRateLimit(t *testing.T) {
	h := &Handler{limit: func(*http.Request, string, string, int, time.Duration) (bool, time.Duration) {
		return false, time.Second
	}}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.SetPathValue("provider", "paytr")
	w := httptest.NewRecorder()
	h.PaymentNotify(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit %d", w.Code)
	}
	h.limit = nil
	r = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, notifyBodyLimit+1)))
	r.SetPathValue("provider", "paytr")
	w = httptest.NewRecorder()
	h.PaymentNotify(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit %d", w.Code)
	}
}

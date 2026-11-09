package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

type addressLimiter struct {
	deny  bool
	calls []string
}

func (l *addressLimiter) Allow(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	l.calls = append(l.calls, action+":"+subject)
	return !l.deny, 30 * time.Second
}

type countingTokens struct{ validated int }

func (c *countingTokens) ValidateAccessToken(context.Context, string, string) (model.AccessToken, error) {
	c.validated++
	return model.AccessToken{}, nil
}
func (c *countingTokens) ResourceMetadataURL(resource string) string {
	return "https://app.test/.well-known/oauth-protected-resource" + resource
}

// The address limit runs before anything is authenticated, so a caller
// without a valid token cannot make the server read bodies and look tokens
// up without bound.
func TestAddressLimitRunsBeforeAuthentication(t *testing.T) {
	post := func(l *addressLimiter, tokens *countingTokens, auth string) *httptest.ResponseRecorder {
		s := New(Config{Tokens: tokens, Catalog: testCatalog(), Limiter: l})
		r := httptest.NewRequest(http.MethodPost, model.ResourceBusiness, strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`))
		r.Header.Set("X-Forwarded-For", "203.0.113.20")
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}

	limiter, tokens := &addressLimiter{deny: true}, &countingTokens{}
	w := post(limiter, tokens, "Bearer whatever")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "30" {
		t.Fatalf("limited caller got %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	if tokens.validated != 0 {
		t.Fatal("the token was looked up for a limited address")
	}
	if len(limiter.calls) != 1 || limiter.calls[0] != "mcp_ip:203.0.113.20" {
		t.Fatalf("limiter calls %v", limiter.calls)
	}

	// Under the limit, a request without a token still gets the 401 challenge.
	limiter, tokens = &addressLimiter{}, &countingTokens{}
	w = post(limiter, tokens, "")
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("unauthenticated caller got %d %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
	if len(limiter.calls) != 1 {
		t.Fatalf("limiter calls %v", limiter.calls)
	}
}

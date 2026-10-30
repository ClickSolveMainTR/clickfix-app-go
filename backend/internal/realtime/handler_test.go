package realtime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

// roleless loads a principal with no platform role or permission, like an
// organization owner who signed up or an invited agent.
type roleless struct{}

func (roleless) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return authctx.Principal{}, err
	}
	return authctx.Principal{UserID: id}, nil
}

type ownChannelOnly struct{}

func (ownChannelOnly) CanSubscribeChannel(_ context.Context, user uuid.UUID, _ bool, channel string) (bool, error) {
	return channel == "user:"+user.String(), nil
}

func TestTokensForUserWithoutPlatformRole(t *testing.T) {
	t.Parallel()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{
		Enabled: true, TokenHMAC: "unit-test-hmac-secret", TokenTTL: time.Hour,
		WSURL: "ws://127.0.0.1:8000/connection/websocket",
	})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwt.NewManager("unit-test-access-secret-32-bytes!!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user, SessionID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	realtime.RegisterRoutes(mux, realtime.NewHandler(issuer, ownChannelOnly{}), tokens, roleless{})

	call := func(path, body string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+access)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call("/v1/realtime/connection-token", ""); got != http.StatusOK {
		t.Fatalf("connection token: status %d, want 200", got)
	}
	own, _ := json.Marshal(map[string]string{"channel": "user:" + user.String()})
	if got := call("/v1/realtime/subscription-token", string(own)); got != http.StatusOK {
		t.Fatalf("own channel: status %d, want 200", got)
	}
	other, _ := json.Marshal(map[string]string{"channel": "user:" + uuid.NewString()})
	if got := call("/v1/realtime/subscription-token", string(other)); got != http.StatusForbidden {
		t.Fatalf("foreign channel: status %d, want 403", got)
	}
}

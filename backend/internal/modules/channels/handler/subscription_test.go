package handler_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/slack"
)

func TestSlackChallengeBeforeWorkspaceRouting(t *testing.T) {
	t.Parallel()
	a := slack.New(slack.Config{SigningSecret: "sign"})
	// A challenge arrives before any workspace/DB row exists.
	h := handler.New(usecase.New(nil, channels.NewRegistry(a), nil), nil, nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/webhooks/channels/{type}", h.SharedWebhook)
	b := `{"type":"url_verification","challenge":"challenge-value"}`
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m := hmac.New(sha256.New, []byte("sign"))
	m.Write([]byte("v0:" + ts + ":" + b))
	for _, tc := range []struct {
		name, signature string
		want            int
	}{{"signed", "v0=" + hex.EncodeToString(m.Sum(nil)), 200}, {"unsigned", "", 401}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/webhooks/channels/slack", strings.NewReader(b))
			r.Header.Set("X-Slack-Request-Timestamp", ts)
			r.Header.Set("X-Slack-Signature", tc.signature)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Code == 200 && w.Body.String() != "challenge-value" {
				t.Fatal("challenge not echoed")
			}
		})
	}
}

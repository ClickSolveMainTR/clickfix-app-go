package teams

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type clientFunc func(*http.Request) (*http.Response, error)

func (f clientFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestJWTAndThreadRoundTrip(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	app, tenant := uuid.NewString(), uuid.NewString()
	service := "https://smba.trafficmanager.net/emea/"
	var sent bool
	client := clientFunc(func(r *http.Request) (*http.Response, error) {
		var response any
		switch {
		case r.URL.Path == "/keys":
			response = map[string]any{"keys": []map[string]any{{"kid": "key1",
				"kty":          "RSA",
				"use":          "sig",
				"n":            base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":            "AQAB",
				"endorsements": []string{"msteams"}}}}
		case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token"):
			_ = r.ParseForm()
			if r.Form.Get("client_id") != app {
				t.Error("app ID missing")
			}
			response = map[string]any{"access_token": "access", "token_type": "Bearer"}
		default:
			sent = true
			if !strings.HasSuffix(r.URL.Path, "/activities/root") || r.Header.Get("Authorization") != "Bearer access" {
				t.Errorf("wrong thread request %s", r.URL.Path)
			}
			response = map[string]any{"id": "reply"}
		}
		b, _ := json.Marshal(response)
		return &http.Response{StatusCode: 200,
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader(string(b)))},
			nil
	})
	a := New(Config{Client: client, KeysURL: "https://keys.example/keys"})
	inst := channels.Instance{Credentials: channels.Credentials{"app_id": app,
		"app_password": "secret",
		"tenant_id":    tenant}}
	if _, err := a.Connect(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"type": "message",
		"id":         "reply1",
		"channelId":  "msteams",
		"serviceUrl": service,
		"text":       "Yardım",
		"replyToId":  "root",
		"from": map[string]string{"id": "user",
			"name": "Elif"},
		"recipient": map[string]string{"id": "bot"},
		"conversation": map[string]string{"id": "conversation",
			"conversationType": "channel",
			"tenantId":         tenant}}
	body, _ := json.Marshal(payload)
	makeRequest := func(audience, serviceClaim string, expiry time.Time) *http.Request {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256,
			jwt.MapClaims{"iss": issuer,
				"aud":        audience,
				"exp":        expiry.Unix(),
				"nbf":        time.Now().Add(-time.Minute).Unix(),
				"serviceurl": serviceClaim})
		token.Header["kid"] = "key1"
		raw, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		r, _ := http.NewRequest("POST", "https://api.example/hook", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		return r
	}
	r := makeRequest(app, service, time.Now().Add(time.Hour))
	if err := a.VerifyWebhook(r, body, inst); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, audience, service string
		expiry                  time.Time
	}{{"audience",
		"wrong",
		service,
		time.Now().Add(time.Hour)},
		{"service",
			app,
			"https://other.botframework.com/",
			time.Now().Add(time.Hour)},
		{"expired",
			app,
			service,
			time.Now().Add(-time.Hour)}} {
		t.Run(tc.name, func(t *testing.T) {
			if a.VerifyWebhook(makeRequest(tc.audience, tc.service, tc.expiry), body, inst) == nil {
				t.Fatal("JWT boundary bypassed")
			}
		})
	}
	result, err := a.ParseWebhook(context.Background(), r, body, inst)
	if err != nil || len(result.Messages) != 1 {
		t.Fatalf("parse: %+v %v", result, err)
	}
	out,
		err := a.Send(context.Background(),
		inst,
		channels.OutboundMessage{ThreadID: result.Messages[0].ThreadID,
			Text: "Yanıt"})
	if err != nil || !sent || out.ExternalID != "conversation:reply" {
		t.Fatalf("reply: %+v %v", out, err)
	}
	payload["channelData"] = map[string]any{"tenant": map[string]string{"id": "foreign"}}
	payload["conversation"] = map[string]string{"id": "c", "tenantId": "foreign"}
	foreign, _ := json.Marshal(payload)
	if a.VerifyWebhook(r, foreign, inst) == nil {
		t.Fatal("foreign tenant accepted")
	}
}

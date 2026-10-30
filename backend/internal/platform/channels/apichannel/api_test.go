package apichannel_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/apichannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/google/uuid"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSignedInboundAndOutbound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	secret := strings.Repeat("s", 32)
	inst := channels.Instance{Credentials: channels.Credentials{"signing_secret": secret,
		"webhook_url": "https://customer.example/hook"}}
	body := []byte(`{"id":"m1","sender_id":"u1","text":"Merhaba","thread_id":"t1"}`)
	req := httptest.NewRequest("POST", "https://api.example/hook", nil)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-ClickFix-Timestamp", ts)
	req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(secret, ts, body))
	a := apichannel.New(nil)
	if err := a.VerifyWebhook(req, body, inst); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body []byte
		ts   string
	}{{"tampered", []byte(`{}`), ts}, {"expired", body, "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := req.Clone(ctx)
			r.Header.Set("X-ClickFix-Timestamp", tc.ts)
			if a.VerifyWebhook(r, tc.body, inst) == nil {
				t.Fatal("accepted invalid signature")
			}
		})
	}
	res, err := a.ParseWebhook(ctx, req, body, inst)
	if err != nil || len(res.Messages) != 1 || res.Messages[0].ThreadID != "t1" || !res.Messages[0].From.InstanceScoped {
		t.Fatalf("parse: %+v %v", res, err)
	}
	id := uuid.New()
	c := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if err := channelhttp.Verify(r, b, secret, time.Now()); err != nil {
			t.Error(err)
		}
		if r.Header.Get("Idempotency-Key") != id.String() {
			t.Error("missing idempotency key")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	a = apichannel.New(c)
	sent,
		err := a.Send(ctx,
		inst,
		channels.OutboundMessage{MessageUUID: id,
			Text:     "Yanıt",
			To:       channels.Recipient{ExternalID: "u1"},
			ThreadID: "t1"})
	if err != nil || sent.ExternalID != id.String() {
		t.Fatalf("send: %+v %v", sent, err)
	}
	inst.Credentials["webhook_url"] = "https://127.0.0.1/private"
	if _, err := a.Connect(ctx, inst); err == nil {
		t.Fatal("accepted private URL")
	}
}
func TestPublicTransportBlocksDNSPrivateAndRedirect(t *testing.T) {
	t.Parallel()
	c := channelhttp.PublicClient()
	r := httptest.NewRequest("POST", "https://localhost/private", nil)
	r.RequestURI = ""
	if _, err := c.Do(r); err == nil {
		t.Fatal("private target accepted")
	}
}

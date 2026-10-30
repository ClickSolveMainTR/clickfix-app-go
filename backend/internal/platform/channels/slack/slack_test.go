package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

func TestWorkspaceOAuthAndThreadReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lookups := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth.v2.access":
			u, p, ok := r.BasicAuth()
			if !ok || u != "client" || p != "secret" {
				t.Error("OAuth credentials missing")
			}
			_, _ = w.Write([]byte(`{"ok":true,"access_token":"bot","bot_user_id":"BOT","team":{"id":"T1"}}`))
		case "/users.conversations":
			if r.Header.Get("Authorization") != "Bearer bot" || r.FormValue("types") != "public_channel,private_channel" {
				t.Error("conversation listing request malformed")
			}
			if r.FormValue("cursor") == "" {
				_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"C1","name":"destek","is_shared":true}],"response_metadata":{"next_cursor":"n2"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"channels":[{"id":"G2","name":"vip","is_private":true}],"response_metadata":{"next_cursor":""}}`))
		case "/conversations.info":
			if r.FormValue("channel") != "C1" {
				t.Error("channel lookup malformed")
			}
			_, _ = w.Write([]byte(`{"ok":true,"channel":{"id":"C1","name":"destek"}}`))
		case "/users.info":
			lookups++
			_, _ = w.Write([]byte(`{"ok":true,"user":{"id":"U1","name":"ayse","profile":{"display_name":"","real_name":"Ayşe Yılmaz"}}}`))
		case "/auth.test":
			_, _ = w.Write([]byte(`{"ok":true,"team_id":"T1"}`))
		case "/chat.postMessage":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["channel"] != "C1" || b["thread_ts"] != "123.1" {
				t.Errorf("thread lost: %+v", b)
			}
			_, _ = w.Write([]byte(`{"ok":true,"ts":"124.1"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	a := New(Config{ClientID: "client",
		ClientSecret:  "secret",
		SigningSecret: "sign",
		RedirectURL:   "https://ui.example/slack",
		BaseURL:       server.URL})
	creds, err := a.ExchangeCode(ctx, "code")
	if err != nil {
		t.Fatal(err)
	}
	inst := channels.Instance{Credentials: creds, Settings: map[string]any{"channel_ids": []string{"C1"}}}
	if _, err := a.Connect(ctx, inst); err != nil {
		t.Fatal(err)
	}
	convs, err := a.Conversations(ctx, inst)
	if err != nil || len(convs) != 2 || convs[0].ID != "C1" || !convs[0].Shared || !convs[1].Private {
		t.Fatalf("conversations = %+v, %v", convs, err)
	}
	body := []byte(`{"type":"event_callback","team_id":"T1","event_id":"E1","event":{"type":"message","channel":"C1","user":"U1","ts":"124.0","thread_ts":"123.1","text":"yardım"}}`)
	req := httptest.NewRequest("POST", "/", nil)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m := hmac.New(sha256.New, []byte("sign"))
	m.Write([]byte("v0:" + ts + ":"))
	m.Write(body)
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(m.Sum(nil)))
	if err := a.VerifyWebhook(req, body, inst); err != nil {
		t.Fatal(err)
	}
	if a.VerifyWebhook(req, append(body, ' '), inst) == nil {
		t.Fatal("accepted changed body")
	}
	res, err := a.ParseWebhook(ctx, req, body, inst)
	if err != nil || len(res.Messages) != 1 || res.Messages[0].ThreadID != "C1:123.1" {
		t.Fatalf("parse: %+v %v", res, err)
	}
	if got := res.Messages[0]; got.From.Name != "#destek" || got.Attributes["author_name"] != "Ayşe Yılmaz" {
		t.Fatalf("names = %q / %v", got.From.Name, got.Attributes["author_name"])
	}
	if _, err := a.ParseWebhook(ctx, req, body, inst); err != nil || lookups != 1 {
		t.Fatalf("names not cached: %d lookups, %v", lookups, err)
	}
	sent,
		err := a.Send(ctx,
		inst,
		channels.OutboundMessage{MessageUUID: uuid.New(),
			Text:     "yanıt",
			ThreadID: res.Messages[0].ThreadID})
	if err != nil || sent.ExternalID != "C1:124.1" {
		t.Fatalf("send: %+v %v", sent, err)
	}
	inst.Settings = nil
	res, err = a.ParseWebhook(ctx, req, body, inst)
	if err != nil || len(res.Messages) != 0 {
		t.Fatal("unselected channel ingested")
	}
	inst.Credentials["team_id"] = "T2"
	if _, err := a.ParseWebhook(ctx, req, body, inst); err == nil {
		t.Fatal("foreign workspace accepted")
	}
}
func TestSignedPostChallenge(t *testing.T) {
	t.Parallel()
	a := New(Config{SigningSecret: "s"})
	b := []byte(`{"type":"url_verification","challenge":"challenge"}`)
	r := httptest.NewRequest("POST", "/", nil)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m := hmac.New(sha256.New, []byte("s"))
	m.Write([]byte("v0:" + ts + ":"))
	m.Write(b)
	r.Header.Set("X-Slack-Request-Timestamp", ts)
	r.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(m.Sum(nil)))
	_, answer, handled, err := a.RespondWebhook(r, b, channels.Instance{})
	if err != nil || !handled || answer != "challenge" {
		t.Fatalf("challenge %s %v", answer, err)
	}
	r.Header.Set("X-Slack-Request-Timestamp", "1")
	if _, _, _, err := a.RespondWebhook(r, b, channels.Instance{}); err == nil {
		t.Fatal("old challenge accepted")
	}
}
func TestNameLookupFailureStillIngests(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"ok":false,"error":"missing_scope"}`))
	}))
	defer server.Close()
	a := New(Config{BaseURL: server.URL})
	inst := channels.Instance{Credentials: channels.Credentials{"team_id": "T1", "bot_token": "bot"},
		Settings: map[string]any{"channel_ids": []string{"C1"}}}
	body := []byte(`{"type":"event_callback","team_id":"T1","event":{"type":"message","channel":"C1","user":"U1","ts":"1.0","text":"selam"}}`)
	res, err := a.ParseWebhook(context.Background(), httptest.NewRequest("POST", "/", nil), body, inst)
	if err != nil || len(res.Messages) != 1 || res.Messages[0].From.Name != "" {
		t.Fatalf("parse: %+v %v", res, err)
	}
	if _, ok := res.Messages[0].Attributes["author_name"]; ok {
		t.Fatal("author name invented")
	}
	if _, err := a.ParseWebhook(context.Background(), httptest.NewRequest("POST", "/", nil), body, inst); err != nil || calls.Load() != 2 {
		t.Fatalf("failed lookups retried at once: %d calls, %v", calls.Load(), err)
	}
}

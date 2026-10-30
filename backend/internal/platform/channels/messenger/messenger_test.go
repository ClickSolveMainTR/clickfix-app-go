package messenger

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/metagraph"
)

const (
	pageID = "104711122233344"
	psid   = "6543210987654321"
	token  = "EAAGpagetoken"
)

type call struct {
	Method, Path string
	Query        url.Values
	Auth         string
	Body         map[string]any
}

// fakeGraph answers by "METHOD /path" and records every call.
type fakeGraph struct {
	*httptest.Server
	mu     sync.Mutex
	calls  []call
	routes map[string]func(w http.ResponseWriter, c call)
}

func newGraph(t *testing.T, routes map[string]func(w http.ResponseWriter, c call)) *fakeGraph {
	f := &fakeGraph{routes: routes}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Auth: r.Header.Get("Authorization")}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &c.Body)
		}
		f.mu.Lock()
		f.calls = append(f.calls, c)
		f.mu.Unlock()
		h, ok := f.routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown path","code":803}}`))
			return
		}
		h(w, c)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGraph) last() call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func reply(body string) func(http.ResponseWriter, call) {
	return func(w http.ResponseWriter, _ call) { _, _ = w.Write([]byte(body)) }
}

func fail(status int, body string) func(http.ResponseWriter, call) {
	return func(w http.ResponseWriter, _ call) { w.WriteHeader(status); _, _ = w.Write([]byte(body)) }
}

func adapter(f *fakeGraph) *Adapter {
	cfg := Config{AppID: "app1", AppSecret: "appsecret", VerifyToken: "vt", RedirectURL: "https://app.example/cb", APIVersion: "v21.0"}
	if f != nil {
		cfg.BaseURL, cfg.HTTPClient = f.URL, f.Client()
	}
	return New(cfg)
}

func inst() channels.Instance {
	return channels.Instance{ID: 1, Type: channels.TypeMessenger, ExternalID: pageID,
		Credentials: channels.Credentials{FieldPageID: pageID, FieldPageAccessToken: token}}
}

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parse(t *testing.T, a *Adapter, body []byte) channels.WebhookResult {
	t.Helper()
	res, err := a.ParseWebhook(context.Background(), nil, body, inst())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestParseWebhookEvents(t *testing.T) {
	a := adapter(nil)

	res := parse(t, a, fixture(t, "text.json"))
	if len(res.Messages) != 1 {
		t.Fatalf("text: %+v", res)
	}
	m := res.Messages[0]
	if m.ExternalID != "m_AbCdEfText01" || m.Kind != "text" || m.Text != "Merhaba, siparişim nerede?" ||
		m.From.ExternalID != psid || !m.From.InstanceScoped || m.ReplyToExternalID != "m_PrevOutbound01" ||
		!m.At.Equal(time.UnixMilli(1761645600000)) {
		t.Fatalf("text: %+v", m)
	}

	res = parse(t, a, fixture(t, "quick_reply.json"))
	if m := res.Messages[0]; m.Kind != "interactive" || m.Text != "Evet" || m.Attributes["quick_reply_payload"] != "CONFIRM_ORDER_42" {
		t.Fatalf("quick reply: %+v", m)
	}

	res = parse(t, a, fixture(t, "postback.json"))
	if m := res.Messages[0]; m.ExternalID != "m_AbCdEfPost01" || m.Kind != "interactive" || m.Text != "Destek al" || m.Attributes["postback_payload"] != "GET_SUPPORT" {
		t.Fatalf("postback: %+v", m)
	}

	if res = parse(t, a, fixture(t, "echo.json")); len(res.Messages)+len(res.Statuses) != 0 {
		t.Fatalf("echo not skipped: %+v", res)
	}

	res = parse(t, a, fixture(t, "delivery.json"))
	if len(res.Messages) != 0 || len(res.Statuses) != 2 || res.Statuses[0].ExternalID != "m_Out01" ||
		res.Statuses[1].Status != channels.StatusDelivered || !res.Statuses[0].At.Equal(time.UnixMilli(1761645604500)) {
		t.Fatalf("delivery: %+v", res)
	}
}

func TestParseWebhookAttachmentOpen(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bytes of " + r.URL.Path))
	}))
	defer cdn.Close()
	a := New(Config{AppSecret: "x", HTTPClient: cdn.Client()})
	body := []byte(strings.ReplaceAll(string(fixture(t, "attachment.json")), "{{CDN}}", cdn.URL))
	res := parse(t, a, body)
	m := res.Messages[0]
	if m.Kind != "media" || len(m.Attachments) != 2 {
		t.Fatalf("attachment: %+v", m)
	}
	if m.Attachments[0].Filename != "photo_123.jpg" || m.Attachments[0].ContentType != "image/jpeg" ||
		m.Attachments[1].Filename != "fatura.pdf" || m.Attachments[1].ContentType != "application/pdf" {
		t.Fatalf("attachment meta: %+v", m.Attachments)
	}
	if types, _ := m.Attributes["attachment_types"].([]string); len(types) != 1 || types[0] != "fallback" {
		t.Fatalf("attributes: %+v", m.Attributes)
	}
	rc, err := m.Attachments[1].Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "bytes of /file_upload/fatura.pdf" {
		t.Fatalf("open: %q", got)
	}
}

func TestParseWebhookIgnoresOtherPagesAndRoute(t *testing.T) {
	a := adapter(nil)
	body := fixture(t, "other_page.json")
	res := parse(t, a, body)
	if len(res.Messages) != 1 || res.Messages[0].ExternalID != "m_Mine01" {
		t.Fatalf("got %+v", res.Messages)
	}
	ids, err := a.Route(nil, body)
	if err != nil || strings.Join(ids, ",") != "999999999999999,"+pageID {
		t.Fatalf("route: %v %v", ids, err)
	}
	if _, err := a.Route(nil, []byte(`{"object":"instagram","entry":[]}`)); err == nil {
		t.Fatal("wrong object routed")
	}
}

func TestVerifyAndChallenge(t *testing.T) {
	a := adapter(nil)
	body := fixture(t, "text.json")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(metagraph.SignatureHeader, metagraph.Sign("appsecret", body))
	if err := a.VerifyWebhook(r, body, inst()); err != nil {
		t.Fatal(err)
	}
	r.Header.Set(metagraph.SignatureHeader, metagraph.Sign("wrong", body))
	if err := a.VerifyWebhook(r, body, inst()); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("bad signature: %v", err)
	}
	q := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=vt&hub.challenge=c1", nil)
	if got, ok := a.ChallengeShared(q); !ok || got != "c1" {
		t.Fatal("shared challenge")
	}
	if got, ok := a.Challenge(q, inst()); !ok || got != "c1" {
		t.Fatal("challenge")
	}
	q = httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=no&hub.challenge=c1", nil)
	if _, ok := a.ChallengeShared(q); ok {
		t.Fatal("wrong verify token accepted")
	}
}

func TestConnectAndHealth(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/" + pageID:    reply(`{"id":"` + pageID + `","name":"ClickFix Shop"}`),
		"GET /v21.0/debug_token":  reply(`{"data":{"is_valid":true,"expires_at":0}}`),
		"GET /v21.0/222222222222": reply(`{"id":"333","name":"Other"}`),
		"GET /v21.0/444444444444": fail(400, `{"error":{"message":"Error validating access token","code":190}}`),
		"GET /v21.0/555555555555": fail(503, `{"error":{"message":"down","code":2}}`),
	})
	a := adapter(f)
	id, err := a.Connect(context.Background(), inst())
	if err != nil || id != pageID {
		t.Fatalf("connect: %q %v", id, err)
	}
	if c := f.calls[0]; c.Auth != "Bearer "+token || c.Query.Get("fields") != "id,name" {
		t.Fatalf("connect call: %+v", c)
	}
	other := inst()
	other.Credentials = channels.Credentials{FieldPageID: "222222222222", FieldPageAccessToken: token}
	if _, err := a.Connect(context.Background(), other); err == nil {
		t.Fatal("token of another page accepted")
	}
	h, err := a.CheckHealth(context.Background(), inst())
	if err != nil || !h.OK || h.TokenExpiresAt != nil {
		t.Fatalf("health: %+v %v", h, err)
	}
	if f.last().Auth != "Bearer app1|appsecret" {
		t.Fatalf("debug_token auth: %+v", f.last())
	}
	bad := inst()
	bad.Credentials = channels.Credentials{FieldPageID: "444444444444", FieldPageAccessToken: token}
	if h, err := a.CheckHealth(context.Background(), bad); err != nil || h.OK || h.Detail == "" {
		t.Fatalf("token health: %+v %v", h, err)
	}
	bad.Credentials[FieldPageID] = "555555555555"
	if _, err := a.CheckHealth(context.Background(), bad); err == nil {
		t.Fatal("outage reported as unhealthy instead of error")
	}
}

func TestSend(t *testing.T) {
	var n int
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"POST /v21.0/" + pageID + "/messages": func(w http.ResponseWriter, c call) {
			n++
			_, _ = w.Write([]byte(`{"recipient_id":"` + psid + `","message_id":"m_sent` + string(rune('0'+n)) + `"}`))
		},
	})
	a := adapter(f)
	res, err := a.Send(context.Background(), inst(), channels.OutboundMessage{
		To: channels.Recipient{ExternalID: psid}, Text: "Merhaba",
		Attachments: []channels.OutboundAttachment{{ContentType: "image/png", URL: "https://api.example/m/1"}, {ContentType: "application/pdf", URL: "https://api.example/m/2"}},
	})
	if err != nil || res.ExternalID != "m_sent1" || len(f.calls) != 3 {
		t.Fatalf("send: %+v %v (%d calls)", res, err, len(f.calls))
	}
	text := f.calls[0].Body
	if text["messaging_type"] != "RESPONSE" || text["tag"] != nil || text["message"].(map[string]any)["text"] != "Merhaba" ||
		text["recipient"].(map[string]any)["id"] != psid || f.calls[0].Auth != "Bearer "+token {
		t.Fatalf("text payload: %+v", text)
	}
	img := f.calls[1].Body["message"].(map[string]any)["attachment"].(map[string]any)
	if img["type"] != "image" || img["payload"].(map[string]any)["url"] != "https://api.example/m/1" {
		t.Fatalf("image payload: %+v", img)
	}
	if f.calls[2].Body["message"].(map[string]any)["attachment"].(map[string]any)["type"] != "file" {
		t.Fatal("pdf not sent as file")
	}

	_, err = a.Send(context.Background(), inst(), channels.OutboundMessage{
		To: channels.Recipient{ExternalID: psid}, Text: "Geç cevap", Attributes: map[string]any{"human_agent": true},
	})
	if b := f.last().Body; err != nil || b["messaging_type"] != "MESSAGE_TAG" || b["tag"] != "HUMAN_AGENT" {
		t.Fatalf("human agent: %+v %v", b, err)
	}
}

const windowErr = `{"error":{"message":"(#10) This message is sent outside of allowed window.","type":"OAuthException","code":10,"error_subcode":2018278}}`

func TestSendWindowFallbackAndErrors(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"POST /v21.0/" + pageID + "/messages": func(w http.ResponseWriter, c call) {
			switch c.Body["tag"] {
			case "HUMAN_AGENT":
				_, _ = w.Write([]byte(`{"message_id":"m_tagged"}`))
			default:
				w.WriteHeader(400)
				_, _ = w.Write([]byte(windowErr))
			}
		},
	})
	a := adapter(f)
	res, err := a.Send(context.Background(), inst(), channels.OutboundMessage{To: channels.Recipient{ExternalID: psid}, Text: "x"})
	if err != nil || res.ExternalID != "m_tagged" || len(f.calls) != 2 {
		t.Fatalf("fallback: %+v %v", res, err)
	}
	_, err = a.Send(context.Background(), inst(), channels.OutboundMessage{
		To: channels.Recipient{ExternalID: psid}, Text: "bot", Attributes: map[string]any{"automated": true},
	})
	if !channels.IsPermanent(err) || !errors.Is(err, channels.ErrWindowClosed) {
		t.Fatalf("window: %v", err)
	}

	for _, c := range []struct {
		status    int
		body      string
		permanent bool
		code      string
	}{
		{500, `{"error":{"message":"oops","code":1}}`, false, "unavailable"},
		{400, `{"error":{"message":"Error validating access token: Session has expired","type":"OAuthException","code":190,"error_subcode":463}}`, true, "token_invalid"},
		{400, `{"error":{"message":"rate","code":4}}`, false, "rate_limited"},
	} {
		g := newGraph(t, map[string]func(http.ResponseWriter, call){"POST /v21.0/" + pageID + "/messages": fail(c.status, c.body)})
		_, err := adapter(g).Send(context.Background(), inst(), channels.OutboundMessage{To: channels.Recipient{ExternalID: psid}, Text: "x"})
		var se *channels.SendError
		if !errors.As(err, &se) || se.Code != c.code || channels.IsPermanent(err) != c.permanent {
			t.Fatalf("%d: %v", c.status, err)
		}
	}
	if _, err := a.Send(context.Background(), inst(), channels.OutboundMessage{To: channels.Recipient{ExternalID: "../x"}, Text: "x"}); !channels.IsPermanent(err) {
		t.Fatalf("bad recipient: %v", err)
	}
}

func TestSendTyping(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){"POST /v21.0/" + pageID + "/messages": reply(`{"recipient_id":"1"}`)})
	if err := adapter(f).SendTyping(context.Background(), inst(), channels.Recipient{ExternalID: psid}, true); err != nil {
		t.Fatal(err)
	}
	if b := f.last().Body; b["sender_action"] != "typing_on" || b["message"] != nil {
		t.Fatalf("typing: %+v", b)
	}
}

func TestReconcile(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/" + pageID + "/conversations": reply(`{"data":[{"id":"t_1","updated_time":"2026-10-28T10:30:00+0000","messages":{"data":[
			{"id":"m_page","created_time":"2026-10-28T10:30:00+0000","from":{"id":"` + pageID + `","name":"ClickFix Shop"},"message":"Cevap"},
			{"id":"m_new","created_time":"2026-10-28T10:20:00+0000","from":{"id":"` + psid + `","name":"Ayşe K","email":"` + psid + `@facebook.com"},"message":"Kaçırılan mesaj"},
			{"id":"m_old","created_time":"2026-10-28T08:00:00+0000","from":{"id":"` + psid + `"},"message":"eski"}]}}]}`),
	})
	since := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	res, err := adapter(f).Reconcile(context.Background(), inst(), since)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ExternalID != "m_new" || res.Messages[0].From.Name != "Ayşe K" || res.Messages[0].Attributes["reconciled"] != true {
		t.Fatalf("reconcile: %+v", res.Messages)
	}
	q := f.last().Query
	if q.Get("platform") != "messenger" || !strings.Contains(q.Get("fields"), "messages.limit(20){id,created_time,from,message,attachments") {
		t.Fatalf("query: %v", q)
	}
}

func TestOAuth(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/oauth/access_token": func(w http.ResponseWriter, c call) {
			if c.Query.Get("code") == "thecode" && c.Query.Get("client_secret") == "appsecret" && c.Query.Get("redirect_uri") == "https://app.example/cb" {
				_, _ = w.Write([]byte(`{"access_token":"short","token_type":"bearer"}`))
				return
			}
			if c.Query.Get("grant_type") == "fb_exchange_token" && c.Query.Get("fb_exchange_token") == "short" {
				_, _ = w.Write([]byte(`{"access_token":"long","token_type":"bearer","expires_in":5183944}`))
				return
			}
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"bad","code":100}}`))
		},
		"GET /v21.0/me/accounts": reply(`{"data":[
			{"id":"` + pageID + `","name":"ClickFix Shop","access_token":"pt1","tasks":["ANALYZE","ADVERTISE","MESSAGING","MODERATE","CREATE_CONTENT","MANAGE"]},
			{"id":"200","name":"Read only","access_token":"pt2","tasks":["ANALYZE"]},
			{"id":"300","name":"No token"}]}`),
		"POST /v21.0/" + pageID + "/subscribed_apps": reply(`{"success":true}`),
	})
	a := adapter(f)
	login, _ := url.Parse(a.LoginURL("st8"))
	if login.Host != "www.facebook.com" || login.Path != "/v21.0/dialog/oauth" || login.Query().Get("state") != "st8" ||
		login.Query().Get("scope") != "pages_messaging,pages_show_list,pages_manage_metadata,pages_read_engagement" ||
		login.Query().Get("client_id") != "app1" || login.Query().Get("redirect_uri") != "https://app.example/cb" {
		t.Fatalf("login url: %s", login)
	}
	tok, err := a.ExchangeCode(context.Background(), "thecode")
	if err != nil || tok != "long" {
		t.Fatalf("exchange: %q %v", tok, err)
	}
	pages, err := a.Pages(context.Background(), tok)
	if err != nil || len(pages) != 1 || pages[0] != (Page{ID: pageID, Name: "ClickFix Shop", AccessToken: "pt1"}) {
		t.Fatalf("pages: %+v %v", pages, err)
	}
	if f.last().Auth != "Bearer long" {
		t.Fatal("pages without user token")
	}
	if err := a.SubscribePage(context.Background(), pageID, "pt1"); err != nil {
		t.Fatal(err)
	}
	if c := f.last(); c.Auth != "Bearer pt1" || c.Query.Get("subscribed_fields") != SubscribedFields {
		t.Fatalf("subscribe: %+v", c)
	}
}

func TestOAuthUserIDForDeauthorization(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{{`{"id":"123456789"}`, true}, {`{}`, false}, {`{"id":"bad"}`, false}} {
		t.Run(tc.body, func(t *testing.T) {
			g := newGraph(t, map[string]func(http.ResponseWriter, call){"GET /v21.0/me": func(w http.ResponseWriter, c call) {
				if c.Query.Get("fields") != "id" {
					t.Error("only subject id should be requested")
				}
				_, _ = w.Write([]byte(tc.body))
			}})
			a := New(Config{AppID: "test", AppSecret: "test-secret", BaseURL: g.URL})
			id, err := a.UserID(t.Context(), "user-token")
			if (err == nil) != tc.valid {
				t.Fatalf("id=%s err=%v", id, err)
			}
		})
	}
}

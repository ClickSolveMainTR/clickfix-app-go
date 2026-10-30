package instagram

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
	igUser = "17841400000000001"
	igsid  = "987654321098765"
	token  = "IGAAtoken"
)

type call struct {
	Method, Path string
	Query        url.Values
	Auth         string
	Form         url.Values
	Body         map[string]any
}

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
		raw, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			c.Form, _ = url.ParseQuery(string(raw))
		} else if len(raw) > 0 {
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
	cfg := Config{AppID: "igapp", AppSecret: "igsecret", VerifyToken: "vt", RedirectURL: "https://app.example/ig/cb", APIVersion: "v21.0"}
	if f != nil {
		cfg.BaseURL, cfg.HTTPClient = f.URL, f.Client()
	}
	a := New(cfg)
	a.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	return a
}

func inst() channels.Instance {
	return channels.Instance{ID: 7, Type: channels.TypeInstagram, ExternalID: igUser,
		Credentials: channels.Credentials{FieldUserID: igUser, FieldAccessToken: token, FieldUsername: "clickfix.shop"}}
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

func TestParseDMs(t *testing.T) {
	a := adapter(nil)
	res := parse(t, a, fixture(t, "dm_text.json"))
	if len(res.Messages) != 1 {
		t.Fatalf("dm: %+v", res)
	}
	m := res.Messages[0]
	if m.Kind != "text" || m.Text != "Ürün stokta var mı?" || m.From.ExternalID != igsid || !m.From.InstanceScoped ||
		m.ThreadID != "" || !strings.HasPrefix(m.ExternalID, "aWdf") {
		t.Fatalf("dm: %+v", m)
	}

	res = parse(t, a, fixture(t, "story_reply.json"))
	if m := res.Messages[0]; m.Kind != "text" || m.Attributes["story_reply"] != true || m.Attributes["story_id"] != "18022233344455566" {
		t.Fatalf("story reply: %+v", m)
	}

	if res = parse(t, a, fixture(t, "echo.json")); len(res.Messages) != 0 {
		t.Fatalf("echo/deleted not skipped: %+v", res.Messages)
	}

	res = parse(t, a, fixture(t, "seen_reaction.json"))
	if len(res.Statuses) != 1 || res.Statuses[0].ExternalID != "aWdfOutbound01" || res.Statuses[0].Status != channels.StatusRead {
		t.Fatalf("seen: %+v", res.Statuses)
	}
	if len(res.Messages) != 1 || res.Messages[0].Text != "❤️" || res.Messages[0].ReplyToExternalID != "aWdfOutbound01" || res.Messages[0].Attributes["reaction"] != "love" {
		t.Fatalf("reaction: %+v", res.Messages)
	}
}

func TestParseStoryMentionOpen(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("story-bytes")) }))
	defer cdn.Close()
	a := New(Config{AppSecret: "x", HTTPClient: cdn.Client()})
	res := parse(t, a, []byte(strings.ReplaceAll(string(fixture(t, "story_mention.json")), "{{CDN}}", cdn.URL)))
	m := res.Messages[0]
	if m.Kind != "media" || len(m.Attachments) != 1 || m.Attributes["story_mention"] != true || m.Attachments[0].Filename != "story_abc.jpg" {
		t.Fatalf("story mention: %+v", m)
	}
	rc, err := m.Attachments[0].Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "story-bytes" {
		t.Fatalf("open: %q", b)
	}
}

func TestParseComments(t *testing.T) {
	res := parse(t, adapter(nil), fixture(t, "comment.json"))
	if len(res.Messages) != 2 {
		t.Fatalf("own comment not skipped: %+v", res.Messages)
	}
	top, sub := res.Messages[0], res.Messages[1]
	if top.ExternalID != "comment:17865799348089039" || top.ThreadID != "comment:17865799348089039" ||
		top.From.ExternalID != "232323232" || top.From.Username != "ayse.k" || top.Attributes["source"] != "comment" ||
		top.Attributes["media_id"] != "18011112222333444" || !top.At.Equal(time.Unix(1761645703, 0)) {
		t.Fatalf("comment: %+v", top)
	}
	if sub.ThreadID != top.ThreadID || sub.Attributes["parent_id"] != "17865799348089039" || sub.Attributes["comment_id"] != "17865799348089040" {
		t.Fatalf("reply comment: %+v", sub)
	}
}

func TestParseMention(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/" + igUser: reply(`{"mentioned_comment":{"id":"17895695668004550","text":"@clickfix.shop bunu gördünüz mü?","timestamp":"2026-10-28T10:00:00+0000","username":"mehmet.y","media":{"id":"17920238422030506","permalink":"https://www.instagram.com/p/abc/"}},"id":"` + igUser + `"}`),
	})
	res := parse(t, adapter(f), fixture(t, "mention.json"))
	if len(res.Messages) != 1 {
		t.Fatalf("mention: %+v", res.Messages)
	}
	m := res.Messages[0]
	if m.ThreadID != "mention:17920238422030506:17895695668004550" || m.Attributes["source"] != "mention" ||
		m.From.Username != "mehmet.y" || m.Attributes["media_permalink"] != "https://www.instagram.com/p/abc/" {
		t.Fatalf("mention: %+v", m)
	}
	if !strings.Contains(f.last().Query.Get("fields"), "mentioned_comment.comment_id(17895695668004550)") {
		t.Fatalf("query: %v", f.last().Query)
	}
	// An outage fails the parse so Meta redelivers.
	down := newGraph(t, map[string]func(http.ResponseWriter, call){"GET /v21.0/" + igUser: fail(503, `{"error":{"message":"down","code":2}}`)})
	if _, err := adapter(down).ParseWebhook(context.Background(), nil, fixture(t, "mention.json"), inst()); err == nil {
		t.Fatal("outage swallowed")
	}
}

func TestRouteAndIgnoreOtherAccounts(t *testing.T) {
	a := adapter(nil)
	body := []byte(`{"object":"instagram","entry":[{"id":"1","time":1,"messaging":[{"sender":{"id":"2"},"recipient":{"id":"1"},"timestamp":1,"message":{"mid":"x","text":"y"}}]},{"id":` + igUser + `,"time":1,"messaging":[]}]}`)
	ids, err := a.Route(nil, body)
	if err != nil || strings.Join(ids, ",") != "1,"+igUser {
		t.Fatalf("route: %v %v", ids, err)
	}
	if res := parse(t, a, body); len(res.Messages) != 0 {
		t.Fatalf("other account parsed: %+v", res)
	}
}

func TestVerifyAndChallenge(t *testing.T) {
	a := adapter(nil)
	body := fixture(t, "dm_text.json")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(metagraph.SignatureHeader, metagraph.Sign("igsecret", body))
	if err := a.VerifyWebhook(r, body, inst()); err != nil {
		t.Fatal(err)
	}
	if err := a.VerifyWebhook(r, append([]byte(" "), body...), inst()); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("tampered body: %v", err)
	}
	q := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=vt&hub.challenge=99", nil)
	if got, ok := a.ChallengeShared(q); !ok || got != "99" {
		t.Fatal("challenge")
	}
}

func TestConnectAndHealth(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/me": func(w http.ResponseWriter, c call) {
			if c.Auth == "Bearer "+token {
				_, _ = w.Write([]byte(`{"user_id":` + igUser + `,"username":"clickfix.shop","id":"9988776655"}`))
				return
			}
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token","type":"OAuthException","code":190}}`))
		},
	})
	a := adapter(f)
	id, err := a.Connect(context.Background(), inst())
	if err != nil || id != igUser || f.last().Query.Get("fields") != "user_id,username" {
		t.Fatalf("connect: %q %v", id, err)
	}
	i := inst()
	i.Credentials[FieldExpiresAt] = "2026-12-27T12:00:00Z"
	h, err := a.CheckHealth(context.Background(), i)
	if err != nil || !h.OK || h.TokenExpiresAt == nil || !h.TokenExpiresAt.Equal(time.Date(2026, 12, 27, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("health: %+v %v", h, err)
	}
	i.Credentials[FieldAccessToken] = "revoked"
	if h, err := a.CheckHealth(context.Background(), i); err != nil || h.OK {
		t.Fatalf("revoked: %+v %v", h, err)
	}
	if _, err := a.Connect(context.Background(), i); !metagraph.IsTokenError(err) {
		t.Fatalf("connect token error: %v", err)
	}
	if exp, ok := ExpiresAt(channels.Credentials{FieldExpiresAt: "1798372800"}); !ok || exp.Unix() != 1798372800 {
		t.Fatal("unix expiry")
	}
}

func TestSendDM(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"POST /v21.0/me/messages": reply(`{"recipient_id":"` + igsid + `","message_id":"aWdfSent01"}`),
	})
	a := adapter(f)
	res, err := a.Send(context.Background(), inst(), channels.OutboundMessage{
		To: channels.Recipient{ExternalID: igsid}, Text: "Stokta var",
		Attachments: []channels.OutboundAttachment{{ContentType: "video/mp4", URL: "https://api.example/m/9"}},
	})
	if err != nil || res.ExternalID != "aWdfSent01" || len(f.calls) != 2 {
		t.Fatalf("dm: %+v %v", res, err)
	}
	if b := f.calls[0].Body; b["recipient"].(map[string]any)["id"] != igsid || b["message"].(map[string]any)["text"] != "Stokta var" || b["tag"] != nil {
		t.Fatalf("text: %+v", b)
	}
	if att := f.calls[1].Body["message"].(map[string]any)["attachment"].(map[string]any); att["type"] != "video" {
		t.Fatalf("media: %+v", att)
	}
	if _, err := a.Send(context.Background(), inst(), channels.OutboundMessage{To: channels.Recipient{ExternalID: igsid}, Text: "x", Attributes: map[string]any{"human_agent": "true"}}); err != nil {
		t.Fatal(err)
	}
	if b := f.last().Body; b["messaging_type"] != "MESSAGE_TAG" || b["tag"] != "HUMAN_AGENT" {
		t.Fatalf("human agent: %+v", b)
	}
}

func TestSendComments(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"POST /v21.0/17865799348089039/replies": reply(`{"id":"17870000000000001"}`),
		"POST /v21.0/me/messages":               reply(`{"recipient_id":"232323232","message_id":"aWdfPrivate01"}`),
		"POST /v21.0/" + igUser + "/mentions":   reply(`{"id":"17870000000000002"}`),
	})
	a := adapter(f)
	ctx := context.Background()
	to := channels.Recipient{ExternalID: "232323232"}

	res, err := a.Send(ctx, inst(), channels.OutboundMessage{To: to, ThreadID: "comment:17865799348089039", Text: "DM'den yazdık"})
	if err != nil || res.ExternalID != "comment:17870000000000001" || f.last().Form.Get("message") != "DM'den yazdık" {
		t.Fatalf("public reply: %+v %v %+v", res, err, f.last())
	}

	res, err = a.Send(ctx, inst(), channels.OutboundMessage{To: to, ThreadID: "comment:17865799348089039", Text: "Fiyat 100 TL",
		Attributes: map[string]any{"reply_mode": "dm", "comment_id": "17865799348089040"}})
	if err != nil || res.ExternalID != "aWdfPrivate01" {
		t.Fatalf("private reply: %+v %v", res, err)
	}
	if r := f.last().Body["recipient"].(map[string]any); r["comment_id"] != "17865799348089040" || r["id"] != nil {
		t.Fatalf("private recipient: %+v", r)
	}

	res, err = a.Send(ctx, inst(), channels.OutboundMessage{To: to, ThreadID: "mention:17920238422030506:17895695668004550", Text: "Teşekkürler"})
	if c := f.last(); err != nil || res.ExternalID != "comment:17870000000000002" || c.Form.Get("media_id") != "17920238422030506" || c.Form.Get("comment_id") != "17895695668004550" {
		t.Fatalf("mention reply: %+v %v %+v", res, err, c)
	}

	_, err = a.Send(ctx, inst(), channels.OutboundMessage{To: to, ThreadID: "comment:17865799348089039", Attachments: []channels.OutboundAttachment{{URL: "https://x"}}})
	if !channels.IsPermanent(err) {
		t.Fatalf("media on comment: %v", err)
	}
}

func TestSendErrors(t *testing.T) {
	for _, c := range []struct {
		status    int
		body      string
		permanent bool
		window    bool
	}{
		{400, `{"error":{"message":"The message is sent outside of allowed window","code":10,"error_subcode":2534022}}`, true, true},
		{500, `{"error":{"message":"Please retry","code":2}}`, false, false},
		{400, `{"error":{"message":"Invalid OAuth access token","code":190}}`, true, false},
	} {
		f := newGraph(t, map[string]func(http.ResponseWriter, call){"POST /v21.0/me/messages": fail(c.status, c.body)})
		_, err := adapter(f).Send(context.Background(), inst(), channels.OutboundMessage{To: channels.Recipient{ExternalID: igsid}, Text: "x"})
		if err == nil || channels.IsPermanent(err) != c.permanent || errors.Is(err, channels.ErrWindowClosed) != c.window {
			t.Fatalf("%s: %v", c.body, err)
		}
	}
}

func TestSendTyping(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){"POST /v21.0/me/messages": reply(`{"recipient_id":"1"}`)})
	if err := adapter(f).SendTyping(context.Background(), inst(), channels.Recipient{ExternalID: igsid}, false); err != nil {
		t.Fatal(err)
	}
	if f.last().Body["sender_action"] != "typing_off" {
		t.Fatalf("typing: %+v", f.last().Body)
	}
}

func TestReconcile(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"GET /v21.0/me/conversations": reply(`{"data":[{"id":"c1","updated_time":"2026-10-28T10:40:00+0000","messages":{"data":[
			{"id":"aWdfOwn","created_time":"2026-10-28T10:40:00+0000","from":{"id":"` + igUser + `","username":"clickfix.shop"},"message":"Cevap"},
			{"id":"aWdfOwnAlt","created_time":"2026-10-28T10:35:00+0000","from":{"id":"9988776655","username":"clickfix.shop"},"message":"Cevap 2"},
			{"id":"aWdfMissed","created_time":"2026-10-28T10:30:00+0000","from":{"id":"` + igsid + `","username":"ayse.k"},"message":"Merhaba?"},
			{"id":"aWdfOld","created_time":"2026-10-27T10:30:00+0000","from":{"id":"` + igsid + `"},"message":"Dün"}]}}]}`),
	})
	res, err := adapter(f).Reconcile(context.Background(), inst(), time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ExternalID != "aWdfMissed" || res.Messages[0].From.Username != "ayse.k" {
		t.Fatalf("reconcile: %+v", res.Messages)
	}
	if f.last().Query.Get("platform") != "instagram" {
		t.Fatalf("query: %v", f.last().Query)
	}
}

func TestOAuth(t *testing.T) {
	f := newGraph(t, map[string]func(http.ResponseWriter, call){
		"POST /oauth/access_token": func(w http.ResponseWriter, c call) {
			if c.Form.Get("code") != "abc" || c.Form.Get("grant_type") != "authorization_code" || c.Form.Get("client_secret") != "igsecret" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error_type":"OAuthException","code":400,"error_message":"Invalid authorization code"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"short","user_id":` + igUser + `,"permissions":["instagram_business_basic"]}`))
		},
		"GET /access_token": func(w http.ResponseWriter, c call) {
			if c.Query.Get("grant_type") != "ig_exchange_token" || c.Query.Get("access_token") != "short" {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"long","token_type":"bearer","expires_in":5184000}`))
		},
		"GET /refresh_access_token":      reply(`{"access_token":"long2","token_type":"bearer","expires_in":5184000}`),
		"POST /v21.0/me/subscribed_apps": reply(`{"success":true}`),
	})
	a := adapter(f)
	login, _ := url.Parse(a.LoginURL("s1"))
	if login.Host != "www.instagram.com" || login.Query().Get("scope") != strings.Join(Scopes, ",") || login.Query().Get("state") != "s1" {
		t.Fatalf("login: %s", login)
	}
	tok, user, exp, err := a.ExchangeCode(context.Background(), "abc#_")
	want := time.Date(2026, 12, 27, 12, 0, 0, 0, time.UTC)
	if err != nil || tok != "long" || user != igUser || !exp.Equal(want) {
		t.Fatalf("exchange: %q %q %v %v", tok, user, exp, err)
	}
	if _, _, _, err := a.ExchangeCode(context.Background(), "bad"); err == nil {
		t.Fatal("bad code accepted")
	}
	tok, exp, err = a.RefreshToken(context.Background(), "long")
	if err != nil || tok != "long2" || !exp.Equal(want) || f.last().Query.Get("grant_type") != "ig_refresh_token" {
		t.Fatalf("refresh: %q %v %v", tok, exp, err)
	}
	if err := a.SubscribeApp(context.Background(), "long"); err != nil {
		t.Fatal(err)
	}
	if c := f.last(); c.Auth != "Bearer long" || c.Query.Get("subscribed_fields") != SubscribedFields {
		t.Fatalf("subscribe: %+v", c)
	}
}

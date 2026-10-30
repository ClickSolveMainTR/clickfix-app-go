package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

const testToken = "123456789:AAH-test_TOKEN_abcdefghijklmnopqrstu"

type apiCall struct {
	method string
	params map[string]any
}

// fakeAPI is a minimal Bot API server.
type fakeAPI struct {
	mu       sync.Mutex
	calls    []apiCall
	nextID   int64
	handlers map[string]func(p map[string]any) (int, string)
	srv      *httptest.Server
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{nextID: 1000, handlers: map[string]func(map[string]any) (int, string){}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func ok(v any) (int, string) {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": v})
	return 200, string(b)
}

func fail(code int, desc string, params map[string]any) (int, string) {
	m := map[string]any{"ok": false, "error_code": code, "description": desc}
	if params != nil {
		m["parameters"] = params
	}
	b, _ := json.Marshal(m)
	return code, string(b)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	if p, found := strings.CutPrefix(r.URL.Path, "/file/bot"+testToken+"/"); found {
		_, _ = io.WriteString(w, "file-bytes:"+p)
		return
	}
	method, found := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
	if !found {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
		return
	}
	params := map[string]any{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(1 << 20)
		for k, v := range r.MultipartForm.Value {
			params[k] = v[0]
		}
		for k, fh := range r.MultipartForm.File {
			file, _ := fh[0].Open()
			b, _ := io.ReadAll(file)
			params[k] = "upload:" + fh[0].Filename + ":" + string(b)
		}
	} else {
		_ = json.NewDecoder(r.Body).Decode(&params)
	}
	f.mu.Lock()
	f.calls = append(f.calls, apiCall{method, params})
	h := f.handlers[method]
	f.nextID++
	id := f.nextID
	f.mu.Unlock()
	var code int
	var body string
	switch {
	case h != nil:
		code, body = h(params)
	case method == "getMe":
		code, body = ok(map[string]any{"id": 123456789, "is_bot": true, "first_name": "ClickFix Destek", "username": "clickfix_test_bot"})
	case method == "getFile":
		code, body = ok(map[string]any{"file_id": params["file_id"], "file_size": 98234, "file_path": "photos/file_7.jpg"})
	case strings.HasPrefix(method, "send") && method != "sendChatAction":
		chatID := params["chat_id"]
		if s, isStr := chatID.(string); isStr {
			chatID = json.Number(s)
		}
		code, body = ok(map[string]any{"message_id": id, "chat": map[string]any{"id": chatID, "type": "private"}, "date": 1792829400})
	default:
		code, body = ok(true)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

func (f *fakeAPI) Calls(method string) []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.calls {
		if method == "" || c.method == method {
			out = append(out, c)
		}
	}
	return out
}

func testInstance() channels.Instance {
	return channels.Instance{
		ID: 7, UUID: uuid.MustParse("6f1c2a52-8a0e-4d0b-9a57-3c4f0a1b2c3d"), Type: channels.TypeTelegram,
		ExternalID: "123456789", Credentials: channels.Credentials{TokenField: testToken},
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parse(t *testing.T, a *Adapter, name string) channels.WebhookResult {
	t.Helper()
	res, err := a.ParseWebhook(context.Background(), nil, fixture(t, name), testInstance())
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func one(t *testing.T, res channels.WebhookResult) channels.InboundMessage {
	t.Helper()
	if len(res.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(res.Messages))
	}
	return res.Messages[0]
}

func TestParsePrivateText(t *testing.T) {
	a := New(Config{BaseURL: "http://127.0.0.1:1"}) // private chats need no API call
	m := one(t, parse(t, a, "private_text.json"))
	if m.ExternalID != "5012345678:42" || m.Kind != "text" || m.Text != "Merhaba, siparişim nerede?" {
		t.Fatalf("message: %+v", m)
	}
	if m.From.ExternalID != "5012345678" || !m.From.InstanceScoped || m.From.Name != "Ayşe Yılmaz" || m.From.Username != "ayseyilmaz" {
		t.Fatalf("sender: %+v", m.From)
	}
	if !m.At.Equal(time.Unix(1792828800, 0)) {
		t.Fatalf("at: %v", m.At)
	}
	if s := one(t, parse(t, a, "private_start.json")); s.Text != "/start" {
		t.Fatalf("start: %+v", s)
	}
	if res := parse(t, a, "edited_message.json"); len(res.Messages) != 0 {
		t.Fatalf("edited messages are ignored: %+v", res)
	}
}

func TestParsePhotoAndOpen(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	m := one(t, parse(t, a, "private_photo_caption.json"))
	if m.Kind != "media" || m.Text != "Kutu hasarlı geldi" || len(m.Attachments) != 1 {
		t.Fatalf("message: %+v", m)
	}
	att := m.Attachments[0]
	if att.ContentType != "image/jpeg" || att.Size != 98234 || att.Filename != "photo_AQADlarge.jpg" {
		t.Fatalf("attachment: %+v", att)
	}
	rc, err := att.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "file-bytes:photos/file_7.jpg" {
		t.Fatalf("download: %q", b)
	}
	if c := f.Calls("getFile"); len(c) != 1 || c[0].params["file_id"] != "AgACAgIAAxkBAAMrZ_large" {
		t.Fatalf("getFile: %+v", c)
	}
}

func TestOpenTooLarge(t *testing.T) {
	f := newFakeAPI(t)
	f.handlers["getFile"] = func(map[string]any) (int, string) {
		return ok(map[string]any{"file_id": "x", "file_size": MaxDownloadSize + 1, "file_path": "big"})
	}
	a := New(Config{BaseURL: f.srv.URL})
	if _, err := a.opener(testToken, "x", 0)(context.Background()); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
	if _, err := a.opener(testToken, "x", MaxDownloadSize+1)(context.Background()); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestParseDocumentReplyContact(t *testing.T) {
	a := New(Config{BaseURL: "http://127.0.0.1:1"})
	d := one(t, parse(t, a, "private_document.json"))
	if d.Kind != "media" || d.Text != "" || len(d.Attachments) != 1 ||
		d.Attachments[0].Filename != "fatura-2026-10.pdf" || d.Attachments[0].ContentType != "application/pdf" || d.Attachments[0].Size != 184320 {
		t.Fatalf("document: %+v", d)
	}
	r := one(t, parse(t, a, "private_reply.json"))
	if r.ReplyToExternalID != "5012345678:45" || r.Text != "CF-10293" {
		t.Fatalf("reply: %+v", r)
	}
	c := one(t, parse(t, a, "private_contact.json"))
	if c.Kind != "text" || c.Text != "Contact: Ayşe Yılmaz +905551112233" ||
		c.Attributes["contact_phone"] != "+905551112233" || c.Attributes["contact_user_id"] != "5012345678" {
		t.Fatalf("contact: %+v", c)
	}
}

func TestParseGroups(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})

	m := one(t, parse(t, a, "group_mention.json"))
	if m.ExternalID != "-1001987654321:310" || m.Text != "Merhaba 👋 @clickfix_test_bot kargo hâlâ gelmedi" {
		t.Fatalf("mention: %+v", m)
	}
	if m.From.ExternalID != "-1001987654321" || m.From.Name != "Acme Lojistik Destek" || m.From.Meta["chat_type"] != "supergroup" {
		t.Fatalf("group sender: %+v", m.From)
	}
	want := map[string]any{"author_id": "6023456789", "author_name": "Mehmet Kaya", "author_username": "mkaya", "chat_type": "supergroup"}
	for k, v := range want {
		if m.Attributes[k] != v {
			t.Fatalf("attr %s = %v, want %v", k, m.Attributes[k], v)
		}
	}

	if res := parse(t, a, "group_no_mention.json"); len(res.Messages) != 0 {
		t.Fatalf("unaddressed group message must be ignored: %+v", res)
	}

	c := one(t, parse(t, a, "group_command.json"))
	if c.Text != "/help@clickfix_test_bot" || c.From.ExternalID != "-4012345678" || c.Attributes["chat_type"] != "group" {
		t.Fatalf("command: %+v", c)
	}
	if n := len(f.Calls("getMe")); n != 1 {
		t.Fatalf("getMe should be cached, called %d times", n)
	}
}

func TestParseGroupSettingsFallback(t *testing.T) {
	a := New(Config{BaseURL: "http://127.0.0.1:1"}) // getMe fails
	inst := testInstance()
	inst.Settings = map[string]any{SettingBotUsername: "@clickfix_test_bot"}
	res, err := a.ParseWebhook(context.Background(), nil, fixture(t, "group_mention.json"), inst)
	if err != nil || len(res.Messages) != 1 {
		t.Fatalf("settings fallback: %v %+v", err, res)
	}
}

func TestParseCallbackQuery(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	m := one(t, parse(t, a, "callback_query.json"))
	if m.ExternalID != "cb:4382910283746529183" || m.Kind != "interactive" || m.Text != "Evet, teşekkürler" ||
		m.Attributes["callback_data"] != "resolved_yes" || m.ReplyToExternalID != "5012345678:48" || m.From.ExternalID != "5012345678" {
		t.Fatalf("callback: %+v", m)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(f.Calls("answerCallbackQuery")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("answerCallbackQuery not called")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c := f.Calls("answerCallbackQuery")[0]; c.params["callback_query_id"] != "4382910283746529183" {
		t.Fatalf("answer: %+v", c)
	}
}

func TestConnect(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL, PublicBaseURL: "https://api.clickfix.app/"})
	inst := testInstance()
	id, err := a.Connect(context.Background(), inst)
	if err != nil || id != "123456789" {
		t.Fatalf("connect: %q %v", id, err)
	}
	sw := f.Calls("setWebhook")
	if len(sw) != 1 {
		t.Fatalf("setWebhook calls: %d", len(sw))
	}
	p := sw[0].params
	if p["url"] != "https://api.clickfix.app/v1/webhooks/channels/telegram/6f1c2a52-8a0e-4d0b-9a57-3c4f0a1b2c3d" {
		t.Fatalf("url: %v", p["url"])
	}
	secret := SecretToken(inst)
	if p["secret_token"] != secret || len(secret) != 48 || p["drop_pending_updates"] != false {
		t.Fatalf("params: %+v", p)
	}
	if fmt.Sprint(p["allowed_updates"]) != "[message edited_message callback_query my_chat_member]" {
		t.Fatalf("allowed_updates: %v", p["allowed_updates"])
	}

	// No public URL: validate only.
	f2 := newFakeAPI(t)
	if _, err := New(Config{BaseURL: f2.srv.URL}).Connect(context.Background(), inst); err != nil || len(f2.Calls("setWebhook")) != 0 {
		t.Fatalf("dev connect: %v %+v", err, f2.Calls(""))
	}

	// Wrong token.
	bad := testInstance()
	bad.Credentials[TokenField] = "999:wrong"
	if _, err := a.Connect(context.Background(), bad); err == nil || strings.Contains(err.Error(), "999:wrong") {
		t.Fatalf("bad token: %v", err)
	}

	if err := a.Disconnect(context.Background(), inst); err != nil || len(f.Calls("deleteWebhook")) != 1 {
		t.Fatalf("disconnect: %v", err)
	}
	b, err := a.BotInfo(context.Background(), inst)
	if err != nil || b != (Bot{ID: 123456789, Username: "clickfix_test_bot", Name: "ClickFix Destek"}) {
		t.Fatalf("bot info: %+v %v", b, err)
	}
}

func TestVerifyWebhook(t *testing.T) {
	a := New(Config{})
	inst := testInstance()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(SecretHeader, SecretToken(inst))
	if err := a.VerifyWebhook(r, nil, inst); err != nil {
		t.Fatalf("valid: %v", err)
	}
	r.Header.Set(SecretHeader, strings.Repeat("0", 48))
	if err := a.VerifyWebhook(r, nil, inst); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("bad secret: %v", err)
	}
	r.Header.Del(SecretHeader)
	if err := a.VerifyWebhook(r, nil, inst); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("missing: %v", err)
	}
	other := testInstance()
	other.UUID = uuid.New()
	if SecretToken(other) == SecretToken(inst) {
		t.Fatal("secret must differ per instance")
	}
}

func sendTo(chat string) channels.OutboundMessage {
	return channels.OutboundMessage{To: channels.Recipient{ExternalID: chat}}
}

func TestSendText(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	msg := sendTo("5012345678")
	msg.Text = "Kargonuz yolda."
	msg.ReplyToExternalID = "5012345678:46"
	msg.Attributes = map[string]any{"buttons": []any{
		map[string]any{"id": "resolved_yes", "title": "Evet"},
		map[string]any{"title": "Takip", "url": "https://kargo.example/track/1"},
	}}
	res, err := a.Send(context.Background(), testInstance(), msg)
	if err != nil {
		t.Fatal(err)
	}
	calls := f.Calls("sendMessage")
	if len(calls) != 1 || res.ExternalID != fmt.Sprintf("5012345678:%d", 1001) {
		t.Fatalf("result %q calls %+v", res.ExternalID, calls)
	}
	p := calls[0].params
	if p["chat_id"] != float64(5012345678) || p["text"] != "Kargonuz yolda." || p["parse_mode"] != nil {
		t.Fatalf("params: %+v", p)
	}
	if rp, _ := p["reply_parameters"].(map[string]any); rp["message_id"] != float64(46) || rp["allow_sending_without_reply"] != true {
		t.Fatalf("reply: %+v", p["reply_parameters"])
	}
	kb, _ := json.Marshal(p["reply_markup"])
	if string(kb) != `{"inline_keyboard":[[{"callback_data":"resolved_yes","text":"Evet"}],[{"text":"Takip","url":"https://kargo.example/track/1"}]]}` {
		t.Fatalf("keyboard: %s", kb)
	}
}

func TestSendLongSplit(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	msg := sendTo("5012345678")
	para := strings.Repeat("çğ🙂 ", 700) // 700*5 = 3500 UTF-16 units
	msg.Text = para + "\n" + para + "\n" + para
	msg.ReplyToExternalID = "5012345678:46"
	res, err := a.Send(context.Background(), testInstance(), msg)
	if err != nil {
		t.Fatal(err)
	}
	calls := f.Calls("sendMessage")
	if len(calls) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(calls))
	}
	joined := ""
	for i, c := range calls {
		s := c.params["text"].(string)
		if utf16Len(s) > maxText {
			t.Fatalf("chunk %d too long: %d", i, utf16Len(s))
		}
		if (c.params["reply_parameters"] != nil) != (i == 0) {
			t.Fatalf("reply only on first chunk")
		}
		joined += s
	}
	if joined != msg.Text {
		t.Fatal("chunks must reassemble the text")
	}
	if !strings.HasSuffix(res.ExternalID, fmt.Sprint(1003)) {
		t.Fatalf("last id: %s", res.ExternalID)
	}
}

func TestSendPhotoAndUpload(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	msg := sendTo("-1001987654321")
	msg.Text = "Fatura ekte"
	msg.Attachments = []channels.OutboundAttachment{
		{Filename: "kutu.jpg", ContentType: "image/jpeg", Size: 2048, URL: "https://api.clickfix.app/v1/media/signed/abc"},
		{Filename: "fatura.pdf", ContentType: "application/pdf", Size: 7, Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("%PDF-1.")), nil
		}},
	}
	if _, err := a.Send(context.Background(), testInstance(), msg); err != nil {
		t.Fatal(err)
	}
	if n := len(f.Calls("sendMessage")); n != 0 {
		t.Fatalf("text should be the caption, got %d sendMessage", n)
	}
	ph := f.Calls("sendPhoto")
	if len(ph) != 1 || ph[0].params["photo"] != "https://api.clickfix.app/v1/media/signed/abc" || ph[0].params["caption"] != "Fatura ekte" ||
		ph[0].params["chat_id"] != float64(-1001987654321) {
		t.Fatalf("sendPhoto: %+v", ph)
	}
	doc := f.Calls("sendDocument")
	if len(doc) != 1 || doc[0].params["document"] != "upload:fatura.pdf:%PDF-1." || doc[0].params["chat_id"] != "-1001987654321" || doc[0].params["caption"] != nil {
		t.Fatalf("sendDocument: %+v", doc)
	}
}

func TestSendLongCaption(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	msg := sendTo("5012345678")
	msg.Text = strings.Repeat("a", maxCaption+1)
	msg.Attachments = []channels.OutboundAttachment{{ContentType: "video/mp4", URL: "https://x/v.mp4"}}
	if _, err := a.Send(context.Background(), testInstance(), msg); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls("sendMessage")) != 1 || len(f.Calls("sendVideo")) != 1 || f.Calls("sendVideo")[0].params["caption"] != nil {
		t.Fatalf("calls: %+v", f.Calls(""))
	}
}

func TestSendErrors(t *testing.T) {
	cases := []struct {
		name      string
		code      int
		desc      string
		params    map[string]any
		permanent bool
		errCode   string
	}{
		{"blocked", 403, "Forbidden: bot was blocked by the user", nil, true, "bot_blocked"},
		{"chat not found", 400, "Bad Request: chat not found", nil, true, "chat_not_found"},
		{"rate limited", 429, "Too Many Requests: retry after 17", map[string]any{"retry_after": 17}, false, "rate_limited"},
		{"server", 502, "Bad Gateway", nil, false, "unavailable"},
		{"unauthorized", 401, "Unauthorized", nil, true, "token_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			f.handlers["sendMessage"] = func(map[string]any) (int, string) { return fail(tc.code, tc.desc, tc.params) }
			msg := sendTo("5012345678")
			msg.Text = "x"
			_, err := New(Config{BaseURL: f.srv.URL}).Send(context.Background(), testInstance(), msg)
			var se *channels.SendError
			if !errors.As(err, &se) || se.Code != tc.errCode || channels.IsPermanent(err) != tc.permanent {
				t.Fatalf("err: %v", err)
			}
			if tc.code == 429 {
				var ae *APIError
				if !errors.As(err, &ae) || ae.RetryAfter != 17 || !strings.Contains(err.Error(), "retry after 17s") {
					t.Fatalf("retry_after: %v", err)
				}
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatal("token leaked into error")
			}
		})
	}

	// Network failure: temporary, token not leaked.
	msg := sendTo("1")
	msg.Text = "x"
	_, err := New(Config{BaseURL: "http://127.0.0.1:1"}).Send(context.Background(), testInstance(), msg)
	if err == nil || channels.IsPermanent(err) || strings.Contains(err.Error(), testToken) {
		t.Fatalf("network: %v", err)
	}
}

func TestSendTyping(t *testing.T) {
	f := newFakeAPI(t)
	a := New(Config{BaseURL: f.srv.URL})
	if err := a.SendTyping(context.Background(), testInstance(), channels.Recipient{ExternalID: "5012345678"}, true); err != nil {
		t.Fatal(err)
	}
	_ = a.SendTyping(context.Background(), testInstance(), channels.Recipient{ExternalID: "5012345678"}, false)
	c := f.Calls("sendChatAction")
	if len(c) != 1 || c[0].params["action"] != "typing" {
		t.Fatalf("calls: %+v", c)
	}
}

func TestCheckHealth(t *testing.T) {
	inst := testInstance()
	now := time.Unix(1792830000, 0)
	cases := []struct {
		name   string
		info   map[string]any
		ok     bool
		detail string
	}{
		{"healthy", map[string]any{"url": "https://api.clickfix.app/v1/webhooks/channels/telegram/" + inst.UUID.String(), "last_error_date": now.Add(-2 * time.Hour).Unix(), "last_error_message": "old"}, true, ""},
		{"recent error", map[string]any{"url": "https://api.clickfix.app/v1/webhooks/channels/telegram/" + inst.UUID.String(), "last_error_date": now.Add(-5 * time.Minute).Unix(), "last_error_message": "Wrong response from the webhook: 502 Bad Gateway"}, false, "webhook: Wrong response from the webhook: 502 Bad Gateway"},
		{"mismatch", map[string]any{"url": "https://elsewhere.example/hook"}, false, "webhook points to https://elsewhere.example/hook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			f.handlers["getWebhookInfo"] = func(map[string]any) (int, string) { return ok(tc.info) }
			a := New(Config{BaseURL: f.srv.URL, PublicBaseURL: "https://api.clickfix.app"})
			a.now = func() time.Time { return now }
			h, err := a.CheckHealth(context.Background(), inst)
			if err != nil || h.OK != tc.ok || h.Detail != tc.detail || h.TokenExpiresAt != nil {
				t.Fatalf("health: %+v %v", h, err)
			}
		})
	}
	f := newFakeAPI(t)
	bad := testInstance()
	bad.Credentials[TokenField] = "1:revoked"
	h, err := New(Config{BaseURL: f.srv.URL}).CheckHealth(context.Background(), bad)
	if err != nil || h.OK || h.Detail != "Unauthorized" {
		t.Fatalf("revoked: %+v %v", h, err)
	}
}

func TestSplitTextAndTruncate(t *testing.T) {
	if got := splitText("  ", 10); got != nil {
		t.Fatalf("blank: %q", got)
	}
	got := splitText("aaaa bbbb cccc", 10)
	if len(got) != 2 || got[0] != "aaaa bbbb " || got[1] != "cccc" {
		t.Fatalf("split: %q", got)
	}
	if s := truncateBytes("ğğğ", 5); s != "ğğ" {
		t.Fatalf("truncate: %q", s)
	}
}

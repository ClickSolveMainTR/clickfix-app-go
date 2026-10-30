package whatsapp_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/whatsapp"
)

const (
	phoneID = "106540352242922"
	wabaID  = "102290129340398"
	token   = "EAAG-business-token"
)

func instance() channels.Instance {
	return channels.Instance{ID: 1, Type: channels.TypeWhatsApp, ExternalID: phoneID, Credentials: channels.Credentials{
		whatsapp.FieldPhoneNumberID: phoneID, whatsapp.FieldWABAID: wabaID, whatsapp.FieldAccessToken: token,
	}}
}

func cfg(base string) whatsapp.Config {
	return whatsapp.Config{AppID: "1234567", AppSecret: "app-secret", VerifyToken: "verify-1", ConfigID: "cfg-9", BaseURL: base}
}

// graph is a fake Graph API: handlers keyed by "METHOD /path".
type graph struct {
	t        *testing.T
	srv      *httptest.Server
	handlers map[string]http.HandlerFunc
	bodies   []map[string]any
}

func newGraph(t *testing.T) *graph {
	g := &graph{t: t, handlers: map[string]http.HandlerFunc{}}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			g.bodies = append(g.bodies, m)
		}
		h, ok := g.handlers[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h(w, r)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *graph) adapter() *whatsapp.Adapter { return whatsapp.New(cfg(g.srv.URL)) }

func (g *graph) on(key string, h http.HandlerFunc) { g.handlers[key] = h }

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConnect(t *testing.T) {
	g := newGraph(t)
	g.on("GET /v21.0/"+phoneID, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || !strings.Contains(r.URL.Query().Get("fields"), "quality_rating") {
			t.Errorf("request %s %v", r.Header.Get("Authorization"), r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"display_phone_number":"+1 555-078-3881","verified_name":"ClickFix","quality_rating":"GREEN","id":"` + phoneID + `"}`))
	})
	id, err := g.adapter().Connect(t.Context(), instance())
	if err != nil || id != phoneID {
		t.Fatalf("connect = %q %v", id, err)
	}

	g.on("GET /v21.0/"+phoneID, reply(400, `{"error":{"message":"Invalid OAuth access token.","code":190}}`))
	if _, err := g.adapter().Connect(t.Context(), instance()); err == nil || !strings.Contains(err.Error(), "Invalid OAuth") {
		t.Fatalf("connect err = %v", err)
	}
	bad := instance()
	bad.Credentials[whatsapp.FieldWABAID] = ""
	if _, err := g.adapter().Connect(t.Context(), bad); err == nil {
		t.Fatal("missing waba_id accepted")
	}
}

func TestSignatureAndChallenge(t *testing.T) {
	a := whatsapp.New(cfg("http://unused"))
	body := fixture(t, "text_message.json")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(whatsapp.SignatureHeader, whatsapp.Sign("app-secret", body))
	if err := a.VerifyWebhook(r, body, instance()); err != nil {
		t.Fatalf("verify = %v", err)
	}
	r.Header.Set(whatsapp.SignatureHeader, whatsapp.Sign("other", body))
	if err := a.VerifyWebhook(r, body, instance()); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("bad signature = %v", err)
	}
	r.Header.Set(whatsapp.SignatureHeader, whatsapp.Sign("", body))
	if err := whatsapp.New(whatsapp.Config{}).VerifyWebhook(r, body, instance()); !errors.Is(err, channels.ErrSignature) {
		t.Fatalf("empty secret = %v", err)
	}

	ok := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=verify-1&hub.challenge=4242", nil)
	if got, accepted := a.ChallengeShared(ok); !accepted || got != "4242" {
		t.Fatalf("challenge = %q %v", got, accepted)
	}
	if got, accepted := a.Challenge(ok, instance()); !accepted || got != "4242" {
		t.Fatalf("instance challenge = %q %v", got, accepted)
	}
	for _, q := range []string{
		"hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=1",
		"hub.mode=unsubscribe&hub.verify_token=verify-1&hub.challenge=1",
		"hub.mode=subscribe&hub.verify_token=verify-1",
	} {
		if _, accepted := a.ChallengeShared(httptest.NewRequest(http.MethodGet, "/?"+q, nil)); accepted {
			t.Fatalf("challenge accepted %s", q)
		}
	}
}

func TestRouteAndFilter(t *testing.T) {
	a := whatsapp.New(cfg("http://unused"))
	body := fixture(t, "multi_entry.json")
	ids, err := a.Route(nil, body)
	if err != nil || !reflect.DeepEqual(ids, []string{phoneID, "207540352242999"}) {
		t.Fatalf("route = %v %v", ids, err)
	}
	if _, err := a.Route(nil, []byte(`{"object":"page"}`)); !errors.Is(err, whatsapp.ErrMalformed) {
		t.Fatalf("route malformed = %v", err)
	}
	res, err := a.ParseWebhook(t.Context(), nil, body, instance())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ExternalID != "wamid.A1" || len(res.Statuses) != 1 || res.Statuses[0].Status != "read" {
		t.Fatalf("parse = %+v", res)
	}
}

func TestParseText(t *testing.T) {
	res, err := whatsapp.New(cfg("http://unused")).ParseWebhook(t.Context(), nil, fixture(t, "text_message.json"), instance())
	if err != nil || len(res.Messages) != 1 {
		t.Fatalf("parse = %+v %v", res, err)
	}
	m := res.Messages[0]
	if m.Kind != "text" || m.Text != "Merhaba, siparişim nerede?" || m.ReplyToExternalID != "wamid.OUTBOUND1" ||
		!m.At.Equal(time.Unix(1761645600, 0)) {
		t.Fatalf("message = %+v", m)
	}
	if m.From.ExternalID != "905321112233" || m.From.Phone != "+905321112233" || m.From.Name != "Ayşe Yılmaz" || m.From.InstanceScoped {
		t.Fatalf("sender = %+v", m.From)
	}
}

func TestParseInteractive(t *testing.T) {
	res, err := whatsapp.New(cfg("http://unused")).ParseWebhook(t.Context(), nil, fixture(t, "interactive_button_reply.json"), instance())
	if err != nil || len(res.Messages) != 1 {
		t.Fatalf("parse = %+v %v", res, err)
	}
	m := res.Messages[0]
	if m.Kind != "interactive" || m.Text != "Evet" || m.Attributes["reply_id"] != "confirm_yes" {
		t.Fatalf("message = %+v", m)
	}
}

func TestParseStatuses(t *testing.T) {
	a := whatsapp.New(cfg("http://unused"))
	res, err := a.ParseWebhook(t.Context(), nil, fixture(t, "status_delivered.json"), instance())
	if err != nil || len(res.Statuses) != 1 {
		t.Fatalf("parse = %+v %v", res, err)
	}
	s := res.Statuses[0]
	want := map[string]any{
		"billable": true, "pricing_category": "utility", "pricing_model": "PMP",
		"conversation_id": "4d2f6b8e9c0a1b2c3d4e5f60718293a4", "conversation_origin": "utility", "recipient_id": "905321112233",
	}
	if s.Status != channels.StatusDelivered || s.ExternalID != "wamid.OUTBOUND1" || !reflect.DeepEqual(s.Attributes, want) {
		t.Fatalf("status = %+v", s)
	}
	res, err = a.ParseWebhook(t.Context(), nil, fixture(t, "status_failed.json"), instance())
	if err != nil || len(res.Statuses) != 1 {
		t.Fatalf("parse = %+v %v", res, err)
	}
	if s := res.Statuses[0]; s.Status != channels.StatusFailed || s.Error != "131047 Re-engagement message" {
		t.Fatalf("failed = %+v", s)
	}
}

func TestParseMediaOpen(t *testing.T) {
	g := newGraph(t)
	g.on("GET /v21.0/1003383421387256", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"url":"` + g.srv.URL + `/media/abc","mime_type":"image/jpeg","file_size":5,"id":"1003383421387256"}`))
	})
	g.on("GET /media/abc", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("download auth = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("JPEG!"))
	})
	res, err := g.adapter().ParseWebhook(t.Context(), nil, fixture(t, "image_message.json"), instance())
	if err != nil || len(res.Messages) != 1 {
		t.Fatalf("parse = %+v %v", res, err)
	}
	m := res.Messages[0]
	if m.Kind != "media" || m.Text != "Kırık parça" || len(m.Attachments) != 1 || m.Attachments[0].ContentType != "image/jpeg" {
		t.Fatalf("message = %+v", m)
	}
	rc, err := m.Attachments[0].Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if raw, _ := io.ReadAll(rc); string(raw) != "JPEG!" {
		t.Fatalf("media = %q", raw)
	}

	g.on("GET /v21.0/1003383421387256", reply(200, `{"url":"`+g.srv.URL+`/media/abc","file_size":99999999}`))
	if _, err := m.Attachments[0].Open(t.Context()); err == nil {
		t.Fatal("oversized media accepted")
	}
}

func sendOK(g *graph) {
	g.on("POST /v21.0/"+phoneID+"/messages", reply(200, `{"messaging_product":"whatsapp","contacts":[{"input":"905321112233","wa_id":"905321112233"}],"messages":[{"id":"wamid.OUT1"}]}`))
}

func to() channels.Recipient { return channels.Recipient{ExternalID: "905321112233"} }

func TestSendText(t *testing.T) {
	g := newGraph(t)
	sendOK(g)
	res, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: channels.Recipient{Phone: "+90 532 111 22 33"}, Text: "Bakın: https://clickfix.app", ReplyToExternalID: "wamid.IN1",
	})
	if err != nil || res.ExternalID != "wamid.OUT1" {
		t.Fatalf("send = %+v %v", res, err)
	}
	b := g.bodies[0]
	if b["to"] != "905321112233" || b["type"] != "text" || b["text"].(map[string]any)["preview_url"] != true ||
		b["context"].(map[string]any)["message_id"] != "wamid.IN1" {
		t.Fatalf("body = %v", b)
	}
}

func TestSendTemplate(t *testing.T) {
	g := newGraph(t)
	sendOK(g)
	_, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: to(), Template: &channels.Template{Name: "siparis_durumu", Parameters: []string{"Ayşe", ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tpl := g.bodies[0]["template"].(map[string]any)
	if tpl["name"] != "siparis_durumu" || tpl["language"].(map[string]any)["code"] != "tr" {
		t.Fatalf("template = %v", tpl)
	}
	params := tpl["components"].([]any)[0].(map[string]any)["parameters"].([]any)
	if params[0].(map[string]any)["text"] != "Ayşe" || params[1].(map[string]any)["text"] != "-" {
		t.Fatalf("params = %v", params)
	}
}

func TestSendMedia(t *testing.T) {
	g := newGraph(t)
	sendOK(g)
	_, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: to(), Text: "Fatura ekte",
		Attachments: []channels.OutboundAttachment{
			{Filename: "fatura.pdf", ContentType: "application/pdf", URL: "https://api.clickfix.app/f/1"},
			{Filename: "foto.jpg", ContentType: "image/jpeg", URL: "https://api.clickfix.app/f/2"},
		},
	})
	if err != nil || len(g.bodies) != 2 {
		t.Fatalf("send = %v bodies %d", err, len(g.bodies))
	}
	doc := g.bodies[0]["document"].(map[string]any)
	if g.bodies[0]["type"] != "document" || doc["caption"] != "Fatura ekte" || doc["filename"] != "fatura.pdf" || doc["link"] != "https://api.clickfix.app/f/1" {
		t.Fatalf("first = %v", g.bodies[0])
	}
	img := g.bodies[1]["image"].(map[string]any)
	if g.bodies[1]["type"] != "image" || img["caption"] != nil {
		t.Fatalf("second = %v", g.bodies[1])
	}
}

func TestSendInteractive(t *testing.T) {
	g := newGraph(t)
	sendOK(g)
	_, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: to(), Text: "Sorun çözüldü mü?",
		Attributes: map[string]any{"buttons": []any{
			map[string]any{"id": "yes", "title": "Evet"}, map[string]any{"id": "no", "title": "Hayır"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := g.bodies[0]["interactive"].(map[string]any)
	btns := in["action"].(map[string]any)["buttons"].([]any)
	if in["type"] != "button" || len(btns) != 2 || btns[1].(map[string]any)["reply"].(map[string]any)["id"] != "no" {
		t.Fatalf("interactive = %v", in)
	}

	_, err = g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: to(), Text: "Konu seçin",
		Attributes: map[string]any{"list": map[string]any{"button": "Konular", "sections": []any{
			map[string]any{"title": "Destek", "rows": []any{map[string]any{"id": "r1", "title": "İade", "description": "İade talebi"}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in = g.bodies[1]["interactive"].(map[string]any)
	act := in["action"].(map[string]any)
	if in["type"] != "list" || act["button"] != "Konular" || len(act["sections"].([]any)) != 1 {
		t.Fatalf("list = %v", in)
	}

	_, err = g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{
		To: to(), Text: "x", Attributes: map[string]any{"buttons": []any{
			map[string]any{"title": "1"}, map[string]any{"title": "2"}, map[string]any{"title": "3"}, map[string]any{"title": "4"},
		}},
	})
	if !channels.IsPermanent(err) {
		t.Fatalf("4 buttons = %v", err)
	}
}

func TestSendErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		permanent bool
		window    bool
		code      string
	}{
		{"window", 400, `{"error":{"message":"Re-engagement message","code":131047}}`, true, true, "window_closed"},
		{"token", 401, `{"error":{"message":"Error validating access token","code":190}}`, true, false, "token_invalid"},
		{"template", 400, `{"error":{"message":"Template name does not exist in the translation","code":132001}}`, true, false, "132001"},
		{"undeliverable", 400, `{"error":{"message":"Message undeliverable","code":131026}}`, true, false, "131026"},
		{"rate", 429, `{"error":{"message":"Rate limit hit","code":130429}}`, false, false, ""},
		{"server", 500, `{"error":{"message":"Service unavailable","code":131000}}`, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newGraph(t)
			g.on("POST /v21.0/"+phoneID+"/messages", reply(c.status, c.body))
			_, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{To: to(), Text: "x"})
			if err == nil || channels.IsPermanent(err) != c.permanent || errors.Is(err, channels.ErrWindowClosed) != c.window {
				t.Fatalf("err = %v", err)
			}
			var se *channels.SendError
			if c.permanent && (!errors.As(err, &se) || se.Code != c.code) {
				t.Fatalf("code = %v", err)
			}
			var we *whatsapp.Error
			if !errors.As(err, &we) {
				t.Fatalf("graph error lost: %v", err)
			}
		})
	}

	g := newGraph(t)
	g.srv.Close()
	_, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{To: to(), Text: "x"})
	if err == nil || channels.IsPermanent(err) {
		t.Fatalf("network err = %v", err)
	}
}

func TestTemplates(t *testing.T) {
	g := newGraph(t)
	pages := 0
	g.on("GET /v21.0/"+wabaID+"/message_templates", func(w http.ResponseWriter, r *http.Request) {
		pages++
		q := r.URL.Query()
		if q.Get("limit") != "100" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		if q.Get("after") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"1","name":"a","language":"tr","status":"APPROVED","category":"UTILITY","components":[{"type":"BODY","text":"Merhaba"}]}],
				"paging":{"cursors":{"before":"B","after":"CUR1"},"next":"https://graph.facebook.com/next"}}`))
			return
		}
		if q.Get("after") != "CUR1" {
			t.Errorf("after = %q", q.Get("after"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2","name":"b","language":"en_US","status":"REJECTED","category":"MARKETING","rejected_reason":"INVALID_FORMAT"}],
			"paging":{"cursors":{"before":"CUR1","after":"CUR2"}}}`))
	})
	list, err := g.adapter().ListTemplates(t.Context(), instance())
	if err != nil || len(list) != 2 || pages != 2 || list[1].RejectedReason != "INVALID_FORMAT" || len(list[0].Components) != 1 {
		t.Fatalf("list = %+v %v pages %d", list, err, pages)
	}

	g.on("POST /v21.0/"+wabaID+"/message_templates", reply(200, `{"id":"5551","status":"PENDING","category":"UTILITY"}`))
	tpl, err := g.adapter().CreateTemplate(t.Context(), instance(), whatsapp.CreateTemplateInput{
		Name: "ticket_update", Category: "utility", Body: "Merhaba {{1}}, {{2}} numaralı talebiniz güncellendi.",
		Example: []string{"Ayşe", "CF-12"}, Footer: "ClickFix",
		Buttons: []whatsapp.TemplateButton{{Type: "QUICK_REPLY", Text: "Teşekkürler"}, {Type: "URL", Text: "Talebi aç", URL: "https://clickfix.app/t"}},
	})
	if err != nil || tpl.ID != "5551" || tpl.Status != "PENDING" || tpl.Language != "tr" {
		t.Fatalf("create = %+v %v", tpl, err)
	}
	sent := g.bodies[len(g.bodies)-1]
	comps := sent["components"].([]any)
	if sent["category"] != "UTILITY" || len(comps) != 3 ||
		!reflect.DeepEqual(comps[0].(map[string]any)["example"], map[string]any{"body_text": []any{[]any{"Ayşe", "CF-12"}}}) {
		t.Fatalf("create body = %v", sent)
	}
	for _, in := range []whatsapp.CreateTemplateInput{
		{Name: "Bad Name", Category: "UTILITY", Body: "x"},
		{Name: "ok", Category: "OTHER", Body: "x"},
		{Name: "ok", Category: "UTILITY", Body: "{{1}} {{3}}", Example: []string{"a", "b"}},
		{Name: "ok", Category: "UTILITY", Body: "{{1}}"},
	} {
		if _, err := g.adapter().CreateTemplate(t.Context(), instance(), in); err == nil {
			t.Fatalf("invalid template accepted: %+v", in)
		}
	}

	g.on("DELETE /v21.0/"+wabaID+"/message_templates", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "ticket_update" {
			t.Errorf("name = %q", r.URL.Query().Get("name"))
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	if err := g.adapter().DeleteTemplate(t.Context(), instance(), "ticket_update"); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateVariables(t *testing.T) {
	for body, want := range map[string]int{
		"Merhaba": 0, "{{1}} ve {{2}}": 2, "{{1}} {{1}}": 1, "{{2}}": -1, "{{1}} {{3}}": -1, "{{ 1 }}": 1,
	} {
		if got := whatsapp.TemplateVariables(body); got != want {
			t.Errorf("TemplateVariables(%q) = %d, want %d", body, got, want)
		}
	}
}

func TestSignup(t *testing.T) {
	g := newGraph(t)
	g.on("GET /v21.0/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "1234567" || q.Get("client_secret") != "app-secret" || q.Get("code") != "CODE1" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{"access_token":"BIZ-TOKEN","token_type":"bearer"}`))
	})
	g.on("GET /v21.0/"+wabaID+"/phone_numbers", reply(200, `{"data":[{"id":"`+phoneID+`","display_phone_number":"+1 555-078-3881","verified_name":"ClickFix"}]}`))
	g.on("POST /v21.0/"+wabaID+"/subscribed_apps", reply(200, `{"success":true}`))
	g.on("POST /v21.0/"+phoneID+"/register", reply(200, `{"success":true}`))
	a := g.adapter()
	tok, err := a.ExchangeCode(t.Context(), "CODE1")
	if err != nil || tok != "BIZ-TOKEN" {
		t.Fatalf("exchange = %q %v", tok, err)
	}
	nums, err := a.PhoneNumbers(t.Context(), tok, wabaID)
	if err != nil || len(nums) != 1 || nums[0].ID != phoneID {
		t.Fatalf("numbers = %+v %v", nums, err)
	}
	if err := a.SubscribeApp(t.Context(), tok, wabaID); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterNumber(t.Context(), tok, phoneID, "123456"); err != nil {
		t.Fatal(err)
	}
	if g.bodies[len(g.bodies)-1]["pin"] != "123456" {
		t.Fatalf("register body = %v", g.bodies)
	}
	if s := a.SignupConfig(); s.AppID != "1234567" || s.ConfigID != "cfg-9" || s.APIVersion != "v21.0" {
		t.Fatalf("signup config = %+v", s)
	}
	if !a.Enabled() || whatsapp.New(whatsapp.Config{AppID: "x"}).Enabled() {
		t.Fatal("enabled")
	}
}

func TestCheckHealth(t *testing.T) {
	g := newGraph(t)
	g.on("GET /v21.0/"+phoneID, reply(200, `{"id":"`+phoneID+`"}`))
	g.on("GET /v21.0/debug_token", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("input_token") != token || q.Get("access_token") != "1234567|app-secret" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{"data":{"app_id":"1234567","type":"USER","is_valid":true,"expires_at":1767225600}}`))
	})
	h, err := g.adapter().CheckHealth(t.Context(), instance())
	if err != nil || !h.OK || h.TokenExpiresAt == nil || !h.TokenExpiresAt.Equal(time.Unix(1767225600, 0)) {
		t.Fatalf("health = %+v %v", h, err)
	}

	g.on("GET /v21.0/debug_token", reply(200, `{"data":{"type":"SYSTEM_USER","is_valid":true,"expires_at":0}}`))
	if h, _ := g.adapter().CheckHealth(t.Context(), instance()); !h.OK || h.TokenExpiresAt != nil {
		t.Fatalf("never expiring = %+v", h)
	}

	g.on("GET /v21.0/"+phoneID, reply(400, `{"error":{"message":"Session has expired","code":190}}`))
	if h, err := g.adapter().CheckHealth(t.Context(), instance()); err != nil || h.OK || !strings.Contains(h.Detail, "Session has expired") {
		t.Fatalf("expired = %+v %v", h, err)
	}
}

func TestSignupUserIDBindsToConfiguredApp(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"data":{"user_id":"456789123","app_id":"1234567","is_valid":true}}`, true},
		{"foreign app", `{"data":{"user_id":"456789123","app_id":"9999999","is_valid":true}}`, false},
		{"revoked", `{"data":{"user_id":"456789123","app_id":"1234567","is_valid":false}}`, false},
		{"missing user", `{"data":{"app_id":"1234567","is_valid":true}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGraph(t)
			g.on("GET /v21.0/debug_token", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("input_token") != token {
					t.Error("wrong token inspected")
				}
				_, _ = w.Write([]byte(tc.body))
			})
			id, err := g.adapter().SignupUserID(t.Context(), token)
			if (err == nil) != tc.valid {
				t.Fatalf("id=%s err=%v", id, err)
			}
		})
	}
}

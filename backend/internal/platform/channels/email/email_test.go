package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

const domain = "in.clickfix.test"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func instance() channels.Instance {
	return channels.Instance{
		ID: 7, UUID: uuid.MustParse("6a1e0c55-7d0b-4c47-9d0e-1f1b2c3d4e5f"), OrganizationID: 3, Type: channels.TypeEmail,
		Name: "Acme Destek", Status: "active", ExternalID: "acme-destek",
		Credentials: channels.Credentials{}, Settings: map[string]any{SettingAlias: "acme-destek"},
	}
}

func TestParseGmailReplyTurkish(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	res, err := a.ParseRaw(fixture(t, "gmail_reply_tr.eml"), instance(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("messages = %d", len(res.Messages))
	}
	m := res.Messages[0]
	if m.From.ExternalID != "ayse@musteri.com.tr" || m.From.Name != "Ayşe Yılmaz" || m.From.InstanceScoped {
		t.Fatalf("sender = %+v", m.From)
	}
	if m.Subject != "Re: Fatura hatası" {
		t.Fatalf("subject = %q", m.Subject)
	}
	// The root is decoded from our own Message-ID in In-Reply-To.
	if m.ThreadID != "<CAF001root@mail.gmail.com>" {
		t.Fatalf("thread = %q", m.ThreadID)
	}
	if m.ExternalID != "<CAF123reply@mail.gmail.com>" {
		t.Fatalf("id = %q", m.ExternalID)
	}
	want := "Merhaba, fatura hâlâ görünmüyor.\nEkte ekran görüntüsü var."
	if m.Text != want {
		t.Fatalf("text = %q", m.Text)
	}
	if !strings.Contains(m.Attributes["full_text"].(string), "faturanız yeniden") {
		t.Fatal("full text lost")
	}
	html := m.Attributes["html"].(string)
	if strings.Contains(html, "script") || strings.Contains(html, "onerror") || strings.Contains(html, "javascript:") {
		t.Fatalf("html not sanitized: %s", html)
	}
	if cc := m.Attributes["cc"].([]Address); len(cc) != 1 || cc[0].Address != "mehmet@musteri.com.tr" {
		t.Fatalf("cc = %v", cc)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "ekran.png" || m.Attachments[0].ContentType != "image/png" {
		t.Fatalf("attachments = %+v", m.Attachments)
	}
	rc, _ := m.Attachments[0].Open(context.Background())
	data, _ := io.ReadAll(rc)
	if !strings.HasPrefix(string(data), "\x89PNG") {
		t.Fatal("attachment not decoded")
	}
}

func TestParseWindows1254AndOutlookQuote(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	res, err := a.ParseRaw(fixture(t, "outlook_new_1254.eml"), instance(), "x")
	if err != nil {
		t.Fatal(err)
	}
	m := res.Messages[0]
	if m.Text != "Siparişim gelmedi, yardımcı olur musunuz?" {
		t.Fatalf("text = %q", m.Text)
	}
	if m.ThreadID != "<AM0PR01first@outlook.com>" {
		t.Fatalf("a new mail roots its own thread: %q", m.ThreadID)
	}
}

func TestAutoReplyAndLoopDropped(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	res, err := a.ParseRaw(fixture(t, "auto_reply.eml"), instance(), "x")
	if err != nil || len(res.Messages) != 0 {
		t.Fatalf("auto reply kept: %v %v", res, err)
	}
	inst := instance()
	loop := "From: Acme <destek@acme.com>\r\nTo: x@y.com\r\nSubject: hi\r\n" + HeaderInstance + ": " + inst.UUID.String() + "\r\nMessage-ID: <a@b>\r\n\r\nhello\r\n"
	res, err = a.ParseRaw([]byte(loop), inst, "x")
	if err != nil || len(res.Messages) != 0 {
		t.Fatalf("own message read back: %v %v", res, err)
	}
	fromSelf := "From: acme-destek@in.clickfix.test\r\nTo: acme-destek@in.clickfix.test\r\nSubject: hi\r\nMessage-ID: <c@d>\r\n\r\nhello\r\n"
	if res, _ := a.ParseRaw([]byte(fromSelf), inst, "x"); len(res.Messages) != 0 {
		t.Fatal("mail from the routing domain kept")
	}
}

func TestBounceReport(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	res, err := a.ParseRaw(fixture(t, "bounce_dsn.eml"), instance(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 0 || len(res.Statuses) != 1 {
		t.Fatalf("res = %+v", res)
	}
	st := res.Statuses[0]
	if st.Status != channels.StatusFailed || st.ExternalID != "<cf.11111111-2222-3333-4444-555555555555@acme.com>" || !strings.Contains(st.Error, "5.1.1") {
		t.Fatalf("status = %+v", st)
	}
}

func TestVisibleReply(t *testing.T) {
	cases := map[string]string{
		"Tamam\n\nOn Tue, 27 Oct 2026 at 10:00, Acme <a@b.c> wrote:\n> eski":       "Tamam",
		"Tamam\nOn Tue, 27 Oct 2026 at 10:00, Acme Destek <a@b.c>\nwrote:\n> eski": "Tamam",
		"Evet\n-----Original Message-----\nFrom: x":                                "Evet",
		"Cevabım:\n> soru 1\ncevap 1\n> soru 2\ncevap 2":                           "Cevabım:\n> soru 1\ncevap 1\n> soru 2\ncevap 2",
		"> sadece alıntı":               "> sadece alıntı",
		"Teşekkürler\n-- \nAhmet\n0555": "Teşekkürler",
		"Kimden: biri\nmetin":           "Kimden: biri\nmetin",
	}
	for in, want := range cases {
		if got := VisibleReply(in); got != want {
			t.Errorf("VisibleReply(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOwnMessageIDRoundTrip(t *testing.T) {
	id := OwnMessageID(uuid.New(), "<root.1@mail.example.com>", "acme.com")
	if got := rootFromOwnID(id); got != "<root.1@mail.example.com>" {
		t.Fatalf("root = %q from %s", got, id)
	}
	if rootFromOwnID("<foreign@x.com>") != "" {
		t.Fatal("foreign id decoded")
	}
	if strings.Contains(OwnMessageID(uuid.New(), "", "acme.com"), "..") {
		t.Fatal("empty root encoded")
	}
}

func svixHeaders(t *testing.T, secret string, body []byte, at time.Time) http.Header {
	t.Helper()
	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	h := http.Header{}
	ts := strconv.FormatInt(at.Unix(), 10)
	h.Set(HeaderSvixID, "msg_1")
	h.Set(HeaderSvixTimestamp, ts)
	h.Set(HeaderSvixSignature, "v1,bogus v1,"+SignSvix(key, "msg_1", ts, body))
	return h
}

func TestVerifyWebhookSvix(t *testing.T) {
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	a := New(Config{InboundDomain: domain, WebhookSecret: secret})
	now := time.Now()
	a.now = func() time.Time { return now }
	body := fixture(t, "received.json")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header = svixHeaders(t, secret, body, now)
	if err := a.VerifyWebhook(r, body, instance()); err != nil {
		t.Fatal(err)
	}
	if err := a.VerifyWebhook(r, append(body, ' '), instance()); !errors.Is(err, channels.ErrSignature) {
		t.Fatal("tampered body accepted")
	}
	r.Header = svixHeaders(t, secret, body, now.Add(-10*time.Minute))
	if err := a.VerifyWebhook(r, body, instance()); !errors.Is(err, channels.ErrSignature) {
		t.Fatal("stale timestamp accepted")
	}
}

func TestRoute(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	ids, err := a.Route(nil, fixture(t, "received.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "acme-destek,acme-satis" {
		t.Fatalf("ids = %v", ids)
	}
	ids, _ = a.Route(nil, fixture(t, "bounced.json"))
	if strings.Join(ids, ",") != "acme-destek" {
		t.Fatalf("bounce ids = %v", ids)
	}
}

// fakeResend serves the receiving API, the raw download and /emails.
func fakeResend(t *testing.T, sent *[]map[string]any) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/emails/receiving/4ef9a417-02e9-4d39-ad75-9611e0fcc33c":
			if r.Header.Get("Authorization") != "Bearer re_test" {
				w.WriteHeader(401)
				return
			}
			_, _ = io.WriteString(w, `{"raw":{"download_url":"`+srv.URL+`/raw/1"}}`)
		case r.URL.Path == "/raw/1":
			if r.Header.Get("Authorization") != "" {
				t.Error("raw download must not carry the api key")
			}
			_, _ = w.Write(fixture(t, "gmail_reply_tr.eml"))
		case r.URL.Path == "/emails" && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["_idempotency"] = r.Header.Get("Idempotency-Key")
			*sent = append(*sent, body)
			if strings.Contains(body["text"].(string), "#reject") {
				w.WriteHeader(422)
				_, _ = io.WriteString(w, `{"name":"validation_error","message":"bad from"}`)
				return
			}
			if strings.Contains(body["text"].(string), "#fail") {
				w.WriteHeader(503)
				return
			}
			_, _ = io.WriteString(w, `{"id":"re_sent_1"}`)
		case r.URL.Path == "/domains":
			_, _ = io.WriteString(w, `{"data":[{"id":"d1","name":"acme.com","status":"verified"},{"id":"d2","name":"pending.com","status":"pending"}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestParseWebhookReceivedDownloadsRaw(t *testing.T) {
	var sent []map[string]any
	srv := fakeResend(t, &sent)
	a := New(Config{InboundDomain: domain, ResendAPIKey: "re_test", BaseURL: srv.URL})
	res, err := a.ParseWebhook(context.Background(), nil, fixture(t, "received.json"), instance())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].From.Email != "ayse@musteri.com.tr" {
		t.Fatalf("res = %+v", res)
	}
	res, err = a.ParseWebhook(context.Background(), nil, fixture(t, "bounced.json"), instance())
	if err != nil || len(res.Statuses) != 1 || res.Statuses[0].ExternalID != "re_sent_1" || res.Statuses[0].Status != channels.StatusFailed {
		t.Fatalf("bounce = %+v %v", res, err)
	}
}

func TestSendResend(t *testing.T) {
	var sent []map[string]any
	srv := fakeResend(t, &sent)
	a := New(Config{InboundDomain: domain, ResendAPIKey: "re_test", BaseURL: srv.URL})
	inst := instance()
	inst.Credentials = channels.Credentials{FieldFromAddress: "destek@acme.com", FieldFromName: "Acme Destek"}
	msgID := uuid.New()
	res, err := a.Send(context.Background(), inst, channels.OutboundMessage{
		MessageUUID: msgID, To: channels.Recipient{ExternalID: "ayse@musteri.com.tr"}, Text: "Merhaba\n\nÇözüldü.",
		ThreadID: "<CAF001root@mail.gmail.com>", Subject: "Fatura hatası", ReplyToExternalID: "<CAF123reply@mail.gmail.com>",
		Attributes:  map[string]any{"cc": []any{"mehmet@musteri.com.tr", "not-an-address"}},
		Attachments: []channels.OutboundAttachment{{Filename: "fatura.pdf", URL: "https://api.example.com/f/1"}},
	})
	if err != nil || res.ExternalID != "re_sent_1" {
		t.Fatalf("send = %v %v", res, err)
	}
	b := sent[0]
	if b["from"] != `"Acme Destek" <destek@acme.com>` || b["subject"] != "Re: Fatura hatası" {
		t.Fatalf("body = %v", b)
	}
	if b["_idempotency"] != "cf-message/"+msgID.String() {
		t.Fatalf("idempotency = %v", b["_idempotency"])
	}
	if rt := b["reply_to"].([]any); rt[0] != "acme-destek@in.clickfix.test" {
		t.Fatalf("reply_to = %v", rt)
	}
	h := b["headers"].(map[string]any)
	if h["References"] != "<CAF001root@mail.gmail.com> <CAF123reply@mail.gmail.com>" || h["In-Reply-To"] != "<CAF123reply@mail.gmail.com>" {
		t.Fatalf("headers = %v", h)
	}
	if rootFromOwnID(h["Message-ID"].(string)) != "<CAF001root@mail.gmail.com>" || h[HeaderInstance] != inst.UUID.String() {
		t.Fatalf("message id = %v", h)
	}
	if cc := b["cc"].([]any); len(cc) != 1 {
		t.Fatalf("cc = %v", cc)
	}
	if !strings.Contains(b["html"].(string), "<p>Çözüldü.</p>") {
		t.Fatalf("html = %v", b["html"])
	}

	_, err = a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), To: channels.Recipient{ExternalID: "a@b.co"}, Text: "#reject"})
	if !channels.IsPermanent(err) {
		t.Fatalf("422 must be permanent: %v", err)
	}
	_, err = a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), To: channels.Recipient{ExternalID: "a@b.co"}, Text: "#fail"})
	if err == nil || channels.IsPermanent(err) {
		t.Fatalf("503 must be temporary: %v", err)
	}
	_, err = a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), To: channels.Recipient{ExternalID: "+905551112233"}, Text: "x"})
	if !channels.IsPermanent(err) {
		t.Fatalf("phone recipient must be permanent: %v", err)
	}
}

func TestSendSMTPBuildsMessage(t *testing.T) {
	a := New(Config{InboundDomain: domain})
	var gotRcpt []string
	var gotMsg string
	a.smtpSend = func(_ context.Context, c SMTPConfig, from string, rcpt []string, msg []byte) error {
		if c.Host != "smtp.acme.com" || c.TLS != "implicit" || from != "destek@acme.com" {
			t.Errorf("smtp config = %+v from %s", c, from)
		}
		gotRcpt, gotMsg = rcpt, string(msg)
		return nil
	}
	inst := instance()
	inst.Credentials = channels.Credentials{
		FieldProvider: "smtp", FieldFromAddress: "destek@acme.com", FieldSMTPHost: "smtp.acme.com", FieldSMTPPort: "465",
		FieldSMTPUsername: "destek@acme.com", FieldSMTPPassword: "secret",
	}
	res, err := a.Send(context.Background(), inst, channels.OutboundMessage{
		MessageUUID: uuid.New(), To: channels.Recipient{Email: "can@firma.com"}, Text: "Kargonuz yolda.", Subject: "Re: Siparis sorunu",
		ThreadID: "<AM0PR01first@outlook.com>", Attributes: map[string]any{"bcc": "arsiv@acme.com"},
		Attachments: []channels.OutboundAttachment{{Filename: "fatura.pdf", ContentType: "application/pdf", Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("%PDF-1.4")), nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.ExternalID, "<cf.") {
		t.Fatalf("smtp external id = %q", res.ExternalID)
	}
	if strings.Join(gotRcpt, ",") != "can@firma.com,arsiv@acme.com" {
		t.Fatalf("rcpt = %v", gotRcpt)
	}
	for _, want := range []string{"Subject: Re: Siparis sorunu", "In-Reply-To: <AM0PR01first@outlook.com>", "Reply-To: acme-destek@in.clickfix.test", "multipart/mixed", "fatura.pdf", "JVBERi0xLjQ="} {
		if !strings.Contains(gotMsg, want) {
			t.Errorf("message lacks %q", want)
		}
	}
	if strings.Contains(gotMsg, "arsiv@acme.com") {
		t.Fatal("bcc leaked into headers")
	}
	parsed, err := Parse([]byte(gotMsg))
	if err != nil || parsed.Text != "Kargonuz yolda." || len(parsed.Parts) != 1 {
		t.Fatalf("built message does not parse back: %+v %v", parsed, err)
	}

	a.smtpSend = func(context.Context, SMTPConfig, string, []string, []byte) error {
		return &SMTPError{Step: "recipient", Code: 550, Err: errors.New("550 no such user")}
	}
	if _, err := a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), To: channels.Recipient{Email: "x@y.co"}, Text: "x"}); !channels.IsPermanent(err) {
		t.Fatalf("550 must be permanent: %v", err)
	}
}

func TestConnect(t *testing.T) {
	var sent []map[string]any
	srv := fakeResend(t, &sent)
	a := New(Config{InboundDomain: domain, ResendAPIKey: "re_test", BaseURL: srv.URL})
	inst := instance()
	if ext, err := a.Connect(context.Background(), inst); err != nil || ext != "acme-destek" {
		t.Fatalf("connect = %q %v", ext, err)
	}
	inst.Settings = map[string]any{}
	if ext, _ := a.Connect(context.Background(), inst); ext != "in-6a1e0c557d0b" {
		t.Fatalf("derived alias = %q", ext)
	}
	inst.Credentials = channels.Credentials{FieldFromAddress: "destek@pending.com"}
	if _, err := a.Connect(context.Background(), inst); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("unverified domain accepted: %v", err)
	}
	inst.Credentials = channels.Credentials{FieldFromAddress: "x@" + domain}
	if _, err := a.Connect(context.Background(), inst); err == nil {
		t.Fatal("from on the receiving domain accepted")
	}
	inst.Settings = map[string]any{SettingAlias: "Bad Alias!"}
	inst.Credentials = channels.Credentials{}
	if _, err := a.Connect(context.Background(), inst); err == nil {
		t.Fatal("bad alias accepted")
	}
	a.smtpTest = func(context.Context, SMTPConfig) error { return errors.New("535 auth failed") }
	inst.Settings = map[string]any{}
	inst.Credentials = channels.Credentials{FieldProvider: "smtp", FieldFromAddress: "d@acme.com", FieldSMTPHost: "smtp.acme.com"}
	if _, err := a.Connect(context.Background(), inst); err == nil || !strings.Contains(err.Error(), "535") {
		t.Fatalf("smtp login failure accepted: %v", err)
	}
}

func TestSMTPPortRestrictionHint(t *testing.T) {
	for _, port := range []string{"465", "587"} {
		err := SMTPConnectionError(port, fmt.Errorf("connect: %w", context.DeadlineExceeded))
		var hint *SMTPPortRestrictionError
		if !errors.As(err, &hint) || !strings.Contains(err.Error(), "Resend") || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("port %s: %v", port, err)
		}
	}
	for _, port := range []string{"25", "2525", "465"} {
		original := errors.New("535 auth failed")
		if SMTPConnectionError(port, original) != original {
			t.Fatal("authentication error mislabeled")
		}
	}
	if _, ok := SMTPConnectionError("2525", context.DeadlineExceeded).(*SMTPPortRestrictionError); ok {
		t.Fatal("unrestricted port mislabeled")
	}
}

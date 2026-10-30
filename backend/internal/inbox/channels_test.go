package inbox_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeResend is the Resend API of the e-mail end to end test.
type fakeResend struct {
	srv  *httptest.Server
	raw  []byte
	mu   sync.Mutex
	sent []map[string]any
}

func newFakeResend(t *testing.T, raw []byte) *fakeResend {
	f := &fakeResend{raw: raw}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/emails/receiving/"):
			_, _ = io.WriteString(w, `{"raw":{"download_url":"`+f.srv.URL+`/raw"}}`)
		case r.URL.Path == "/raw":
			_, _ = w.Write(f.raw)
		case r.URL.Path == "/emails":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.sent = append(f.sent, body)
			n := len(f.sent)
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"id":"re_out_`+strconv.Itoa(n)+`"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

const (
	inboundDomain = "in.clickfix.test"
	svixKey       = "0123456789abcdef0123456789abcdef"
)

func svixPost(t *testing.T, mux http.Handler, path string, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set(email.HeaderSvixID, "msg_"+uuid.NewString())
	req.Header.Set(email.HeaderSvixTimestamp, ts)
	req.Header.Set(email.HeaderSvixSignature, "v1,"+email.SignSvix([]byte(svixKey), req.Header.Get(email.HeaderSvixID), ts, body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

type emailEnv struct {
	ib     *inbox.Inbox
	pool   *pgxpool.Pool
	org    int64
	agent  int64
	mux    *http.ServeMux
	resend *fakeResend
}

func setupEmail(t *testing.T) emailEnv {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for inbox tests")
	}
	raw, err := os.ReadFile("../platform/channels/email/testdata/gmail_reply_tr.eml")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e := emailEnv{pool: pool, resend: newFakeResend(t, raw)}
	var cfg config.Config
	cfg.App.Env = "test"
	cfg.Encryption.Key = "0123456789abcdef0123456789abcdef"
	cfg.Channels.Email = config.EmailChannelConfig{
		InboundDomain: inboundDomain, ResendAPIKey: "re_test", ResendBaseURL: e.resend.srv.URL,
		WebhookSecret: "whsec_" + base64.StdEncoding.EncodeToString([]byte(svixKey)),
	}
	e.ib, err = inbox.New(inbox.Deps{Config: cfg, DB: pool, Queries: db.New(pool)})
	if err != nil {
		t.Fatal(err)
	}
	if e.ib.Adapters.Email == nil {
		t.Fatal("e-mail channel not registered")
	}
	q := &syncQueue{ib: e.ib, retries: map[int64]int{}}
	e.ib.Conversations.WithQueue(q).WithNotifier(&recorder{})
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Mail') RETURNING id", "ml-"+uuid.NewString()[:12]).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Şahin', 'active') RETURNING id", uuid.NewString()+"@example.test").Scan(&e.agent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", e.agent)
	})
	h, err := inbox.ChannelsHandler(cfg, e.ib, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.mux = http.NewServeMux()
	// The real route table: a pattern conflict would panic here.
	channelsmodule.RegisterRoutes(e.mux, h, nil, nil, stubResolver{})
	return e
}

type stubResolver struct{}

func (stubResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, orgctx.ErrNotMember
}

func TestEmailChannelEndToEnd(t *testing.T) {
	e := setupEmail(t)
	ctx := context.Background()
	alias := "acme-" + uuid.NewString()[:8]
	inst, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeEmail, Name: "Acme Destek",
		Settings: map[string]any{email.SettingAlias: alias},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sending from a domain the organization never registered is refused
	// before Resend is asked (the Resend account is shared).
	_, err = e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeEmail, Name: "Spoof",
		Credentials: channels.Credentials{email.FieldFromAddress: "ceo@other-company.test"},
	})
	if !errors.Is(err, channelsusecase.ErrInvalid) {
		t.Fatalf("unowned sender domain accepted: %v", err)
	}
	if err := e.ib.Channels.ClaimEmailDomain(ctx, e.org, e.agent, "own-"+alias+".test", "d1"); err != nil {
		t.Fatal(err)
	}
	var otherOrg int64
	if err := e.pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Other') RETURNING id", "ot-"+uuid.NewString()[:12]).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", otherOrg) })
	if err := e.ib.Channels.ClaimEmailDomain(ctx, otherOrg, 0, "own-"+alias+".test", "d2"); !errors.Is(err, channelsusecase.ErrDomainTaken) {
		t.Fatalf("domain claimed twice: %v", err)
	}
	if inst.ExternalID == nil || *inst.ExternalID != alias || inst.SharedWebhookPath != "/v1/webhooks/channels/email" {
		t.Fatalf("instance = %+v", inst)
	}
	// webhook_url is absolute only once the API knows its public origin.
	if inst.WebhookURL != "" {
		t.Fatalf("webhook_url without PUBLIC_API_URL = %q", inst.WebhookURL)
	}
	e.ib.Channels.WithPublicAPIURL("https://api.example.test/")
	if v, err := e.ib.Channels.Get(ctx, e.org, inst.UUID); err != nil || v.WebhookURL != "https://api.example.test/v1/webhooks/channels/email" {
		t.Fatalf("webhook_url = %q, %v", v.WebhookURL, err)
	}
	e.ib.Channels.WithPublicAPIURL("")

	received := []byte(`{"type":"email.received","created_at":"2026-10-28T07:15:02.000Z","data":{"email_id":"rcv-1","from":"Ayşe <ayse@musteri.com.tr>","to":["` + alias + `@` + inboundDomain + `"],"cc":[],"subject":"Re: Fatura"}}`)
	if code := svixPost(t, e.mux, "/v1/webhooks/channels/email", append([]byte(nil), received...)); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	// Redelivery stores nothing new; a bad signature is rejected.
	if code := svixPost(t, e.mux, "/v1/webhooks/channels/email", received); code != http.StatusOK {
		t.Fatalf("redelivery = %d", code)
	}
	bad := httptest.NewRequest(http.MethodPost, "/v1/webhooks/channels/email", bytes.NewReader(received))
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, bad)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned webhook = %d", rec.Code)
	}

	var convUUID uuid.UUID
	var thread, subject string
	var messages int
	if err := e.pool.QueryRow(ctx, `SELECT c.uuid, COALESCE(c.external_thread_id, ''), c.subject,
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id)
		FROM conversations c WHERE c.organization_id = $1`, e.org).Scan(&convUUID, &thread, &subject, &messages); err != nil {
		t.Fatal(err)
	}
	if messages != 1 || thread != "<CAF001root@mail.gmail.com>" || subject != "Re: Fatura hatası" {
		t.Fatalf("conversation thread=%q subject=%q messages=%d", thread, subject, messages)
	}
	var lastWebhook *time.Time
	_ = e.pool.QueryRow(ctx, "SELECT last_webhook_at FROM channel_instances WHERE uuid = $1", inst.UUID).Scan(&lastWebhook)
	if lastWebhook == nil {
		t.Fatal("webhook last-seen not recorded")
	}

	// The agent replies: Resend gets a threaded mail from the routing address.
	posted, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{
		OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "Faturanızı yeniden gönderdik.", ClientID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.resend.sent) != 1 {
		t.Fatalf("resend sends = %d", len(e.resend.sent))
	}
	out := e.resend.sent[0]
	headers := out["headers"].(map[string]any)
	if out["to"].([]any)[0] != "ayse@musteri.com.tr" || out["subject"] != "Re: Fatura hatası" ||
		!strings.HasPrefix(headers["References"].(string), "<CAF001root@mail.gmail.com>") {
		t.Fatalf("sent mail = %v", out)
	}
	var status, external string
	_ = e.pool.QueryRow(ctx, "SELECT status, COALESCE(external_id, '') FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status, &external)
	if status != convmodel.MessageSent || external != "re_out_1" {
		t.Fatalf("after send status=%s external=%s", status, external)
	}

	// Resend reports a bounce for the sent mail, routed by its tag.
	bounce := []byte(`{"type":"email.bounced","data":{"email_id":"re_out_1","created_at":"2026-10-28T08:00:00.000Z","bounce":{"message":"mailbox full"},"tags":{"cf_instance":"` + alias + `"}}}`)
	if code := svixPost(t, e.mux, "/v1/webhooks/channels/email", bounce); code != http.StatusOK {
		t.Fatalf("bounce webhook = %d", code)
	}
	_ = e.pool.QueryRow(ctx, "SELECT status FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status)
	if status != convmodel.MessageFailed {
		t.Fatalf("after bounce status = %s", status)
	}

	// Health: the list view carries the outbound failure counts, the sweep
	// checks the instance with the provider.
	if _, _, err := e.ib.Channels.HealthSweep(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := e.ib.Channels.Get(ctx, e.org, inst.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Health.State != "ok" || v.Health.Failed24h != 1 || v.Health.CheckedAt == nil || v.Health.LastWebhookAt == nil {
		t.Fatalf("health = %+v", v.Health)
	}
}

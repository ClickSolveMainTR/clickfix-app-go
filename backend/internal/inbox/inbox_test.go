package inbox_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const secret = "test-webhook-secret-0123456789"

// syncQueue delivers right away, like the worker would, and remembers
// delayed (rate limited) sends.
type syncQueue struct {
	ib      *inbox.Inbox
	mu      sync.Mutex
	retries map[int64]int
	lastErr error
}

func (q *syncQueue) EnqueueSend(ctx context.Context, orgID, messageID int64, _ time.Duration) error {
	q.mu.Lock()
	n := q.retries[messageID]
	q.mu.Unlock()
	err := q.ib.Deliver(ctx, orgID, messageID, n, 2)
	q.mu.Lock()
	q.retries[messageID] = n + 1
	q.lastErr = err
	q.mu.Unlock()
	return nil
}

type recorder struct {
	mu     sync.Mutex
	events []convmodel.LiveEvent
}

func (r *recorder) Notify(_ context.Context, _ int64, ev convmodel.LiveEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, e := range r.events {
		out = append(out, e.Type)
	}
	return out
}

type env struct {
	ib    *inbox.Inbox
	pool  *pgxpool.Pool
	org   int64
	agent int64
	inst  channelsusecase.InstanceView
	mux   *http.ServeMux
	queue *syncQueue
	live  *recorder
}

func setup(t *testing.T) env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for inbox tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var cfg config.Config
	cfg.App.Env = "test"
	cfg.Encryption.Key = "0123456789abcdef0123456789abcdef"
	ib, err := inbox.New(inbox.Deps{Config: cfg, DB: pool, Queries: q})
	if err != nil {
		t.Fatal(err)
	}
	e := env{ib: ib, pool: pool, live: &recorder{}}
	e.queue = &syncQueue{ib: ib, retries: map[int64]int{}}
	ib.Conversations.WithQueue(e.queue).WithNotifier(e.live)
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Inbox') RETURNING id", "ib-"+uuid.NewString()[:12]).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Şahin', 'active') RETURNING id", uuid.NewString()+"@example.test").Scan(&e.agent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", e.agent)
	})
	e.inst, err = ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeTest, Name: "Test",
		Credentials: channels.Credentials{testchannel.SecretField: secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := channelshandler.New(ib.Channels, ib.Receive, nil, nil)
	e.mux = http.NewServeMux()
	e.mux.HandleFunc("POST /v1/webhooks/channels/{type}/{instance}", h.Webhook)
	return e
}

func (e env) post(t *testing.T, body string, sig string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, e.inst.WebhookPath, bytes.NewBufferString(body))
	if sig == "" {
		sig = testchannel.Sign(secret, []byte(body))
	}
	req.Header.Set(testchannel.SignatureHeader, sig)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestTestChannelEndToEnd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	inbound := `{"messages":[{"id":"m-1","from":{"id":"visitor-9","name":"Can Öztürk","email":"can@example.test"},"text":"Faturam gelmedi"}]}`

	if code := e.post(t, inbound, "sha256=bad"); code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d", code)
	}
	if code := e.post(t, inbound, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	if code := e.post(t, inbound, ""); code != http.StatusOK {
		t.Fatalf("redelivery = %d", code)
	}
	var convUUID uuid.UUID
	var messages int
	if err := e.pool.QueryRow(ctx, `SELECT c.uuid, (SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id)
		FROM conversations c WHERE c.organization_id = $1`, e.org).Scan(&convUUID, &messages); err != nil {
		t.Fatal(err)
	}
	if messages != 1 {
		t.Fatalf("redelivered webhook stored %d messages", messages)
	}
	if got := e.live.types(); len(got) != 2 || got[0] != convmodel.EventConversationCreated || got[1] != convmodel.EventMessageCreated {
		t.Fatalf("live events = %v", got)
	}
	var outboxRows int
	_ = e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE payload->>'entity_uuid' = $1`, convUUID.String()).Scan(&outboxRows)
	if outboxRows != 2 { // conversations.created + messages.received
		t.Fatalf("outbox rows = %d", outboxRows)
	}

	// The agent answers; the queue delivers through the test adapter.
	posted, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{
		OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "Hemen kontrol ediyorum.",
		ClientID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sent := e.ib.Test.Sent(0)
	if len(sent) != 1 || sent[0].Message.To.ExternalID != "visitor-9" || sent[0].Message.Text != "Hemen kontrol ediyorum." {
		t.Fatalf("test channel sent %+v", sent)
	}
	var status, external string
	if err := e.pool.QueryRow(ctx, "SELECT status, COALESCE(external_id, '') FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status, &external); err != nil {
		t.Fatal(err)
	}
	if status != convmodel.MessageSent || external != sent[0].ExternalID {
		t.Fatalf("after delivery status=%s external=%s", status, external)
	}

	// A delivery receipt arrives by webhook.
	receipt := fmt.Sprintf(`{"statuses":[{"id":%q,"status":"read"}]}`, external)
	if code := e.post(t, receipt, ""); code != http.StatusOK {
		t.Fatalf("receipt webhook = %d", code)
	}
	_ = e.pool.QueryRow(ctx, "SELECT status FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status)
	if status != convmodel.MessageRead {
		t.Fatalf("after receipt status = %s", status)
	}
	var waiting bool
	_ = e.pool.QueryRow(ctx, "SELECT waiting_since IS NOT NULL FROM conversations WHERE uuid = $1", convUUID).Scan(&waiting)
	if waiting {
		t.Fatal("answered conversation still waiting")
	}
}

func TestDeliveryFailures(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"x-1","from":{"id":"v-1"},"text":"selam"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var convUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1", e.org).Scan(&convUUID); err != nil {
		t.Fatal(err)
	}
	status := func(id uuid.UUID) (string, string) {
		var s, msg string
		_ = e.pool.QueryRow(ctx, "SELECT status, error FROM messages WHERE uuid = $1", id).Scan(&s, &msg)
		return s, msg
	}

	rejected, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "#reject bu"})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := status(rejected.Message.UUID); s != convmodel.MessageFailed {
		t.Fatalf("permanent failure status = %s", s)
	}

	// A temporary failure is retried and fails after the last attempt.
	flaky, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "#fail sonra"})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := status(flaky.Message.UUID); s != convmodel.MessageQueued {
		t.Fatalf("temporary failure should stay queued, got %s", s)
	}
	var id int64
	_ = e.pool.QueryRow(ctx, "SELECT id FROM messages WHERE uuid = $1", flaky.Message.UUID).Scan(&id)
	if err := e.ib.Deliver(ctx, e.org, id, 2, 2); err != nil {
		t.Fatalf("last attempt should not ask for a retry: %v", err)
	}
	if s, msg := status(flaky.Message.UUID); s != convmodel.MessageFailed || msg == "" {
		t.Fatalf("last attempt status = %s (%s)", s, msg)
	}

	// Private notes never reach the channel.
	before := len(e.ib.Test.Sent(0))
	if _, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "iç not", Private: true}); err != nil {
		t.Fatal(err)
	}
	if len(e.ib.Test.Sent(0)) != before {
		t.Fatal("a private note was sent to the contact")
	}
}

// A resolved conversation no longer waits for an answer (QA round 2: the
// inbox showed "bekliyor" on resolved conversations).
func TestResolveClearsWaiting(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"w-1","from":{"id":"v-w"},"text":"merhaba"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var convUUID uuid.UUID
	var waiting bool
	if err := e.pool.QueryRow(ctx, "SELECT uuid, waiting_since IS NOT NULL FROM conversations WHERE organization_id = $1", e.org).Scan(&convUUID, &waiting); err != nil {
		t.Fatal(err)
	}
	if !waiting {
		t.Fatal("inbound conversation should wait")
	}
	resolved := convmodel.StatusResolved
	conv, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: convUUID, Status: &resolved})
	if err != nil {
		t.Fatal(err)
	}
	if conv.WaitingSince.Valid {
		t.Fatalf("resolved conversation still waiting since %v", conv.WaitingSince.Time)
	}
	// The contact writes again: the conversation reopens and waits.
	if code := e.post(t, `{"messages":[{"id":"w-2","from":{"id":"v-w"},"text":"bir şey daha"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var status string
	_ = e.pool.QueryRow(ctx, "SELECT status, waiting_since IS NOT NULL FROM conversations WHERE uuid = $1", convUUID).Scan(&status, &waiting)
	if status != "open" || !waiting {
		t.Fatalf("after new inbound status=%s waiting=%v", status, waiting)
	}
}

func TestPausedInstanceDropsTraffic(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	paused := channelsusecase.StatusPaused
	if _, err := e.ib.Channels.Update(ctx, e.org, e.inst.UUID, channelsusecase.UpdateInput{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	if code := e.post(t, `{"messages":[{"id":"p-1","from":{"id":"v-2"},"text":"hey"}]}`, ""); code != http.StatusOK {
		t.Fatalf("paused webhook = %d", code)
	}
	var n int
	_ = e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM conversations WHERE organization_id = $1", e.org).Scan(&n)
	if n != 0 {
		t.Fatalf("paused inbox stored %d conversations", n)
	}
	view, err := e.ib.Channels.Get(ctx, e.org, e.inst.UUID)
	if err != nil || view.CredentialHints[testchannel.SecretField] == secret || view.CredentialHints[testchannel.SecretField] == "" {
		t.Fatalf("credential hint must be masked: %+v", view.CredentialHints)
	}
}

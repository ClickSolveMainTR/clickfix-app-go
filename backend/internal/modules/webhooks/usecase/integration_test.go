package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

type retryTransport struct {
	t          *testing.T
	secret     string
	calls      int
	deliveryID string
}

func (rt *retryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.calls++
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rt.t.Fatal(err)
	}
	stamp, _ := strconv.ParseInt(r.Header.Get("ClickFix-Webhook-Timestamp"), 10, 64)
	if r.Header.Get("ClickFix-Webhook-Signature") != SignWebhook(rt.secret, stamp, body) {
		rt.t.Fatal("invalid signature")
	}
	id := r.Header.Get("ClickFix-Webhook-Id")
	if rt.deliveryID != "" && id != rt.deliveryID {
		rt.t.Fatal("retry changed delivery identity")
	}
	rt.deliveryID = id
	status := 204
	if rt.calls == 1 {
		status = 503
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
}
func TestWebhookDedupBackoffDeliveryReplayAndOrganizationScope(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	var org int64
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Webhook test',$1) RETURNING id", uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	box, err := crypto.NewSecretBox(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(q, box)
	endpoint, err := svc.Save(ctx, org, uuid.Nil, model.Input{Name: "events", URL: "https://hooks.example.com/events", Events: []string{events.MessageReceived}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	transport := &retryTransport{t: t, secret: endpoint["secret"].(string)}
	svc.Client = &http.Client{Transport: transport}
	ev := events.New(events.MessageReceived).WithOrganization(org).WithPayload(map[string]any{"content": "test"})
	for range 2 {
		if err = svc.Consume(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	n, err := q.CountOutboundDeliveries(ctx, org)
	if err != nil || n != 1 {
		t.Fatal("event duplicate", n, err)
	}
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListOutboundDeliveries(ctx, db.ListOutboundDeliveriesParams{OrganizationID: org, Limit: 100})
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" || rows[0].Attempts != 1 || rows[0].LastStatus != 503 {
		t.Fatal(rows, err)
	}
	if err = svc.Sweep(ctx); err != nil || transport.calls != 1 {
		t.Fatal("ignored backoff", transport.calls, err)
	}
	if _, err = tx.Exec(ctx, "UPDATE outbound_webhook_deliveries SET next_attempt_at=NOW(),lease_until=NOW() WHERE organization_id=$1", org); err != nil {
		t.Fatal(err)
	}
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err = q.ListOutboundDeliveries(ctx, db.ListOutboundDeliveriesParams{OrganizationID: org, Limit: 100})
	if err != nil || rows[0].Status != "succeeded" || rows[0].Attempts != 2 || !rows[0].DeliveredAt.Valid {
		t.Fatal(rows, err)
	}
	n, err = q.CountOutboundDeliveries(ctx, org+100000)
	if err != nil || n != 0 {
		t.Fatal("cross-org delivery log", n, err)
	}
	replayed, err := q.ReplayOutboundDelivery(ctx, db.ReplayOutboundDeliveryParams{OrganizationID: org + 100000, Uuid: rows[0].Uuid})
	if err != nil || replayed != 0 {
		t.Fatal("foreign replay", err)
	}
	if _, err = tx.Exec(ctx, "UPDATE outbound_webhook_deliveries SET lease_until=NOW() WHERE organization_id=$1", org); err != nil {
		t.Fatal(err)
	}
	replayed, err = q.ReplayOutboundDelivery(ctx, db.ReplayOutboundDeliveryParams{OrganizationID: org, Uuid: rows[0].Uuid})
	if err != nil || replayed != 1 {
		t.Fatal("replay", err)
	}
	if err = svc.Sweep(ctx); err != nil || transport.calls != 3 {
		t.Fatal(transport.calls, err)
	}
}

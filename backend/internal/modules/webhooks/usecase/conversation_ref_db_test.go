package usecase

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CF-245: the stored delivery body and the polling trigger both carry the
// conversation's uuid for conversation.resolved.
func TestConsumeStoresConversationUUID(t *testing.T) {
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
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Webhook conv',$1) RETURNING id", uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	box, err := crypto.NewSecretBox(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(q, box)
	if _, err = svc.Save(ctx, org, uuid.Nil, model.Input{Name: "resolved", URL: "https://hooks.example.com/events", Events: []string{events.ConversationResolved}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	conv, id := uuid.New(), int64(42)
	ev := events.New(events.ConversationResolved).WithOrganization(org).WithEntity("conversation", &id, &conv).
		WithPayload(map[string]any{"change": "status_changed", "from": "open", "to": "resolved"})
	if err = svc.Consume(ctx, ev); err != nil {
		t.Fatal(err)
	}
	var delivery, trigger []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM outbound_webhook_deliveries WHERE organization_id=$1", org).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Event string         `json:"event"`
		Data  map[string]any `json:"data"`
	}
	if err = json.Unmarshal(delivery, &body); err != nil || body.Data["conversation_uuid"] != conv.String() || body.Data["to"] != "resolved" {
		t.Fatalf("delivery %s %v", delivery, err)
	}
	if err = tx.QueryRow(ctx, "SELECT payload FROM integration_trigger_events WHERE organization_id=$1", org).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	ref := map[string]any{}
	if err = json.Unmarshal(trigger, &ref); err != nil || ref["conversation_uuid"] != conv.String() || ref["entity_uuid"] != conv.String() {
		t.Fatalf("trigger %s %v", trigger, err)
	}
}

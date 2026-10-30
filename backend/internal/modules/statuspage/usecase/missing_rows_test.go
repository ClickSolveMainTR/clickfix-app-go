package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Events for an organization without a status page, or for a row deleted
// before the consumer ran, finish quietly instead of retrying into the
// dead-letter queue.
func TestNotificationEventsWithMissingRows(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var org int64
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Status test') RETURNING id", uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	s := New(repository.New(pool, db.New(pool)), nil, nil, "secret", "https://example.test")
	if err = s.NotifySubscribers(ctx, org, uuid.New(), "body"); err != nil {
		t.Fatalf("no status page: %v", err)
	}
	if err = s.SendConfirmation(ctx, org, uuid.New()); err != nil {
		t.Fatalf("missing subscription: %v", err)
	}
	if err = s.NotifyContacts(ctx, org, uuid.New()); err != nil {
		t.Fatalf("missing incident: %v", err)
	}
}

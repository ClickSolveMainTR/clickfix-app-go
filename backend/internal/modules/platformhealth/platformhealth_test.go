package platformhealth

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeAI []AIProvider

func (f fakeAI) Providers(context.Context) []AIProvider { return f }

type fakeMail mail.Status

func (f fakeMail) Status(context.Context) mail.Status { return mail.Status(f) }

// TestOverview runs the overview against the migrated schema: without a
// queue inspector the status is degraded with queues_unavailable, and AI
// providers that are down are reported.
func TestOverview(t *testing.T) {
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
	svc := NewService(db.New(pool), nil)
	svc.SetAIStatus(fakeAI{{Name: "openai", Status: "down", Detail: "timeout"}})
	svc.SetMailStatus(fakeMail{State: mail.StateDown, Cause: mail.CauseTimeout})
	o, err := svc.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.Mail.State != mail.StateDown || !slices.Contains(o.Issues, "mail_unreachable:timeout") {
		t.Fatalf("mail = %+v issues = %v", o.Mail, o.Issues)
	}
	if o.Status != "degraded" || !slices.Contains(o.Issues, "queues_unavailable") || !slices.Contains(o.Issues, "ai_down:openai") {
		t.Fatalf("overview = %+v", o)
	}
	if o.Queues == nil || o.Channels == nil || o.WindowHours != 24 {
		t.Fatalf("sections = %+v", o)
	}
}

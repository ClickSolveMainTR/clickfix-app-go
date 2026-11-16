package desktopreleases_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/desktopreleases"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestReleasesAgainstPostgres runs publish, update check, list and withdraw
// on the real schema (000240): SQL filters, CHECK constraints and the
// unique key. Platform table: no organization anywhere.
func TestReleasesAgainstPostgres(t *testing.T) {
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
	// A target nobody else uses keeps parallel test runs apart.
	target := "linux-armv7"
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM desktop_releases WHERE target = $1", target)
	}
	cleanup()
	t.Cleanup(cleanup)

	svc := &desktopreleases.Service{Q: db.New(pool), Store: storage.NewMemory(), BaseURL: "https://app.example.test"}
	publish := func(version, channel string) desktopreleases.Release {
		t.Helper()
		body := strings.NewReader("pkg-" + version)
		rel, err := svc.Publish(ctx, 0, desktopreleases.PublishInput{
			App: "agent", Channel: channel, Version: version, Target: target, Notes: "n",
			Signature: sig(version), Updater: desktopreleases.Upload{Name: "clickfix-agent.AppImage", Size: int64(body.Len()), Body: body},
		})
		if err != nil {
			t.Fatalf("publish %s: %v", version, err)
		}
		return rel
	}
	publish("1.9.0", "stable")
	r10 := publish("1.10.0", "stable")
	publish("1.11.0-beta.2", "beta")
	if _, err := svc.Publish(ctx, 0, desktopreleases.PublishInput{App: "agent", Channel: "stable", Version: "1.10.0", Target: target,
		Signature: sig("x"), Updater: desktopreleases.Upload{Name: "a.AppImage", Size: 1, Body: strings.NewReader("x")}}); err != desktopreleases.ErrConflict {
		t.Fatalf("duplicate: %v", err)
	}

	m, err := svc.Update(ctx, "agent", target, "1.2.0", "")
	if err != nil || m == nil || m.Version != "1.10.0" {
		t.Fatalf("stable update: %+v %v", m, err)
	}
	if m, err := svc.Update(ctx, "agent", target, "1.10.0", "beta"); err != nil || m == nil || m.Version != "1.11.0-beta.2" {
		t.Fatalf("beta update: %+v %v", m, err)
	}
	if m, err := svc.Update(ctx, "support", target, "1.2.0", ""); err != nil || m != nil {
		t.Fatalf("other app: %+v %v", m, err)
	}

	if _, err := svc.Withdraw(ctx, 0, r10.UUID, "regresyon"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(ctx, 0, r10.UUID, "regresyon"); err != desktopreleases.ErrWithdrawn {
		t.Fatalf("second withdraw: %v", err)
	}
	if _, err := svc.Withdraw(ctx, 0, uuid.New(), "yok"); err != desktopreleases.ErrNotFound {
		t.Fatalf("unknown withdraw: %v", err)
	}
	if m, err := svc.Update(ctx, "agent", target, "1.2.0", ""); err != nil || m == nil || m.Version != "1.9.0" {
		t.Fatalf("after withdraw: %+v %v", m, err)
	}
	items, total, err := svc.List(ctx, desktopreleases.ListFilter{App: "agent", Status: "withdrawn", Limit: 50})
	if err != nil || total < 1 {
		t.Fatalf("list: %v %d", err, total)
	}
	found := false
	for _, it := range items {
		found = found || it.UUID == r10.UUID && it.WithdrawnAt != nil && it.WithdrawnReason == "regresyon"
	}
	if !found {
		t.Fatalf("withdrawn release missing from list: %+v", items)
	}
}

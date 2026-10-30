package onboarding_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/onboarding"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
)

// TestOnboardingAgainstPostgres runs the flow on the real schema: self-serve
// sign-up makes the caller owner, the counts query sees channels, members
// and invitations, manual steps are idempotent and finish is final.
func TestOnboardingAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for onboarding DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	var userID, peerID int64
	email := uuid.NewString() + "@example.test"
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Yıldız', 'active') RETURNING id", email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Mert', 'Demir', 'active') RETURNING id", uuid.NewString()+"@example.test").Scan(&peerID); err != nil {
		t.Fatal(err)
	}

	orgs := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	svc := onboarding.New(q, orgs, onboarding.Config{SelfServe: true})
	org, err := svc.CreateOrganization(ctx, onboarding.CreateInput{UserID: userID, Name: "Onboarding Test " + uuid.NewString()[:6]})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", []int64{userID, peerID})
	})

	owned, err := q.CountOwnedOrganizations(ctx, userID)
	if err != nil || owned != 1 {
		t.Fatalf("owned = %d, %v", owned, err)
	}

	st, err := svc.Get(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Completed != 1 {
		t.Fatalf("fresh: %+v", st)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO channel_instances (organization_id, channel_type, name, external_id) VALUES ($1, 'widget', 'Web sitesi', $2)", org.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND slug = 'member' AND organization_id IS NULL LIMIT 1", org.ID, peerID); err != nil {
		t.Fatal(err)
	}
	st, err = svc.Get(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	// organization + channel + team + widget (widget channel type).
	if st.Completed != 4 {
		t.Fatalf("after channel and member: %+v", st)
	}

	for range 2 {
		if _, err := svc.CompleteStep(ctx, org.ID, onboarding.StepTeam); err != nil {
			t.Fatal(err)
		}
	}
	row, err := q.GetOrganizationOnboarding(ctx, org.ID)
	if err != nil || len(row.Steps) != 1 {
		t.Fatalf("manual steps must be idempotent: %+v %v", row.Steps, err)
	}

	st, err = svc.Finish(ctx, org.ID)
	if err != nil || st.Active || st.FinishedAt == nil {
		t.Fatalf("finish: %+v %v", st, err)
	}
}

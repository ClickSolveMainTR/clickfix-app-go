package demoseed

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/password"
)

// TestSeedOrganizationFillsCustomerOrganization creates an organization the
// way the platform wizard does with demo_data: the trial starts on the
// demo-full plan, then the demo content is written next to the real owner.
func TestSeedOrganizationFillsCustomerOrganization(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	billing := billingusecase.New(billingrepo.New(pool, q))

	hash, err := password.Hash("Owner12345")
	if err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().Format("150405.000000")
	var firstOrg uuid.UUID
	owner, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "owner-" + suffix + "@musteri.example.com", PasswordHash: hash, Name: "Gerçek", Surname: "Sahip", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	price, err := EnsureFullPlan(ctx, billing, owner.ID)
	if err != nil {
		t.Fatalf("ensure plan: %v", err)
	}
	again, err := EnsureFullPlan(ctx, billing, owner.ID)
	if err != nil || again != price {
		t.Fatalf("second ensure must reuse the published plan: %v %v (err %v)", price, again, err)
	}

	orgs := orgusecase.New(orgrepo.NewPostgres(pool, q), billingusecase.TrialHooks{Service: billing})
	// Two demo organizations side by side: nothing in the fixtures may be
	// unique across the installation.
	for _, name := range []string{" A", " B"} {
		org, err := orgs.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
			Name: "Müşteri Demo " + suffix + name, OwnerUserID: owner.ID, TrialPriceUUID: price,
		})
		if err != nil {
			t.Fatalf("create organization: %v", err)
		}
		if firstOrg == uuid.Nil {
			firstOrg = org.UUID
		}
		res, err := seedOrganization(ctx, pool, org.UUID, Options{Conversations: 25})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}

		var conversations, members, owners, articles, appointments int
		var planCode, status string
		err = pool.QueryRow(ctx, `
			WITH o AS (SELECT id FROM organizations WHERE uuid = $1)
			SELECT (SELECT count(*) FROM conversations WHERE organization_id = o.id),
			       (SELECT count(*) FROM organization_members WHERE organization_id = o.id),
			       (SELECT count(*) FROM organization_members m JOIN roles r ON r.id = m.role_id WHERE m.organization_id = o.id AND r.slug = 'owner'),
			       (SELECT count(*) FROM knowledge_articles WHERE organization_id = o.id AND published_version IS NOT NULL),
			       (SELECT count(*) FROM appointments WHERE organization_id = o.id),
			       (SELECT b.code FROM billing_subscriptions s JOIN billing_plan_versions v ON v.id = s.plan_version_id
			          JOIN billing_plans b ON b.id = v.plan_id WHERE s.organization_id = o.id ORDER BY s.id DESC LIMIT 1),
			       (SELECT s.status FROM billing_subscriptions s WHERE s.organization_id = o.id ORDER BY s.id DESC LIMIT 1)
			FROM o`, org.UUID).Scan(&conversations, &members, &owners, &articles, &appointments, &planCode, &status)
		if err != nil {
			t.Fatal(err)
		}
		if conversations != 25 || members != len(demoUsers)+1 || owners != 1 || articles != len(demoArticles) || appointments == 0 {
			t.Fatalf("unexpected content: conversations=%d members=%d owners=%d articles=%d appointments=%d",
				conversations, members, owners, articles, appointments)
		}
		if planCode != FullPlanCode || status != "trialing" {
			t.Fatalf("subscription on %q (%s), want %s trialing", planCode, status, FullPlanCode)
		}
		for _, a := range res.Accounts {
			if a.Role == "owner" {
				t.Fatalf("demo account %s must not be an owner of a customer organization", a.Email)
			}
		}
	}
	if _, err := seedOrganization(ctx, pool, firstOrg, Options{Conversations: 5}); err == nil {
		t.Fatal("seeding an organization that already has conversations must fail")
	}
}

// TestPlatformStartRecordsOutcome runs the background fill used by the
// platform wizard and waits for its activity event.
func TestPlatformStartRecordsOutcome(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	billing := billingusecase.New(billingrepo.New(pool, q))
	hash, err := password.Hash("Owner12345")
	if err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().Format("150405.000000")
	owner, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "start-" + suffix + "@musteri.example.com", PasswordHash: hash, Name: "Başlat", Surname: "Sahip", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPlatform(ctx, pool, billing, nil, nil)
	price, err := p.TrialPrice(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	pw, err := p.NewPassword()
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgusecase.New(orgrepo.NewPostgres(pool, q), billingusecase.TrialHooks{Service: billing}).
		CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Başlat Demo " + suffix, OwnerUserID: owner.ID, TrialPriceUUID: price})
	if err != nil {
		t.Fatal(err)
	}
	p.Start(org.UUID, org.Slug, pw, owner.ID)
	for {
		var action string
		err := pool.QueryRow(ctx, `SELECT action FROM activity_events WHERE resource_uuid = $1 AND action LIKE 'organizations.demo_data_%' ORDER BY id DESC LIMIT 1`, org.UUID).Scan(&action)
		if err == nil {
			if action != "organizations.demo_data_finished" {
				t.Fatalf("fill outcome %s", action)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("no demo outcome event")
		case <-time.After(200 * time.Millisecond):
		}
	}
	var hashed string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = $1`, Accounts(org.Slug)[0].Email).Scan(&hashed); err != nil {
		t.Fatal(err)
	}
	if ok, err := password.Verify(hashed, pw); err != nil || !ok {
		t.Fatal("demo staff cannot sign in with the returned password")
	}
}

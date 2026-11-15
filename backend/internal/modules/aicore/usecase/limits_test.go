package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	airepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/jackc/pgx/v5/pgtype"
)

// usedToday records n finished calls for org today, as a history import would.
func usedToday(t *testing.T, svc *Service, org int64, n int) {
	t.Helper()
	_, err := svc.Repo.Billing.Pool.Exec(context.Background(), `INSERT INTO ai_runs(organization_id,task,provider,model,status,input)
		SELECT $1,'triage','anthropic','m','completed','x' FROM generate_series(1,$2)`, org, n)
	if err != nil {
		t.Fatal(err)
	}
}

func followPlatform(t *testing.T, svc *Service, org int64) {
	t.Helper()
	if _, err := svc.Repo.SavePolicy(context.Background(), db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true}); err != nil {
		t.Fatal(err)
	}
}

func override(t *testing.T, svc *Service, org, actor int64, key string, limit int64, unlimited bool) {
	t.Helper()
	g := billingmodel.Grant{Unlimited: unlimited}
	if !unlimited {
		g.Limit = &limit
	}
	if err := svc.Repo.Entitlements.SetOverride(context.Background(), org, key, &billingmodel.Override{Grant: g, Reason: "CF-229 test"}, actor); err != nil {
		t.Fatal(err)
	}
}

// CF-229: the platform's unlimited daily calls are not capped by the former
// tenant default (100) once the tenant policy follows the platform.
func TestPlatformUnlimitedDailyCallsNotBlockedByTenantDefault(t *testing.T) {
	svc, fake, org, actor := aiFixture(t)
	ctx := context.Background()
	followPlatform(t, svc, org)
	override(t, svc, org, actor, airepo.FeatureDailyCalls, 0, true)
	usedToday(t, svc, org, 150)
	if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "hello"}); err != nil || fake.calls.Load() != 1 {
		t.Fatalf("unlimited blocked: %v", err)
	}
	p, err := svc.Repo.Policy(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	limits, err := svc.Repo.Limits(ctx, p)
	if err != nil || !limits.Platform.DailyCalls.Unlimited || !limits.Effective.DailyCalls.Unlimited {
		t.Fatalf("%+v %v", limits, err)
	}
}

// The tenant can lower the platform limit but never raise it.
func TestTenantLowersButCannotExceedPlatformLimit(t *testing.T) {
	svc, fake, org, actor := aiFixture(t)
	ctx := context.Background()
	override(t, svc, org, actor, airepo.FeatureDailyCalls, 5, false)
	// Tenant asks for 100000: the platform's 5 still applies.
	if _, err := svc.Repo.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true, DailyCalls: pgtype.Int4{Int32: 100000, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	usedToday(t, svc, org, 5)
	if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "over"}); !errors.Is(err, model.ErrDaily) || fake.calls.Load() != 0 {
		t.Fatalf("tenant exceeded the platform: %v", err)
	}
	// Platform unlimited, tenant 3: the tenant's lower limit applies.
	override(t, svc, org, actor, airepo.FeatureDailyCalls, 0, true)
	if _, err := svc.Repo.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true, DailyCalls: pgtype.Int4{Int32: 3, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "lowered"}); !errors.Is(err, model.ErrDaily) || fake.calls.Load() != 0 {
		t.Fatalf("tenant limit ignored: %v", err)
	}
	// Platform budget below the tenant's: the platform wins.
	followPlatform(t, svc, org)
	override(t, svc, org, actor, airepo.FeatureDailyBudget, 1, false)
	if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "budget"}); !errors.Is(err, model.ErrBudget) || fake.calls.Load() != 0 {
		t.Fatalf("platform budget ignored: %v", err)
	}
}

// One organization's platform override and usage do not reach another.
func TestDailyLimitsAreTenantIsolated(t *testing.T) {
	a, fakeA, orgA, actorA := aiFixture(t)
	b, fakeB, orgB, actorB := aiFixture(t)
	ctx := context.Background()
	followPlatform(t, a, orgA)
	followPlatform(t, b, orgB)
	override(t, a, orgA, actorA, airepo.FeatureDailyCalls, 0, true)
	override(t, b, orgB, actorB, airepo.FeatureDailyCalls, 2, false)
	usedToday(t, a, orgA, 150)
	usedToday(t, b, orgB, 2)
	if _, err := a.Run(ctx, orgA, actorA, model.RunInput{Task: "reply", Input: "a"}); err != nil || fakeA.calls.Load() != 1 {
		t.Fatalf("org A: %v", err)
	}
	if _, err := b.Run(ctx, orgB, actorB, model.RunInput{Task: "reply", Input: "b"}); !errors.Is(err, model.ErrDaily) || fakeB.calls.Load() != 0 {
		t.Fatalf("org B got org A's unlimited: %v", err)
	}
}

// CF-228: batch work stops at its share of the daily limit while live AI of
// the same organization keeps answering; batch calls are logged as such.
func TestBatchShareKeepsLiveAIWorking(t *testing.T) {
	svc, fake, org, actor := aiFixture(t)
	ctx := context.Background()
	override(t, svc, org, actor, airepo.FeatureDailyCalls, 10, false)
	if _, err := svc.Repo.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true, BatchSharePercent: pgtype.Int2{Int16: 40, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	batch := ai.WithBatch(ctx)
	for i := 0; i < 4; i++ {
		if _, err := svc.Run(batch, org, actor, model.RunInput{Task: "reply", Input: "toplu"}); err != nil {
			t.Fatalf("batch call %d within the share: %v", i, err)
		}
	}
	if _, err := svc.Run(batch, org, actor, model.RunInput{Task: "reply", Input: "toplu"}); !errors.Is(err, model.ErrDaily) || fake.calls.Load() != 4 {
		t.Fatalf("batch exceeded its share: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "canlı"}); err != nil {
			t.Fatalf("live call %d blocked by batch work: %v", i, err)
		}
	}
	// The total limit still applies to live calls.
	if _, err := svc.Run(ctx, org, actor, model.RunInput{Task: "reply", Input: "canlı"}); !errors.Is(err, model.ErrDaily) {
		t.Fatalf("total limit ignored: %v", err)
	}
	var lanes map[string]int
	rows, err := svc.Repo.Billing.Pool.Query(ctx, "SELECT lane,count(*) FROM ai_runs WHERE organization_id=$1 AND status='completed' GROUP BY lane", org)
	if err != nil {
		t.Fatal(err)
	}
	lanes = map[string]int{}
	for rows.Next() {
		var lane string
		var n int
		if err := rows.Scan(&lane, &n); err != nil {
			t.Fatal(err)
		}
		lanes[lane] = n
	}
	rows.Close()
	if lanes[ai.LaneBatch] != 4 || lanes[ai.LaneLive] != 6 {
		t.Fatalf("lanes %v", lanes)
	}
	// The policy shows the batch share; an omitted share keeps it.
	followPlatform(t, svc, org)
	p, err := svc.Repo.Policy(ctx, org)
	if err != nil || p.BatchSharePercent != 40 {
		t.Fatalf("share not kept: %d %v", p.BatchSharePercent, err)
	}
	if _, err := svc.Repo.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, BatchSharePercent: pgtype.Int2{Int16: 95, Valid: true}}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("share above 90 accepted: %v", err)
	}
}

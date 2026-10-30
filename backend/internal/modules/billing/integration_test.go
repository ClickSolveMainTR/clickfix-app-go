package billing_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
)

type billingFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	q          *db.Queries
	svc        *usecase.Service
	ent        *entitlements.Service
	actor, org int64
	orgUUID    uuid.UUID
	plan       model.Plan
	version    model.Version
	sub        model.Subscription
	input      model.VersionInput
}

func fixture(t *testing.T) *billingFixture {
	t.Helper()
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
	f := &billingFixture{ctx: ctx, pool: pool, q: db.New(pool)}
	repo := repository.New(pool, f.q)
	f.svc = usecase.New(repo)
	var cache *redis.Client
	if raw := os.Getenv("TEST_REDIS_URL"); raw != "" {
		opt, err := redis.ParseURL(raw)
		if err != nil {
			t.Fatal(err)
		}
		cache = redis.NewClient(opt)
		t.Cleanup(func() { _ = cache.Close() })
	}
	f.ent = entitlements.New(repo, cache, "s4-test-"+uuid.NewString())
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'test-hash','Billing','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Billing test') RETURNING id,uuid", uuid.NewString()).Scan(&f.org, &f.orgUUID); err != nil {
		t.Fatal(err)
	}
	f.plan, err = f.svc.Create(ctx, model.PlanInput{Code: "test-" + uuid.NewString(), Name: "Fixture", Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	fs, err := f.svc.Features(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.input = model.VersionInput{Prices: []model.Price{{Interval: "monthly", Currency: "TRY", Amount: 10000, VAT: 2000}}}
	for _, feature := range fs {
		g := model.Grant{Feature: feature.Key, Enforcement: "hard", WarnPercent: 80}
		if feature.Kind == "boolean" {
			v := true
			g.Enabled = &v
		} else {
			v := int64(10)
			if feature.Key == "ai.credits" {
				v = 0
			}
			g.Limit = &v
		}
		f.input.Entitlements = append(f.input.Entitlements, g)
	}
	f.version, err = f.svc.SaveVersion(ctx, f.plan.UUID, uuid.Nil, f.input)
	if err != nil {
		t.Fatal(err)
	}
	f.version, err = f.svc.Publish(ctx, f.plan.UUID, f.version.UUID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	f.sub, err = f.svc.CreateSubscription(ctx, model.CreateSubscription{OrganizationUUID: f.orgUUID, VersionUUID: f.version.UUID, PriceUUID: f.version.Prices[0].UUID, Status: "active", PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now().Add(time.Hour)}, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func TestCatalogPublishingAndGrandfathering(t *testing.T) {
	f := fixture(t)
	f.input.Prices[0].Amount = 20000
	next, err := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Publish(f.ctx, f.plan.UUID, next.UUID, f.actor); err != nil {
		t.Fatal(err)
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub.VersionUUID != f.version.UUID || sub.PriceUUID != f.version.Prices[0].UUID {
		t.Fatalf("grandfathering: %+v %v", sub, err)
	}
	if _, err = f.svc.SaveVersion(f.ctx, f.plan.UUID, f.version.UUID, f.input); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("published edit: %v", err)
	}
	migration := model.Migration{VersionUUID: next.UUID, SubscriptionUUIDs: []uuid.UUID{sub.UUID}, OrganizationUUIDs: []uuid.UUID{f.orgUUID}, Reason: "Explicit migration"}
	if err = f.svc.ScheduleMigration(f.ctx, migration, f.actor); err != nil {
		t.Fatal(err)
	}
	sub, err = f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub.VersionUUID != f.version.UUID || sub.PendingVersionUUID == nil || *sub.PendingVersionUUID != next.UUID {
		t.Fatalf("scheduled migration: %+v %v", sub, err)
	}
	other, err := f.svc.Create(f.ctx, model.PlanInput{Code: "test-" + uuid.NewString(), Name: "Hidden", Visibility: "hidden"})
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := f.svc.List(f.ctx, true, other.Code, 100, 0)
	if err != nil || len(items) != 0 {
		t.Fatalf("hidden public: %v %v", items, err)
	}
	if err = f.svc.Archive(f.ctx, f.plan.UUID); err != nil {
		t.Fatal(err)
	}
	items, _, err = f.svc.List(f.ctx, true, f.plan.Code, 100, 0)
	if err != nil || len(items) != 0 {
		t.Fatalf("archived public: %v %v", items, err)
	}
}
func TestEntitlementResolutionAndAtomicLimit(t *testing.T) {
	f := fixture(t)
	if err := f.ent.Require(f.ctx, f.org, "ai.enabled"); err != nil {
		t.Fatal(err)
	}
	// Cached plan -> add-on -> override precedence and revision invalidation.
	if _, err := f.pool.Exec(f.ctx, `WITH addon AS (INSERT INTO billing_addons(code,name,feature_id,units,amount_minor,currency)
 SELECT $1,'Test addon',id,5,100,'TRY' FROM billing_features WHERE key='ai.tokens' RETURNING id)
 INSERT INTO billing_subscription_items(organization_id,subscription_id,addon_id,quantity,unit_amount_minor,currency)
 SELECT $2,s.id,a.id,2,100,'TRY' FROM billing_subscriptions s CROSS JOIN addon a WHERE s.uuid=$3`, uuid.NewString(), f.org, f.sub.UUID); err != nil {
		t.Fatal(err)
	}
	snap, err := f.ent.Resolve(f.ctx, f.org)
	if err != nil || snap.Values["ai.tokens"].Limit != 20 {
		t.Fatalf("addon: %+v %v", snap, err)
	}
	limit := int64(10)
	if err = f.ent.SetOverride(f.ctx, f.org, "ai.tokens", &model.Override{Grant: model.Grant{Limit: &limit}, Reason: "Contract limit"}, f.actor); err != nil {
		t.Fatal(err)
	}
	snap, err = f.ent.Resolve(f.ctx, f.org)
	if err != nil || snap.Values["ai.tokens"].Limit != 10 {
		t.Fatalf("override: %+v %v", snap, err)
	}
	var success atomic.Int64
	var wg sync.WaitGroup
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := f.ent.Consume(f.ctx, f.org, "ai.tokens", 1, fmt.Sprintf("token-%d", n))
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, entitlement.ErrDenied) {
				t.Errorf("consume: %v", err)
			}
		}(n)
	}
	wg.Wait()
	if success.Load() != 10 {
		t.Fatalf("concurrent quota accepted %d, want 10", success.Load())
	}
	var replayKey string
	if err = f.pool.QueryRow(f.ctx, "SELECT idempotency_key FROM billing_usage_records WHERE organization_id=$1 ORDER BY id LIMIT 1", f.org).Scan(&replayKey); err != nil {
		t.Fatal(err)
	}
	replay, err := f.ent.Consume(f.ctx, f.org, "ai.tokens", 1, replayKey)
	if err != nil || replay.Used != 10 {
		t.Fatalf("idempotent replay: %+v %v", replay, err)
	}
	if _, err = f.ent.Consume(f.ctx, f.org, "ai.tokens", 2, replayKey); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("idempotency mismatch accepted: %v", err)
	}
	if err = f.ent.SetOverride(f.ctx, f.org, "ai.tokens", nil, f.actor); err != nil {
		t.Fatal(err)
	}
	snap, err = f.ent.Resolve(f.ctx, f.org)
	if err != nil || snap.Values["ai.tokens"].Limit != 20 {
		t.Fatalf("delete override: %+v %v", snap, err)
	}
}
func TestLifecycleAuditReadOnlyAndTenantIsolation(t *testing.T) {
	f := fixture(t)
	if _, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "suspended", Reason: "Invalid jump"}, f.actor); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("invalid transition: %v", err)
	}
	var other int64
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Other') RETURNING id", uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Transition(f.ctx, other, f.sub.UUID, model.Transition{To: "past_due", Reason: "Wrong org"}, f.actor); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("tenant isolation: %v", err)
	}
	grace := time.Now().Add(time.Minute)
	for _, in := range []model.Transition{{To: "past_due", Reason: "Payment failed"}, {To: "grace", Reason: "Dunning", GraceEndsAt: &grace}, {To: "suspended", Reason: "Grace ended"}} {
		if _, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, in, f.actor); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.ent.RequireWritable(f.ctx, f.org); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("suspended writable: %v", err)
	}
	history, err := f.svc.History(f.ctx, f.org, f.sub.UUID)
	if err != nil || len(history) != 4 {
		t.Fatalf("history len=%d err=%v", len(history), err)
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM outbox_events WHERE event_name='subscription.suspended' AND payload->'data'->>'organization_id'=$1", fmt.Sprint(f.org)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox %d %v", count, err)
	}
	// Suspension does not mutate organizations.status, so reads and inbound paths keep resolving.
	var status string
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM organizations WHERE id=$1", f.org).Scan(&status); err != nil || status != "active" {
		t.Fatalf("org status: %s %v", status, err)
	}
}
func TestScheduledCancellationAndOutboxTenant(t *testing.T) {
	f := fixture(t)
	if _, err := f.svc.Cancel(f.ctx, f.org, f.sub.UUID, false, f.actor); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Advance(f.ctx, f.org, f.sub.UUID, f.sub.PeriodEnd.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Advance(f.ctx, f.org, f.sub.UUID, f.sub.PeriodEnd.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	history, err := f.svc.History(f.ctx, f.org, f.sub.UUID)
	if err != nil || len(history) != 2 || history[0].To != "canceled" {
		t.Fatalf("history %+v %v", history, err)
	}
	bus := events.NewBus(nil)
	var seen bool
	bus.Subscribe("subscription.canceled", func(_ context.Context, e events.Event) error {
		if e.Payload["organization_id"] == f.org {
			seen = e.TenantID != nil && *e.TenantID == f.org
		}
		return nil
	})
	pub := outbox.NewPublisher(outbox.NewStore(f.pool, f.q), bus, nil, outbox.WithBatchSize(1000))
	if _, err = pub.Drain(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("outbox lost tenant scope")
	}
}
func TestBillingRoutesAuthorizationAndPublicFiltering(t *testing.T) {
	f := fixture(t)
	mux := http.NewServeMux()
	billingmodule.RegisterRoutes(mux, handler.New(f.svc, f.ent, nil), nil, nil, nil)
	for _, path := range []string{"/v1/platform/plans", "/v1/platform/organizations/" + f.orgUUID.String() + "/entitlements"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("%s => %d", path, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/v1/public/pricing?q="+f.plan.Code, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), f.version.UUID.String()) {
		t.Fatalf("pricing %d %s", w.Code, w.Body.String())
	}
}

func TestOverrideExpirySoftLimitsAndTransactionalRollback(t *testing.T) {
	f := fixture(t)
	off := false
	expiry := time.Now().Add(300 * time.Millisecond)
	if err := f.ent.SetOverride(f.ctx, f.org, "ai.enabled", &model.Override{Grant: model.Grant{Enabled: &off}, Reason: "Temporary disable", ExpiresAt: &expiry}, f.actor); err != nil {
		t.Fatal(err)
	}
	if err := f.ent.Require(f.ctx, f.org, "ai.enabled"); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("expected disabled: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	if err := f.ent.Require(f.ctx, f.org, "ai.enabled"); err != nil {
		t.Fatalf("expired cached override: %v", err)
	}
	// Outbox/history write failure must roll back the state mutation.
	if _, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "past_due", Reason: "Invalid actor FK"}, -1); err != nil {
		t.Fatal(err)
	}
	// Negative/zero actors are background workers, so use a nonexistent positive FK instead.
	if _, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "grace", Reason: "Must rollback", GraceEndsAt: ptrTime(time.Now().Add(time.Hour))}, 9223372036854775807); err == nil {
		t.Fatal("expected actor FK failure")
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub.Status != "past_due" {
		t.Fatalf("partial commit: %+v %v", sub, err)
	}
}
func ptrTime(v time.Time) *time.Time { return &v }
func TestRenewalAppliesScheduledVersion(t *testing.T) {
	f := fixture(t)
	f.input.Prices[0].Amount = 50000
	next, err := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if err != nil {
		t.Fatal(err)
	}
	next, err = f.svc.Publish(f.ctx, f.plan.UUID, next.UUID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ScheduleMigration(f.ctx, model.Migration{VersionUUID: next.UUID, SubscriptionUUIDs: []uuid.UUID{f.sub.UUID}, OrganizationUUIDs: []uuid.UUID{f.orgUUID}, Reason: "Renewal opt-in"}, f.actor); err != nil {
		t.Fatal(err)
	}
	start := f.sub.PeriodEnd
	end := start.AddDate(0, 1, 0)
	sub, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "active", Reason: "Settled renewal", PeriodStart: &start, PeriodEnd: &end}, f.actor)
	if err != nil || sub.VersionUUID != next.UUID || sub.PriceUUID != next.Prices[0].UUID || sub.PendingVersionUUID != nil {
		t.Fatalf("renewal: %+v %v", sub, err)
	}
	if _, err = f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "active", Reason: "Duplicate period", PeriodStart: &start, PeriodEnd: &end}, f.actor); err == nil {
		t.Fatal("duplicate renewal accepted")
	}
}

func TestResourceQuotaSerializesMutationAndRollsBack(t *testing.T) {
	f := fixture(t)
	var role int64
	if err := f.pool.QueryRow(f.ctx, "SELECT id FROM roles WHERE slug='member' AND scope='tenant' AND organization_id IS NULL").Scan(&role); err != nil {
		t.Fatal(err)
	}
	count := func(ctx context.Context, q *db.Queries) (int64, error) {
		return q.CountOrganizationMembersByOrg(ctx, db.CountOrganizationMembersByOrgParams{OrganizationID: f.org})
	}
	var success atomic.Int64
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := f.ent.WithinLimit(f.ctx, f.org, "seats", 1, count, func(ctx context.Context, q *db.Queries) error {
				user, err := q.CreateUser(ctx, db.CreateUserParams{Email: fmt.Sprintf("%s-%d@example.test", f.orgUUID, n), PasswordHash: "test-hash", Name: "Seat", Surname: "Test", Status: "active"})
				if err != nil {
					return err
				}
				_, err = q.AddOrganizationMember(ctx, db.AddOrganizationMemberParams{OrganizationID: f.org, UserID: user.ID, RoleID: role})
				return err
			})
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, entitlement.ErrDenied) {
				t.Errorf("seat guard: %v", err)
			}
		}(n)
	}
	wg.Wait()
	if success.Load() != 10 {
		t.Fatalf("seat quota overshoot: %d", success.Load())
	}
	// A mutation failure rolls back its own writes and never reserves a phantom seat.
	limit := int64(11)
	if err := f.ent.SetOverride(f.ctx, f.org, "seats", &model.Override{Grant: model.Grant{Limit: &limit}, Reason: "One more seat"}, f.actor); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("business write failed")
	if err := f.ent.WithinLimit(f.ctx, f.org, "seats", 1, count, func(context.Context, *db.Queries) error { return fail }); !errors.Is(err, fail) {
		t.Fatal(err)
	}
	used, err := count(f.ctx, f.q)
	if err != nil || used != 10 {
		t.Fatalf("count=%d err=%v", used, err)
	}
}

func TestRedisFailureFallsBackToDatabase(t *testing.T) {
	f := fixture(t)
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 10 * time.Millisecond, ReadTimeout: 10 * time.Millisecond, WriteTimeout: 10 * time.Millisecond})
	defer func() { _ = broken.Close() }()
	svc := entitlements.New(repository.New(f.pool, f.q), broken, "s4-unavailable")
	if err := svc.Require(f.ctx, f.org, "ai.enabled"); err != nil {
		t.Fatalf("cache outage blocked DB resolution: %v", err)
	}
}

package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func TestMeteringDeliveryAndPeriodClose(t *testing.T) {
	f := fixture(t)
	bus := events.NewBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.ent.RegisterMetering(bus)
	event := events.New(events.BillingUsageRecorded).WithTenant(f.org).WithPayload(map[string]any{"feature": "ai.tokens", "quantity": int64(8)})
	for range 2 {
		if err := bus.Publish(f.ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := f.ent.Usage(f.ctx, f.org)
	if err != nil || len(usage) != 1 || usage[0].Quantity != 8 {
		t.Fatalf("usage: %+v %v", usage, err)
	}
	// A hard denial emits one durable signal; duplicate deliveries do not count.
	denied := events.New(events.BillingUsageRecorded).WithTenant(f.org).WithPayload(map[string]any{"feature": "ai.tokens", "quantity": int64(3)})
	for range 2 {
		if err = bus.Publish(f.ctx, denied); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_usage_denials WHERE organization_id=$1", f.org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("denials=%d %v", count, err)
	}
	// Soft quota preserves usage and emits warning at the crossing.
	limit := int64(5)
	if err = f.ent.SetOverride(f.ctx, f.org, "ai.tokens", &model.Override{Grant: model.Grant{Limit: &limit}, Reason: "fixture soft quota"}, f.actor); err != nil {
		t.Fatal(err)
	}
	// Overrides intentionally retain plan enforcement; create a soft plan revision.
	f.input.Entitlements[0].Enforcement = "soft"
	for i := range f.input.Entitlements {
		if f.input.Entitlements[i].Feature == "sms" {
			f.input.Entitlements[i].Enforcement = "soft"
		}
	}
	v, err := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.Publish(f.ctx, f.plan.UUID, v.UUID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	// Published-plan audit is immutable; use a second isolated org for soft quota.
	var org int64
	var orgUUID uuid.UUID
	if err = f.pool.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'soft') RETURNING id,uuid", uuid.NewString()).Scan(&org, &orgUUID); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateSubscription(f.ctx, model.CreateSubscription{OrganizationUUID: orgUUID, VersionUUID: v.UUID, PriceUUID: v.Prices[0].UUID, Status: "active", PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now().Add(time.Hour)}, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	d, err := f.ent.Consume(f.ctx, org, "sms", 12, "soft-event")
	if err != nil || !d.Allowed || d.WarningPercent != 100 || d.Used != 12 {
		t.Fatalf("soft %+v %v", d, err)
	}
	daily, err := f.ent.Daily(f.ctx, f.org, time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(daily) != 1 || daily[0].Quantity != 8 {
		t.Fatalf("daily %+v %v", daily, err)
	}
	// Source event from a past month is aggregated by occurrence, not ingestion.
	// Mid-month of the previous month: AddDate(0, -1, 0) on the 31st normalizes back into this month.
	now := time.Now().UTC()
	past := time.Date(now.Year(), now.Month()-1, 15, 12, 0, 0, 0, time.UTC)
	_, err = f.ent.ConsumeAt(f.ctx, f.org, "sms", 2, "past-event", past)
	if err != nil {
		t.Fatal(err)
	}
	usage, err = f.ent.Usage(f.ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	var period uuid.UUID
	for _, u := range usage {
		if u.Key == "sms" {
			period = u.PeriodUuid
		}
	}
	if period == uuid.Nil {
		t.Fatal("historical period missing")
	}
	for range 2 {
		if err = f.ent.ClosePeriod(f.ctx, f.org, period, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.ent.ConsumeAt(f.ctx, f.org, "sms", 1, "late-closed", past)
	if !errors.Is(err, model.ErrConflict) {
		t.Fatalf("closed write: %v", err)
	}
	// Exact delivery replay still succeeds after close.
	if _, err = f.ent.ConsumeAt(f.ctx, f.org, "sms", 2, "past-event", past); err != nil {
		t.Fatal(err)
	}
	if err = f.ent.ClosePeriod(f.ctx, org, period, time.Now()); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("cross-org close: %v", err)
	}
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_usage_settlements WHERE organization_id=$1", f.org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("settlement %d %v", count, err)
	}
}

func TestGaugeKeepsLatestObservation(t *testing.T) {
	f := fixture(t)
	now := time.Now().UTC()
	for _, in := range []entitlements.MeterEvent{
		{Feature: "seats", Quantity: 7, EventKey: "seat-new", OccurredAt: now},
		{Feature: "seats", Quantity: 9, EventKey: "seat-old", OccurredAt: now.Add(-time.Minute)},
		{Feature: "storage.bytes", Quantity: 0, EventKey: "empty-storage", OccurredAt: now},
	} {
		if _, err := f.ent.Meter(f.ctx, f.org, in); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := f.ent.Usage(f.ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range usage {
		if u.Key == "seats" && u.Quantity != 7 {
			t.Fatalf("out of order overwritten: %+v", u)
		}
	}
	_, err = f.ent.Meter(f.ctx, f.org, entitlements.MeterEvent{Feature: "seats", Quantity: 11, EventKey: "seat-over", OccurredAt: now})
	if !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("hard gauge: %v", err)
	}
}

func trialFixture(t *testing.T) *billingFixture {
	t.Helper()
	f := fixture(t)
	if _, err := f.svc.Cancel(f.ctx, f.org, f.sub.UUID, true, f.actor); err != nil {
		t.Fatal(err)
	}
	f.input.TrialDays = 14
	v, err := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if err != nil {
		t.Fatal(err)
	}
	f.version, err = f.svc.Publish(f.ctx, f.plan.UUID, v.UUID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.StartTrial(f.ctx, f.org, f.version.Prices[0].UUID, f.actor); err != nil {
		t.Fatal(err)
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	f.sub = *sub
	return f
}
func TestTrialEnrollmentRemindersAndExpiry(t *testing.T) {
	f := trialFixture(t)
	if f.sub.TrialEndsAt == nil || f.sub.TrialDaysRemaining != 14 {
		t.Fatalf("trial bounds %+v", f.sub)
	}
	if err := f.svc.StartTrial(f.ctx, f.org, f.version.Prices[0].UUID, f.actor); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("retrial %v", err)
	}
	for _, days := range []int{7, 3, 1} {
		now := f.sub.TrialEndsAt.Add(-time.Duration(days) * 24 * time.Hour)
		for range 2 {
			if err := f.svc.TrialSweep(f.ctx, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_trial_reminders WHERE organization_id=$1", f.org).Scan(&count); err != nil || count != 3 {
		t.Fatalf("reminders=%d %v", count, err)
	}
	end := f.sub.TrialEndsAt.Add(7 * 24 * time.Hour)
	if err := f.svc.ExtendTrial(f.ctx, f.org, f.sub.UUID, end, "Sales extension", f.actor); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.TrialSweep(f.ctx, f.sub.TrialEndsAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub == nil || sub.Status != "trialing" {
		t.Fatalf("extension %+v %v", sub, err)
	}
	if err = f.svc.TrialSweep(f.ctx, end.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&status); err != nil || status != "expired" {
		t.Fatalf("expiry=%s %v", status, err)
	}
	// Expired trial cannot be reused even after the live subscription disappears.
	if err = f.svc.StartTrial(f.ctx, f.org, f.version.Prices[0].UUID, f.actor); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("expired retrial %v", err)
	}
}

func TestTrialSignupTransactionRollback(t *testing.T) {
	f := trialFixture(t)
	repo := repository.New(f.pool, f.q)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	var org int64
	if err = tx.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'signup rollback') RETURNING id", uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	// StartTrial's transaction-bound path must not escape the signup transaction.
	hooks := usecase.TrialHooks{Service: usecase.New(repo)}
	if err = hooks.OnCreatedWithTrial(f.ctx, tx, orgmodel.Organization{ID: org}, orgmodel.User{ID: f.actor}, f.version.Prices[0].UUID); err != nil {
		t.Fatal(err)
	}
	var inside int
	if err = tx.QueryRow(f.ctx, "SELECT count(*) FROM billing_subscriptions WHERE organization_id=$1", org).Scan(&inside); err != nil || inside != 1 {
		t.Fatalf("signup trial missing %d %v", inside, err)
	}
	if err = tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_subscriptions WHERE organization_id=$1", org).Scan(&n); err != nil || n != 0 {
		t.Fatalf("signup rollback=%d %v", n, err)
	}
}

type timeoutGateway struct {
	*payments.Fake
	calls atomic.Int64
}

func (p *timeoutGateway) ChargeSavedCard(ctx context.Context, a payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	p.calls.Add(1)
	if _, err := p.Fake.ChargeSavedCard(ctx, a, in); err != nil {
		return payments.ChargeResult{}, err
	}
	return payments.ChargeResult{}, context.DeadlineExceeded
}
func TestGatewayAttemptConcurrencyAndReconciliation(t *testing.T) {
	f := fixture(t)
	reg := payments.NewRegistry(f.svc, false)
	gateway := &timeoutGateway{Fake: payments.NewFake("https://example.test")}
	if err := reg.Register(gateway); err != nil {
		t.Fatal(err)
	}
	executor := usecase.NewGatewayExecutor(repository.New(f.pool, f.q), reg)
	in := payments.ChargeRequest{Amount: 100, Currency: "TRY", Card: payments.SavedCard{OwnerKey: "owner", Token: "token"}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = executor.Charge(f.ctx, f.org, "same-key", gateway, payments.Account{}, in)
		}()
	}
	wg.Wait()
	if n := gateway.calls.Load(); n != 1 {
		t.Fatalf("duplicate external charges: %d", n)
	}
	result, err := executor.ReconcileCharge(f.ctx, f.org, "same-key", gateway, payments.Account{}, in)
	if err != nil || result.PaidAmount != 100 {
		t.Fatalf("reconcile %+v %v", result, err)
	}
	for range 2 {
		if _, err = executor.Charge(f.ctx, f.org, "same-key", gateway, payments.Account{}, in); err != nil {
			t.Fatal(err)
		}
	}
	in.Amount = 101
	if _, err = executor.Charge(f.ctx, f.org, "same-key", gateway, payments.Account{}, in); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("changed key payload %v", err)
	}
	var raw []byte
	if err = f.pool.QueryRow(f.ctx, "SELECT result FROM billing_payment_attempts WHERE organization_id=$1", f.org).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved["Card"]; ok {
		t.Fatal("card token leaked into audit")
	}
	body, _ := json.Marshal(payments.Notification{OrderRef: "ref", EventKey: "event", Status: payments.StatusSucceeded, Amount: 100})
	headers := http.Header{}
	account := payments.Account{Credentials: map[string]string{"secret_key": "test-secret"}}
	if _, err = executor.VerifyCallback(gateway, account, headers, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatal("unsigned callback accepted")
	}
	headers.Set("X-Fake-Signature", payments.FakeSignature("test-secret", body))
	if _, err = executor.VerifyCallback(gateway, account, headers, body); err != nil {
		t.Fatal(err)
	}
}

func TestTrialConvertsOnlyAfterGatewaySettlement(t *testing.T) {
	f := trialFixture(t)
	vault, err := payments.NewCardVault("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	cardID := uuid.New()
	merchant := uuid.New()
	owner, token, err := vault.Seal(fmt.Sprintf("org:%d", f.org), cardID, payments.SavedCard{OwnerKey: "owner", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	var method int64
	if err = f.pool.QueryRow(f.ctx, "INSERT INTO billing_payment_methods(uuid,organization_id,provider,merchant_account_uuid,customer_token_enc,card_token_enc,token_hash,last4) VALUES($1,$2,'fake',$3,$4,$5,$6,'4242') RETURNING id", cardID, f.org, merchant, owner, token, vault.TokenHash(fmt.Sprintf("org:%d", f.org), "fake", "token")).Scan(&method); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET payment_method_id=$1,auto_pay=true,auto_pay_consented_at=now() WHERE uuid=$2", method, f.sub.UUID); err != nil {
		t.Fatal(err)
	}
	reg := payments.NewRegistry(f.svc, false)
	if err = reg.Register(payments.NewFake("https://example.test")); err != nil {
		t.Fatal(err)
	}
	f.svc.SetTrialPayments(&usecase.TrialGateway{Executor: usecase.NewGatewayExecutor(repository.New(f.pool, f.q), reg), Vault: vault, Account: func(context.Context, string, string) (payments.Account, error) { return payments.Account{}, nil }})
	for range 2 {
		if err = f.svc.TrialSweep(f.ctx, f.sub.TrialEndsAt.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub == nil || sub.Status != "active" {
		t.Fatalf("conversion %+v %v", sub, err)
	}
	var n int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_payment_attempts WHERE organization_id=$1 AND status='succeeded'", f.org).Scan(&n); err != nil || n != 1 {
		t.Fatalf("attempts=%d %v", n, err)
	}
}

type queuedNotifications struct{}

func (queuedNotifications) Enqueue(*asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}
func TestTrialNotificationReplayCreatesOnePerChannel(t *testing.T) {
	f := fixture(t)
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", f.org, f.actor); err != nil {
		t.Fatal(err)
	}
	svc := notifusecase.New(f.q, queuedNotifications{}, nil, nil)
	bus := events.NewBus(nil)
	billingmodule.RegisterTrialNotifications(bus, f.q, svc)
	event := events.New(events.BillingTrialReminder).WithTenant(f.org).WithPayload(map[string]any{"days": 3})
	for range 2 {
		if err := bus.Publish(f.ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM notifications WHERE user_id=$1 AND source_event='billing.trial_reminder'", f.actor).Scan(&count); err != nil || count != 2 {
		t.Fatalf("notification rows=%d %v", count, err)
	}
}

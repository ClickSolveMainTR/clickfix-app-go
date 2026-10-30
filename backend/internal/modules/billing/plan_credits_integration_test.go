package billing_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
)

func creditVersion(t *testing.T, f *billingFixture, credits, price int64) model.Version {
	t.Helper()
	in := f.input
	in.Entitlements = append([]model.Grant(nil), in.Entitlements...)
	in.Prices = append([]model.Price(nil), in.Prices...)
	in.Prices[0].Amount = price
	for i := range in.Entitlements {
		if in.Entitlements[i].Feature == "ai.credits" {
			in.Entitlements[i].Limit = &credits
		}
	}
	v, e := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, in)
	if e != nil {
		t.Fatal(e)
	}
	v, e = f.svc.Publish(f.ctx, f.plan.UUID, v.UUID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func creditBalance(t *testing.T, f *billingFixture, want int64) {
	t.Helper()
	w, e := f.svc.Wallet(f.ctx, f.org, 100, 0)
	if e != nil || w.AICredits != want {
		t.Fatalf("wallet=%d want=%d error=%v", w.AICredits, want, e)
	}
}
func TestPlanCreditsBackfillRenewalExpiry(t *testing.T) {
	f := fixture(t)
	v := creditVersion(t, f, 100, 10000)
	// Simulate a pre-fix current subscription without a plan lot.
	if _, e := f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET plan_version_id=(SELECT id FROM billing_plan_versions WHERE uuid=$1),price_id=(SELECT id FROM billing_prices WHERE uuid=$2) WHERE organization_id=$3", v.UUID, v.Prices[0].UUID, f.org); e != nil {
		t.Fatal(e)
	}
	if _, e := f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: 10000, Reason: "Manual production remediation", IdempotencyKey: "manual:2026-10-29:ai-credits-10000"}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if e := f.svc.BackfillOrganizationPlanCredits(f.ctx, f.org); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	// Exercise the paged deployment job as well as concurrent per-org retries.
	if e := f.svc.BackfillPlanCredits(f.ctx); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 10100)
	var count int64
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_credit_ledger WHERE organization_id=$1 AND starts_with(idempotency_key,'plan-credits:')", f.org).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	start := f.sub.PeriodEnd
	end := start.AddDate(0, 1, 0)
	if _, e := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "active", Reason: "Renewal", PeriodStart: &start, PeriodEnd: &end}, f.actor); e != nil {
		t.Fatal(e)
	}
	if e := f.svc.BackfillOrganizationPlanCredits(f.ctx, f.org); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 10200)
	if e := f.svc.ExpireWallet(f.ctx, f.org, end); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 10000)
	// Expiry sweep at the same boundary is idempotent.
	if e := f.svc.ExpireWallet(f.ctx, f.org, end); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 10000)
}
func TestPlanCreditsPaidUpgradeTopupAndReplay(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	v := creditVersion(t, f, 100, 20000)
	fake := payments.NewFake("https://example.test")
	configureGateway(t, f, fake)
	in := usecase.PlanChangeInput{PriceUUID: v.Prices[0].UUID, Seats: 1, IdempotencyKey: "credit-upgrade"}
	change, e := f.svc.ApplyPlanChange(f.ctx, f.org, in)
	if e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 0)
	var invoice uuid.UUID
	if e = f.pool.QueryRow(f.ctx, "SELECT uuid FROM billing_invoices WHERE id=$1", change.InvoiceID.Int64).Scan(&invoice); e != nil {
		t.Fatal(e)
	}
	payWithFake(t, f, fake, invoice, "credit-upgrade-payment")
	creditBalance(t, f, 100)
	if _, e = f.svc.ApplyPlanChange(f.ctx, f.org, in); e != nil {
		t.Fatal(e)
	}
	// Spend part of the grant, then upgrade again: only the allowance delta is added.
	if _, e = f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: -40, Reason: "AI usage", IdempotencyKey: "consume"}); e != nil {
		t.Fatal(e)
	}
	next := creditVersion(t, f, 250, 30000)
	change, e = f.svc.ApplyPlanChange(f.ctx, f.org, usecase.PlanChangeInput{PriceUUID: next.Prices[0].UUID, Seats: 1, IdempotencyKey: "second-credit-upgrade"})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.ctx, "SELECT uuid FROM billing_invoices WHERE id=$1", change.InvoiceID.Int64).Scan(&invoice); e != nil {
		t.Fatal(e)
	}
	payWithFake(t, f, fake, invoice, "second-credit-upgrade-payment")
	creditBalance(t, f, 210)
	if e = f.svc.BackfillPlanCredits(f.ctx); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 210)
	if e = f.svc.BackfillOrganizationPlanCredits(f.ctx, f.org); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 210)
	var expires time.Time
	if e = f.pool.QueryRow(f.ctx, "SELECT expires_at FROM billing_credit_ledger WHERE organization_id=$1 AND starts_with(idempotency_key,'plan-credits:') ORDER BY id DESC LIMIT 1", f.org).Scan(&expires); e != nil || !expires.Equal(f.sub.PeriodEnd) {
		t.Fatal(fmt.Sprint(expires), e)
	}
}

func TestPlanCreditsTrialActivationAndSamePeriodReplay(t *testing.T) {
	f := fixture(t)
	if _, e := f.svc.Cancel(f.ctx, f.org, f.sub.UUID, true, f.actor); e != nil {
		t.Fatal(e)
	}
	v := creditVersion(t, f, 100, 10000)
	start := time.Now().UTC().Add(-time.Minute)
	end := start.Add(24 * time.Hour)
	sub, e := f.svc.CreateSubscription(f.ctx, model.CreateSubscription{OrganizationUUID: f.orgUUID, VersionUUID: v.UUID, PriceUUID: v.Prices[0].UUID, Status: "trialing", PeriodStart: start, PeriodEnd: end, TrialEndsAt: &end}, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 100)
	if _, e = f.svc.Transition(f.ctx, f.org, sub.UUID, model.Transition{To: "active", Reason: "Trial payment settled"}, f.actor); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 100)
	if _, e = f.svc.Transition(f.ctx, f.org, sub.UUID, model.Transition{To: "past_due", Reason: "Payment overdue"}, f.actor); e != nil {
		t.Fatal(e)
	}
	grace := time.Now().Add(2 * time.Hour)
	if _, e = f.svc.Transition(f.ctx, f.org, sub.UUID, model.Transition{To: "grace", Reason: "Grace", GraceEndsAt: &grace}, f.actor); e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.Transition(f.ctx, f.org, sub.UUID, model.Transition{To: "active", Reason: "Recovery"}, f.actor); e != nil {
		t.Fatal(e)
	}
	creditBalance(t, f, 100)
	var count int64
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_credit_ledger WHERE organization_id=$1 AND starts_with(idempotency_key,'plan-credits:')", f.org).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}

func TestPlanCreditsZeroInvoiceIntervalUpgrade(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	f.input.Prices[0].Interval = "yearly"
	v := creditVersion(t, f, 100, 0)
	change, e := f.svc.ApplyPlanChange(f.ctx, f.org, usecase.PlanChangeInput{PriceUUID: v.Prices[0].UUID, Seats: 1, IdempotencyKey: "free-interval-change"})
	if e != nil || change.State != "applied" {
		t.Fatal(change, e)
	}
	creditBalance(t, f, 100)
	sub, e := f.svc.Subscription(f.ctx, f.org)
	if e != nil || !sub.PeriodStart.After(f.sub.PeriodStart) || !sub.PeriodEnd.After(f.sub.PeriodEnd) {
		t.Fatal(sub, e)
	}
	var expiry time.Time
	if e = f.pool.QueryRow(f.ctx, "SELECT expires_at FROM billing_credit_ledger WHERE organization_id=$1 AND starts_with(idempotency_key,'plan-credits:')", f.org).Scan(&expiry); e != nil || !expiry.Equal(sub.PeriodEnd) {
		t.Fatal(expiry, e)
	}
}

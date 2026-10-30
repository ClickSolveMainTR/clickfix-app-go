package billing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
)

func TestTrialInvoiceIsIdempotentAndStartsAtTrialEnd(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)

	// The fixture subscription is active: no trial invoice for it.
	if _, err := f.svc.TrialInvoice(f.ctx, f.org, f.sub.UUID); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("active subscription err=%v", err)
	}
	if _, err := f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "canceled", Reason: "trial checkout test"}, f.actor); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	trialEnd := now.Add(7 * 24 * time.Hour)
	sub, err := f.svc.CreateSubscription(f.ctx, model.CreateSubscription{OrganizationUUID: f.orgUUID, VersionUUID: f.version.UUID, PriceUUID: f.version.Prices[0].UUID, Status: "trialing", PeriodStart: now, PeriodEnd: trialEnd, TrialEndsAt: &trialEnd}, f.actor)
	if err != nil {
		t.Fatal(err)
	}

	first, err := f.svc.TrialInvoice(f.ctx, f.org, sub.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "open" || first.TotalMinor <= 0 || !first.PeriodStart.Equal(sub.PeriodEnd) {
		t.Fatalf("unexpected invoice %+v (trial end %s)", first, sub.PeriodEnd)
	}
	again, err := f.svc.TrialInvoice(f.ctx, f.org, sub.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if again.UUID != first.UUID {
		t.Fatalf("second call created another invoice: %s != %s", again.UUID, first.UUID)
	}
}

// Paying during the trial activates the first paid period, which starts at
// the trial end (in the future). The plan's entitlements must keep applying
// until then; they used to resolve to nothing because the paid period had
// not started yet, cutting a customer who paid early down to zero limits.
func TestEntitlementsKeepApplyingAfterEarlyTrialPayment(t *testing.T) {
	f := fixture(t)
	trialEnd := time.Now().UTC().Add(7 * 24 * time.Hour)
	if _, err := f.pool.Exec(f.ctx, `UPDATE billing_subscriptions SET status='active', trial_ends_at=$2,
		period_start=$2, period_end=$3 WHERE organization_id=$1`, f.org, trialEnd, trialEnd.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := f.q.BillingResolveEntitlements(f.ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Key == "seats" {
			found = true
			if row.ValueLimit != 10 {
				t.Fatalf("seats limit = %d during the prepaid trial, want the plan's 10", row.ValueLimit)
			}
		}
	}
	if !found {
		t.Fatal("seats feature not resolved")
	}
	snap, err := f.ent.Resolve(f.ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Writable {
		t.Fatal("subscription read-only during the prepaid trial")
	}
}

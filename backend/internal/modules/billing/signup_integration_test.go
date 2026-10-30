package billing_test

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/google/uuid"
)

// CF-42: a signup on a plan without a trial opens pending_payment for the
// payment window and its first invoice can be paid right away.
func TestSignupOnPlanWithoutTrialAwaitsPayment(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	var org int64
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Signup test') RETURNING id", uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveProfile(f.ctx, org, validBillingProfile()); err != nil {
		t.Fatal(err)
	}
	// The fixture version has no trial days.
	if err := f.svc.StartTrial(f.ctx, org, f.version.Prices[0].UUID, f.actor); err != nil {
		t.Fatal(err)
	}
	sub, err := f.svc.Subscription(f.ctx, org)
	if err != nil || sub == nil {
		t.Fatalf("subscription %+v err=%v", sub, err)
	}
	if sub.Status != "pending_payment" || sub.TrialEndsAt != nil {
		t.Fatalf("expected pending_payment without trial, got %+v", sub)
	}
	if got := sub.PeriodEnd.Sub(sub.PeriodStart); got != usecase.SignupPaymentWindow {
		t.Fatalf("payment window %s", got)
	}
	invoice, err := f.svc.TrialInvoice(f.ctx, org, sub.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.Status != "open" || invoice.TotalMinor <= 0 {
		t.Fatalf("invoice %+v", invoice)
	}
}

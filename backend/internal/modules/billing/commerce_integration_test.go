package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func validBillingProfile() usecase.ProfileInput {
	return usecase.ProfileInput{Kind: "company", LegalName: "Fixture Teknoloji AŞ", TaxID: "1234567890", TaxOffice: "Konak", CountryCode: "TR", Address: usecase.BillingAddress{Street: "Test Caddesi 1", District: "Konak", City: "İzmir", PostalCode: "35000"}, Emails: []string{"billing@example.test"}}
}
func configureInvoiceFixture(t *testing.T, f *billingFixture) {
	t.Helper()
	p := validBillingProfile()
	if _, err := f.svc.SaveProfile(f.ctx, f.org, p); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ConfigureInvoicing(&usecase.FakeEInvoiceProvider{}, p, "CFX", "test"); err != nil {
		t.Fatal(err)
	}
}
func commerceInvoice(t *testing.T, f *billingFixture, key string) db.BillingInvoice {
	t.Helper()
	now := time.Now().UTC()
	out, err := f.svc.GenerateInvoice(f.ctx, f.org, usecase.InvoiceInput{Kind: "manual", IdempotencyKey: key, Currency: "TRY", PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0), DueAt: now, Lines: []usecase.InvoiceLineInput{{Description: "ClickFix subscription", Quantity: 1, UnitAmountMinor: 10000, VATBasisPoints: 2000}}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func payCommerceInvoice(t *testing.T, f *billingFixture, i db.BillingInvoice) {
	t.Helper()
	if err := f.q.BillingPayInvoice(f.ctx, db.BillingPayInvoiceParams{OrganizationID: f.org, ID: i.ID}); err != nil {
		t.Fatal(err)
	}
}
func TestInvoiceNumberingDocumentsAndFrozenProfile(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	i := commerceInvoice(t, f, "document")
	payCommerceInvoice(t, f, i)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.svc.IssueInvoice(f.ctx, f.org, i.Uuid) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	record, lines, e := f.svc.InvoiceDetail(f.ctx, f.org, i.Uuid)
	if e != nil || record.EinvoiceStatus != "issued" || !record.Number.Valid || len(record.Number.String) != 16 || len(lines) != 1 {
		t.Fatalf("record %+v %v", record, e)
	}
	doc, e := f.svc.PrepareInvoice(f.ctx, f.org, i.Uuid)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(string(doc.Pdf), "%PDF-") || !strings.Contains(string(doc.Xml), "120.00") || !strings.Contains(string(doc.Html), "Fixture") {
		t.Fatalf("missing invoice artifacts: pdf=%d xml=%s html=%d", len(doc.Pdf), doc.Xml, len(doc.Html))
	}
	p := validBillingProfile()
	p.LegalName = "Edited name"
	if _, e = f.svc.SaveProfile(f.ctx, f.org, p); e != nil {
		t.Fatal(e)
	}
	again, e := f.svc.PrepareInvoice(f.ctx, f.org, i.Uuid)
	if e != nil || string(again.Xml) != string(doc.Xml) {
		t.Fatal("document changed", e)
	}
	if _, _, e = f.svc.InvoiceDetail(f.ctx, f.org+100000, i.Uuid); e == nil {
		t.Fatal("cross-tenant read")
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE billing_invoices SET subtotal_minor=1,tax_minor=0,total_minor=1 WHERE id=$1", i.ID); e == nil {
		t.Fatal("numbered invoice changed")
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE billing_invoice_documents SET xml='tampered' WHERE invoice_id=$1", i.ID); e == nil {
		t.Fatal("document changed")
	}
	second := commerceInvoice(t, f, "number-two")
	payCommerceInvoice(t, f, second)
	if e = f.svc.IssueInvoice(f.ctx, f.org, second.Uuid); e != nil {
		t.Fatal(e)
	}
	var n string
	if e = f.pool.QueryRow(f.ctx, "SELECT number FROM billing_invoices WHERE id=$1", second.ID).Scan(&n); e != nil || n == record.Number.String {
		t.Fatal("number reuse", e)
	}
	if e = f.svc.ConfigureInvoicing(nil, p, "CFX", "test"); e != nil {
		t.Fatal(e)
	}
	historical, e := f.svc.PrepareInvoice(f.ctx, f.org, i.Uuid)
	if e != nil || string(historical.Pdf) != string(doc.Pdf) {
		t.Fatal("archived invoice unavailable without provider", e)
	}
	if e = f.svc.ConfigureInvoicing(&usecase.FakeEInvoiceProvider{}, p, "CFX", "production"); !errors.Is(e, payments.ErrUnsupported) {
		t.Fatal("production fake", e)
	}
}
func TestBillingProfileValidationAndFrozenTCMB(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	for _, mutate := range []func(*usecase.ProfileInput){func(p *usecase.ProfileInput) { p.TaxID = "1234567891" }, func(p *usecase.ProfileInput) { p.Kind = "individual"; p.TaxID = "10000000147" }, func(p *usecase.ProfileInput) { p.Address.City = "" }, func(p *usecase.ProfileInput) { p.Emails = []string{"bad"} }, func(p *usecase.ProfileInput) { p.ExportExemption = true }} {
		p := validBillingProfile()
		mutate(&p)
		if _, e := f.svc.SaveProfile(f.ctx, f.org, p); !errors.Is(e, model.ErrInvalid) {
			t.Fatalf("invalid accepted %+v %v", p, e)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<Tarih_Date Date="%s"><Currency CurrencyCode="USD"><Unit>1</Unit><ForexSelling>40.12345678</ForexSelling></Currency><Currency CurrencyCode="EUR"><Unit>1</Unit><ForexSelling>45.5000</ForexSelling></Currency></Tarih_Date>`, time.Now().UTC().Format("01/02/2006"))
	}))
	defer srv.Close()
	if e := f.svc.FetchRates(f.ctx, srv.Client(), srv.URL); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	in := usecase.InvoiceInput{Kind: "manual", IdempotencyKey: "fx", Currency: "USD", PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0), DueAt: now, Lines: []usecase.InvoiceLineInput{{Description: "USD plan", Quantity: 1, UnitAmountMinor: 100, VATBasisPoints: 2000}}}
	i, e := f.svc.GenerateInvoice(f.ctx, f.org, in)
	if e != nil || i.SubtotalMinor != 4012 || i.TaxMinor != 802 || i.TotalMinor != 4814 {
		t.Fatalf("fx %+v %v", i, e)
	}
	again, e := f.svc.GenerateInvoice(f.ctx, f.org, in)
	if e != nil || again.ID != i.ID {
		t.Fatal("replay", e)
	}
	p := validBillingProfile()
	p.CountryCode = "DE"
	p.TaxID = "DE123"
	p.ExportExemption = true
	if _, e = f.svc.SaveProfile(f.ctx, f.org, p); e != nil {
		t.Fatal(e)
	}
	in.IdempotencyKey = "export"
	i, e = f.svc.GenerateInvoice(f.ctx, f.org, in)
	if e != nil || i.TaxMinor != 0 {
		t.Fatal("export", e)
	}
}
func TestCouponLimitsAndInvoiceDiscount(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	percent := int32(2500)
	code := "LAUNCH_" + strings.ToUpper(uuid.NewString()[:8])
	_, e := f.svc.CreateCoupon(f.ctx, usecase.CouponInput{Code: code, PercentBasisPoints: &percent, PerOrgLimit: 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.svc.AttachCoupon(f.ctx, f.org, strings.ToLower(code)); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	i, e := f.svc.GenerateInvoice(f.ctx, f.org, usecase.InvoiceInput{Kind: "renewal", IdempotencyKey: "coupon", Currency: "TRY", PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0), DueAt: now, Lines: []usecase.InvoiceLineInput{{Description: "Plan", Quantity: 1, UnitAmountMinor: 10000, VATBasisPoints: 2000}}})
	if e != nil || i.DiscountMinor != 2500 || i.TotalMinor != 9000 {
		t.Fatalf("discount %+v %v", i, e)
	}
	if e = f.svc.AttachCoupon(f.ctx, f.org, code); !errors.Is(e, model.ErrConflict) {
		t.Fatalf("reuse %v", e)
	}
}
func TestWalletConcurrentSpendAndExpiryLots(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	expiry := time.Now().Add(time.Hour)
	if _, e := f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: 10, Reason: "grant", IdempotencyKey: "grant", ExpiresAt: &expiry}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	spent := 0
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: -1, Reason: "AI usage", IdempotencyKey: fmt.Sprintf("spend:%d", i)})
			if e == nil {
				mu.Lock()
				spent++
				mu.Unlock()
			} else if !errors.Is(e, model.ErrConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if spent != 10 {
		t.Fatalf("spent %d", spent)
	}
	summary, e := f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || summary.AICredits != 0 || summary.AIAction != "stop_and_handoff" {
		t.Fatalf("balance %+v %v", summary, e)
	}
	// Expire an untouched lot while preserving a later permanent purchase.
	var subID int64
	if e = f.pool.QueryRow(f.ctx, "SELECT id FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&subID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.q.BillingWalletEntry(f.ctx, db.BillingWalletEntryParams{OrganizationID: f.org, SubscriptionID: subID, Unit: "ai_credits", Delta: 5, Reason: "expired grant", IdempotencyKey: "expired", ExpiresAt: repository.TS(time.Now().Add(-time.Hour))}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: 7, Reason: "permanent", IdempotencyKey: "permanent"}); e != nil {
		t.Fatal(e)
	}
	summary, e = f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || summary.AICredits != 7 {
		t.Fatal("expiry burned later credits", summary, e)
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE billing_credit_ledger SET delta=100 WHERE organization_id=$1", f.org); e == nil {
		t.Fatal("mutable ledger")
	}
}
func TestPlanChangePreviewPaidActivationAndDowngrade(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	f.input.Prices[0].Amount = 20000
	version, e := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if e != nil {
		t.Fatal(e)
	}
	version, e = f.svc.Publish(f.ctx, f.plan.UUID, version.UUID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	input := usecase.PlanChangeInput{PriceUUID: version.Prices[0].UUID, Seats: 1, IdempotencyKey: "upgrade"}
	preview, e := f.svc.PreviewPlanChange(f.ctx, f.org, input)
	if e != nil || !preview.Immediate || preview.AmountMinor < 5900 || preview.AmountMinor > 6100 {
		t.Fatalf("preview %+v %v", preview, e)
	}
	change, e := f.svc.ApplyPlanChange(f.ctx, f.org, input)
	if e != nil || change.State != "awaiting_payment" {
		t.Fatalf("apply %+v %v", change, e)
	}
	sub, e := f.svc.Subscription(f.ctx, f.org)
	if e != nil || sub.PriceUUID != f.sub.PriceUUID {
		t.Fatal("activated without payment", e)
	}
	// A signed payment enters the existing settlement path and applies the change.
	fake := payments.NewFake("https://example.test")
	a := configureGateway(t, f, fake)
	_ = a
	var invoiceUUID uuid.UUID
	if e = f.pool.QueryRow(f.ctx, "SELECT uuid FROM billing_invoices WHERE id=$1", change.InvoiceID.Int64).Scan(&invoiceUUID); e != nil {
		t.Fatal(e)
	}
	payWithFake(t, f, fake, invoiceUUID, "upgrade-payment")
	sub, e = f.svc.Subscription(f.ctx, f.org)
	if e != nil || sub.PriceUUID != version.Prices[0].UUID {
		t.Fatalf("paid activation %+v %v", sub, e)
	}
	downgrade, e := f.svc.ApplyPlanChange(f.ctx, f.org, usecase.PlanChangeInput{PriceUUID: f.sub.PriceUUID, Seats: 1, IdempotencyKey: "downgrade"})
	if e != nil || downgrade.State != "scheduled" {
		t.Fatalf("downgrade %+v %v", downgrade, e)
	}
}

// TestPlanChangeRejectsHiddenPlans keeps hidden plans (the free demo-full
// plan) out of self-service plan changes; only platform billing assigns them.
func TestPlanChangeRejectsHiddenPlans(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	hidden, e := f.svc.Create(f.ctx, model.PlanInput{Code: "hidden-" + uuid.NewString(), Name: "Hidden", Visibility: "hidden"})
	if e != nil {
		t.Fatal(e)
	}
	f.input.Prices[0].Amount = 0
	version, e := f.svc.SaveVersion(f.ctx, hidden.UUID, uuid.Nil, f.input)
	if e != nil {
		t.Fatal(e)
	}
	if version, e = f.svc.Publish(f.ctx, hidden.UUID, version.UUID, f.actor); e != nil {
		t.Fatal(e)
	}
	input := usecase.PlanChangeInput{PriceUUID: version.Prices[0].UUID, Seats: 1, IdempotencyKey: "hidden"}
	if _, e = f.svc.PreviewPlanChange(f.ctx, f.org, input); !errors.Is(e, model.ErrNotFound) {
		t.Fatalf("preview of a hidden plan: %v", e)
	}
	if _, e = f.svc.ApplyPlanChange(f.ctx, f.org, input); !errors.Is(e, model.ErrNotFound) {
		t.Fatalf("apply of a hidden plan: %v", e)
	}
}
func payWithFake(t *testing.T, f *billingFixture, p *payments.Fake, invoice uuid.UUID, key string) uuid.UUID {
	t.Helper()
	out, e := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice, Provider: "fake", IdempotencyKey: key}, payments.Buyer{Name: "Buyer", Email: "buyer@example.test", IP: "127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	var payment db.BillingPayment
	var raw []byte
	if e = f.pool.QueryRow(f.ctx, "SELECT row_to_json(p) FROM billing_payments p WHERE uuid=$1", out.PaymentUUID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &payment); e != nil {
		t.Fatal(e)
	}
	body, headers := fakeNotification(out.OrderReference, payment.AmountMinor, "succeeded")
	if _, e = f.svc.Notify(f.ctx, "fake", headers, body); e != nil {
		t.Fatal(e)
	}
	return out.PaymentUUID
}

func fakeNotification(order string, amount int64, status string) ([]byte, http.Header) {
	body, _ := json.Marshal(payments.Notification{OrderRef: order, EventKey: "capture:" + order, Status: status, Amount: amount, PaidAmount: amount, Currency: "TRY", ProviderPaymentID: "fake:" + order, Test: true})
	header := http.Header{}
	header.Set("X-Fake-Signature", payments.FakeSignature("sandbox-secret-key", body))
	return body, header
}

type lostRefundGateway struct{ *payments.Fake }

func (p *lostRefundGateway) Refund(ctx context.Context, a payments.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	_, e := p.Fake.Refund(ctx, a, in)
	if e != nil {
		return payments.RefundResult{}, e
	}
	return payments.RefundResult{}, &payments.ProviderError{Code: "lost_response", Temporary: true}
}
func TestAmbiguousRefundReconciliationAndCreditNote(t *testing.T) {
	f := fixture(t)
	g := &lostRefundGateway{payments.NewFake("https://example.test")}
	now := prepareRenewal(t, f, g)
	if e := f.svc.RenewalSweep(f.ctx, now); e != nil {
		t.Fatal(e)
	}
	var payment db.BillingPayment
	var raw []byte
	if e := f.pool.QueryRow(f.ctx, "SELECT row_to_json(p) FROM billing_payments p WHERE organization_id=$1 AND kind='recurring'", f.org).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(raw, &payment); e != nil {
		t.Fatal(e)
	}
	invoice, e := f.q.BillingInvoiceID(f.ctx, db.BillingInvoiceIDParams{OrganizationID: f.org, ID: payment.InvoiceID})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.svc.IssueInvoice(f.ctx, f.org, invoice.Uuid); e != nil {
		t.Fatal(e)
	}
	in := usecase.RefundInput{AmountMinor: 2500, IdempotencyKey: "lost"}
	if e = f.svc.RefundPayment(f.ctx, f.org, payment.Uuid, in); e == nil {
		t.Fatal("expected ambiguous provider error")
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '3 minutes' WHERE organization_id=$1 AND kind='refund'", f.org); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ReconcileRefunds(f.ctx); e != nil {
		t.Fatal(e)
	}
	var status string
	if e = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_payments WHERE organization_id=$1 AND kind='refund'", f.org).Scan(&status); e != nil || status != "succeeded" {
		t.Fatal(status, e)
	}
	if e = f.svc.RefundPayment(f.ctx, f.org, payment.Uuid, in); e != nil {
		t.Fatal("refund replay", e)
	}
	// Older fixture refunds can be independently un-invoiceable; the worker must
	// still create this note and report those failures without rolling it back.
	_ = f.svc.CreditNoteSweep(f.ctx)
	var noteUUID uuid.UUID
	var total int64
	if e = f.pool.QueryRow(f.ctx, "SELECT uuid,total_minor FROM billing_invoices WHERE organization_id=$1 AND kind='credit_note'", f.org).Scan(&noteUUID, &total); e != nil || total != 2500 {
		t.Fatal("credit note", total, e)
	}
	if e = f.svc.IssueInvoice(f.ctx, f.org, noteUUID); e != nil {
		t.Fatal(e)
	}
	doc, e := f.svc.PrepareInvoice(f.ctx, f.org, noteUUID)
	if e != nil || !strings.Contains(string(doc.Xml), "IADE") {
		t.Fatal("IADE XML", e)
	}
	// A refund to wallet is also reserved against the original charge ceiling.
	if e = f.svc.RefundPayment(f.ctx, f.org, payment.Uuid, usecase.RefundInput{Destination: "wallet", AmountMinor: 1000, IdempotencyKey: "wallet-refund"}); e != nil {
		t.Fatal(e)
	}
	summary, e := f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || summary.TRYMinor != 1000 {
		t.Fatal(summary, e)
	}
	if e = f.svc.RefundPayment(f.ctx, f.org, payment.Uuid, usecase.RefundInput{Destination: "card", AmountMinor: 1000, IdempotencyKey: "wallet-refund"}); !errors.Is(e, model.ErrConflict) {
		t.Fatal("refund destination replay", e)
	}
}
func TestStatementOrphanAndExactRefundBinding(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	invoice := commerceInvoice(t, f, "statement")
	merchant := uuid.New()
	var subID int64
	if e := f.pool.QueryRow(f.ctx, "SELECT id FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&subID); e != nil {
		t.Fatal(e)
	}
	original, e := f.q.BillingNewPayment(f.ctx, db.BillingNewPaymentParams{OrganizationID: f.org, InvoiceID: invoice.ID, Provider: "fake", MerchantAccountUuid: pgtype.UUID{Bytes: merchant, Valid: true}, Kind: "checkout", IdempotencyKey: "statement-pay", AmountMinor: 12000, OrderReference: pgtype.Text{String: "statement-" + uuid.NewString(), Valid: true}})
	if e != nil {
		t.Fatal(e)
	}
	rows := []usecase.StatementInput{{Provider: "fake", MerchantAccountUUID: merchant, ExternalID: uuid.NewString(), ProviderReference: "payment-proof", OrderReference: "orphan-" + uuid.NewString(), Operation: "charge", Status: "succeeded", AmountMinor: 12000, Currency: "TRY", ObservedAt: time.Now()}}
	if e = f.svc.ImportStatement(f.ctx, rows); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ReconcileStatements(f.ctx); e != nil {
		t.Fatal(e)
	}
	var mismatch string
	if e = f.pool.QueryRow(f.ctx, "SELECT mismatch FROM billing_provider_statement WHERE external_id=$1", rows[0].ExternalID).Scan(&mismatch); e != nil || mismatch != "orphan_provider_record" {
		t.Fatal(mismatch, e)
	}
	rows[0].ExternalID = uuid.NewString()
	rows[0].OrderReference = original.OrderReference.String
	if e = f.svc.ImportStatement(f.ctx, rows); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ReconcileStatements(f.ctx); e != nil {
		t.Fatal(e)
	}
	original, e = f.q.BillingPaymentID(f.ctx, db.BillingPaymentIDParams{OrganizationID: f.org, ID: original.ID})
	if e != nil || original.Status != "succeeded" {
		t.Fatal(original.Status, e)
	}
	refund, e := f.q.BillingNewRefund(f.ctx, db.BillingNewRefundParams{OrganizationID: f.org, InvoiceID: invoice.ID, Provider: "fake", MerchantAccountUuid: original.MerchantAccountUuid, OriginalPaymentID: pgtype.Int8{Int64: original.ID, Valid: true}, IdempotencyKey: "statement-refund", AmountMinor: 1000, OrderReference: pgtype.Text{String: "refund-" + uuid.NewString(), Valid: true}, RefundDestination: "card"})
	if e != nil {
		t.Fatal(e)
	}
	rows[0].ExternalID = uuid.NewString()
	rows[0].Operation = "refund"
	rows[0].AmountMinor = 1000
	rows[0].OrderReference = original.OrderReference.String
	if e = f.svc.ImportStatement(f.ctx, rows); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ReconcileStatements(f.ctx); e != nil {
		t.Fatal(e)
	}
	current, e := f.q.BillingPaymentID(f.ctx, db.BillingPaymentIDParams{OrganizationID: f.org, ID: refund.ID})
	if e != nil || current.Status != "pending" {
		t.Fatal("original order incorrectly resolved a refund", e)
	}
	rows[0].ExternalID = uuid.NewString()
	rows[0].OrderReference = refund.OrderReference.String
	if e = f.svc.ImportStatement(f.ctx, rows); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.ReconcileStatements(f.ctx); e != nil {
		t.Fatal(e)
	}
	current, e = f.q.BillingPaymentID(f.ctx, db.BillingPaymentIDParams{OrganizationID: f.org, ID: refund.ID})
	if e != nil || current.Status != "succeeded" {
		t.Fatal("refund exact binding", e)
	}
	rows[0].AmountMinor = 1001
	if e = f.svc.ImportStatement(f.ctx, rows); !errors.Is(e, model.ErrConflict) {
		t.Fatal("statement payload mutation", e)
	}
}
func TestAddonAndCreditPurchaseActivateOnlyAfterPayment(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	fake := payments.NewFake("https://example.test")
	configureGateway(t, f, fake)
	addon, e := f.svc.CreateAddon(f.ctx, usecase.AddonInput{Code: "AI_" + strings.ToUpper(uuid.NewString()[:8]), Name: "AI extra", Feature: "ai.tokens", Units: 100, AmountMinor: 1000, Currency: "TRY"})
	if e != nil {
		t.Fatal(e)
	}
	before, e := f.ent.Resolve(f.ctx, f.org)
	if e != nil {
		t.Fatal(e)
	}
	invoice, e := f.svc.ChangeAddon(f.ctx, f.org, usecase.AddonChangeInput{AddonUUID: addon.Uuid, Quantity: 2, IdempotencyKey: "addon"})
	if e != nil || invoice == nil {
		t.Fatal(e)
	}
	middle, e := f.ent.Resolve(f.ctx, f.org)
	if e != nil || middle.Values["ai.tokens"].Limit != before.Values["ai.tokens"].Limit {
		t.Fatal("addon before payment", e)
	}
	payWithFake(t, f, fake, invoice.Uuid, "addon-pay")
	after, e := f.ent.Resolve(f.ctx, f.org)
	if e != nil || after.Values["ai.tokens"].Limit != before.Values["ai.tokens"].Limit+200 {
		t.Fatal("addon after payment", e)
	}
	pkg, e := f.svc.CreateCreditPackage(f.ctx, usecase.CreditPackageInput{Code: "PACK_" + strings.ToUpper(uuid.NewString()[:8]), Name: "AI pack", Credits: 500, AmountMinor: 10000})
	if e != nil {
		t.Fatal(e)
	}
	purchase, e := f.svc.PurchaseCredits(f.ctx, f.org, usecase.CreditPurchaseInput{PackageUUID: pkg.UUID, IdempotencyKey: "pack"})
	if e != nil {
		t.Fatal(e)
	}
	wallet, e := f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || wallet.AICredits != 0 {
		t.Fatal("credit before payment", e)
	}
	payWithFake(t, f, fake, purchase.Uuid, "credit-pay")
	wallet, e = f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || wallet.AICredits != 500 {
		t.Fatal("credit after payment", e)
	}
}
func TestCreditRefundClawsBackOnlyUnspentPurchase(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	fake := payments.NewFake("https://example.test")
	configureGateway(t, f, fake)
	pkg, e := f.svc.CreateCreditPackage(f.ctx, usecase.CreditPackageInput{Code: "REFUND_" + strings.ToUpper(uuid.NewString()[:8]), Name: "Refundable AI", Credits: 500, AmountMinor: 10000})
	if e != nil {
		t.Fatal(e)
	}
	invoice, e := f.svc.PurchaseCredits(f.ctx, f.org, usecase.CreditPurchaseInput{PackageUUID: pkg.UUID, IdempotencyKey: "pack"})
	if e != nil {
		t.Fatal(e)
	}
	payment := payWithFake(t, f, fake, invoice.Uuid, "pay-pack")
	if e = f.svc.RefundPayment(f.ctx, f.org, payment, usecase.RefundInput{Destination: "wallet", AmountMinor: 6000, IdempotencyKey: "half"}); e != nil {
		t.Fatal(e)
	}
	wallet, e := f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || wallet.AICredits != 250 || wallet.TRYMinor != 6000 {
		t.Fatal("refund credit clawback", wallet, e)
	}
	if _, e = f.svc.WalletCommand(f.ctx, f.org, usecase.WalletInput{Unit: "ai_credits", Delta: -250, Reason: "AI usage", IdempotencyKey: "spent"}); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.RefundPayment(f.ctx, f.org, payment, usecase.RefundInput{Destination: "wallet", AmountMinor: 6000, IdempotencyKey: "spent-refund"}); !errors.Is(e, model.ErrConflict) {
		t.Fatal("refunded consumed credits", e)
	}
	next := commerceInvoice(t, f, "wallet-pay")
	input := usecase.WalletPaymentInput{InvoiceUUID: next.Uuid, IdempotencyKey: "pay"}
	id, e := f.svc.PayFromWallet(f.ctx, f.org, input)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.svc.PayFromWallet(f.ctx, f.org, input)
	if e != nil || again != id {
		t.Fatal("wallet payment replay", e)
	}
	wallet, e = f.svc.Wallet(f.ctx, f.org, 20, 0)
	if e != nil || wallet.TRYMinor != 0 {
		t.Fatal(wallet, e)
	}
	var paid int64
	if e = f.pool.QueryRow(f.ctx, "SELECT sum(amount_minor) FROM billing_payments WHERE invoice_id=$1 AND status='succeeded'", next.ID).Scan(&paid); e != nil || paid != 6000 {
		t.Fatal("wallet settlement", paid, e)
	}
}
func TestFullyDiscountedRenewalAdvancesWithoutGateway(t *testing.T) {
	f := fixture(t)
	g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	now := prepareRenewal(t, f, g)
	percent := int32(10000)
	code := "FREE_" + strings.ToUpper(uuid.NewString()[:8])
	if _, e := f.svc.CreateCoupon(f.ctx, usecase.CouponInput{Code: code, PercentBasisPoints: &percent}); e != nil {
		t.Fatal(e)
	}
	if e := f.svc.AttachCoupon(f.ctx, f.org, code); e != nil {
		t.Fatal(e)
	}
	if e := f.svc.RenewalSweep(f.ctx, now); e != nil {
		t.Fatal(e)
	}
	sub, e := f.svc.Subscription(f.ctx, f.org)
	if e != nil || !sub.PeriodEnd.After(now) || g.calls.Load() != 0 {
		t.Fatal("zero due renewal", sub, e)
	}
}

func TestBillingInvoiceNotificationLocaleEmailsAndReplay(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	if _, e := f.pool.Exec(f.ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", f.org, f.actor); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE users SET locale='en' WHERE id=$1", f.actor); e != nil {
		t.Fatal(e)
	}
	bus := events.NewBus(nil)
	notifications := notifusecase.New(f.q, queuedNotifications{}, nil, nil)
	billingmodule.RegisterPaymentNotifications(bus, f.q, notifications)
	event := events.New("billing.invoice_issued").WithTenant(f.org).WithPayload(map[string]any{"billing_emails": []string{"accounts@example.test", "archive@example.test"}, "invoice_url": "https://example.test/billing"})
	for range 2 {
		if e := bus.Publish(f.ctx, event); e != nil {
			t.Fatal(e)
		}
	}
	var count int
	var body, title string
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM notifications WHERE user_id=$1 AND source_event='billing.invoice_issued'", f.actor).Scan(&count); e != nil || count != 3 {
		t.Fatal("invoice notice dedupe", count, e)
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT title,body FROM notifications WHERE user_id=$1 AND recipient='accounts@example.test'", f.actor).Scan(&title, &body); e != nil || title != "Your invoice is ready" || !strings.Contains(body, "example.test/billing") {
		t.Fatal("invoice locale", title, body, e)
	}
}

type lostEInvoiceResponse struct {
	*usecase.FakeEInvoiceProvider
	sends atomic.Int32
}

func (p *lostEInvoiceResponse) Send(ctx context.Context, in usecase.EInvoiceEnvelope) (usecase.EInvoiceStatus, error) {
	p.sends.Add(1)
	out, err := p.FakeEInvoiceProvider.Send(ctx, in)
	if err != nil {
		return out, err
	}
	return out, fmt.Errorf("lost provider response")
}
func TestEInvoiceLostResponseQueriesBeforeRetry(t *testing.T) {
	f := fixture(t)
	configureInvoiceFixture(t, f)
	p := &lostEInvoiceResponse{FakeEInvoiceProvider: &usecase.FakeEInvoiceProvider{}}
	if e := f.svc.ConfigureInvoicing(p, validBillingProfile(), "CFX", "test"); e != nil {
		t.Fatal(e)
	}
	invoice := commerceInvoice(t, f, "lost-einvoice")
	payCommerceInvoice(t, f, invoice)
	if e := f.svc.IssueInvoice(f.ctx, f.org, invoice.Uuid); e == nil {
		t.Fatal("expected lost result")
	}
	if e := f.svc.IssueInvoice(f.ctx, f.org, invoice.Uuid); e != nil {
		t.Fatal(e)
	}
	if p.sends.Load() != 1 {
		t.Fatal("resubmitted an issued invoice")
	}
}
func TestRenewalClaimsClosedCalendarUsageOnce(t *testing.T) {
	f := fixture(t)
	upper := int64(10)
	f.input.Prices[0].Tiers = []model.Tier{{Feature: "ai.tokens", Lower: 0, Upper: &upper, UnitAmount: 0}, {Feature: "ai.tokens", Lower: 10, UnitAmount: 100}}
	version, e := f.svc.SaveVersion(f.ctx, f.plan.UUID, uuid.Nil, f.input)
	if e != nil {
		t.Fatal(e)
	}
	version, e = f.svc.Publish(f.ctx, f.plan.UUID, version.UUID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET price_id=$2,plan_version_id=(SELECT id FROM billing_plan_versions WHERE uuid=$3) WHERE uuid=$1", f.sub.UUID, func() int64 {
		p, e := f.q.BillingCommercePrice(f.ctx, version.Prices[0].UUID)
		if e != nil {
			t.Fatal(e)
		}
		return p.ID
	}(), version.UUID); e != nil {
		t.Fatal(e)
	}
	g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	now := prepareRenewal(t, f, g)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	period, e := f.q.BillingEnsureUsagePeriod(f.ctx, db.BillingEnsureUsagePeriodParams{OrganizationID: f.org, StartsAt: repository.TS(month.AddDate(0, -1, 0)), EndsAt: repository.TS(month)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, "INSERT INTO billing_usage_totals(organization_id,usage_period_id,feature_id,quantity) SELECT $1,$2,id,15 FROM billing_features WHERE key='ai.tokens'", f.org, period.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.ent.ClosePeriod(f.ctx, f.org, period.Uuid, now); e != nil {
		t.Fatal(e)
	}
	if e = f.svc.RenewalSweep(f.ctx, now); e != nil {
		t.Fatal(e)
	}
	var billed, claims int64
	if e = f.pool.QueryRow(f.ctx, "SELECT COALESCE(sum(l.total_minor),0) FROM billing_invoice_lines l WHERE l.organization_id=$1 AND description LIKE 'Usage:%'", f.org).Scan(&billed); e != nil || billed != 600 {
		t.Fatal("calendar usage missing", billed, e)
	}
	if e = f.svc.RenewalSweep(f.ctx, now); e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_invoice_usage_periods WHERE organization_id=$1", f.org).Scan(&claims); e != nil || claims != 1 {
		t.Fatal("duplicate usage handoff", claims, e)
	}
}

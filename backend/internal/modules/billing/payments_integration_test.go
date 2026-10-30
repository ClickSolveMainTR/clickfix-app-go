package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/paytr"
	"github.com/google/uuid"
)

const paymentTestKey = "01234567890123456789012345678901"

func configureGateway(t *testing.T, f *billingFixture, p payments.Provider) payments.Account {
	t.Helper()
	reg := payments.NewRegistry(f.svc, false)
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	a := payments.Account{UUID: uuid.NewString(), Mode: "sandbox", Credentials: map[string]string{"secret_key": "sandbox-secret-key", "api_key": "sandbox-api-key", "merchant_id": "123456", "merchant_key": "KEY123", "merchant_salt": "SALT456"}}
	vault, err := payments.NewCardVault(paymentTestKey)
	if err != nil {
		t.Fatal(err)
	}
	g := &usecase.TrialGateway{Executor: usecase.NewGatewayExecutor(repository.New(f.pool, f.q), reg), Vault: vault, Account: func(_ context.Context, provider, merchant string) (payments.Account, error) {
		if provider != p.Code() || (merchant != "" && merchant != a.UUID) {
			return payments.Account{}, payments.ErrNotConfigured
		}
		return a, nil
	}}
	if err = f.svc.ConfigurePayments(g, paymentTestKey, "https://example.test"); err != nil {
		t.Fatal(err)
	}
	return a
}
func invoiceFixture(t *testing.T, f *billingFixture, status string, starts time.Time) db.BillingInvoice {
	t.Helper()
	configureInvoiceFixture(t, f)
	snapshot, err := json.Marshal(validBillingProfile())
	if err != nil {
		t.Fatal(err)
	}
	var subID int64
	if err := f.pool.QueryRow(f.ctx, "SELECT id FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	invoice, err := f.q.BillingRenewalInvoice(f.ctx, db.BillingRenewalInvoiceParams{OrganizationID: f.org, SubscriptionID: subID, PeriodStart: repository.TS(starts), PeriodEnd: repository.TS(usecase.NextPeriod(starts, false)), IdempotencyKey: uuid.NewString(), BuyerSnapshot: snapshot, SellerSnapshot: snapshot, SubtotalMinor: 10000, TaxMinor: 0, TotalMinor: 10000, DueAt: repository.TS(starts)})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.q.BillingCreateInvoiceLine(f.ctx, db.BillingCreateInvoiceLineParams{OrganizationID: f.org, InvoiceID: invoice.ID, Description: "Legacy fixture invoice", Quantity: 1, UnitAmountMinor: 10000, SubtotalMinor: 10000, TotalMinor: 10000}); err != nil {
		t.Fatal(err)
	}
	if status == "paid" {
		if _, err = f.pool.Exec(f.ctx, "UPDATE billing_invoices SET status='paid',paid_at=now() WHERE id=$1", invoice.ID); err != nil {
			t.Fatal(err)
		}
		invoice.Status = "paid"
	}
	return invoice
}
func TestPayTRCheckoutTrialSettlementAndReplay(t *testing.T) {
	f := fixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/odeme/capi/list" {
			_, _ = w.Write([]byte(`[{"ctoken":"token","last_4":"4242"}]`))
			return
		}
		if r.URL.Path != "/odeme/api/get-token" {
			t.Errorf("path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("store_card") != "1" || r.Form.Get("payment_amount") != "10000" {
			t.Errorf("checkout form %+v", r.Form)
		}
		_, _ = w.Write([]byte(`{"status":"success","token":"checkout-token"}`))
	}))
	defer srv.Close()
	p := paytr.New(srv.URL, srv.Client())
	a := configureGateway(t, f, p)
	if _, err := f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET status='trialing',trial_ends_at=period_end WHERE uuid=$1", f.sub.UUID); err != nil {
		t.Fatal(err)
	}
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	input := usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "paytr", IdempotencyKey: "first", StoreCard: true, AutoPayConsent: true}
	buyer := payments.Buyer{Name: "Test Buyer", Email: "buyer@example.test", IP: "127.0.0.1"}
	out, err := f.svc.Checkout(f.ctx, f.org, input, buyer)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := f.svc.Checkout(f.ctx, f.org, input, buyer)
	if err != nil || repeat != out {
		t.Fatalf("checkout retry %+v %v", repeat, err)
	}
	form := url.Values{"merchant_oid": {out.OrderReference}, "status": {"success"}, "total_amount": {"10000"}, "payment_amount": {"10000"}, "currency": {"TL"}, "test_mode": {"1"}, "utoken": {"owner"}, "ctoken": {"token"}, "last_4": {"4242"}}
	form.Set("hash", "forged")
	if _, err = f.svc.Notify(f.ctx, "paytr", nil, []byte(form.Encode())); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("forged %v", err)
	}
	form.Set("hash", paytr.NotificationHash(a, out.OrderReference, "success", "10000"))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.svc.Notify(f.ctx, "paytr", nil, []byte(form.Encode())); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var status string
	var paymentsCount, receipts, cards int
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM billing_payments WHERE organization_id=$1 AND status='succeeded'),(SELECT count(*) FROM billing_webhook_receipts WHERE organization_id=$1),(SELECT count(*) FROM billing_payment_methods WHERE organization_id=$1)", f.org).Scan(&paymentsCount, &receipts, &cards); err != nil {
		t.Fatal(err)
	}
	if status != "active" || paymentsCount != 1 || receipts != 1 || cards != 1 {
		t.Fatalf("status=%s payments=%d receipts=%d cards=%d", status, paymentsCount, receipts, cards)
	}
	var bodyEnc string
	if err = f.pool.QueryRow(f.ctx, "SELECT body_enc FROM billing_webhook_receipts WHERE organization_id=$1", f.org).Scan(&bodyEnc); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bodyEnc, "utoken") || bodyEnc == form.Encode() {
		t.Fatal("raw webhook was not encrypted")
	}
	if _, err = f.svc.Checkout(f.ctx, f.org+1, input, buyer); err == nil {
		t.Fatal("cross-org invoice accepted")
	}
}
func TestIyzicoFakeSignedCheckoutAndTokenBinding(t *testing.T) {
	f := fixture(t)
	fake := iyzicofake.New()
	defer fake.Server.Close()
	fake.AddAccount("sandbox-api-key", "sandbox-secret-key")
	p := iyzico.New(fake.URL(), fake.Server.Client())
	configureGateway(t, f, p)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	out, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "iyzico", IdempotencyKey: "iyzi"}, payments.Buyer{Name: "Test Buyer", Email: "test@example.test", IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	tokens := fake.Tokens()
	if len(tokens) != 1 {
		t.Fatalf("tokens %v", tokens)
	}
	if err = f.svc.ConfirmCheckout(f.ctx, out.OrderReference, "other-token"); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("wrong token %v", err)
	}
	raw, sig, err := fake.Complete(tokens[0], iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	if _, err = f.svc.Notify(f.ctx, "iyzico", headers, raw); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("unsigned %v", err)
	}
	headers.Set(iyzico.SignatureHeader, sig)
	for range 2 {
		if _, err = f.svc.Notify(f.ctx, "iyzico", headers, raw); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_payments WHERE organization_id=$1 AND status='succeeded'", f.org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("payments %d %v", count, err)
	}
}
func TestBankTransferPartialExcessAndTenantReceipt(t *testing.T) {
	f := fixture(t)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	bank, err := f.svc.SaveBankAccount(f.ctx, usecase.BankAccountInput{BankName: "Fixture bank", AccountName: "Fixture seller", IBAN: "TR330006100519786457841326", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.SaveBankAccount(f.ctx, usecase.BankAccountInput{BankName: "bad", AccountName: "bad", IBAN: "TR000006100519786457841326", Active: true}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("invalid IBAN %v", err)
	}
	var attachment uuid.UUID
	if err = f.pool.QueryRow(f.ctx, "INSERT INTO attachments(organization_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'receipt.pdf','application/pdf',1,'digest',$2) RETURNING uuid", f.org, "test/"+uuid.NewString()).Scan(&attachment); err != nil {
		t.Fatal(err)
	}
	input := usecase.TransferInput{InvoiceUUID: invoice.Uuid, BankAccountUUID: bank.UUID, AttachmentUUID: attachment, AmountMinor: 4000}
	if _, err = f.svc.ReportTransfer(f.ctx, f.org+1, input); err == nil {
		t.Fatal("cross-org receipt accepted")
	}
	id, err := f.svc.ReportTransfer(f.ctx, f.org, input)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.svc.ReviewTransfer(f.ctx, f.org, id, usecase.TransferReview{Approve: true}, f.actor); err != nil {
			t.Fatal(err)
		}
	}
	current, err := f.q.BillingInvoiceID(f.ctx, db.BillingInvoiceIDParams{OrganizationID: f.org, ID: invoice.ID})
	if err != nil || current.Status != "open" {
		t.Fatalf("partial invoice %+v %v", current, err)
	}
	if _, err = f.svc.ReportTransfer(f.ctx, f.org, input); err == nil {
		t.Fatal("receipt reused")
	}
	if err = f.pool.QueryRow(f.ctx, "INSERT INTO attachments(organization_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'receipt2.pdf','application/pdf',1,'digest',$2) RETURNING uuid", f.org, "test/"+uuid.NewString()).Scan(&attachment); err != nil {
		t.Fatal(err)
	}
	input.AttachmentUUID = attachment
	input.AmountMinor = 8000
	id, err = f.svc.ReportTransfer(f.ctx, f.org, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ReviewTransfer(f.ctx, f.org, id, usecase.TransferReview{Approve: true}, f.actor); err != nil {
		t.Fatal(err)
	}
	current, err = f.q.BillingInvoiceID(f.ctx, db.BillingInvoiceIDParams{OrganizationID: f.org, ID: invoice.ID})
	if err != nil || current.Status != "paid" {
		t.Fatalf("paid invoice %+v %v", current, err)
	}
	var credit int64
	if err = f.pool.QueryRow(f.ctx, "SELECT sum(delta) FROM billing_credit_ledger WHERE organization_id=$1 AND unit='TRY'", f.org).Scan(&credit); err != nil || credit != 2000 {
		t.Fatalf("credit %d %v", credit, err)
	}
	if err = f.svc.ReviewTransfer(f.ctx, f.org, id, usecase.TransferReview{Reason: "later reject"}, f.actor); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("terminal review %v", err)
	}
}

type renewalGateway struct {
	*payments.Fake
	mode  atomic.Int32 // 0 succeeds, 1 declines, 2 capture then timeout
	calls atomic.Int32
}

func (p *renewalGateway) ChargeSavedCard(ctx context.Context, a payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	p.calls.Add(1)
	switch p.mode.Load() {
	case 1:
		return payments.ChargeResult{}, &payments.ProviderError{Code: "declined"}
	case 2:
		_, _ = p.Fake.ChargeSavedCard(ctx, a, in)
		return payments.ChargeResult{}, &payments.ProviderError{Code: "timeout", Temporary: true}
	}
	return p.Fake.ChargeSavedCard(ctx, a, in)
}
func prepareRenewal(t *testing.T, f *billingFixture, p payments.Provider) time.Time {
	t.Helper()
	t.Cleanup(func() {
		_, _ = f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET renewal_enabled=false WHERE organization_id=$1", f.org)
	})
	a := configureGateway(t, f, p)
	vault, _ := payments.NewCardVault(paymentTestKey)
	id := uuid.New()
	c := payments.SavedCard{OwnerKey: "owner", Token: "card", Last4: "4242"}
	owner, token, err := vault.Seal(fmt.Sprintf("org:%d", f.org), id, c)
	if err != nil {
		t.Fatal(err)
	}
	method, err := f.q.BillingSaveCard(f.ctx, db.BillingSaveCardParams{Uuid: id, OrganizationID: f.org, Provider: "fake", MerchantAccountUuid: uuid.MustParse(a.UUID), CustomerTokenEnc: owner, CardTokenEnc: token, TokenHash: vault.TokenHash(fmt.Sprintf("org:%d", f.org), "fake", "card"), Last4: "4242"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	start := now.AddDate(0, -1, 0)
	end := now.Add(-time.Minute)
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET period_start=$2,period_end=$3,payment_method_id=$4,auto_pay=true,auto_pay_consented_at=now(),renewal_enabled=true WHERE uuid=$1", f.sub.UUID, start, end, method.ID); err != nil {
		t.Fatal(err)
	}
	configureInvoiceFixture(t, f)
	invoiceFixture(t, f, "paid", start)
	return now
}
func TestRenewalClaimConcurrencyAndUnknownRecheck(t *testing.T) {
	f := fixture(t)
	gateway := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	gateway.mode.Store(2)
	now := prepareRenewal(t, f, gateway)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.svc.RenewalSweep(f.ctx, now) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if gateway.calls.Load() != 1 {
		t.Fatalf("calls %d", gateway.calls.Load())
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '3 minutes' WHERE organization_id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ReconcilePending(f.ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := f.pool.QueryRow(f.ctx, "SELECT status FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&status); err != nil || status != "active" {
		t.Fatalf("status %s %v", status, err)
	}
	if err := f.svc.RenewalSweep(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	if gateway.calls.Load() != 1 {
		t.Fatal("recharged unknown capture")
	}
}
func TestDunningRetriesRecoverOrSuspend(t *testing.T) {
	for _, recoverPayment := range []bool{true, false} {
		t.Run(fmt.Sprint(recoverPayment), func(t *testing.T) {
			f := fixture(t)
			g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
			g.mode.Store(1)
			now := prepareRenewal(t, f, g)
			if err := f.svc.RenewalSweep(f.ctx, now); err != nil {
				t.Fatal(err)
			}
			var grace time.Time
			if err := f.pool.QueryRow(f.ctx, "SELECT grace_ends_at FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&grace); err != nil {
				t.Fatal(err)
			}
			if recoverPayment {
				g.mode.Store(0)
			}
			for _, day := range []int{1, 3, 5, 7} {
				if err := f.svc.RenewalSweep(f.ctx, now.AddDate(0, 0, day).Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if recoverPayment {
					break
				}
			}
			sub, err := f.svc.Subscription(f.ctx, f.org)
			if err != nil {
				t.Fatal(err)
			}
			if recoverPayment {
				if sub.Status != "active" || g.calls.Load() != 2 {
					t.Fatalf("recovery %+v calls %d", sub, g.calls.Load())
				}
			} else {
				if g.calls.Load() != 5 {
					t.Fatalf("calls %d", g.calls.Load())
				}
				if err = f.svc.Advance(f.ctx, f.org, f.sub.UUID, grace.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				sub, err = f.svc.Subscription(f.ctx, f.org)
				if err != nil || sub.Status != "suspended" {
					t.Fatalf("suspension %+v %v", sub, err)
				}
			}
		})
	}
}
func TestPeriodClampAndRetryOffsets(t *testing.T) {
	jan := time.Date(2025, 1, 31, 10, 0, 0, 0, time.UTC)
	if got := usecase.NextPeriod(jan, false); got.Day() != 28 || got.Month() != time.February {
		t.Fatal(got)
	}
	leap := time.Date(2024, 2, 29, 10, 0, 0, 0, time.UTC)
	if got := usecase.NextPeriod(leap, true); got.Day() != 28 || got.Month() != time.February {
		t.Fatal(got)
	}
	for i, day := range []int{1, 3, 5, 7} {
		got := usecase.RetryAt(jan, int32(i+1))
		if got == nil || !got.Equal(jan.AddDate(0, 0, day)) {
			t.Fatalf("retry %v", got)
		}
	}
	if usecase.RetryAt(jan, 5) != nil {
		t.Fatal("unexpected extra retry")
	}
}
func TestPaymentCardCiphertextIsScoped(t *testing.T) {
	f := fixture(t)
	g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	prepareRenewal(t, f, g)
	var card db.BillingPaymentMethod
	var err error
	var raw []byte
	if err = f.pool.QueryRow(f.ctx, "SELECT row_to_json(m) FROM billing_payment_methods m WHERE organization_id=$1", f.org).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &card); err != nil {
		t.Fatal(err)
	}
	vault, _ := payments.NewCardVault(paymentTestKey)
	if _, _, err = vault.Open("other-org", card.Uuid, card.CustomerTokenEnc, card.CardTokenEnc); err == nil {
		t.Fatal("cross-scope decryption")
	}
}

func TestIyzicoStoredCardChargeAndRefundLedger(t *testing.T) {
	f := fixture(t)
	fake := iyzicofake.New()
	defer fake.Server.Close()
	fake.AddAccount("sandbox-api-key", "sandbox-secret-key")
	p := iyzico.New(fake.URL(), fake.Server.Client())
	configureGateway(t, f, p)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	out, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "iyzico", IdempotencyKey: "card", StoreCard: true, AutoPayConsent: true}, payments.Buyer{Name: "Test Buyer", Email: "buyer@example.test", IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	tokens := fake.Tokens()
	raw, sig, err := fake.Complete(tokens[0], iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set(iyzico.SignatureHeader, sig)
	if _, err = f.svc.Notify(f.ctx, "iyzico", headers, raw); err != nil {
		t.Fatal(err)
	}
	cards, err := f.svc.Cards(f.ctx, f.org)
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards %+v %v", cards, err)
	}
	// A full duplicate refund returns the prior result and never calls the gateway twice.
	input := usecase.RefundInput{AmountMinor: 2500, IdempotencyKey: "partial"}
	for range 2 {
		if err = f.svc.RefundPayment(f.ctx, f.org, out.PaymentUUID, input); err != nil {
			t.Fatal(err)
		}
	}
	if fake.Refunds.Load() != 1 {
		t.Fatalf("refund calls %d", fake.Refunds.Load())
	}
	if err = f.svc.RefundPayment(f.ctx, f.org, out.PaymentUUID, usecase.RefundInput{AmountMinor: 8000, IdempotencyKey: "too-much"}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("over-refund %v", err)
	}
	// Advance a period through the same registry, with consented encrypted card tokens.
	now := time.Now().UTC()
	configureInvoiceFixture(t, f)
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET period_start=now()-interval '1 month',period_end=now()-interval '1 minute' WHERE uuid=$1", f.sub.UUID); err != nil {
		t.Fatal(err)
	}
	if err = f.svc.RenewalSweep(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	if fake.Charges.Load() != 1 {
		t.Fatalf("recurring calls %d", fake.Charges.Load())
	}
	if err = f.svc.RemovePaymentCard(f.ctx, f.org, cards[0].Uuid); err != nil {
		t.Fatal(err)
	}
	var autopay bool
	if err = f.pool.QueryRow(f.ctx, "SELECT auto_pay FROM billing_subscriptions WHERE uuid=$1", f.sub.UUID).Scan(&autopay); err != nil || autopay {
		t.Fatalf("removed card auto pay %v %v", autopay, err)
	}
}

func TestWebhookSettlementRollbackAllowsRedelivery(t *testing.T) {
	f := fixture(t)
	gateway := payments.NewFake("https://example.test")
	a := configureGateway(t, f, gateway)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	out, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "fake", IdempotencyKey: "rollback"}, payments.Buyer{Name: "Buyer", Email: "buyer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(payments.Notification{OrderRef: out.OrderReference, EventKey: "capture", Status: payments.StatusSucceeded, Amount: 10000, PaidAmount: 10000, Currency: "TRY", Test: true, ProviderPaymentID: out.OrderReference})
	headers := http.Header{}
	headers.Set("X-Fake-Signature", payments.FakeSignature(a.Credentials["secret_key"], body))
	triggerName := "s4_reject_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	create := fmt.Sprintf("CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.organization_id=%d THEN RAISE EXCEPTION 'test settlement failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE UPDATE ON billing_invoices FOR EACH ROW EXECUTE FUNCTION %s()", triggerName, f.org, triggerName, triggerName)
	if _, err = f.pool.Exec(f.ctx, create); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = f.pool.Exec(f.ctx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON billing_invoices; DROP FUNCTION IF EXISTS %s()", triggerName, triggerName))
	}
	defer cleanup()
	if _, err = f.svc.Notify(f.ctx, "fake", headers, body); err == nil {
		t.Fatal("expected transactional failure")
	}
	var state string
	var receipts int
	if err = f.pool.QueryRow(f.ctx, "SELECT status,(SELECT count(*) FROM billing_webhook_receipts WHERE organization_id=$1) FROM billing_payments WHERE organization_id=$1", f.org).Scan(&state, &receipts); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || receipts != 0 {
		t.Fatalf("partial settlement state=%s receipts=%d", state, receipts)
	}
	cleanup()
	if _, err = f.svc.Notify(f.ctx, "fake", headers, body); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredTrialUsesInvoiceAndAsyncPaymentLedger(t *testing.T) {
	f := fixture(t)
	g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	g.mode.Store(2)
	now := prepareRenewal(t, f, g)
	var end time.Time
	if err := f.pool.QueryRow(f.ctx, "UPDATE billing_subscriptions SET status='trialing',trial_ends_at=period_end WHERE uuid=$1 RETURNING period_end", f.sub.UUID).Scan(&end); err != nil {
		t.Fatal(err)
	}
	invoice := invoiceFixture(t, f, "open", end)
	// Timeout keeps a durable pending payment and grace; no untracked capture.
	if err := f.svc.TrialSweep(f.ctx, now); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("unknown trial %v", err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_payments WHERE organization_id=$1 AND status='unknown'", f.org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unknown claims %d %v", count, err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '3 minutes' WHERE organization_id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ReconcilePending(f.ctx); err != nil {
		t.Fatal(err)
	}
	current, err := f.q.BillingInvoiceID(f.ctx, db.BillingInvoiceIDParams{OrganizationID: f.org, ID: invoice.ID})
	if err != nil || current.Status != "paid" {
		t.Fatalf("trial invoice %+v %v", current, err)
	}
	sub, err := f.svc.Subscription(f.ctx, f.org)
	if err != nil || sub.Status != "active" || g.calls.Load() != 1 {
		t.Fatalf("trial %+v calls=%d err=%v", sub, g.calls.Load(), err)
	}
}

type mismatchGateway struct{ *payments.Fake }

func (p *mismatchGateway) CheckCharge(_ context.Context, _ payments.Account, in payments.ChargeRequest) (payments.Notification, error) {
	return payments.Notification{OrderRef: in.OrderRef, Status: payments.StatusSucceeded, Amount: in.Amount + 1, PaidAmount: in.Amount + 1, Currency: in.Currency, Test: true}, nil
}
func TestReconciliationMismatchReportsOnceWithoutSettling(t *testing.T) {
	f := fixture(t)
	g := &mismatchGateway{payments.NewFake("https://example.test")}
	configureGateway(t, f, g)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	_, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "fake", IdempotencyKey: "mismatch"}, payments.Buyer{Name: "Buyer", Email: "buyer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '3 minutes' WHERE organization_id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.svc.ReconcilePending(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var reports int
	var status string
	if err = f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM billing_reconciliation_mismatches WHERE organization_id=$1),status FROM billing_payments WHERE organization_id=$1", f.org).Scan(&reports, &status); err != nil {
		t.Fatal(err)
	}
	if reports != 1 || status != "pending" {
		t.Fatalf("reports=%d status=%s", reports, status)
	}
	// The isolated test fixture is finalized after asserting production leaves it pending.
	t.Cleanup(func() {
		_, _ = f.pool.Exec(f.ctx, "UPDATE billing_payments SET status='failed',failure_code='fixture_cleanup',completed_at=now() WHERE organization_id=$1", f.org)
	})
}

type unverifiedGateway struct{ *payments.Fake }

func (*unverifiedGateway) Confirm(context.Context, payments.Account, string, string) (payments.Notification, error) {
	return payments.Notification{}, payments.ErrInvalidSignature
}

// A stale page whose answer never verifies is reported once and stays
// pending instead of failing every provider sync.
func TestStaleUnverifiedCheckoutReportsOnceWithoutFailingSync(t *testing.T) {
	f := fixture(t)
	g := &unverifiedGateway{payments.NewFake("https://example.test")}
	configureGateway(t, f, g)
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	_, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, Provider: "fake", IdempotencyKey: "unverified"}, payments.Buyer{Name: "Buyer", Email: "buyer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '2 hours' WHERE organization_id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.svc.ReconcilePending(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var reports int
	var status string
	if err = f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM billing_reconciliation_mismatches WHERE organization_id=$1 AND reason='provider_signature_invalid'),status FROM billing_payments WHERE organization_id=$1", f.org).Scan(&reports, &status); err != nil {
		t.Fatal(err)
	}
	if reports != 1 || status != "pending" {
		t.Fatalf("reports=%d status=%s", reports, status)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(f.ctx, "UPDATE billing_payments SET status='failed',failure_code='fixture_cleanup',completed_at=now() WHERE organization_id=$1", f.org)
	})
}

func TestMissingTrialInvoiceDoesNotBlockOtherLifecycleDeadlines(t *testing.T) {
	f := fixture(t)
	g := &renewalGateway{Fake: payments.NewFake("https://example.test")}
	prepareRenewal(t, f, g)
	if _, err := f.pool.Exec(f.ctx, "UPDATE billing_profiles SET tax_id='incomplete' WHERE organization_id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE billing_subscriptions SET status='trialing',trial_ends_at=period_end WHERE uuid=$1", f.sub.UUID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "expired", Reason: "Fixture cleanup"}, f.actor)
	}()
	other := fixture(t)
	if _, err := other.pool.Exec(other.ctx, "UPDATE billing_subscriptions SET status='grace',period_start=now()-interval '1 month',period_end=now()-interval '2 days',grace_ends_at=now()-interval '1 minute' WHERE uuid=$1", other.sub.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Sweep(f.ctx); err == nil {
		t.Fatal("missing invoice must remain visible to worker retry")
	}
	sub, err := other.svc.Subscription(other.ctx, other.org)
	if err != nil || sub.Status != "suspended" {
		t.Fatalf("unrelated deadline %+v %v", sub, err)
	}
}

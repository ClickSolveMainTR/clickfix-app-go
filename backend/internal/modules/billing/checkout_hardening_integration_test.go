package billing_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/paytr"
	"github.com/google/uuid"
)

// CF-154 end-to-end checks of the payment flow through the HTTP handlers with
// fake providers (httptest / iyzicofake). No real provider is contacted.

type apiResult struct {
	status int
	header http.Header
	body   []byte
}

func (a apiResult) envelope(t *testing.T) (data json.RawMessage, code, message string) {
	t.Helper()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(a.body, &env); err != nil {
		t.Fatalf("envelope %s: %v", a.body, err)
	}
	if env.Error != nil {
		return env.Data, env.Error.Code, env.Error.Message
	}
	return env.Data, "", ""
}

// call runs one tenant request against the handler (org scope + principal
// as the real middleware chain sets them).
func call(t *testing.T, f *billingFixture, org int64, fn http.HandlerFunc, method, target string, body any, pathValues map[string]string, header http.Header) apiResult {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	for k, v := range header {
		r.Header[k] = v
	}
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	ctx := orgctx.WithScope(f.ctx, orgctx.Scope{ID: org, UUID: f.orgUUID})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: f.actor})
	w := httptest.NewRecorder()
	fn(w, r.WithContext(ctx))
	return apiResult{status: w.Code, header: w.Header(), body: w.Body.Bytes()}
}

// settleLeftovers finalizes this fixture's unresolved payments after the
// test: the reconciliation sweep is global and would query their (gone)
// fake providers from other tests.
func settleLeftovers(t *testing.T, f *billingFixture) {
	t.Cleanup(func() {
		_, _ = f.pool.Exec(f.ctx, "UPDATE billing_payments SET status='failed',failure_code='fixture_cleanup',completed_at=now() WHERE organization_id=$1 AND status IN ('pending','unknown')", f.org)
	})
}

func creditPackage(t *testing.T, f *billingFixture) usecase.CreditPackage {
	t.Helper()
	pkg, err := f.svc.CreateCreditPackage(f.ctx, usecase.CreditPackageInput{Code: "CF154_" + strings.ToUpper(uuid.NewString()[:8]), Name: "AI pack", Credits: 500, AmountMinor: 10000})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

// Sentry CLICKFIX-BACK-END-6: POST /v1/tenant/billing/credit-purchases
// answered 503 PAYMENT_GATEWAY_NOT_CONFIGURED while iyzico was configured.
// The cause was the invoice seller: BILLING_SELLER_JSON empty and no earlier
// invoice to copy it from. It is now a 409 with its own code and message.
func TestCreditPurchaseWithoutSellerIsNot503(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	if _, err := f.svc.SaveProfile(f.ctx, f.org, validBillingProfile()); err != nil {
		t.Fatal(err)
	}
	configureGateway(t, f, payments.NewFake("https://example.test"))
	h := handler.New(f.svc, f.ent, nil)
	pkg := creditPackage(t, f)
	res := call(t, f, f.org, h.CommerceAudit(h.PurchaseCredits), http.MethodPost, "/v1/tenant/billing/credit-purchases", map[string]any{"package_uuid": pkg.UUID, "idempotency_key": "k1"}, nil, http.Header{"Accept-Language": {"en"}})
	_, code, message := res.envelope(t)
	if res.status != http.StatusConflict || code != "BILLING_SELLER_NOT_CONFIGURED" || !strings.Contains(message, "invoicing company") {
		t.Fatalf("status=%d code=%s message=%q", res.status, code, message)
	}
	av := call(t, f, f.org, h.PaymentAvailability, http.MethodGet, "/v1/tenant/billing/payment-availability", nil, nil, nil)
	var out usecase.PaymentAvailability
	data, _, _ := av.envelope(t)
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Card.Ready || out.Card.Provider != "fake" || out.Invoicing.Ready || out.Invoicing.Reason != "BILLING_SELLER_NOT_CONFIGURED" || out.CreditPurchase.Ready {
		t.Fatalf("availability %+v", out)
	}
	configureInvoiceFixture(t, f)
	res = call(t, f, f.org, h.CommerceAudit(h.PurchaseCredits), http.MethodPost, "/v1/tenant/billing/credit-purchases", map[string]any{"package_uuid": pkg.UUID, "idempotency_key": "k1"}, nil, nil)
	if res.status != http.StatusCreated {
		t.Fatalf("after seller %d %s", res.status, res.body)
	}
	av = call(t, f, f.org, h.PaymentAvailability, http.MethodGet, "/v1/tenant/billing/payment-availability", nil, nil, nil)
	data, _, _ = av.envelope(t)
	if err := json.Unmarshal(data, &out); err != nil || !out.Ready || !out.CreditPurchase.Ready || out.Card.Mode != "sandbox" {
		t.Fatalf("availability after seller %+v %v", out, err)
	}
}

func TestCreditPurchaseBusinessErrors(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	configureGateway(t, f, payments.NewFake("https://example.test"))
	h := handler.New(f.svc, f.ent, nil)
	purchase := h.CommerceAudit(h.PurchaseCredits)
	pkg := creditPackage(t, f)
	for _, tc := range []struct {
		name   string
		org    int64
		pkg    uuid.UUID
		setup  func()
		status int
		code   string
	}{
		{name: "profile missing", org: f.org, pkg: pkg.UUID, status: http.StatusUnprocessableEntity, code: "BILLING_PROFILE_INCOMPLETE"},
		{name: "unknown package", org: f.org, pkg: uuid.New(), setup: func() { configureInvoiceFixture(t, f) }, status: http.StatusNotFound, code: "CREDIT_PACKAGE_UNAVAILABLE"},
		{name: "account missing", org: orgWithoutSubscription(t, f), pkg: pkg.UUID, status: http.StatusConflict, code: "BILLING_ACCOUNT_MISSING"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup()
			}
			res := call(t, f, tc.org, purchase, http.MethodPost, "/v1/tenant/billing/credit-purchases", map[string]any{"package_uuid": tc.pkg, "idempotency_key": uuid.NewString()}, nil, nil)
			_, code, message := res.envelope(t)
			if res.status != tc.status || code != tc.code || message == "" {
				t.Fatalf("status=%d code=%s message=%q", res.status, code, message)
			}
		})
	}
	// Explicitly asking for a provider that is not enabled is a 409, not 503.
	configureInvoiceFixture(t, f)
	invoice, err := f.svc.PurchaseCredits(f.ctx, f.org, usecase.CreditPurchaseInput{PackageUUID: pkg.UUID, IdempotencyKey: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	res := call(t, f, f.org, h.Checkout, http.MethodPost, "/v1/tenant/billing/checkout", map[string]any{"invoice_uuid": invoice.Uuid, "provider": "paytr", "idempotency_key": "x"}, nil, nil)
	if _, code, _ := res.envelope(t); res.status != http.StatusConflict || code != "PAYMENT_PROVIDER_DISABLED" {
		t.Fatalf("disabled provider %d %s", res.status, res.body)
	}
}

func orgWithoutSubscription(t *testing.T, f *billingFixture) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'No subscription') RETURNING id", uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The whole purchase with iyzico: credit purchase -> checkout -> browser
// return -> signed webhook (+ replays) -> credits granted exactly once.
func TestCreditPurchaseIyzicoEndToEnd(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	configureInvoiceFixture(t, f)
	fake := iyzicofake.New()
	defer fake.Server.Close()
	fake.AddAccount("sandbox-api-key", "sandbox-secret-key")
	configureGateway(t, f, iyzico.New(fake.URL(), fake.Server.Client()))
	f.svc.SetPaymentAPI("https://api.example.test")
	h := handler.New(f.svc, f.ent, nil)
	pkg := creditPackage(t, f)

	res := call(t, f, f.org, h.CommerceAudit(h.PurchaseCredits), http.MethodPost, "/v1/tenant/billing/credit-purchases", map[string]any{"package_uuid": pkg.UUID, "idempotency_key": "e2e"}, nil, nil)
	data, _, _ := res.envelope(t)
	var invoice struct {
		UUID       uuid.UUID `json:"uuid"`
		TotalMinor int64     `json:"total_minor"`
	}
	if res.status != http.StatusCreated || json.Unmarshal(data, &invoice) != nil {
		t.Fatalf("purchase %d %s", res.status, res.body)
	}
	checkout := map[string]any{"invoice_uuid": invoice.UUID, "idempotency_key": "dialog-1"}
	var outs [2]usecase.CheckoutOutput
	for i := range outs {
		// A double submit with the same key opens one page, not two.
		res = call(t, f, f.org, h.Checkout, http.MethodPost, "/v1/tenant/billing/checkout", checkout, nil, nil)
		data, _, _ = res.envelope(t)
		if res.status != http.StatusCreated || json.Unmarshal(data, &outs[i]) != nil {
			t.Fatalf("checkout %d %s", res.status, res.body)
		}
	}
	if outs[0] != outs[1] || len(fake.Tokens()) != 1 {
		t.Fatalf("double submit %+v tokens=%d", outs, len(fake.Tokens()))
	}
	// A new dialog (new key) while the page is live resumes the same page.
	res = call(t, f, f.org, h.Checkout, http.MethodPost, "/v1/tenant/billing/checkout", map[string]any{"invoice_uuid": invoice.UUID, "idempotency_key": "dialog-2"}, nil, nil)
	var resumed usecase.CheckoutOutput
	data, _, _ = res.envelope(t)
	if res.status != http.StatusCreated || json.Unmarshal(data, &resumed) != nil || resumed != outs[0] {
		t.Fatalf("resume %d %s", res.status, res.body)
	}
	token := fake.Tokens()[0]
	raw, sig, err := fake.Complete(token, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}

	// Browser return: verified through the API, then 303 to the panel.
	form := url.Values{"token": {token}}
	r := httptest.NewRequest(http.MethodPost, "/v1/public/billing/payments/iyzico/return/"+outs[0].OrderReference, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("reference", outs[0].OrderReference)
	w := httptest.NewRecorder()
	h.IyzicoReturn(w, r.WithContext(f.ctx))
	if loc := w.Header().Get("Location"); w.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "https://example.test/billing?") || !strings.Contains(loc, "payment=succeeded") {
		t.Fatalf("return %d %q", w.Code, loc)
	}

	notify := func(body []byte, signature string) int {
		r := httptest.NewRequest(http.MethodPost, "/v1/public/billing/payments/iyzico/notify", bytes.NewReader(body))
		r.SetPathValue("provider", "iyzico")
		r.RemoteAddr = "203.0.113.7:4000"
		if signature != "" {
			r.Header.Set(iyzico.SignatureHeader, signature)
		}
		w := httptest.NewRecorder()
		h.PaymentNotify(w, r.WithContext(f.ctx))
		return w.Code
	}
	if code := notify(raw, strings.Repeat("0", 64)); code != http.StatusBadRequest {
		t.Fatalf("forged webhook %d", code)
	}
	if code := notify([]byte("{"), sig); code != http.StatusBadRequest {
		t.Fatalf("malformed webhook %d", code)
	}
	for range 3 {
		if code := notify(raw, sig); code != http.StatusOK {
			t.Fatalf("webhook %d", code)
		}
	}
	wallet, err := f.svc.Wallet(f.ctx, f.org, 20, 0)
	if err != nil || wallet.AICredits != 500 {
		t.Fatalf("credits %d %v", wallet.AICredits, err)
	}
	var ledger int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_credit_ledger WHERE organization_id=$1 AND idempotency_key LIKE 'purchase:%'", f.org).Scan(&ledger); err != nil || ledger != 1 {
		t.Fatalf("ledger entries %d %v", ledger, err)
	}
	st := call(t, f, f.org, h.CheckoutStatus, http.MethodGet, "/v1/tenant/billing/checkout/"+outs[0].OrderReference, nil, map[string]string{"reference": outs[0].OrderReference}, nil)
	var status usecase.CheckoutStatus
	data, _, _ = st.envelope(t)
	if err = json.Unmarshal(data, &status); err != nil || status.Status != "succeeded" || status.InvoiceStatus != "paid" {
		t.Fatalf("status %+v %v", status, err)
	}
	// Paying a paid invoice again is refused with its own code.
	res = call(t, f, f.org, h.Checkout, http.MethodPost, "/v1/tenant/billing/checkout", map[string]any{"invoice_uuid": invoice.UUID, "idempotency_key": "dialog-3"}, nil, nil)
	if _, code, _ := res.envelope(t); res.status != http.StatusConflict || code != "INVOICE_NOT_PAYABLE" {
		t.Fatalf("paid invoice %d %s", res.status, res.body)
	}
}

// An abandoned page no longer blocks the invoice: past CheckoutTTL the
// payment expires (checkout or reconciliation), and a late success still
// settles it; the surplus of a second payment goes to the wallet.
func TestAbandonedCheckoutExpiresAndLateResultSettles(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	fake := iyzicofake.New()
	defer fake.Server.Close()
	fake.AddAccount("sandbox-api-key", "sandbox-secret-key")
	configureGateway(t, f, iyzico.New(fake.URL(), fake.Server.Client()))
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	buyer := payments.Buyer{Name: "Buyer", Email: "buyer@example.test", IP: "127.0.0.1"}
	first, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, IdempotencyKey: "first"}, buyer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_payments SET created_at=now()-interval '2 hours' WHERE uuid=$1", first.PaymentUUID); err != nil {
		t.Fatal(err)
	}
	// The sweep is global: payments of other fixtures (other gateways) may
	// report errors here; only this payment's outcome is asserted.
	_ = f.svc.ReconcilePending(f.ctx)
	var status string
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_payments WHERE uuid=$1", first.PaymentUUID).Scan(&status); err != nil || status != "expired" {
		t.Fatalf("reconciled abandoned checkout %s %v", status, err)
	}
	second, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, IdempotencyKey: "second"}, buyer)
	if err != nil || second.PaymentUUID == first.PaymentUUID {
		t.Fatalf("new checkout after expiry %+v %v", second, err)
	}
	// The DB refuses moves outside the state machine.
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_payments SET status='pending' WHERE uuid=$1", first.PaymentUUID); err == nil {
		t.Fatal("expired -> pending accepted")
	}
	// The payer completes the old page late: the expired payment settles.
	var token string
	if err = f.pool.QueryRow(f.ctx, "SELECT s.token FROM billing_checkout_sessions s JOIN billing_payments p ON p.id=s.payment_id WHERE p.uuid=$1", first.PaymentUUID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	raw, sig, err := fake.Complete(token, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set(iyzico.SignatureHeader, sig)
	if _, err = f.svc.Notify(f.ctx, "iyzico", headers, raw); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_payments WHERE uuid=$1", first.PaymentUUID).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("late result %s %v", status, err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE billing_payments SET status='failed' WHERE uuid=$1", first.PaymentUUID); err == nil {
		t.Fatal("succeeded -> failed accepted")
	}
}

// Same invoice, a definite provider refusal at page creation: the payment
// fails at once and the invoice can be paid with a new attempt.
func TestCheckoutRefusalReleasesInvoice(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	refuse := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refuse {
			_, _ = w.Write([]byte(`{"status":"failed","reason":"invalid merchant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","token":"page-token"}`))
	}))
	defer srv.Close()
	configureGateway(t, f, paytr.New(srv.URL, srv.Client()))
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	buyer := payments.Buyer{Name: "Buyer", Email: "buyer@example.test", IP: "127.0.0.1"}
	_, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, IdempotencyKey: "a"}, buyer)
	var refusal *payments.ProviderError
	if !errors.As(err, &refusal) {
		t.Fatalf("refusal %v", err)
	}
	refuse = false
	if _, err = f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, IdempotencyKey: "b"}, buyer); err != nil {
		t.Fatalf("retry after refusal %v", err)
	}
	h := handler.New(f.svc, f.ent, nil)
	res := call(t, f, f.org, h.Checkout, http.MethodPost, "/v1/tenant/billing/checkout", map[string]any{"invoice_uuid": invoice.Uuid, "idempotency_key": "a"}, nil, nil)
	if _, code, _ := res.envelope(t); res.status != http.StatusConflict || code != "CHECKOUT_FINISHED" {
		t.Fatalf("finished key %d %s", res.status, res.body)
	}
}

// A correctly signed PayTR notification whose amount differs from the
// server-side payment is never credited and is not retried (400).
func TestNotifyAmountMismatchAndUnknownOrder(t *testing.T) {
	f := fixture(t)
	settleLeftovers(t, f)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","token":"page-token"}`))
	}))
	defer srv.Close()
	a := configureGateway(t, f, paytr.New(srv.URL, srv.Client()))
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	out, err := f.svc.Checkout(f.ctx, f.org, usecase.CheckoutInput{InvoiceUUID: invoice.Uuid, IdempotencyKey: "a"}, payments.Buyer{Name: "Buyer", Email: "buyer@example.test", IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	h := handler.New(f.svc, f.ent, nil)
	post := func(form url.Values) (int, string) {
		r := httptest.NewRequest(http.MethodPost, "/v1/public/billing/payments/paytr/notify", strings.NewReader(form.Encode()))
		r.SetPathValue("provider", "paytr")
		w := httptest.NewRecorder()
		h.PaymentNotify(w, r.WithContext(f.ctx))
		return w.Code, w.Body.String()
	}
	form := url.Values{"merchant_oid": {out.OrderReference}, "status": {"success"}, "total_amount": {"100"}, "payment_amount": {"100"}, "currency": {"TL"}, "test_mode": {"1"}}
	form.Set("hash", paytr.NotificationHash(a, out.OrderReference, "success", "100"))
	if code, _ := post(form); code != http.StatusBadRequest {
		t.Fatalf("amount mismatch %d", code)
	}
	var status string
	var mismatches int
	if err = f.pool.QueryRow(f.ctx, "SELECT status FROM billing_payments WHERE uuid=$1", out.PaymentUUID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("mismatch settled %s %v", status, err)
	}
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_reconciliation_mismatches WHERE organization_id=$1", f.org).Scan(&mismatches); err != nil || mismatches != 1 {
		t.Fatalf("mismatch rows %d %v", mismatches, err)
	}
	// Signed, but not one of ours: acknowledged so PayTR stops retrying.
	unknown := url.Values{"merchant_oid": {"notours123"}, "status": {"failed"}, "total_amount": {"100"}}
	unknown.Set("hash", paytr.NotificationHash(a, "notours123", "failed", "100"))
	if code, body := post(unknown); code != http.StatusOK || body != "OK" {
		t.Fatalf("unknown order %d %q", code, body)
	}
	unknown.Set("hash", "forged")
	if code, _ := post(unknown); code != http.StatusBadRequest {
		t.Fatalf("unknown forged order %d", code)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/public/billing/payments/stripe/notify", strings.NewReader("x"))
	r.SetPathValue("provider", "stripe")
	w := httptest.NewRecorder()
	h.PaymentNotify(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown provider %d", w.Code)
	}
}

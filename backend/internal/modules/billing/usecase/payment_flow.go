package usecase

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	platformcrypto "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrUnknownOrder: a correctly signed notification names an order this
// deployment never created (another integration on the same merchant).
var ErrUnknownOrder = errors.New("billing: notification for an unknown order")

// ErrAmountMismatch: the provider reports an amount, currency or mode that
// differs from the server-side payment. It is never credited.
var ErrAmountMismatch = fmt.Errorf("%w: amount, currency or mode mismatch", model.ErrConflict)

// sameToken compares provider page tokens in constant time.
func sameToken(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
func puuid(v string) pgtype.UUID {
	u, e := uuid.Parse(v)
	return pgtype.UUID{Bytes: u, Valid: e == nil}
}
func scope(org int64) string { return fmt.Sprintf("org:%d", org) }

// ConfigurePayments is shared by API/worker and injectable fake integration tests.
func (s *Service) ConfigurePayments(g *TrialGateway, key, frontend string) error {
	box, err := platformcrypto.NewSecretBox(key)
	if err != nil {
		return err
	}
	g.Service = s
	s.gateway = g
	s.paymentBox = box
	s.paymentFrontend = strings.TrimRight(frontend, "/")
	s.SetTrialPayments(g)
	return nil
}

type CheckoutInput struct {
	InvoiceUUID    uuid.UUID `json:"invoice_uuid"`
	Provider       string    `json:"provider"`
	IdempotencyKey string    `json:"idempotency_key"`
	StoreCard      bool      `json:"store_card"`
	AutoPayConsent bool      `json:"auto_pay_consent"`
}
type CheckoutOutput struct {
	PaymentUUID    uuid.UUID `json:"payment_uuid"`
	OrderReference string    `json:"order_reference"`
	PageURL        string    `json:"page_url"`
}

type InvoiceSummary struct {
	Number           *string   `json:"number"`
	EInvoiceStatus   string    `json:"einvoice_status"`
	PDFURL           string    `json:"pdf_url"`
	XMLURL           string    `json:"xml_url"`
	UUID             uuid.UUID `json:"uuid"`
	PaymentReference string    `json:"payment_reference"`
	Status           string    `json:"status"`
	Currency         string    `json:"currency"`
	TotalMinor       int64     `json:"total_minor"`
	DueAt            time.Time `json:"due_at"`
	PeriodStart      time.Time `json:"period_start"`
	PeriodEnd        time.Time `json:"period_end"`
}

func (s *Service) InvoiceList(ctx context.Context, org int64, paging ...int32) ([]InvoiceSummary, error) {
	limit, offset := paymentPaging(paging)

	rows, err := s.repo.Q.BillingInvoices(ctx, db.BillingInvoicesParams{OrganizationID: org, Limit: limit, Offset: offset})
	out := make([]InvoiceSummary, 0, len(rows))
	for _, r := range rows {
		var number *string
		pdfURL, xmlURL := "", ""
		if r.Number.Valid {
			v := r.Number.String
			number = &v
			pdfURL = "/api/v1/tenant/billing/invoices/" + r.Uuid.String() + "/pdf"
			xmlURL = "/api/v1/tenant/billing/invoices/" + r.Uuid.String() + "/xml"
		}
		out = append(out, InvoiceSummary{Number: number, EInvoiceStatus: r.EinvoiceStatus, PDFURL: pdfURL, XMLURL: xmlURL, UUID: r.Uuid, PaymentReference: r.PaymentReference, Status: r.Status, Currency: r.Currency, TotalMinor: r.TotalMinor, DueAt: r.DueAt.Time, PeriodStart: r.PeriodStart.Time, PeriodEnd: r.PeriodEnd.Time})
	}
	return out, err
}
func (s *Service) Checkout(ctx context.Context, org int64, in CheckoutInput, buyer payments.Buyer) (CheckoutOutput, error) {
	var out CheckoutOutput
	if s.gateway == nil {
		return out, payments.ErrNotConfigured
	}
	if len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 120 || (in.AutoPayConsent && !in.StoreCard) {
		return out, model.ErrInvalid
	}
	var invoice db.BillingInvoice
	var payment db.BillingPayment
	var resumed *payments.Checkout
	now := time.Now().UTC()
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		var e error
		invoice, e = r.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: in.InvoiceUUID})
		if e != nil {
			return e
		}
		// The payer is the invoice's buyer snapshot (part of the attempt
		// fingerprint, so it is resolved before an idempotent replay).
		var profile ProfileInput
		if e = json.Unmarshal(invoice.BuyerSnapshot, &profile); e != nil {
			return model.ErrProfileIncomplete
		}
		if e = profile.Validate(); e != nil {
			return model.ErrProfileIncomplete
		}
		buyer.Name = profile.LegalName
		buyer.Email = profile.Emails[0]
		key := "checkout:" + in.IdempotencyKey
		payment, e = r.Q.BillingPaymentKey(ctx, db.BillingPaymentKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			// Same key again (double submit, retry): the same payment and page.
			if payment.InvoiceID != invoice.ID || (in.Provider != "" && payment.Provider != in.Provider) {
				return model.ErrConflict
			}
			sess, e := r.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: org, PaymentID: payment.ID})
			if e != nil {
				return e
			}
			if sess.StoreCard != in.StoreCard || sess.AutoPayConsent != in.AutoPayConsent {
				return model.ErrConflict
			}
			switch payment.Status {
			case "succeeded":
				return model.ErrInvoiceNotPayable
			case "failed", "expired":
				return model.ErrCheckoutFinished
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if invoice.Status != "open" || invoice.Kind == "credit_note" {
			return model.ErrInvoiceNotPayable
		}
		// One unresolved charge per invoice (uq_billing_payments_unresolved):
		// resume a live page, expire an abandoned one, refuse otherwise.
		if open, e := r.Q.BillingLockUnresolvedPayment(ctx, db.BillingLockUnresolvedPaymentParams{OrganizationID: org, InvoiceID: invoice.ID}); e == nil {
			resumed, e = s.openCheckout(ctx, r, open, now)
			if e != nil {
				return e
			}
			if resumed != nil {
				payment = open
				return nil
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		provider, e := s.gateway.Executor.Registry.Select(ctx, invoice.Currency, in.Provider)
		if e != nil {
			return e
		}
		a, e := s.gateway.Account(ctx, provider.Code(), "")
		if e != nil {
			return e
		}
		paid, e := r.Q.BillingPaidTotal(ctx, db.BillingPaidTotalParams{OrganizationID: org, InvoiceID: invoice.ID})
		if e != nil {
			return e
		}
		if invoice.TotalMinor <= paid {
			return model.ErrInvoiceNotPayable
		}
		// The amount comes from the server-side invoice, never the client.
		payment, e = r.Q.BillingNewPayment(ctx, db.BillingNewPaymentParams{OrganizationID: org, InvoiceID: invoice.ID, Provider: provider.Code(), MerchantAccountUuid: puuid(a.UUID), Kind: "checkout", IdempotencyKey: key, AmountMinor: invoice.TotalMinor - paid, OrderReference: text(OrderReference(org, key))})
		if e != nil {
			return e
		}
		return r.Q.BillingNewCheckoutSession(ctx, db.BillingNewCheckoutSessionParams{OrganizationID: org, PaymentID: payment.ID, StoreCard: in.StoreCard, AutoPayConsent: in.AutoPayConsent})
	})
	if err != nil {
		return out, err
	}
	if resumed != nil {
		return CheckoutOutput{PaymentUUID: payment.Uuid, OrderReference: payment.OrderReference.String, PageURL: resumed.PageURL}, nil
	}
	p, err := s.gateway.Executor.Registry.Get(payment.Provider)
	if err != nil {
		return out, err
	}
	a, err := s.gateway.Account(ctx, p.Code(), uuid.UUID(payment.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return out, err
	}
	// Buyer identity is resolved server-side; persisted attempts fingerprint only the
	// provider request. An idempotent retry must use the same authenticated payer.
	// Browser returns only say where to look; the billing page polls the
	// verified status (CheckoutStatus) before it shows a result.
	result := s.paymentFrontend + "/billing?payment_ref=" + url.QueryEscape(payment.OrderReference.String)
	req := payments.CheckoutRequest{Amount: payment.AmountMinor, Currency: invoice.Currency, ItemName: "ClickFix " + invoice.PaymentReference, Buyer: buyer, StoreCard: in.StoreCard, Recurring: true, SuccessURL: result + "&payment=processing", FailURL: result + "&payment=failed", ReturnURL: s.ReturnURL(payment.OrderReference.String)}
	checkout, err := s.gateway.Executor.Checkout(ctx, org, payment.IdempotencyKey, p, a, req, false)
	if err != nil {
		// A definite refusal opened no page: release the invoice for another
		// attempt instead of blocking it until reconciliation.
		var refusal *payments.ProviderError
		if errors.As(err, &refusal) && !refusal.Temporary {
			code := refusal.Code
			if code == "" {
				code = "checkout_refused"
			}
			persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayPersistTimeout)
			defer cancel()
			_, _ = s.repo.Q.BillingFailPayment(persist, db.BillingFailPaymentParams{OrganizationID: org, ID: payment.ID, FailureCode: text(code)})
		}
		return out, err
	}
	if err = s.repo.Q.BillingSetCheckoutToken(ctx, db.BillingSetCheckoutTokenParams{OrganizationID: org, PaymentID: payment.ID, Token: checkout.Token}); err != nil {
		return out, err
	}
	return CheckoutOutput{PaymentUUID: payment.Uuid, OrderReference: payment.OrderReference.String, PageURL: checkout.PageURL}, nil
}

// ReturnURL is where a Confirmer provider (iyzico) posts the browser back.
// It must reach the API directly: the BFF refuses cross-site POSTs.
func (s *Service) ReturnURL(ref string) string {
	base := s.paymentAPI
	if base == "" {
		base = s.paymentFrontend + "/api"
	}
	return base + "/v1/public/billing/payments/iyzico/return/" + url.PathEscape(ref)
}

// PaymentFrontend is the panel origin the browser returns to.
func (s *Service) PaymentFrontend() string { return s.paymentFrontend }

// SetPaymentAPI sets the public API origin used for provider return URLs.
func (s *Service) SetPaymentAPI(publicAPIURL string) {
	s.paymentAPI = strings.TrimRight(publicAPIURL, "/")
}

// openCheckout handles the unresolved charge of an invoice (locked by the
// caller). It returns the live page to resume, or nil after expiring an
// abandoned checkout (no page was ever opened, or the page outlived
// CheckoutTTL). Anything else still in flight is ErrCheckoutInProgress.
func (s *Service) openCheckout(ctx context.Context, r *repository.Repository, open db.BillingPayment, now time.Time) (*payments.Checkout, error) {
	if open.Kind != "checkout" {
		return nil, model.ErrCheckoutInProgress
	}
	sess, err := r.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: open.OrganizationID, PaymentID: open.ID})
	if err != nil {
		return nil, err
	}
	stale := now.Sub(open.CreatedAt.Time) > CheckoutTTL
	if sess.Token == "" || stale {
		code := "checkout_abandoned"
		if sess.Token == "" {
			code = "checkout_not_opened"
		}
		if _, err = r.Q.BillingExpirePayment(ctx, db.BillingExpirePaymentParams{OrganizationID: open.OrganizationID, ID: open.ID, FailureCode: text(code)}); err != nil {
			return nil, err
		}
		return nil, r.Emit(ctx, events.New(events.PaymentExpired).WithTenant(open.OrganizationID).WithEntity("billing_payments", &open.ID, &open.Uuid).WithPayload(map[string]any{"reason": code}))
	}
	attempt, err := r.Q.BillingAttempt(ctx, db.BillingAttemptParams{OrganizationID: open.OrganizationID, IdempotencyKey: open.IdempotencyKey})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrCheckoutInProgress
		}
		return nil, err
	}
	var page payments.Checkout
	if attempt.Status != "succeeded" || json.Unmarshal(attempt.Result, &page) != nil || page.PageURL == "" {
		return nil, model.ErrCheckoutInProgress
	}
	return &page, nil
}

// Notify verifies before persisting encrypted raw evidence. Settlement and replay
// receipt are committed together, so a rollback permits safe provider redelivery.
//
// Errors: payments.ErrInvalidSignature / ErrMalformed (reject), ErrUnknownOrder
// (signed but not ours: acknowledge), ErrProviderDisabled (no such provider
// here), ErrAmountMismatch and other model errors (permanent rejection, never
// credited); anything else is transient and the provider may retry.
func (s *Service) Notify(ctx context.Context, provider string, headers http.Header, body []byte) (payments.Provider, error) {
	if s.gateway == nil {
		return nil, payments.ErrProviderDisabled
	}
	p, err := s.gateway.Executor.Registry.Get(provider)
	if err != nil {
		return nil, payments.ErrProviderDisabled
	}
	n, err := p.ParseNotification(headers, body)
	if err != nil {
		return p, err
	}
	payment, err := s.repo.Q.BillingPaymentOrder(ctx, db.BillingPaymentOrderParams{Provider: provider, OrderReference: text(n.OrderRef)})
	if errors.Is(err, pgx.ErrNoRows) {
		// Only a correctly signed message earns an acknowledgement; anything
		// else is rejected like a bad signature.
		a, e := s.gateway.Account(ctx, provider, "")
		if e != nil || p.VerifyNotification(a, n, headers, body) != nil {
			return p, payments.ErrInvalidSignature
		}
		return p, ErrUnknownOrder
	}
	if err != nil {
		return p, err
	}
	a, err := s.gateway.Account(ctx, provider, uuid.UUID(payment.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return p, err
	}
	if err = p.VerifyNotification(a, n, headers, body); err != nil {
		return p, err
	}
	if confirmer, ok := payments.ConfirmerOf(p); ok {
		if payment.Kind == "checkout" {
			sess, e := s.repo.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: payment.OrganizationID, PaymentID: payment.ID})
			if e != nil {
				return p, e
			}
			if sess.Token != "" && !sameToken(sess.Token, n.Token) {
				return p, payments.ErrInvalidSignature
			}
			n, err = confirmer.Confirm(ctx, a, n.OrderRef, n.Token)
		} else if checker, ok := payments.ChargeCheckerOf(p); ok {
			n, err = checker.CheckCharge(ctx, a, payments.ChargeRequest{OrderRef: n.OrderRef, Amount: payment.AmountMinor, Currency: payment.Currency})
		} else {
			return p, payments.ErrUnsupported
		}
		if err != nil {
			return p, err
		}
	}
	if n.OrderRef != payment.OrderReference.String {
		return p, payments.ErrInvalidSignature
	}
	// PayTR does not sign currency or payment_amount. Signed total must cover the
	// stored amount; unsigned optional fields are checked only for consistency.
	if n.Status == payments.StatusSucceeded && (n.PaidAmount < payment.AmountMinor || n.Amount != payment.AmountMinor || n.Currency != payment.Currency || n.Test != (a.Mode != "live")) {
		// Never credited. Recorded for the reconciliation report so a person
		// looks at it; the provider gets a permanent rejection, not a retry.
		if e := s.mismatch(ctx, payment, 0, "notify_amount_currency_mode_mismatch"); e != nil {
			return p, e
		}
		return p, ErrAmountMismatch
	}
	// Card tokens are not covered by PayTR's callback hash. Confirm membership
	// through the authenticated CAPI list before persisting a recurring mandate.
	if provider == "paytr" && n.Status == payments.StatusSucceeded && n.Card != nil {
		cards, e := p.(payments.CardLister).ListSavedCards(ctx, a, n.Card.OwnerKey)
		if e != nil {
			return p, e
		}
		selected := (*payments.SavedCard)(nil)
		for _, card := range cards {
			if (n.Card.Token != "" && card.Token == n.Card.Token) || (n.Card.Token == "" && len(cards) == 1) {
				copy := card
				selected = &copy
				break
			}
		}
		n.Card = selected
	}
	if n.EventKey == "" {
		n.EventKey = n.OrderRef + ":" + n.Status
	}
	raw, err := s.paymentBox.Encrypt(string(body))
	if err != nil {
		return p, err
	}
	digest := sha256.Sum256(body)
	signature := headers.Get("X-IYZ-SIGNATURE-V3")
	if provider == "paytr" {
		v, _ := url.ParseQuery(string(body))
		signature = v.Get("hash")
	}
	err = s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, payment.OrganizationID); e != nil {
			return e
		}
		locked, e := r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: payment.OrganizationID, ID: payment.ID})
		if e != nil {
			return e
		}
		added, e := r.Q.BillingWebhookReceipt(ctx, db.BillingWebhookReceiptParams{OrganizationID: payment.OrganizationID, PaymentID: payment.ID, Provider: provider, EventKey: n.EventKey, BodyEnc: raw, PayloadDigest: hex.EncodeToString(digest[:]), Signature: signature})
		if e != nil || added == 0 {
			return e
		}
		if e = r.Q.BillingGatewayFact(ctx, db.BillingGatewayFactParams{OrganizationID: payment.OrganizationID, PaymentID: payment.ID, EventKey: n.EventKey, Operation: "callback", PayloadDigest: hex.EncodeToString(digest[:]), ResultCode: n.Status}); e != nil {
			return e
		}
		if e = s.settle(ctx, r, locked, n, time.Now().UTC()); e != nil {
			return e
		}
		if n.Status == payments.StatusFailed && locked.Kind == "recurring" && locked.Status != "failed" {
			invoice, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: locked.OrganizationID, ID: locked.InvoiceID})
			if e != nil {
				return e
			}
			sub, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: locked.OrganizationID, ID: invoice.SubscriptionID})
			if e != nil {
				return e
			}
			return s.dunning(ctx, r, invoice, sub, time.Now().UTC(), n.FailureCode, true)
		}
		return nil
	})
	return p, err
}
func (s *Service) ConfirmCheckout(ctx context.Context, ref, token string) error {
	_, err := s.ReturnCheckout(ctx, ref, token)
	return err
}

// ReturnCheckout confirms an iyzico browser return through the provider API
// and answers the payment status afterwards (succeeded, failed, expired or
// pending). The posted form is never trusted beyond its token.
func (s *Service) ReturnCheckout(ctx context.Context, ref, token string) (string, error) {
	if s.gateway == nil {
		return "", payments.ErrProviderDisabled
	}
	status := ""
	err := s.confirmCheckout(ctx, ref, token, &status)
	return status, err
}

func (s *Service) confirmCheckout(ctx context.Context, ref, token string, status *string) error {
	payment, err := s.repo.Q.BillingPaymentOrder(ctx, db.BillingPaymentOrderParams{Provider: "iyzico", OrderReference: text(ref)})
	if err != nil {
		return repository.Normalize(err)
	}
	sess, err := s.repo.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: payment.OrganizationID, PaymentID: payment.ID})
	if err != nil {
		return err
	}
	if token == "" || !sameToken(sess.Token, token) {
		return payments.ErrInvalidSignature
	}
	p, err := s.gateway.Executor.Registry.Get("iyzico")
	if err != nil {
		return err
	}
	confirmer, ok := payments.ConfirmerOf(p)
	if !ok {
		return payments.ErrUnsupported
	}
	a, err := s.gateway.Account(ctx, "iyzico", uuid.UUID(payment.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return err
	}
	*status = payment.Status
	if payment.Status == "succeeded" || payment.Status == "failed" {
		return nil
	}
	n, err := confirmer.Confirm(ctx, a, ref, token)
	if err != nil {
		return err
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, payment.OrganizationID); e != nil {
			return e
		}
		locked, e := r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: payment.OrganizationID, ID: payment.ID})
		if e != nil {
			return e
		}
		if e = s.settle(ctx, r, locked, n, time.Now().UTC()); e != nil {
			return e
		}
		*status = locked.Status
		if n.Status == payments.StatusSucceeded || n.Status == payments.StatusFailed {
			*status = n.Status
		}
		return nil
	})
}

// CheckoutStatus is what the billing page polls after a provider return.
type CheckoutStatus struct {
	PaymentUUID   uuid.UUID `json:"payment_uuid"`
	Status        string    `json:"status"`
	FailureCode   string    `json:"failure_code,omitempty"`
	Provider      string    `json:"provider"`
	InvoiceUUID   uuid.UUID `json:"invoice_uuid"`
	InvoiceStatus string    `json:"invoice_status"`
}

func (s *Service) CheckoutStatus(ctx context.Context, org int64, ref string) (CheckoutStatus, error) {
	if ref == "" || len(ref) > 64 {
		return CheckoutStatus{}, model.ErrInvalid
	}
	row, err := s.repo.Q.BillingPaymentStatus(ctx, db.BillingPaymentStatusParams{OrganizationID: org, OrderReference: text(ref)})
	if err != nil {
		return CheckoutStatus{}, repository.Normalize(err)
	}
	return CheckoutStatus{PaymentUUID: row.Uuid, Status: row.Status, FailureCode: row.FailureCode.String, Provider: row.Provider, InvoiceUUID: row.InvoiceUuid, InvoiceStatus: row.InvoiceStatus}, nil
}
func (s *Service) settle(ctx context.Context, r *repository.Repository, p db.BillingPayment, n payments.Notification, now time.Time) error {
	if n.Status == payments.StatusPending {
		return nil
	}
	if n.Status != payments.StatusSucceeded && n.Status != payments.StatusFailed {
		return model.ErrInvalid
	}
	if p.Status == "succeeded" || p.Status == "failed" {
		if p.Status == n.Status {
			return nil
		}
		return model.ErrConflict
	}
	if n.OrderRef != p.OrderReference.String {
		return model.ErrConflict
	}
	if n.Status == payments.StatusSucceeded && (n.Amount != p.AmountMinor || n.PaidAmount < p.AmountMinor || n.Currency != p.Currency) {
		return model.ErrConflict
	}
	invoice, err := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: p.OrganizationID, ID: p.InvoiceID})
	if err != nil {
		return err
	}
	if invoice.Status != "open" && invoice.Status != "paid" {
		return model.ErrConflict
	}
	before, err := r.Q.BillingPaidTotal(ctx, db.BillingPaidTotalParams{OrganizationID: p.OrganizationID, InvoiceID: p.InvoiceID})
	if err != nil {
		return err
	}
	if p.AmountMinor > math.MaxInt64-before {
		return model.ErrInvalid
	}
	_, err = r.Q.BillingSetPayment(ctx, db.BillingSetPaymentParams{OrganizationID: p.OrganizationID, ID: p.ID, Status: n.Status, ProviderReference: text(n.ProviderPaymentID), FailureCode: text(n.FailureCode), CompletedAt: repository.TS(now)})
	if err != nil {
		return err
	}
	if err = r.Emit(ctx, events.New("payment."+n.Status).WithTenant(p.OrganizationID).WithEntity("billing_payments", &p.ID, &p.Uuid).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "amount_minor": p.AmountMinor, "currency": p.Currency})); err != nil {
		return err
	}
	if n.Status == payments.StatusFailed {
		return nil
	}
	sub, err := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: p.OrganizationID, ID: invoice.SubscriptionID})
	if err != nil {
		return err
	}
	excess := max(before+p.AmountMinor-invoice.TotalMinor, 0) - max(before-invoice.TotalMinor, 0)
	if excess > 0 {
		if err = r.Q.BillingCreditOverpayment(ctx, db.BillingCreditOverpaymentParams{OrganizationID: p.OrganizationID, SubscriptionID: sub.ID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}, Delta: excess, IdempotencyKey: "excess:" + p.Uuid.String()}); err != nil {
			return err
		}
	}
	if before+p.AmountMinor >= invoice.TotalMinor && invoice.Status == "open" {
		if err = r.Q.BillingEnableRenewal(ctx, db.BillingEnableRenewalParams{OrganizationID: p.OrganizationID, ID: sub.ID, RenewalEnabled: true}); err != nil {
			return err
		}
		if err = r.Q.BillingPayInvoice(ctx, db.BillingPayInvoiceParams{OrganizationID: p.OrganizationID, ID: invoice.ID}); err != nil {
			return err
		}
		if err = r.Q.BillingResolveDunning(ctx, db.BillingResolveDunningParams{OrganizationID: p.OrganizationID, InvoiceID: invoice.ID}); err != nil {
			return err
		}
		if err = s.settleCommerce(ctx, r, invoice, sub); err != nil {
			return err
		}
		sub, err = r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: p.OrganizationID, ID: sub.ID})
		if err != nil {
			return err
		}
		if invoice.Kind == "renewal" && sub.Status != "canceled" && sub.Status != "expired" && sub.Status != "paused" {
			transition := model.Transition{To: "active", Reason: "Invoice payment settled"}
			if invoice.Kind == "renewal" && !invoice.PeriodStart.Time.Before(sub.PeriodEnd.Time) && !sub.CancelAtPeriodEnd {
				start, end := invoice.PeriodStart.Time, invoice.PeriodEnd.Time
				transition.PeriodStart = &start
				transition.PeriodEnd = &end
			}
			if sub.Status != "active" || transition.PeriodStart != nil {
				if _, err = applyTransition(ctx, r, sub, transition, 0, now); err != nil {
					return err
				}
			}
		}
	}
	if n.Card != nil && s.gateway != nil && p.Kind == "checkout" {
		sess, e := r.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: p.OrganizationID, PaymentID: p.ID})
		if e != nil {
			return e
		}
		if sess.StoreCard {
			if err = s.savePaymentCard(ctx, r, p, sub, *n.Card, sess.AutoPayConsent); err != nil {
				return err
			}
		}
	}
	return nil
}

var last4RE = regexp.MustCompile(`^[0-9]{4}$`)

func (s *Service) savePaymentCard(ctx context.Context, r *repository.Repository, p db.BillingPayment, sub db.BillingSubscription, c payments.SavedCard, consent bool) error {
	if !last4RE.MatchString(c.Last4) || c.OwnerKey == "" || c.Token == "" {
		return nil
	}
	id := uuid.New()
	owner, token, err := s.gateway.Vault.Seal(scope(p.OrganizationID), id, c)
	if err != nil {
		return err
	}
	card, err := r.Q.BillingSaveCard(ctx, db.BillingSaveCardParams{Uuid: id, OrganizationID: p.OrganizationID, Provider: p.Provider, MerchantAccountUuid: uuid.UUID(p.MerchantAccountUuid.Bytes), CustomerTokenEnc: owner, CardTokenEnc: token, TokenHash: s.gateway.Vault.TokenHash(scope(p.OrganizationID), p.Provider, c.Token), Last4: c.Last4, Brand: c.Brand})
	if err != nil {
		return err
	}
	// Existing token rows retain their original UUID-bound ciphertext.
	return r.Q.BillingUseCard(ctx, db.BillingUseCardParams{OrganizationID: p.OrganizationID, ID: sub.ID, PaymentMethodID: pgtype.Int8{Int64: card.ID, Valid: true}, AutoPay: consent})
}
func (s *Service) Cards(ctx context.Context, org int64, paging ...int32) ([]db.BillingCardsRow, error) {
	limit, offset := paymentPaging(paging)

	return s.repo.Q.BillingCards(ctx, db.BillingCardsParams{OrganizationID: org, Limit: limit, Offset: offset})
}
func (s *Service) RemovePaymentCard(ctx context.Context, org int64, id uuid.UUID) error {
	if s.gateway == nil {
		return payments.ErrNotConfigured
	}
	card, err := s.repo.Q.BillingCardUUID(ctx, db.BillingCardUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return repository.Normalize(err)
	}
	p, err := s.gateway.Executor.Registry.Get(card.Provider)
	if err != nil {
		return err
	}
	a, err := s.gateway.Account(ctx, card.Provider, card.MerchantAccountUuid.String())
	if err != nil {
		return err
	}
	owner, token, err := s.gateway.Vault.Open(scope(org), card.Uuid, card.CustomerTokenEnc, card.CardTokenEnc)
	if err != nil {
		return err
	}
	if err = s.gateway.Executor.RemoveCard(ctx, org, "remove:"+id.String(), p, a, payments.SavedCard{OwnerKey: owner, Token: token}); err != nil {
		return err
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		if e := r.Q.BillingDisableCardAutoPay(ctx, db.BillingDisableCardAutoPayParams{OrganizationID: org, PaymentMethodID: pgtype.Int8{Int64: card.ID, Valid: true}}); e != nil {
			return e
		}
		return r.Q.BillingRemoveCard(ctx, db.BillingRemoveCardParams{OrganizationID: org, ID: card.ID})
	})
}

func (s *Service) CheckoutBuyer(ctx context.Context, org int64, ip string) (payments.Buyer, error) {
	profile, err := s.repo.Q.BillingProfile(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return payments.Buyer{}, model.ErrProfileIncomplete
	}
	if err != nil {
		return payments.Buyer{}, err
	}
	if profile.Email == "" || profile.LegalName == "" {
		return payments.Buyer{}, model.ErrProfileIncomplete
	}
	return payments.Buyer{Name: profile.LegalName, Email: profile.Email, IP: ip}, nil
}

func paymentPaging(p []int32) (int32, int32) {
	limit, offset := int32(20), int32(0)
	if len(p) > 0 && p[0] > 0 && p[0] <= 100 {
		limit = p[0]
	}
	if len(p) > 1 && p[1] > 0 {
		offset = p[1]
	}
	return limit, offset
}
func (s *Service) PaymentListCounts(ctx context.Context, org int64, admin bool) (db.BillingPaymentListCountsRow, error) {
	return s.repo.Q.BillingPaymentListCounts(ctx, db.BillingPaymentListCountsParams{OrganizationID: org, IncludeInactive: admin})
}

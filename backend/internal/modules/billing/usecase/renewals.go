package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// NextPeriod clamps month ends (Jan 31 -> Feb 28) instead of Go's rollover.
func NextPeriod(start time.Time, annual bool) time.Time {
	months := 1
	if annual {
		months = 12
	}
	first := time.Date(start.Year(), start.Month(), 1, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), start.Location()).AddDate(0, months, 0)
	last := first.AddDate(0, 1, -1).Day()
	day := min(start.Day(), last)
	return time.Date(first.Year(), first.Month(), day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), start.Location())
}

// RetryAt uses offsets from the first refusal: initial call + days 1, 3, 5, 7.
func RetryAt(first time.Time, failedAttempts int32) *time.Time {
	days := []int{1, 3, 5, 7}
	if failedAttempts < 1 || int64(failedAttempts) > int64(len(days)) {
		return nil
	}
	at := first.AddDate(0, 0, days[failedAttempts-1])
	return &at
}

func (s *Service) RenewalSweep(ctx context.Context, now time.Time) error {
	failures := []error{}
	candidates, err := s.repo.Q.BillingRenewalCandidates(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	for _, sub := range candidates {
		if err = s.openRenewal(ctx, sub, now); err != nil {
			failures = append(failures, fmt.Errorf("renewal %s: %w", sub.Uuid, err))
		}
	}
	invoices, err := s.repo.Q.BillingDueInvoices(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	for _, invoice := range invoices {
		if err = s.chargeRenewal(ctx, invoice, now); err != nil {
			failures = append(failures, fmt.Errorf("invoice %s: %w", invoice.Uuid, err))
		}
	}
	if err = s.renewalReminders(ctx, now); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (s *Service) openRenewal(ctx context.Context, sub db.BillingSubscription, now time.Time) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, sub.OrganizationID); e != nil {
			return e
		}
		current, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: sub.OrganizationID, ID: sub.ID})
		if e != nil {
			return e
		}
		if !current.RenewalEnabled || current.Status != "active" || current.CancelAtPeriodEnd || current.PeriodEnd.Time.After(now) {
			return nil
		}
		invoice, e := s.periodInvoice(ctx, r, current, now)
		if e != nil {
			return e
		}
		// Unique outbox event id is derived from the invoice, so reopening is harmless.
		event := events.New("billing.renewal_due").WithTenant(sub.OrganizationID).WithEntity("billing_invoices", &invoice.ID, &invoice.Uuid).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "payment_reference": invoice.PaymentReference, "due_at": invoice.DueAt.Time})
		event.EventID = uuid.NewSHA1(invoice.Uuid, []byte("renewal_due"))
		return emitPaymentNotice(ctx, r, event)
	})
}

func (s *Service) chargeRenewal(ctx context.Context, invoice db.BillingInvoice, now time.Time) error {
	var payment db.BillingPayment
	var method db.BillingPaymentMethod
	call := false
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, invoice.OrganizationID); e != nil {
			return e
		}
		current, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: invoice.OrganizationID, ID: invoice.ID})
		if e != nil {
			return e
		}
		if current.Status != "open" {
			return nil
		}
		sub, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: invoice.OrganizationID, ID: invoice.SubscriptionID})
		if e != nil {
			return e
		}
		// Auto top-up (wallet invoices) runs on the consented card whatever
		// the renewal state; only subscription invoices follow it.
		if invoice.Kind != "wallet" && (sub.CancelAtPeriodEnd || !sub.RenewalEnabled) {
			return nil
		}
		d, e := r.Q.BillingDunning(ctx, db.BillingDunningParams{OrganizationID: invoice.OrganizationID, InvoiceID: invoice.ID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e == nil && (d.ResolvedAt.Valid || !d.NextAttemptAt.Valid || d.NextAttemptAt.Time.After(now) || !d.GraceEndsAt.Time.After(now)) {
			return nil
		}
		// Existing unresolved claim is only reconciled. No expiry unlocks a new charge.
		_, e = r.Q.BillingUnresolvedPayment(ctx, db.BillingUnresolvedPaymentParams{OrganizationID: invoice.OrganizationID, InvoiceID: invoice.ID})
		if e == nil {
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if !sub.AutoPay || !sub.PaymentMethodID.Valid {
			return s.dunning(ctx, r, current, sub, now, "bank_transfer_due", false)
		}
		if s.gateway == nil {
			return payments.ErrNotConfigured
		}
		method, e = r.Q.BillingPaymentMethod(ctx, db.BillingPaymentMethodParams{OrganizationID: invoice.OrganizationID, ID: sub.PaymentMethodID.Int64})
		if e != nil {
			return e
		}
		// Alternate only among consented merchant tokens from this provider; the
		// checkout consent authorizes this subscription, not arbitrary merchant cards.
		if d.Attempts > 0 {
			cards, e := r.Q.BillingActiveCards(ctx, db.BillingActiveCardsParams{OrganizationID: sub.OrganizationID, Provider: method.Provider, MerchantAccountUuid: method.MerchantAccountUuid})
			if e != nil {
				return e
			}
			if len(cards) > 0 {
				method = cards[int(d.Attempts)%len(cards)]
			}
		}
		key := fmt.Sprintf("renewal:%s:%d", invoice.Uuid, d.Attempts)
		paid, e := r.Q.BillingPaidTotal(ctx, db.BillingPaidTotalParams{OrganizationID: invoice.OrganizationID, InvoiceID: invoice.ID})
		if e != nil {
			return e
		}
		if current.TotalMinor <= paid {
			return model.ErrConflict
		}
		payment, e = r.Q.BillingNewPayment(ctx, db.BillingNewPaymentParams{OrganizationID: invoice.OrganizationID, InvoiceID: invoice.ID, Provider: method.Provider, MerchantAccountUuid: puuid(method.MerchantAccountUuid.String()), Kind: "recurring", IdempotencyKey: key, AmountMinor: current.TotalMinor - paid, PaymentMethodID: pgtype.Int8{Int64: method.ID, Valid: true}, OrderReference: text(OrderReference(invoice.OrganizationID, key))})
		if e != nil {
			return e
		}
		call = true
		return nil
	})
	if err != nil || !call {
		return err
	}
	p, a, in, err := s.recurringRequest(ctx, payment, method)
	if err != nil {
		return err
	}
	result, callErr := s.gateway.Executor.Charge(ctx, payment.OrganizationID, payment.IdempotencyKey, p, a, in)
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, payment.OrganizationID); e != nil {
			return e
		}
		locked, e := r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: payment.OrganizationID, ID: payment.ID})
		if e != nil {
			return e
		}
		if locked.Status == "succeeded" || locked.Status == "failed" {
			return nil
		}
		if callErr == nil {
			return s.settle(ctx, r, locked, payments.Notification{OrderRef: payment.OrderReference.String, Status: payments.StatusSucceeded, Amount: payment.AmountMinor, PaidAmount: result.PaidAmount, Currency: payment.Currency, ProviderPaymentID: result.ProviderPaymentID}, now)
		}
		var refusal *payments.ProviderError
		if errors.As(callErr, &refusal) && !refusal.Temporary {
			if e = s.settle(ctx, r, locked, payments.Notification{OrderRef: payment.OrderReference.String, Status: payments.StatusFailed, FailureCode: refusal.Code}, now); e != nil {
				return e
			}
			sub, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: payment.OrganizationID, ID: invoice.SubscriptionID})
			if e != nil {
				return e
			}
			return s.dunning(ctx, r, invoice, sub, now, refusal.Code, true)
		}
		_, e = r.Q.BillingSetPayment(ctx, db.BillingSetPaymentParams{OrganizationID: payment.OrganizationID, ID: payment.ID, Status: "unknown", FailureCode: text("await_reconciliation")})
		if e != nil {
			return e
		}
		// Unknown money outcomes still start access deadlines, but schedule no charge.
		sub, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: payment.OrganizationID, ID: invoice.SubscriptionID})
		if e != nil {
			return e
		}
		return s.dunning(ctx, r, invoice, sub, now, "await_reconciliation", false)
	})
}

func (s *Service) recurringRequest(ctx context.Context, p db.BillingPayment, method db.BillingPaymentMethod) (payments.Provider, payments.Account, payments.ChargeRequest, error) {
	var in payments.ChargeRequest
	if s.gateway == nil {
		return nil, payments.Account{}, in, payments.ErrNotConfigured
	}
	provider, err := s.gateway.Executor.Registry.Get(p.Provider)
	if err != nil {
		return nil, payments.Account{}, in, err
	}
	a, err := s.gateway.Account(ctx, p.Provider, uuid.UUID(p.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return nil, a, in, err
	}
	owner, token, err := s.gateway.Vault.Open(scope(p.OrganizationID), method.Uuid, method.CustomerTokenEnc, method.CardTokenEnc)
	if err != nil {
		return nil, a, in, err
	}
	invoice, err := s.repo.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: p.OrganizationID, ID: p.InvoiceID})
	if err != nil {
		return nil, a, in, err
	}
	var buyer struct {
		LegalName string   `json:"legal_name"`
		Email     string   `json:"email"`
		Emails    []string `json:"emails"`
	}
	if err = json.Unmarshal(invoice.BuyerSnapshot, &buyer); err != nil {
		return nil, a, in, err
	}
	if buyer.Email == "" && len(buyer.Emails) > 0 {
		buyer.Email = buyer.Emails[0]
	}
	in = payments.ChargeRequest{OrderRef: p.OrderReference.String, Amount: p.AmountMinor, Currency: p.Currency, ItemName: "ClickFix " + invoice.PaymentReference, Buyer: payments.Buyer{Name: buyer.LegalName, Email: buyer.Email, IP: "127.0.0.1"}, Card: payments.SavedCard{OwnerKey: owner, Token: token}, Recurring: true}
	return provider, a, in, nil
}

func (s *Service) dunning(ctx context.Context, r *repository.Repository, invoice db.BillingInvoice, sub db.BillingSubscription, now time.Time, code string, failed bool) error {
	d, err := r.Q.BillingBeginDunning(ctx, db.BillingBeginDunningParams{OrganizationID: invoice.OrganizationID, SubscriptionID: invoice.SubscriptionID, InvoiceID: invoice.ID, FirstFailureAt: repository.TS(now), GraceEndsAt: repository.TS(now.AddDate(0, 0, 14))})
	if err != nil {
		return err
	}
	attempts := d.Attempts
	var next *time.Time
	if failed {
		attempts++
		next = RetryAt(d.FirstFailureAt.Time, attempts)
	}
	if !failed && code == "bank_transfer_due" {
		nextTime := now.AddDate(0, 0, 1)
		next = &nextTime
	}
	if err = r.Q.BillingScheduleDunning(ctx, db.BillingScheduleDunningParams{OrganizationID: invoice.OrganizationID, InvoiceID: invoice.ID, Attempts: attempts, NextAttemptAt: optionalTS(next), LastErrorCode: code}); err != nil {
		return err
	}
	if invoice.Kind != "renewal" {
		event := events.New("billing.payment_required").WithTenant(sub.OrganizationID).WithEntity("billing_invoices", &invoice.ID, &invoice.Uuid).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "code": code, "attempt": attempts})
		event.EventID = uuid.NewSHA1(invoice.Uuid, []byte(fmt.Sprintf("purchase:%d:%s", attempts, code)))
		return emitPaymentNotice(ctx, r, event)
	}
	if sub.Status == "active" || sub.Status == "trialing" {
		sub, err = applyTransition(ctx, r, sub, model.Transition{To: "past_due", Reason: "Invoice overdue"}, 0, now)
		if err != nil {
			return err
		}
	}
	if sub.Status == "past_due" {
		_, err = applyTransition(ctx, r, sub, model.Transition{To: "grace", Reason: "Payment grace period", GraceEndsAt: &d.GraceEndsAt.Time}, 0, now)
		if err != nil {
			return err
		}
	}
	event := events.New("billing.payment_required").WithTenant(sub.OrganizationID).WithEntity("billing_invoices", &invoice.ID, &invoice.Uuid).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "attempt": attempts, "code": code, "grace_ends_at": d.GraceEndsAt.Time, "billing_path": "/billing"})
	event.EventID = uuid.NewSHA1(invoice.Uuid, []byte(fmt.Sprintf("dunning:%d:%s:%s", attempts, code, now.Format("2006-01-02"))))
	return emitPaymentNotice(ctx, r, event)
}

func (s *Service) renewalReminders(ctx context.Context, now time.Time) error {
	subs, err := s.repo.Q.BillingRenewalReminderCandidates(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if err = s.repo.Within(ctx, func(r *repository.Repository) error {
			days := int32(30)
			if sub.PeriodEnd.Time.Sub(now) <= 7*24*time.Hour {
				days = 7
			}
			n, e := r.Q.BillingRenewalReminder(ctx, db.BillingRenewalReminderParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, PeriodEnd: sub.PeriodEnd, Days: days})
			if e != nil || n == 0 {
				return e
			}
			return r.Emit(ctx, events.New("billing.renewal_reminder").WithTenant(sub.OrganizationID).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"days": days, "period_end": sub.PeriodEnd.Time}))
		}); err != nil {
			return err
		}
	}
	return nil
}

func emitPaymentNotice(ctx context.Context, r *repository.Repository, event events.Event) error {
	n, err := r.Q.BillingPaymentNotice(ctx, db.BillingPaymentNoticeParams{OrganizationID: *event.TenantID, NoticeKey: event.EventID.String()})
	if err != nil || n == 0 {
		return err
	}
	return r.Emit(ctx, event)
}

// A CF-25 invoice is required before trial capture. Missing legal invoice data
// never triggers an unledgered provider request.
func (s *Service) chargeTrialInvoice(ctx context.Context, sub db.BillingSubscription) (bool, error) {
	invoice, err := s.repo.Q.BillingOpenTrialInvoice(ctx, db.BillingOpenTrialInvoiceParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, PeriodStart: sub.PeriodEnd})
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.repo.Within(ctx, func(r *repository.Repository) error {
			if _, e := r.Q.BillingLockOrganizationID(ctx, sub.OrganizationID); e != nil {
				return e
			}
			var e error
			invoice, e = s.periodInvoice(ctx, r, sub, time.Now().UTC())
			return e
		})
	}
	if err != nil {
		return false, err
	}
	if err = s.repo.Q.BillingEnableRenewal(ctx, db.BillingEnableRenewalParams{OrganizationID: sub.OrganizationID, ID: sub.ID, RenewalEnabled: true}); err != nil {
		return false, err
	}
	if err = s.chargeRenewal(ctx, invoice, time.Now().UTC()); err != nil {
		return false, err
	}
	current, err := s.repo.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: sub.OrganizationID, ID: invoice.ID})
	if err != nil {
		return false, err
	}
	if current.Status == "paid" {
		return true, nil
	}
	if _, err = s.repo.Q.BillingUnresolvedPayment(ctx, db.BillingUnresolvedPaymentParams{OrganizationID: sub.OrganizationID, InvoiceID: invoice.ID}); err == nil {
		return false, model.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	return false, nil
}

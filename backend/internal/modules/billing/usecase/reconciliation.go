package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) ReconciliationReport(ctx context.Context, org int64, paging ...int32) ([]db.BillingReconciliationReportRow, error) {
	limit, offset := paymentPaging(paging)

	return s.repo.Q.BillingReconciliationReport(ctx, db.BillingReconciliationReportParams{OrganizationID: org, Limit: limit, Offset: offset})
}
func (s *Service) ReconcilePending(ctx context.Context) error {
	return s.reconcilePayments(ctx, false, time.Now().UTC())
}
func (s *Service) NightlyReconcile(ctx context.Context) error {
	return s.reconcilePayments(ctx, true, time.Now().UTC())
}
func (s *Service) reconcilePayments(ctx context.Context, nightly bool, now time.Time) error {
	if s.gateway == nil {
		return nil
	}
	var run db.BillingReconciliationRun
	var err error
	if nightly {
		run, err = s.repo.Q.BillingReconciliationRun(ctx, pgtype.Date{Time: now, Valid: true})
		if err != nil {
			return err
		}
		if run.CompletedAt.Valid {
			return nil
		}
	}
	failures := []error{}
	after := int64(0)
	for {
		rows, e := s.repo.Q.BillingReconcileCandidates(ctx, db.BillingReconcileCandidatesParams{IncludeSettled: nightly, AfterID: after})
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			break
		}
		for _, p := range rows {
			after = p.ID
			if e = s.reconcilePayment(ctx, p, run.ID, now); e != nil {
				failures = append(failures, fmt.Errorf("payment %s: %w", p.Uuid, e))
			}
		}
	}
	if err = errors.Join(failures...); err != nil {
		return err
	}
	if nightly {
		return s.repo.Q.BillingCompleteReconciliation(ctx, run.ID)
	}
	return nil
}
func (s *Service) reconcilePayment(ctx context.Context, p db.BillingPayment, runID int64, now time.Time) error {
	provider, err := s.gateway.Executor.Registry.Get(p.Provider)
	if err != nil {
		return err
	}
	a, err := s.gateway.Account(ctx, p.Provider, uuid.UUID(p.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return err
	}
	// An abandoned hosted page (CheckoutTTL) is expired once the provider has
	// nothing final for it, so the invoice can be paid again.
	stale := p.Kind == "checkout" && (p.Status == "pending" || p.Status == "unknown") && now.Sub(p.CreatedAt.Time) > CheckoutTTL
	var n payments.Notification
	if p.Kind == "checkout" {
		if confirmer, ok := payments.ConfirmerOf(provider); ok {
			sess, e := s.repo.Q.BillingCheckoutSession(ctx, db.BillingCheckoutSessionParams{OrganizationID: p.OrganizationID, PaymentID: p.ID})
			if e != nil {
				return e
			}
			if sess.Token == "" {
				if stale {
					return s.expireCheckout(ctx, p, "checkout_not_opened")
				}
				if p.Status == "succeeded" {
					return s.mismatch(ctx, p, runID, "checkout_token_missing")
				}
				return nil
			}
			n, err = confirmer.Confirm(ctx, a, p.OrderReference.String, sess.Token)
		} else if checker, ok := payments.ChargeCheckerOf(provider); ok {
			n, err = checker.CheckCharge(ctx, a, payments.ChargeRequest{OrderRef: p.OrderReference.String, Amount: p.AmountMinor, Currency: p.Currency})
		} else {
			return payments.ErrUnsupported
		}
	} else if checker, ok := payments.ChargeCheckerOf(provider); ok {
		n, err = checker.CheckCharge(ctx, a, payments.ChargeRequest{OrderRef: p.OrderReference.String, Amount: p.AmountMinor, Currency: p.Currency})
	} else {
		return payments.ErrUnsupported
	}
	if err != nil {
		var refusal *payments.ProviderError
		if stale && errors.As(err, &refusal) && !refusal.Temporary {
			return s.expireCheckout(ctx, p, "checkout_abandoned")
		}
		return err
	}
	if n.Status == payments.StatusPending && stale {
		return s.expireCheckout(ctx, p, "checkout_abandoned")
	}
	if n.Status == payments.StatusPending {
		if p.Status == "succeeded" || now.Sub(p.CreatedAt.Time) > 24*time.Hour {
			return s.mismatch(ctx, p, runID, "provider_pending")
		}
		return nil
	}
	if n.Status == payments.StatusSucceeded && (n.Amount != p.AmountMinor || n.PaidAmount < p.AmountMinor || n.Currency != p.Currency || n.OrderRef != p.OrderReference.String) {
		return s.mismatch(ctx, p, runID, "amount_currency_reference_mismatch")
	}
	if p.Status == "succeeded" {
		if n.Status != "succeeded" {
			return s.mismatch(ctx, p, runID, "provider_local_status_mismatch")
		}
		return nil
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, p.OrganizationID); e != nil {
			return e
		}
		locked, e := r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: p.OrganizationID, ID: p.ID})
		if e != nil {
			return e
		}
		if locked.Status == "succeeded" || locked.Status == "failed" {
			return nil
		}
		if e = s.settle(ctx, r, locked, n, now); e != nil {
			return e
		}
		if n.Status == payments.StatusFailed && p.Kind == "recurring" {
			invoice, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: p.OrganizationID, ID: p.InvoiceID})
			if e != nil {
				return e
			}
			sub, e := r.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: p.OrganizationID, ID: invoice.SubscriptionID})
			if e != nil {
				return e
			}
			if e = s.dunning(ctx, r, invoice, sub, now, n.FailureCode, true); e != nil {
				return e
			}
		}
		return r.Q.BillingGatewayFact(ctx, db.BillingGatewayFactParams{OrganizationID: p.OrganizationID, PaymentID: p.ID, EventKey: "reconcile:" + n.Status, Operation: "reconcile", ResultCode: n.Status, PayloadDigest: "provider_authenticated_query"})
	})
}

// expireCheckout ends an abandoned checkout. Late provider results still
// settle it (expired -> succeeded/failed is an allowed transition).
func (s *Service) expireCheckout(ctx context.Context, p db.BillingPayment, code string) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, p.OrganizationID); e != nil {
			return e
		}
		n, e := r.Q.BillingExpirePayment(ctx, db.BillingExpirePaymentParams{OrganizationID: p.OrganizationID, ID: p.ID, FailureCode: text(code)})
		if e != nil || n == 0 {
			return e
		}
		return r.Emit(ctx, events.New(events.PaymentExpired).WithTenant(p.OrganizationID).WithEntity("billing_payments", &p.ID, &p.Uuid).WithPayload(map[string]any{"reason": code}))
	})
}
func (s *Service) mismatch(ctx context.Context, p db.BillingPayment, runID int64, reason string) error {
	if runID == 0 {
		run, err := s.repo.Q.BillingReconciliationRun(ctx, pgtype.Date{Time: time.Now().UTC(), Valid: true})
		if err != nil {
			return err
		}
		runID = run.ID
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		n, e := r.Q.BillingReconciliationMismatch(ctx, db.BillingReconciliationMismatchParams{OrganizationID: p.OrganizationID, PaymentID: p.ID, RunID: runID, Reason: reason})
		if e != nil || n == 0 {
			return e
		}
		return r.Emit(ctx, events.New("billing.reconciliation_mismatch").WithTenant(p.OrganizationID).WithEntity("billing_payments", &p.ID, &p.Uuid).WithPayload(map[string]any{"reason": reason, "provider": p.Provider, "payment_uuid": p.Uuid}))
	})
}

type RefundInput struct {
	Destination       string `json:"destination"`
	EvidenceReference string `json:"evidence_reference"`
	Cancel            bool   `json:"cancel"`
	AmountMinor       int64  `json:"amount_minor"`
	IdempotencyKey    string `json:"idempotency_key"`
}

func (s *Service) RefundPayment(ctx context.Context, org int64, id uuid.UUID, in RefundInput) error {
	if in.Destination != "" && in.Destination != "card" {
		return s.localRefund(ctx, org, id, in)
	}
	if s.gateway == nil {
		return payments.ErrNotConfigured
	}
	if in.AmountMinor <= 0 || len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 120 {
		return model.ErrInvalid
	}
	var original, payment db.BillingPayment
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		var e error
		original, e = r.Q.BillingPaymentUUID(ctx, db.BillingPaymentUUIDParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if in.Cancel && (original.Provider != "iyzico" || in.AmountMinor != original.AmountMinor) {
			return model.ErrInvalid
		}
		if original.Status != "succeeded" || original.Kind == "refund" || (original.Provider == "bank_transfer" || original.Provider == "wallet") {
			return model.ErrConflict
		}
		key := "refund:" + in.IdempotencyKey
		existing, e := r.Q.BillingPaymentKey(ctx, db.BillingPaymentKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			if existing.OriginalPaymentID.Int64 != original.ID || existing.AmountMinor != in.AmountMinor || existing.RefundDestination != "card" || existing.RefundCancel != in.Cancel {
				return model.ErrConflict
			}
			payment = existing
			return nil
		}
		total, e := r.Q.BillingRefundTotal(ctx, db.BillingRefundTotalParams{OrganizationID: org, OriginalPaymentID: pgtype.Int8{Int64: original.ID, Valid: true}})
		if e != nil {
			return e
		}
		if in.AmountMinor > original.AmountMinor-total {
			return model.ErrInvalid
		}
		payment, e = r.Q.BillingNewRefund(ctx, db.BillingNewRefundParams{OrganizationID: org, InvoiceID: original.InvoiceID, Provider: original.Provider, MerchantAccountUuid: original.MerchantAccountUuid, OriginalPaymentID: pgtype.Int8{Int64: original.ID, Valid: true}, IdempotencyKey: key, AmountMinor: in.AmountMinor, OrderReference: text(OrderReference(org, key)), RefundDestination: "card", RefundCancel: in.Cancel})
		if e != nil {
			return e
		}
		return s.reserveRefundCredits(ctx, r, payment)
	})
	if err != nil {
		return err
	}
	if payment.Status == "succeeded" {
		return nil
	}
	if payment.Status == "failed" || payment.Status == "unknown" {
		return model.ErrConflict
	}
	p, err := s.gateway.Executor.Registry.Get(original.Provider)
	if err != nil {
		return err
	}
	a, err := s.gateway.Account(ctx, p.Code(), uuid.UUID(original.MerchantAccountUuid.Bytes).String())
	if err != nil {
		return err
	}
	request := payments.RefundRequest{OrderRef: original.OrderReference.String, ProviderPaymentID: original.ProviderReference.String, Amount: in.AmountMinor, Currency: original.Currency}
	var result payments.RefundResult
	var callErr error
	if in.Cancel {
		result, callErr = s.gateway.Executor.Cancel(ctx, org, payment.IdempotencyKey, p, a, request)
	} else {
		result, callErr = s.gateway.Executor.Refund(ctx, org, payment.IdempotencyKey, p, a, request)
	}
	status, code := "succeeded", ""
	if callErr != nil {
		status = "unknown"
		var refusal *payments.ProviderError
		if errors.As(callErr, &refusal) && !refusal.Temporary {
			status = "failed"
			code = refusal.Code
		}
	}
	// Refunds deliberately remain unknown on ambiguous results; never resubmit.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayPersistTimeout)
	defer cancel()
	err = s.repo.Within(persist, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(persist, org); e != nil {
			return e
		}
		locked, e := r.Q.BillingPaymentID(persist, db.BillingPaymentIDParams{OrganizationID: org, ID: payment.ID})
		if e != nil {
			return e
		}
		if locked.Status == "succeeded" || locked.Status == "failed" {
			return nil
		}
		_, e = r.Q.BillingSetPayment(persist, db.BillingSetPaymentParams{OrganizationID: org, ID: payment.ID, Status: status, ProviderReference: text(result.ProviderRefundID), FailureCode: text(code), CompletedAt: repository.TS(time.Now().UTC())})
		if e != nil {
			return e
		}
		if status == "failed" {
			if e = s.releaseRefundCredits(persist, r, payment); e != nil {
				return e
			}
		}
		return r.Emit(persist, events.New("billing.refund_"+status).WithTenant(org).WithEntity("billing_payments", &payment.ID, &payment.Uuid).WithPayload(map[string]any{"original_payment_uuid": id, "amount_minor": in.AmountMinor}))
	})
	if err != nil {
		return err
	}
	return callErr
}

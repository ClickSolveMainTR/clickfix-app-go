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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// StatementInput is a normalized, authenticated merchant statement row. Only
// platform billing administrators or a server-side source may import it.
// Refund rows MUST name our refund attempt reference, never the original order.
type StatementInput struct {
	ProviderReference   string    `json:"provider_reference"`
	Provider            string    `json:"provider"`
	MerchantAccountUUID uuid.UUID `json:"merchant_account_uuid"`
	ExternalID          string    `json:"external_id"`
	OrderReference      string    `json:"order_reference"`
	Operation           string    `json:"operation"`
	Status              string    `json:"status"`
	AmountMinor         int64     `json:"amount_minor"`
	Currency            string    `json:"currency"`
	ObservedAt          time.Time `json:"observed_at"`
}
type StatementSource interface {
	Fetch(context.Context, time.Time) ([]StatementInput, error)
}

func (s *Service) SetStatementSource(source StatementSource) { s.statements = source }
func (s *Service) ImportStatement(ctx context.Context, rows []StatementInput) error {
	if len(rows) == 0 || len(rows) > 1000 {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		for _, in := range rows {
			if (in.Provider != "paytr" && in.Provider != "iyzico" && in.Provider != "fake") || in.MerchantAccountUUID == uuid.Nil || in.ExternalID == "" || len(in.ExternalID) > 160 || in.OrderReference == "" || len(in.OrderReference) > 160 || in.Operation != "charge" && in.Operation != "refund" || in.Status != "succeeded" && in.Status != "failed" && in.Status != "pending" || in.AmountMinor <= 0 || in.Currency != "TRY" || in.ObservedAt.IsZero() {
				return model.ErrInvalid
			}
			row, e := r.Q.BillingSaveStatement(ctx, db.BillingSaveStatementParams{Provider: in.Provider, MerchantAccountUuid: in.MerchantAccountUUID, ExternalID: in.ExternalID, OrderReference: in.OrderReference, Operation: in.Operation, Status: in.Status, AmountMinor: in.AmountMinor, Currency: in.Currency, ObservedAt: repository.TS(in.ObservedAt), ProviderReference: in.ProviderReference})
			if e != nil {
				return e
			}
			if row.OrderReference != in.OrderReference || row.Operation != in.Operation || row.Status != in.Status || row.AmountMinor != in.AmountMinor || row.Currency != in.Currency || row.ProviderReference != in.ProviderReference {
				return model.ErrConflict
			}
		}
		return nil
	})
}
func (s *Service) StatementReport(ctx context.Context, limit, offset int32) ([]db.BillingProviderStatement, error) {
	return s.repo.Q.BillingStatementReport(ctx, db.BillingStatementReportParams{Limit: limit, Offset: offset})
}
func (s *Service) ReconcileStatements(ctx context.Context) error {
	var failures []error
	after := int64(0)
	for {
		rows, err := s.repo.Q.BillingStatementPending(ctx, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			after = row.ID
			err = s.repo.Within(ctx, func(r *repository.Repository) error {
				p, e := r.Q.BillingStatementPayment(ctx, db.BillingStatementPaymentParams{Provider: row.Provider, MerchantAccountUuid: pgtype.UUID{Bytes: row.MerchantAccountUuid, Valid: true}, OrderReference: text(row.OrderReference)})
				if errors.Is(e, pgx.ErrNoRows) {
					if e = r.Q.BillingMatchStatement(ctx, db.BillingMatchStatementParams{ID: row.ID, Mismatch: "orphan_provider_record"}); e != nil {
						return e
					}
					if row.Mismatch != "orphan_provider_record" {
						event := events.New("billing.statement_orphan").WithPayload(map[string]any{"statement_id": row.ID, "provider": row.Provider})
						event.EventID = uuid.NewSHA1(row.MerchantAccountUuid, []byte(row.ExternalID))
						return r.Emit(ctx, event)
					}
					return nil
				}
				if e != nil {
					return e
				}
				if _, e = r.Q.BillingLockOrganizationID(ctx, p.OrganizationID); e != nil {
					return e
				}
				p, e = r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: p.OrganizationID, ID: p.ID})
				if e != nil {
					return e
				}
				mismatch := ""
				if row.Status == "succeeded" && row.Operation == "charge" && row.ProviderReference == "" {
					mismatch = "provider_reference_missing"
				}
				refund := p.Kind == "refund"
				if refund != (row.Operation == "refund") || p.AmountMinor != row.AmountMinor || p.Currency != row.Currency {
					mismatch = "amount_currency_operation_mismatch"
				} else if p.Status == "succeeded" && row.Status != "succeeded" || p.Status == "failed" && row.Status != "failed" {
					mismatch = "status_mismatch"
				}
				if e = r.Q.BillingMatchStatement(ctx, db.BillingMatchStatementParams{ID: row.ID, OrganizationID: pgtype.Int8{Int64: p.OrganizationID, Valid: true}, PaymentID: pgtype.Int8{Int64: p.ID, Valid: true}, Mismatch: mismatch}); e != nil {
					return e
				}
				if mismatch != "" {
					run, e := r.Q.BillingReconciliationRun(ctx, pgtype.Date{Time: time.Now().UTC(), Valid: true})
					if e != nil {
						return e
					}
					_, e = r.Q.BillingReconciliationMismatch(ctx, db.BillingReconciliationMismatchParams{OrganizationID: p.OrganizationID, PaymentID: p.ID, RunID: run.ID, Reason: "statement:" + mismatch})
					if e != nil {
						return e
					}
					event := events.New("billing.reconciliation_mismatch").WithTenant(p.OrganizationID).WithPayload(map[string]any{"statement_id": row.ID, "reason": mismatch})
					event.EventID = uuid.NewSHA1(p.Uuid, []byte(fmt.Sprint("statement:", row.ID)))
					return emitPaymentNotice(ctx, r, event)
				}
				if p.Status != "pending" && p.Status != "unknown" || row.Status == "pending" {
					return nil
				}
				if refund {
					return s.settleRefund(ctx, r, p, row.Status, row.ExternalID)
				}
				return s.settle(ctx, r, p, payments.Notification{OrderRef: row.OrderReference, Status: row.Status, Amount: row.AmountMinor, PaidAmount: row.AmountMinor, Currency: row.Currency, ProviderPaymentID: row.ProviderReference}, time.Now().UTC())
			})
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
func (s *Service) ReconcileRefunds(ctx context.Context) error {
	if s.gateway == nil {
		return nil
	}
	after := int64(0)
	var failures []error
	for {
		rows, err := s.repo.Q.BillingAmbiguousRefunds(ctx, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, refund := range rows {
			after = refund.ID
			original, e := s.repo.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: refund.OrganizationID, ID: refund.OriginalPaymentID.Int64})
			if e != nil {
				failures = append(failures, e)
				continue
			}
			p, e := s.gateway.Executor.Registry.Get(refund.Provider)
			if e != nil {
				failures = append(failures, e)
				continue
			}
			checker, ok := payments.RefundCheckerOf(p)
			if !ok {
				continue
			}
			a, e := s.gateway.Account(ctx, p.Code(), uuid.UUID(refund.MerchantAccountUuid.Bytes).String())
			if e != nil {
				failures = append(failures, e)
				continue
			}
			result, e := checker.CheckRefund(ctx, a, payments.RefundRequest{OrderRef: original.OrderReference.String, ProviderPaymentID: original.ProviderReference.String, RefundRef: refund.OrderReference.String, Amount: refund.AmountMinor, Currency: refund.Currency})
			if e != nil {
				failures = append(failures, e)
				continue
			}
			if result.Status == payments.RefundStillPending {
				continue
			}
			status := "succeeded"
			if result.Status == payments.RefundNotDone {
				status = "failed"
			} else if result.Status != payments.RefundFound {
				continue
			}
			e = s.repo.Within(ctx, func(r *repository.Repository) error {
				if _, e := r.Q.BillingLockOrganizationID(ctx, refund.OrganizationID); e != nil {
					return e
				}
				locked, e := r.Q.BillingPaymentID(ctx, db.BillingPaymentIDParams{OrganizationID: refund.OrganizationID, ID: refund.ID})
				if e != nil {
					return e
				}
				return s.settleRefund(ctx, r, locked, status, result.ProviderRefundID)
			})
			if e != nil {
				failures = append(failures, e)
			}
		}
	}
	return errors.Join(failures...)
}
func (s *Service) settleRefund(ctx context.Context, r *repository.Repository, p db.BillingPayment, status, reference string) error {
	if p.Status == "succeeded" || p.Status == "failed" {
		if p.Status == status {
			return nil
		}
		return model.ErrConflict
	}
	if status != "succeeded" && status != "failed" {
		return model.ErrInvalid
	}
	if _, e := r.Q.BillingSetPayment(ctx, db.BillingSetPaymentParams{OrganizationID: p.OrganizationID, ID: p.ID, Status: status, ProviderReference: text(reference), CompletedAt: repository.TS(time.Now().UTC())}); e != nil {
		return e
	}
	if status == "failed" {
		if e := s.releaseRefundCredits(ctx, r, p); e != nil {
			return e
		}
	}
	event := events.New("billing.refund_"+status).WithTenant(p.OrganizationID).WithEntity("billing_payments", &p.ID, &p.Uuid).WithPayload(map[string]any{"amount_minor": p.AmountMinor})
	event.EventID = uuid.NewSHA1(p.Uuid, []byte(status))
	return emitPaymentNotice(ctx, r, event)
}

package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) localRefund(ctx context.Context, org int64, id uuid.UUID, in RefundInput) error {
	if in.AmountMinor <= 0 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 120 || in.Destination != "wallet" && in.Destination != "manual_bank" || in.Destination == "manual_bank" && in.EvidenceReference == "" {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		original, e := r.Q.BillingPaymentUUID(ctx, db.BillingPaymentUUIDParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if original.Status != "succeeded" || original.Kind == "refund" || original.Provider == "wallet" && in.Destination != "wallet" || in.Destination == "manual_bank" && original.Provider != "bank_transfer" {
			return model.ErrConflict
		}
		key := "refund:" + in.IdempotencyKey
		old, e := r.Q.BillingPaymentKey(ctx, db.BillingPaymentKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			if old.OriginalPaymentID.Int64 != original.ID || old.AmountMinor != in.AmountMinor || old.RefundDestination != in.Destination {
				return model.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		refunded, e := r.Q.BillingRefundTotal(ctx, db.BillingRefundTotalParams{OrganizationID: org, OriginalPaymentID: pgtype.Int8{Int64: original.ID, Valid: true}})
		if e != nil {
			return e
		}
		if in.AmountMinor > original.AmountMinor-refunded {
			return model.ErrInvalid
		}
		refund, e := r.Q.BillingNewRefund(ctx, db.BillingNewRefundParams{OrganizationID: org, InvoiceID: original.InvoiceID, Provider: original.Provider, MerchantAccountUuid: original.MerchantAccountUuid, OriginalPaymentID: pgtype.Int8{Int64: original.ID, Valid: true}, IdempotencyKey: key, AmountMinor: in.AmountMinor, OrderReference: text(OrderReference(org, key)), RefundDestination: in.Destination})
		if e != nil {
			return e
		}
		if e = s.reserveRefundCredits(ctx, r, refund); e != nil {
			return e
		}
		if in.Destination == "wallet" {
			invoice, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: org, ID: original.InvoiceID})
			if e != nil {
				return e
			}
			if _, e = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: invoice.SubscriptionID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}, Unit: "TRY", Delta: in.AmountMinor, Reason: "Payment refunded to wallet", IdempotencyKey: key}); e != nil {
				return e
			}
		}
		if _, e = r.Q.BillingSetPayment(ctx, db.BillingSetPaymentParams{OrganizationID: org, ID: refund.ID, Status: "succeeded", ProviderReference: text("local:" + refund.Uuid.String()), FailureCode: text("destination:" + in.Destination), CompletedAt: repository.TS(time.Now().UTC())}); e != nil {
			return e
		}
		return r.Emit(ctx, events.New("billing.refund_succeeded").WithTenant(org).WithEntity("billing_payments", &refund.ID, &refund.Uuid).WithPayload(map[string]any{"destination": in.Destination, "amount_minor": in.AmountMinor, "evidence_reference": in.EvidenceReference}))
	})
}

// Credit notes use the original legal snapshots and VAT, even if the customer
// edited their profile or TCMB published a new rate after the sale.
func (s *Service) CreditNoteSweep(ctx context.Context) error {
	refunds, err := s.repo.Q.BillingRefundsWithoutNotes(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, refund := range refunds {
		err = s.repo.Within(ctx, func(r *repository.Repository) error {
			if _, e := r.Q.BillingLockOrganizationID(ctx, refund.OrganizationID); e != nil {
				return e
			}
			original, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: refund.OrganizationID, ID: refund.InvoiceID})
			if e != nil {
				return e
			}
			key := "credit-note:" + refund.Uuid.String()
			note, e := r.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: refund.OrganizationID, IdempotencyKey: key})
			if e == nil {
				return r.Q.BillingRefundNote(ctx, db.BillingRefundNoteParams{OrganizationID: refund.OrganizationID, PaymentID: refund.ID, InvoiceID: note.ID})
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			lines, e := r.Q.BillingInvoiceLines(ctx, db.BillingInvoiceLinesParams{OrganizationID: refund.OrganizationID, InvoiceID: original.ID})
			if e != nil {
				return e
			}
			if len(lines) == 0 || original.TotalMinor == 0 || refund.AmountMinor > original.TotalMinor {
				return model.ErrConflict
			}
			var noteLines []db.BillingCreateInvoiceLineParams
			remaining := refund.AmountMinor
			netTotal, taxTotal := int64(0), int64(0)
			for index, line := range lines {
				gross := money.New(refund.AmountMinor).Ratio(line.TotalMinor, original.TotalMinor).Minor()
				gross = min(gross, remaining)
				if index == len(lines)-1 {
					gross = remaining
				}
				remaining -= gross
				if gross == 0 {
					continue
				}
				net := money.New(gross).Ratio(10000, 10000+int64(line.VatBasisPoints)).Minor()
				tax := money.New(net).Ratio(int64(line.VatBasisPoints), 10000).Minor()
				if net+tax > gross {
					net--
					tax = money.New(net).Ratio(int64(line.VatBasisPoints), 10000).Minor()
				}
				if net > 0 {
					noteLines = append(noteLines, db.BillingCreateInvoiceLineParams{OrganizationID: refund.OrganizationID, Description: "Refund: " + line.Description, Quantity: 1, UnitAmountMinor: net, SubtotalMinor: net, VatBasisPoints: line.VatBasisPoints, TaxMinor: tax, TotalMinor: net + tax})
					netTotal += net
					taxTotal += tax
				}
				rounding := gross - net - tax
				if rounding > 0 {
					noteLines = append(noteLines, db.BillingCreateInvoiceLineParams{OrganizationID: refund.OrganizationID, Description: "Refund rounding", Quantity: 1, UnitAmountMinor: rounding, SubtotalMinor: rounding, TotalMinor: rounding})
					netTotal += rounding
				}
			}
			if netTotal+taxTotal != refund.AmountMinor {
				return model.ErrConflict
			}
			one := pgtype.Numeric{}
			_ = one.Scan("1")
			note, e = r.Q.BillingCreateInvoice(ctx, db.BillingCreateInvoiceParams{OrganizationID: refund.OrganizationID, SubscriptionID: original.SubscriptionID, PeriodStart: original.PeriodStart, PeriodEnd: original.PeriodEnd, Kind: "credit_note", IdempotencyKey: key, SourceCurrency: "TRY", FxRate: one, FxSource: "original_invoice", FxObservedAt: original.FxObservedAt, BuyerSnapshot: original.BuyerSnapshot, SellerSnapshot: original.SellerSnapshot, SubtotalMinor: netTotal, TaxMinor: taxTotal, TotalMinor: refund.AmountMinor, DueAt: repository.TS(time.Now().UTC()), OriginalInvoiceID: pgtype.Int8{Int64: original.ID, Valid: true}})
			if e != nil {
				return e
			}
			for _, line := range noteLines {
				line.InvoiceID = note.ID
				if e = r.Q.BillingCreateInvoiceLine(ctx, line); e != nil {
					return e
				}
			}
			if e = r.Q.BillingRefundNote(ctx, db.BillingRefundNoteParams{OrganizationID: refund.OrganizationID, PaymentID: refund.ID, InvoiceID: note.ID}); e != nil {
				return e
			}
			return r.Emit(ctx, events.New("billing.credit_note_created").WithTenant(refund.OrganizationID).WithEntity("billing_invoices", &note.ID, &note.Uuid).WithPayload(map[string]any{"invoice_uuid": note.Uuid, "original_invoice_uuid": original.Uuid, "refund_uuid": refund.Uuid}))
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

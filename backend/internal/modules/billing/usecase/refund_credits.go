package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reserve purchased credits (AI credits or video minutes, the unit of the
// refunded pack) before refund I/O. Unknown outcomes retain the
// reservation; a definitive refusal compensates it with another ledger entry.
// Consumed credits cannot be refunded automatically.
func (s *Service) reserveRefundCredits(ctx context.Context, r *repository.Repository, refund db.BillingPayment) error {
	purchase, err := r.Q.BillingPurchase(ctx, db.BillingPurchaseParams{OrganizationID: refund.OrganizationID, InvoiceID: refund.InvoiceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !purchase.GrantedAt.Valid {
		return nil
	}
	invoice, err := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: refund.OrganizationID, ID: refund.InvoiceID})
	if err != nil {
		return err
	}
	total, err := r.Q.BillingInvoiceRefundTotal(ctx, db.BillingInvoiceRefundTotalParams{OrganizationID: refund.OrganizationID, InvoiceID: refund.InvoiceID})
	if err != nil {
		return err
	}
	reserved, err := r.Q.BillingReservedRefundCredits(ctx, db.BillingReservedRefundCreditsParams{OrganizationID: refund.OrganizationID, InvoiceID: refund.InvoiceID})
	if err != nil {
		return err
	}
	target := money.New(purchase.Credits).Ratio(total, invoice.TotalMinor).Minor()
	credits := target - reserved
	if credits <= 0 {
		return nil
	}
	if err = s.expireWallet(ctx, r, refund.OrganizationID, time.Now().UTC()); err != nil {
		return err
	}
	lots, err := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: refund.OrganizationID, Unit: purchase.Unit})
	if err != nil {
		return err
	}
	// Refund only the unused purchased lot, never unrelated promotional credits.
	available := int64(0)
	var expiry pgtype.Timestamptz
	var eligible []db.BillingCreditLotsRow
	for _, lot := range lots {
		if lot.InvoiceID.Valid && lot.InvoiceID.Int64 == invoice.ID && (lot.IdempotencyKey == "purchase:"+invoice.Uuid.String() || strings.HasPrefix(lot.IdempotencyKey, "refund-restore:")) && lot.Remaining > 0 && (!lot.ExpiresAt.Valid || lot.ExpiresAt.Time.After(time.Now())) {
			available += lot.Remaining
			eligible = append(eligible, lot)
			if lot.ExpiresAt.Valid && (!expiry.Valid || lot.ExpiresAt.Time.Before(expiry.Time)) {
				expiry = lot.ExpiresAt
			}
		}
	}
	if available < credits {
		return model.ErrConflict
	}
	debit, err := r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: refund.OrganizationID, SubscriptionID: invoice.SubscriptionID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}, Unit: purchase.Unit, Delta: -credits, Reason: "Refund credit reservation", IdempotencyKey: "refund-credits:" + refund.Uuid.String(), ExpiresAt: expiry})
	if err != nil {
		return err
	}
	remaining := credits
	for _, lot := range eligible {
		amount := min(remaining, lot.Remaining)
		if amount <= 0 {
			continue
		}
		if err = r.Q.BillingAllocateCredits(ctx, db.BillingAllocateCreditsParams{OrganizationID: refund.OrganizationID, DebitID: debit.ID, CreditID: lot.ID, Amount: amount}); err != nil {
			return err
		}
		remaining -= amount
		if remaining == 0 {
			break
		}
	}
	return r.Q.BillingReserveRefundCredits(ctx, db.BillingReserveRefundCreditsParams{OrganizationID: refund.OrganizationID, PaymentID: refund.ID, DebitID: debit.ID, Credits: credits})
}
func (s *Service) releaseRefundCredits(ctx context.Context, r *repository.Repository, refund db.BillingPayment) error {
	reservation, err := r.Q.BillingRefundCreditReservation(ctx, db.BillingRefundCreditReservationParams{OrganizationID: refund.OrganizationID, PaymentID: refund.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if reservation.ReleasedAt.Valid {
		return nil
	}
	purchase, err := r.Q.BillingPurchase(ctx, db.BillingPurchaseParams{OrganizationID: refund.OrganizationID, InvoiceID: refund.InvoiceID})
	if err != nil {
		return err
	}
	if _, err = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: refund.OrganizationID, SubscriptionID: reservation.SubscriptionID, InvoiceID: pgtype.Int8{Int64: refund.InvoiceID, Valid: true}, Unit: purchase.Unit, Delta: reservation.Credits, Reason: "Refund refused: credits restored", IdempotencyKey: "refund-restore:" + refund.Uuid.String(), ReversesID: pgtype.Int8{Int64: reservation.DebitID, Valid: true}, ExpiresAt: reservation.ExpiresAt}); err != nil {
		return err
	}
	return r.Q.BillingReleaseRefundCredits(ctx, db.BillingReleaseRefundCreditsParams{OrganizationID: refund.OrganizationID, PaymentID: refund.ID})
}

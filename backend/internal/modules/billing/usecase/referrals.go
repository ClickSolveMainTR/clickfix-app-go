package usecase

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) ReferralCode(ctx context.Context, org int64) (string, error) {
	return s.repo.Q.BillingReferralCode(ctx, org)
}
func (s *Service) RedeemReferral(ctx context.Context, org int64, code string) error {
	reward, e := strconv.ParseInt(os.Getenv("BILLING_REFERRAL_CREDITS"), 10, 64)
	if e != nil || reward < 1 || reward > 1000000 {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		owner, e := r.Q.BillingReferralOwner(ctx, strings.ToUpper(strings.TrimSpace(code)))
		if e != nil {
			return e
		}
		if owner == org {
			return model.ErrInvalid
		}
		n, e := r.Q.BillingPaidInvoiceCount(ctx, org)
		if e != nil {
			return e
		}
		if n > 0 {
			return model.ErrConflict
		}
		old, e := r.Q.BillingReferral(ctx, org)
		if e == nil {
			if old.InviterOrganizationID != owner {
				return model.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		return r.Q.BillingNewReferral(ctx, db.BillingNewReferralParams{OrganizationID: org, InviterOrganizationID: owner, RewardCredits: reward})
	})
}

// Award in a separate transaction after settlement. Lock organizations in id
// order to prevent two reciprocal referral settlements from deadlocking.
func (s *Service) AwardReferral(ctx context.Context, invoice db.BillingInvoice) error {
	ref, err := s.repo.Q.BillingReferral(ctx, invoice.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) || ref.AwardedAt.Valid {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		for _, org := range []int64{min(ref.OrganizationID, ref.InviterOrganizationID), max(ref.OrganizationID, ref.InviterOrganizationID)} {
			if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
				return e
			}
		}
		n, e := r.Q.BillingAwardReferral(ctx, db.BillingAwardReferralParams{OrganizationID: ref.OrganizationID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}})
		if e != nil || n == 0 {
			return e
		}
		for _, org := range []int64{ref.OrganizationID, ref.InviterOrganizationID} {
			sub, e := r.Q.BillingGetSubscription(ctx, org)
			if e != nil {
				return e
			}
			if _, e = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: sub.ID, Unit: "ai_credits", Delta: ref.RewardCredits, Reason: "Referral reward", IdempotencyKey: "referral:" + invoice.Uuid.String()}); e != nil {
				return e
			}
		}
		return r.Emit(ctx, events.New("billing.referral_awarded").WithTenant(ref.OrganizationID).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "credits": ref.RewardCredits}))
	})
}
func (s *Service) ReferralSweep(ctx context.Context) error {
	invoices, err := s.repo.Q.BillingPendingReferralAwards(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, invoice := range invoices {
		if err = s.AwardReferral(ctx, invoice); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

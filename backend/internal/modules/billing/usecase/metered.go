package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
)

// MeteredBalance expires stale lots under the wallet lock and returns the
// spendable balance of a metered unit (ai_credits, video_minutes).
func (s *Service) MeteredBalance(ctx context.Context, org int64, unit string) (int64, error) {
	if !validCreditUnit(unit) {
		return 0, model.ErrInvalid
	}
	var balance int64
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		if e := s.expireWallet(ctx, r, org, time.Now().UTC()); e != nil {
			return e
		}
		var e error
		balance, e = r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: unit})
		return e
	})
	return balance, err
}

// MeterUsage debits up to `amount` of a metered unit for usage that already
// happened (a closed video connection). The wallet never goes negative: when
// the balance is short, only the available part is debited and the caller
// records the rest as unbilled. Replays with the same key return the first
// debit. Returns the debited amount and the balance left.
func (s *Service) MeterUsage(ctx context.Context, org int64, unit string, amount int64, key, reason string) (int64, int64, error) {
	if !validCreditUnit(unit) || amount <= 0 || key == "" || len(key) > 120 || reason == "" {
		return 0, 0, model.ErrInvalid
	}
	var debited, balance int64
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		old, e := r.Q.BillingWalletKey(ctx, db.BillingWalletKeyParams{OrganizationID: org, Unit: unit, IdempotencyKey: key})
		if e == nil {
			debited = -old.Delta
			balance, e = r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: unit})
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = s.expireWallet(ctx, r, org, time.Now().UTC()); e != nil {
			return e
		}
		available, e := r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: unit})
		if e != nil {
			return e
		}
		debited = min(amount, max(available, 0))
		balance = available - debited
		if debited == 0 {
			return nil
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		entry, e := r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: sub.ID, Unit: unit, Delta: -debited, Reason: reason, IdempotencyKey: key})
		if e != nil {
			return e
		}
		lots, e := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: org, Unit: unit})
		if e != nil {
			return e
		}
		remaining := debited
		for _, lot := range lots {
			part := min(remaining, lot.Remaining)
			if part <= 0 {
				continue
			}
			if e = r.Q.BillingAllocateCredits(ctx, db.BillingAllocateCreditsParams{OrganizationID: org, DebitID: entry.ID, CreditID: lot.ID, Amount: part}); e != nil {
				return e
			}
			if remaining -= part; remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return model.ErrConflict
		}
		return r.Emit(ctx, events.New("billing.wallet_changed").WithTenant(org).WithEntity("billing_credit_ledger", &entry.ID, &entry.Uuid).WithPayload(map[string]any{"unit": unit, "delta": -debited, "balance": balance}).WithIdempotencyKey("metered:"+key))
	})
	return debited, balance, err
}

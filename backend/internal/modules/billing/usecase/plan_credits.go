package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
)

// grantPlanCredits runs inside the subscription mutation's transaction and org
// lock. Upgrades top up the full period allowance; spent credits never replenish.
// Each metered limit (ai.credits, video.minutes) funds its own wallet unit.
func grantPlanCredits(ctx context.Context, r *repository.Repository, sub db.BillingSubscription) error {
	if sub.Status != "active" && sub.Status != "trialing" {
		return nil
	}
	limits, err := r.Q.BillingTargetLimits(ctx, sub.PlanVersionID)
	if err != nil {
		return err
	}
	for _, grant := range planAllowances {
		var allowance int64
		for _, l := range limits {
			if l.Key == grant.feature && !l.Unlimited && l.ValueLimit.Valid {
				allowance = l.ValueLimit.Int64
			}
		}
		if err = grantPlanAllowance(ctx, r, sub, grant.unit, grant.reason, grant.keyPrefix, allowance); err != nil {
			return err
		}
	}
	return nil
}

var planAllowances = []struct{ feature, unit, reason, keyPrefix string }{
	{"ai.credits", UnitAICredits, "Plan period AI allowance", "plan-credits:"},
	{"video.minutes", UnitVideoMinutes, "Plan period video minutes", "plan-video:"},
}

func grantPlanAllowance(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, unit, reason, prefix string, allowance int64) error {
	if allowance <= 0 {
		return nil
	}
	key := prefix + sub.Uuid.String() + ":" + sub.PeriodStart.Time.UTC().Format(time.RFC3339Nano)
	granted, err := r.Q.BillingPlanCreditGranted(ctx, db.BillingPlanCreditGrantedParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, Unit: unit, PeriodKey: key})
	if err != nil || granted >= allowance {
		return err
	}
	if granted > 0 {
		key += fmt.Sprintf(":upgrade:%d", sub.PlanVersionID)
	}
	lot, err := r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, Unit: unit, Delta: allowance - granted, Reason: reason, IdempotencyKey: key, ExpiresAt: sub.PeriodEnd})
	if err != nil {
		return err
	}
	return r.Emit(ctx, events.New("billing.wallet_changed").WithTenant(sub.OrganizationID).WithEntity("billing_credit_ledger", &lot.ID, &lot.Uuid).WithPayload(map[string]any{"unit": unit, "delta": lot.Delta, "source": "plan"}).WithIdempotencyKey(key))
}

func grantCurrentPlanCredits(ctx context.Context, r *repository.Repository, org int64) error {
	sub, err := r.Q.BillingGetSubscription(ctx, org)
	if err != nil {
		return err
	}
	return grantPlanCredits(ctx, r, sub)
}

// BackfillOrganizationPlanCredits funds only the current period, re-reading it
// under the org lock so a concurrent renewal cannot grant a stale period.
func (s *Service) BackfillOrganizationPlanCredits(ctx context.Context, org int64) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		sub, err := r.Q.BillingGetSubscription(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now()
		if sub.PeriodStart.Time.After(now) || !sub.PeriodEnd.Time.After(now) {
			return nil
		}
		return grantPlanCredits(ctx, r, sub)
	})
}

// BackfillPlanCredits pages current subscriptions. Each grant independently
// locks and re-reads its organization; reruns cannot replenish spent credits.
func (s *Service) BackfillPlanCredits(ctx context.Context) error {
	var after int64
	for {
		rows, err := s.repo.Q.BillingPlanCreditPage(ctx, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			if err = s.BackfillOrganizationPlanCredits(ctx, row.OrganizationID); err != nil {
				return err
			}
			after = row.ID
		}
	}
}

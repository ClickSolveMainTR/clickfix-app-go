package usecase

import (
	"context"
	"errors"
	"strings"
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

type PlanChangeInput struct {
	PriceUUID      uuid.UUID `json:"price_uuid"`
	Seats          int64     `json:"seats"`
	IdempotencyKey string    `json:"idempotency_key"`
}
type PlanChangePreview struct {
	SourceProrationMinor int64            `json:"-"`
	AmountMinor          int64            `json:"amount_minor"`
	Currency             string           `json:"currency"`
	EffectiveAt          time.Time        `json:"effective_at"`
	Immediate            bool             `json:"immediate"`
	Reductions           map[string]int64 `json:"reductions"`
}

func (s *Service) planPreview(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, in PlanChangeInput, now time.Time) (PlanChangePreview, db.BillingCommercePriceRow, error) {
	out := PlanChangePreview{Reductions: map[string]int64{}}
	target, err := r.Q.BillingCommercePrice(ctx, in.PriceUUID)
	if err != nil {
		return out, target, err
	}
	if err = tenantCanBuy(ctx, r, sub, target); err != nil {
		return out, target, err
	}
	current, err := r.Q.BillingGetPriceID(ctx, sub.PriceID)
	if err != nil {
		return out, target, err
	}
	if in.Seats < 1 || in.Seats > 100000 || sub.Status != "active" || sub.CancelAtPeriodEnd || sub.PendingPriceID.Valid || !sub.PeriodEnd.Time.After(now) || current.Currency != target.Currency {
		return out, target, model.ErrConflict
	}
	actual, err := r.Q.BillingInvoiceSeats(ctx, sub.OrganizationID)
	if err != nil {
		return out, target, err
	}
	if in.Seats < actual {
		out.Reductions["seats"] = actual - in.Seats
	}
	limits, err := r.Q.BillingTargetLimits(ctx, target.PlanVersionID)
	if err != nil {
		return out, target, err
	}
	for _, l := range limits {
		if l.Unlimited || !l.ValueLimit.Valid {
			continue
		}
		used, e := r.Q.BillingGetUsage(ctx, db.BillingGetUsageParams{OrganizationID: sub.OrganizationID, Key: l.Key})
		if e != nil {
			return out, target, e
		}
		if l.Key == "seats" {
			used = actual
		}
		if used > l.ValueLimit.Int64 {
			out.Reductions[l.Key] = used - l.ValueLimit.Int64
		}
	}
	if target.AmountMinor > money.MaxMinor || target.SeatAmountMinor > money.MaxMinor/in.Seats || current.SeatAmountMinor > money.MaxMinor/sub.ContractedSeats {
		return out, target, model.ErrInvalid
	}
	old := current.AmountMinor + current.SeatAmountMinor*sub.ContractedSeats
	next := target.AmountMinor + target.SeatAmountMinor*in.Seats
	if old > money.MaxMinor || next > money.MaxMinor {
		return out, target, model.ErrInvalid
	}
	oldMonthly, nextMonthly := old, next
	if current.Interval == "yearly" {
		oldMonthly = money.New(old).Ratio(1, 12).Minor()
	}
	if target.Interval == "yearly" {
		nextMonthly = money.New(next).Ratio(1, 12).Minor()
	}
	out.Currency = target.Currency
	out.Immediate = nextMonthly > oldMonthly || current.Interval == "monthly" && target.Interval == "yearly"
	if current.Interval == "yearly" && target.Interval == "monthly" {
		out.Immediate = false
	}
	if target.ID == current.ID && in.Seats == sub.ContractedSeats {
		return out, target, model.ErrInvalid
	}
	out.EffectiveAt = sub.PeriodEnd.Time
	if out.Immediate {
		remaining := sub.PeriodEnd.Time.Sub(now).Nanoseconds()
		period := sub.PeriodEnd.Time.Sub(sub.PeriodStart.Time).Nanoseconds()
		if period <= 0 {
			return out, target, model.ErrInvalid
		}
		remaining = min(max(remaining, 0), period)
		if target.Interval == current.Interval {
			out.AmountMinor = money.New(max(next-old, 0)).Ratio(remaining, period).Minor()
		} else {
			out.AmountMinor = max(next-money.New(old).Ratio(remaining, period).Minor(), 0)
		}
		out.EffectiveAt = now
		out.SourceProrationMinor = out.AmountMinor
		// Preview includes VAT with exactly the builder's rounding.
		if !target.TaxInclusive {
			out.AmountMinor += money.New(out.AmountMinor).Ratio(int64(target.VatBasisPoints), 10000).Minor()
		}
	}
	return out, target, nil
}

// tenantCanBuy limits self-service plan changes to public plans. Hidden and
// custom plans (such as the free demo-full plan) are assigned only from
// platform billing; an organization already on such a plan may still switch
// between that plan's own prices.
func tenantCanBuy(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, target db.BillingCommercePriceRow) error {
	plan, err := r.Q.BillingGetPlanID(ctx, target.PlanID)
	if err != nil {
		return err
	}
	if plan.Visibility == "public" {
		return nil
	}
	current, err := r.Q.BillingGetVersionID(ctx, sub.PlanVersionID)
	if err != nil {
		return err
	}
	if current.PlanID == plan.ID {
		return nil
	}
	return model.ErrNotFound
}

func (s *Service) PreviewPlanChange(ctx context.Context, org int64, in PlanChangeInput) (PlanChangePreview, error) {
	var out PlanChangePreview
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		out, _, e = s.planPreview(ctx, r, sub, in, time.Now().UTC())
		return e
	})
	return out, err
}
func (s *Service) ApplyPlanChange(ctx context.Context, org int64, in PlanChangeInput) (db.BillingPlanChange, error) {
	var out db.BillingPlanChange
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 120 {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		existing, e := r.Q.BillingChangeKey(ctx, db.BillingChangeKeyParams{OrganizationID: org, IdempotencyKey: in.IdempotencyKey})
		if e == nil {
			p, pe := r.Q.BillingGetPriceID(ctx, existing.PriceID)
			if pe != nil {
				return pe
			}
			if p.Uuid != in.PriceUUID || existing.Seats != in.Seats {
				return model.ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		preview, target, e := s.planPreview(ctx, r, sub, in, now)
		if e != nil {
			return e
		}
		if len(preview.Reductions) > 0 {
			return model.ErrConflict
		}
		state := "scheduled"
		newEnd := pgtype.Timestamptz{}
		currentPrice, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
		if e != nil {
			return e
		}
		if preview.Immediate && currentPrice.Interval != target.Interval {
			newEnd = repository.TS(NextPeriod(now, target.Interval == "yearly"))
		}
		invoiceID := pgtype.Int8{}
		if preview.Immediate {
			amount := preview.SourceProrationMinor
			invoiceEnd := sub.PeriodEnd.Time
			if newEnd.Valid {
				invoiceEnd = newEnd.Time
			}
			invoice, e := s.buildInvoice(ctx, r, sub, InvoiceInput{IdempotencyKey: "change:" + in.IdempotencyKey, Kind: "upgrade", Currency: target.Currency, PeriodStart: now, PeriodEnd: invoiceEnd, DueAt: now, Lines: []InvoiceLineInput{{Description: "Prorated plan / seat upgrade", Quantity: 1, UnitAmountMinor: amount, VATBasisPoints: target.VatBasisPoints, TaxInclusive: target.TaxInclusive}}})
			if e != nil {
				return e
			}
			invoiceID = pgtype.Int8{Int64: invoice.ID, Valid: true}
			state = "awaiting_payment"
			if invoice.TotalMinor == 0 {
				state = "applied"
				if e = r.Q.BillingApplyChange(ctx, db.BillingApplyChangeParams{OrganizationID: org, ID: sub.ID, PriceID: target.ID, PlanVersionID: target.PlanVersionID, ContractedSeats: in.Seats}); e != nil {
					return e
				}
				if newEnd.Valid {
					if e = r.Q.BillingChangePeriod(ctx, db.BillingChangePeriodParams{OrganizationID: org, ID: sub.ID, PeriodStart: repository.TS(now), PeriodEnd: newEnd}); e != nil {
						return e
					}
				}
			}
		} else {
			if e = r.Q.BillingScheduleChange(ctx, db.BillingScheduleChangeParams{OrganizationID: org, ID: sub.ID, PendingPriceID: pgtype.Int8{Int64: target.ID, Valid: true}, PendingPlanVersionID: pgtype.Int8{Int64: target.PlanVersionID, Valid: true}}); e != nil {
				return e
			}
		}
		if state == "applied" {
			if e = grantCurrentPlanCredits(ctx, r, org); e != nil {
				return e
			}
		}
		out, e = r.Q.BillingNewChange(ctx, db.BillingNewChangeParams{OrganizationID: org, SubscriptionID: sub.ID, PriceID: target.ID, InvoiceID: invoiceID, IdempotencyKey: in.IdempotencyKey, Seats: in.Seats, EffectiveAt: repository.TS(preview.EffectiveAt), State: state, NewPeriodEnd: newEnd})
		if e != nil {
			return e
		}
		return r.Emit(ctx, events.New("billing.plan_change_"+state).WithTenant(org).WithEntity("billing_plan_changes", &out.ID, &out.Uuid).WithPayload(map[string]any{"change_uuid": out.Uuid, "amount_minor": preview.AmountMinor}))
	})
	return out, err
}
func (s *Service) settleCommerce(ctx context.Context, r *repository.Repository, invoice db.BillingInvoice, sub db.BillingSubscription) error {
	change, err := r.Q.BillingPaidChange(ctx, db.BillingPaidChangeParams{OrganizationID: sub.OrganizationID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}})
	if err == nil {
		p, e := r.Q.BillingGetPriceID(ctx, change.PriceID)
		if e != nil {
			return e
		}
		if e = r.Q.BillingApplyChange(ctx, db.BillingApplyChangeParams{OrganizationID: sub.OrganizationID, ID: sub.ID, PriceID: p.ID, PlanVersionID: p.PlanVersionID, ContractedSeats: change.Seats}); e != nil {
			return e
		}
		if change.NewPeriodEnd.Valid {
			if e = r.Q.BillingChangePeriod(ctx, db.BillingChangePeriodParams{OrganizationID: sub.OrganizationID, ID: sub.ID, PeriodStart: change.EffectiveAt, PeriodEnd: change.NewPeriodEnd}); e != nil {
				return e
			}
		}
		if e = grantCurrentPlanCredits(ctx, r, sub.OrganizationID); e != nil {
			return e
		}
		if e = r.Q.BillingCompleteChange(ctx, db.BillingCompleteChangeParams{OrganizationID: sub.OrganizationID, ID: change.ID}); e != nil {
			return e
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if invoice.Kind == "renewal" {
		c, e := r.Q.BillingScheduledChange(ctx, db.BillingScheduledChangeParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID})
		if e == nil {
			if e = r.Q.BillingSetContractedSeats(ctx, db.BillingSetContractedSeatsParams{OrganizationID: sub.OrganizationID, ID: sub.ID, ContractedSeats: c.Seats}); e != nil {
				return e
			}
			if e = r.Q.BillingCompleteChange(ctx, db.BillingCompleteChangeParams{OrganizationID: sub.OrganizationID, ID: c.ID}); e != nil {
				return e
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = r.Q.BillingDeleteItems(ctx, db.BillingDeleteItemsParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID}); e != nil {
			return e
		}
		if e = r.Q.BillingApplyItems(ctx, db.BillingApplyItemsParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID}); e != nil {
			return e
		}
	}
	item, err := r.Q.BillingPaidItemChange(ctx, db.BillingPaidItemChangeParams{OrganizationID: sub.OrganizationID, InvoiceID: invoice.ID})
	if err == nil {
		if err = r.Q.BillingSetItem(ctx, db.BillingSetItemParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, AddonID: item.AddonID, Quantity: item.Quantity, UnitAmountMinor: item.UnitAmountMinor, Currency: item.Currency}); err != nil {
			return err
		}
		if err = r.Q.BillingCompleteItemChange(ctx, db.BillingCompleteItemChangeParams{OrganizationID: sub.OrganizationID, InvoiceID: invoice.ID}); err != nil {
			return err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	purchase, err := r.Q.BillingPurchase(ctx, db.BillingPurchaseParams{OrganizationID: sub.OrganizationID, InvoiceID: invoice.ID})
	if err == nil && !purchase.GrantedAt.Valid {
		var expiry *time.Time
		if purchase.ExpiresDays.Valid {
			at := time.Now().AddDate(0, 0, int(purchase.ExpiresDays.Int32))
			expiry = &at
		}
		_, err = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}, Unit: purchase.Unit, Delta: purchase.Credits, Reason: "Credit package purchase", IdempotencyKey: "purchase:" + invoice.Uuid.String(), ExpiresAt: optionalTS(expiry)})
		if err != nil {
			return err
		}
		return r.Q.BillingGrantPurchase(ctx, db.BillingGrantPurchaseParams{OrganizationID: sub.OrganizationID, InvoiceID: invoice.ID})
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

// AdminPlanChange moves an organization to another published price from the
// platform (sales-led deals, corrections). "now" switches the plan without an
// invoice (the admin bills separately, e.g. a manual or bank transfer
// invoice); "period_end" schedules it for the next renewal like a tenant
// downgrade. Upgrades top up the period allowance; nothing is clawed back.
type AdminPlanChange struct {
	PriceUUID uuid.UUID `json:"price_uuid"`
	Timing    string    `json:"timing"`
	Reason    string    `json:"reason"`
}

func (s *Service) AdminChangePlan(ctx context.Context, org int64, in AdminPlanChange, actor int64) (model.Subscription, error) {
	var out model.Subscription
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Timing != "now" && in.Timing != "period_end" || in.Reason == "" || len(in.Reason) > 1000 {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		switch sub.Status {
		case "trialing", "active", "past_due", "grace", "pending_payment":
		default:
			return model.ErrConflict
		}
		if sub.CancelAtPeriodEnd {
			return model.ErrConflict
		}
		target, e := r.Q.BillingCommercePrice(ctx, in.PriceUUID)
		if e != nil {
			return e
		}
		current, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
		if e != nil {
			return e
		}
		if target.ID == sub.PriceID || target.Currency != current.Currency {
			return model.ErrInvalid
		}
		reason := "Platform plan change: " + in.Reason
		if in.Timing == "period_end" {
			if e = r.Q.BillingScheduleChange(ctx, db.BillingScheduleChangeParams{OrganizationID: org, ID: sub.ID, PendingPriceID: pgtype.Int8{Int64: target.ID, Valid: true}, PendingPlanVersionID: pgtype.Int8{Int64: target.PlanVersionID, Valid: true}}); e != nil {
				return e
			}
		} else {
			if e = r.Q.BillingApplyChange(ctx, db.BillingApplyChangeParams{OrganizationID: org, ID: sub.ID, PriceID: target.ID, PlanVersionID: target.PlanVersionID, ContractedSeats: sub.ContractedSeats}); e != nil {
				return e
			}
			if e = r.Q.BillingScheduleChange(ctx, db.BillingScheduleChangeParams{OrganizationID: org, ID: sub.ID}); e != nil {
				return e
			}
			now := time.Now().UTC()
			if sub.Status == "active" && current.Interval != target.Interval {
				if e = r.Q.BillingChangePeriod(ctx, db.BillingChangePeriodParams{OrganizationID: org, ID: sub.ID, PeriodStart: repository.TS(now), PeriodEnd: repository.TS(NextPeriod(now, target.Interval == "yearly"))}); e != nil {
					return e
				}
			}
			if e = grantCurrentPlanCredits(ctx, r, org); e != nil {
				return e
			}
		}
		var actorID *int64
		if actor > 0 {
			actorID = &actor
		}
		if e = r.Q.BillingRecordTransition(ctx, db.BillingRecordTransitionParams{OrganizationID: org, SubscriptionID: sub.ID, FromStatus: sub.Status, ToStatus: sub.Status, Reason: reason, ActorUserID: repository.Int(actorID)}); e != nil {
			return e
		}
		next, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		name := events.BillingPlanChangeApplied
		if in.Timing == "period_end" {
			name = events.BillingPlanChangeScheduled
		}
		ev := events.New(name).WithTenant(org).WithEntity("billing_subscriptions", &next.ID, &next.Uuid).WithPayload(map[string]any{"price_uuid": in.PriceUUID, "timing": in.Timing, "source": "platform"})
		if actor > 0 {
			ev = ev.WithActor(actor)
		}
		if e = r.Emit(ctx, ev); e != nil {
			return e
		}
		out, e = subscription(ctx, r, next)
		return e
	})
	return out, err
}

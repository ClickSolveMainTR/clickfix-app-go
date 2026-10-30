package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func subscription(ctx context.Context, r *repository.Repository, sub db.BillingSubscription) (model.Subscription, error) {
	v, err := r.Q.BillingGetVersionID(ctx, sub.PlanVersionID)
	if err != nil {
		return model.Subscription{}, err
	}
	p, err := r.Q.BillingGetPriceID(ctx, sub.PriceID)
	if err != nil {
		return model.Subscription{}, err
	}
	out := model.Subscription{UUID: sub.Uuid, VersionUUID: v.Uuid, PriceUUID: p.Uuid, Status: sub.Status, PeriodStart: sub.PeriodStart.Time, PeriodEnd: sub.PeriodEnd.Time, GraceEndsAt: repository.TimePtr(sub.GraceEndsAt), CanceledAt: repository.TimePtr(sub.CanceledAt), CancelAtPeriodEnd: sub.CancelAtPeriodEnd}
	out.TrialEndsAt = repository.TimePtr(sub.TrialEndsAt)
	if sub.Status == "trialing" && sub.TrialEndsAt.Valid {
		remaining := time.Until(sub.TrialEndsAt.Time)
		if remaining > 0 {
			out.TrialDaysRemaining = int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
		}
	}
	if sub.PendingPlanVersionID.Valid {
		v, err = r.Q.BillingGetVersionID(ctx, sub.PendingPlanVersionID.Int64)
		if err != nil {
			return out, err
		}
		out.PendingVersionUUID = &v.Uuid
	}
	return out, nil
}
func (s *Service) OrganizationID(ctx context.Context, id uuid.UUID) (int64, error) {
	org, err := s.repo.Q.BillingGetOrganizationID(ctx, id)
	return org, repository.Normalize(err)
}
func (s *Service) Subscription(ctx context.Context, org int64) (*model.Subscription, error) {
	sub, err := s.repo.Q.BillingGetSubscription(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := subscription(ctx, s.repo, sub)
	return &out, err
}
func (s *Service) CreateSubscription(ctx context.Context, in model.CreateSubscription, actor int64) (model.Subscription, error) {
	var out model.Subscription
	if in.Status == "" {
		in.Status = "pending_payment"
	}
	if in.Status != "pending_payment" && in.Status != "trialing" && in.Status != "active" {
		return out, model.ErrInvalid
	}
	if in.PeriodStart.IsZero() || !in.PeriodEnd.After(in.PeriodStart) || (in.Status == "trialing" && (in.TrialEndsAt == nil || !in.TrialEndsAt.After(in.PeriodStart) || in.TrialEndsAt.After(in.PeriodEnd))) {
		return out, fmt.Errorf("%w: period or trial bounds", model.ErrInvalid)
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		org, err := r.Q.BillingLockOrganization(ctx, in.OrganizationUUID)
		if err != nil {
			return err
		}
		v, err := r.Q.BillingGetVersion(ctx, in.VersionUUID)
		if err != nil {
			return err
		}
		if v.Status != "published" {
			return model.ErrConflict
		}
		p, err := r.Q.BillingGetPrice(ctx, in.PriceUUID)
		if err != nil {
			return err
		}
		if p.PlanVersionID != v.ID {
			return model.ErrInvalid
		}
		sub, err := r.Q.BillingCreateSubscription(ctx, db.BillingCreateSubscriptionParams{OrganizationID: org, PlanVersionID: v.ID, PriceID: p.ID, Status: in.Status, PeriodStart: repository.TS(in.PeriodStart), PeriodEnd: repository.TS(in.PeriodEnd), TrialEndsAt: optionalTS(in.TrialEndsAt)})
		if err != nil {
			return err
		}
		if err = recordTransition(ctx, r, sub, "", "Subscription created", actor); err != nil {
			return err
		}
		out, err = subscription(ctx, r, sub)
		return err
	})
	return out, err
}
func optionalTS(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return repository.TS(*t)
}
func recordTransition(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, from, reason string, actor int64) error {
	if err := grantPlanCredits(ctx, r, sub); err != nil {
		return err
	}
	var actorID *int64
	if actor > 0 {
		actorID = &actor
	}
	if err := r.Q.BillingRecordTransition(ctx, db.BillingRecordTransitionParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, FromStatus: from, ToStatus: sub.Status, Reason: reason, ActorUserID: repository.Int(actorID)}); err != nil {
		return err
	}
	ev := events.New("subscription."+sub.Status).WithTenant(sub.OrganizationID).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"organization_id": sub.OrganizationID, "from": from, "to": sub.Status, "reason": reason})
	if actor > 0 {
		ev = ev.WithActor(actor)
	}
	return r.Emit(ctx, ev)
}
func params(sub db.BillingSubscription) db.BillingSetSubscriptionParams {
	return db.BillingSetSubscriptionParams{OrganizationID: sub.OrganizationID, ID: sub.ID, Status: sub.Status, PeriodStart: sub.PeriodStart, PeriodEnd: sub.PeriodEnd, GraceEndsAt: sub.GraceEndsAt, CanceledAt: sub.CanceledAt, CancelAtPeriodEnd: sub.CancelAtPeriodEnd, PlanVersionID: sub.PlanVersionID, PriceID: sub.PriceID, PendingPlanVersionID: sub.PendingPlanVersionID, PendingPriceID: sub.PendingPriceID}
}
func (s *Service) Transition(ctx context.Context, org int64, id uuid.UUID, in model.Transition, actor int64) (model.Subscription, error) {
	var out model.Subscription
	if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		sub, err := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		next, err := applyTransition(ctx, r, sub, in, actor, time.Now().UTC())
		if err != nil {
			return err
		}
		out, err = subscription(ctx, r, next)
		return err
	})
	return out, err
}
func applyTransition(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, in model.Transition, actor int64, now time.Time) (db.BillingSubscription, error) {
	renewal := sub.Status == "active" && in.To == "active" && in.PeriodStart != nil && in.PeriodEnd != nil
	if err := model.ValidateTransition(sub.Status, in.To); err != nil && !renewal {
		return sub, err
	}
	p := params(sub)
	p.Status = in.To
	if in.To == "grace" {
		if in.GraceEndsAt == nil || !in.GraceEndsAt.After(now) {
			return sub, model.ErrInvalid
		}
		p.GraceEndsAt = optionalTS(in.GraceEndsAt)
	}
	if in.To == "active" {
		p.GraceEndsAt = pgtype.Timestamptz{}
	}
	if in.To == "canceled" || in.To == "expired" {
		p.CanceledAt = repository.TS(now)
		p.CancelAtPeriodEnd = false
		p.PendingPlanVersionID = pgtype.Int8{}
		p.PendingPriceID = pgtype.Int8{}
	}
	// Renewals explicitly provide both boundaries. Payment adapters call this only after settlement.
	if (in.PeriodStart == nil) != (in.PeriodEnd == nil) {
		return sub, model.ErrInvalid
	}
	if in.PeriodStart != nil {
		if sub.CancelAtPeriodEnd {
			return sub, model.ErrConflict
		}
		if in.To != "active" || in.PeriodStart.Before(sub.PeriodEnd.Time) || !in.PeriodEnd.After(*in.PeriodStart) {
			return sub, model.ErrInvalid
		}
		p.PeriodStart = optionalTS(in.PeriodStart)
		p.PeriodEnd = optionalTS(in.PeriodEnd)
		if sub.PendingPlanVersionID.Valid {
			p.PlanVersionID = sub.PendingPlanVersionID.Int64
			p.PriceID = sub.PendingPriceID.Int64
			p.PendingPlanVersionID = pgtype.Int8{}
			p.PendingPriceID = pgtype.Int8{}
		}
	}
	next, err := r.Q.BillingSetSubscription(ctx, p)
	if err != nil {
		return sub, err
	}
	if err = recordTransition(ctx, r, next, sub.Status, in.Reason, actor); err != nil {
		return sub, err
	}
	if in.PeriodStart != nil {
		ev := events.New(events.SubscriptionRenewed).WithTenant(sub.OrganizationID).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"organization_id": sub.OrganizationID, "period_start": next.PeriodStart.Time, "period_end": next.PeriodEnd.Time})
		if actor > 0 {
			ev = ev.WithActor(actor)
		}
		if err = r.Emit(ctx, ev); err != nil {
			return sub, err
		}
	}
	return next, nil
}
func (s *Service) Cancel(ctx context.Context, org int64, id uuid.UUID, immediate bool, actor int64) (model.Subscription, error) {
	var out model.Subscription
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		sub, err := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if sub.Status == "canceled" || sub.Status == "expired" {
			out, err = subscription(ctx, r, sub)
			return err
		}
		if immediate {
			sub, err = applyTransition(ctx, r, sub, model.Transition{To: "canceled", Reason: "Immediate cancellation"}, actor, time.Now().UTC())
			if err != nil {
				return err
			}
		} else if !sub.CancelAtPeriodEnd {
			p := params(sub)
			p.CancelAtPeriodEnd = true
			sub, err = r.Q.BillingSetSubscription(ctx, p)
			if err != nil {
				return err
			}
			if err = r.Emit(ctx, events.New(events.SubscriptionCancelScheduled).WithActor(actor).WithTenant(org).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"organization_id": org, "period_end": sub.PeriodEnd.Time})); err != nil {
				return err
			}
		}
		out, err = subscription(ctx, r, sub)
		return err
	})
	return out, err
}

// Advance is called by renewal/dunning workers: it does not imply a successful charge.
// It is idempotent at a fixed time and always prioritizes scheduled cancellation.
func (s *Service) Advance(ctx context.Context, org int64, id uuid.UUID, now time.Time) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		sub, err := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		to := ""
		switch {
		case sub.Status == "canceled" || sub.Status == "expired":
			return nil
		case sub.CancelAtPeriodEnd && !now.Before(sub.PeriodEnd.Time):
			to = "canceled"
		case sub.Status == "grace" && sub.GraceEndsAt.Valid && !now.Before(sub.GraceEndsAt.Time):
			to = "suspended"
		case sub.Status == "trialing" && !now.Before(sub.TrialEndsAt.Time):
			to = "expired"
		case sub.Status == "pending_payment" && !now.Before(sub.PeriodEnd.Time):
			to = "expired"
		}
		if to == "" {
			return nil
		}
		_, err = applyTransition(ctx, r, sub, model.Transition{To: to, Reason: "Scheduled lifecycle deadline"}, 0, now)
		return err
	})
}
func (s *Service) History(ctx context.Context, org int64, id uuid.UUID) ([]model.HistoryEntry, error) {
	sub, err := s.repo.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, repository.Normalize(err)
	}
	rows, err := s.repo.Q.BillingListTransitions(ctx, db.BillingListTransitionsParams{OrganizationID: org, SubscriptionID: sub.ID})
	if err != nil {
		return nil, err
	}
	out := make([]model.HistoryEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.HistoryEntry{UUID: row.Uuid, From: row.FromStatus, To: row.ToStatus, Reason: row.Reason, CreatedAt: row.CreatedAt.Time})
	}
	return out, nil
}

// Sweep advances deadlines only. It never charges cards or infers payment success.
func (s *Service) Sweep(ctx context.Context) error {
	now := time.Now().UTC()
	failures := []error{}
	if err := entitlements.New(s.repo, nil, "").Sweep(ctx, now); err != nil {
		failures = append(failures, err)
	}
	trialErr := s.TrialSweep(ctx, now)
	if trialErr != nil {
		failures = append(failures, trialErr)
	}
	rows, err := s.repo.Q.BillingDueLifecycle(ctx, repository.TS(now))
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, row := range rows {
		// A failed invoice/provider call must not expire an unresolved auto-pay
		// trial or prevent unrelated organizations from reaching their deadline.
		if trialErr != nil {
			sub, e := s.repo.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams(row))
			if e != nil {
				failures = append(failures, e)
				continue
			}
			if sub.Status == "trialing" && sub.AutoPay && sub.PaymentMethodID.Valid && !sub.CancelAtPeriodEnd {
				continue
			}
		}
		if err = s.Advance(ctx, row.OrganizationID, row.Uuid, now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

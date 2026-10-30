// Package entitlements resolves versioned plan -> add-on -> organization overrides.
package entitlements

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type Snapshot struct {
	Values      map[string]entitlement.Value `json:"values"`
	Writable    bool                         `json:"writable"`
	ValidUntil  time.Time                    `json:"valid_until"`
	UpgradePath string                       `json:"upgrade_path"`
}
type Service struct {
	repo  *repository.Repository
	cache *redis.Client
	env   string
}

func New(repo *repository.Repository, cache *redis.Client, env string) *Service {
	return &Service{repo: repo, cache: cache, env: env}
}
func (s *Service) Resolve(ctx context.Context, org int64) (Snapshot, error) {
	revision, err := s.repo.Q.BillingRevision(ctx, org)
	if err != nil {
		return Snapshot{}, err
	}
	key := fmt.Sprintf("app:%s:entitlements:%d:%d", s.env, org, revision)
	if s.cache != nil {
		raw, err := s.cache.Get(ctx, key).Bytes()
		if err == nil {
			var snap Snapshot
			if json.Unmarshal(raw, &snap) == nil && time.Now().Before(snap.ValidUntil) {
				return snap, nil
			}
		}
	}
	snap, err := resolve(ctx, s.repo, org)
	if err != nil {
		return snap, err
	}
	if s.cache != nil {
		if raw, err := json.Marshal(snap); err == nil && time.Until(snap.ValidUntil) > 0 {
			_ = s.cache.Set(ctx, key, raw, time.Until(snap.ValidUntil)).Err()
		}
	}
	return snap, nil
}
func resolve(ctx context.Context, r *repository.Repository, org int64) (Snapshot, error) {
	now := time.Now().UTC()
	snap := Snapshot{Values: map[string]entitlement.Value{}, ValidUntil: now.Add(30 * time.Second), UpgradePath: entitlement.UpgradePath}
	sub, err := r.Q.BillingGetSubscription(ctx, org)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return snap, err
	}
	bound := func(t time.Time) {
		if t.After(now) && t.Before(snap.ValidUntil) {
			snap.ValidUntil = t
		}
	}
	if err == nil {
		// A trial paid early is active with its first paid period starting at
		// the trial end; it stays writable in between (same rule as
		// BillingResolveEntitlements).
		started := !now.Before(sub.PeriodStart.Time) ||
			sub.TrialEndsAt.Valid && !sub.PeriodStart.Time.After(sub.TrialEndsAt.Time)
		snap.Writable = model.CanWrite(sub.Status) && started
		bound(sub.PeriodStart.Time)
		if sub.GraceEndsAt.Valid {
			bound(sub.GraceEndsAt.Time)
			if !now.Before(sub.GraceEndsAt.Time) {
				snap.Writable = false
			}
		}
		if sub.CancelAtPeriodEnd {
			bound(sub.PeriodEnd.Time)
			if !now.Before(sub.PeriodEnd.Time) {
				snap.Writable = false
			}
		}
		if sub.Status == "trialing" {
			bound(sub.TrialEndsAt.Time)
			if !now.Before(sub.TrialEndsAt.Time) {
				snap.Writable = false
			}
		}
	}
	expiry, err := r.Q.BillingGetOverrideExpiry(ctx, org)
	if err != nil {
		return snap, err
	}
	if expiry.Valid {
		bound(expiry.Time)
	}
	rows, err := r.Q.BillingResolveEntitlements(ctx, org)
	if err != nil {
		return snap, err
	}
	for _, row := range rows {
		limit := row.ValueLimit
		if !row.Overridden && !row.Unlimited {
			if limit > math.MaxInt64-row.AddonUnits {
				return snap, fmt.Errorf("entitlement limit overflow")
			}
			limit += row.AddonUnits
		}
		enabled := row.Enabled
		if row.Kind == "boolean" && !row.Overridden && row.AddonUnits > 0 {
			enabled = true
		}
		snap.Values[row.Key] = entitlement.Value{Feature: row.Key, Kind: row.Kind, Enabled: enabled, Limit: limit, Unlimited: row.Unlimited, Enforcement: row.Enforcement, WarnPercent: row.WarnPercent, Aggregation: row.Aggregation, Period: row.Period}
	}
	return snap, nil
}
func (s *Service) RequireWritable(ctx context.Context, org int64) error {
	snap, err := s.Resolve(ctx, org)
	if err != nil {
		return err
	}
	if !snap.Writable {
		return &entitlement.Error{Code: entitlement.CodeReadOnly}
	}
	return nil
}
func (s *Service) Require(ctx context.Context, org int64, key string) error {
	snap, err := s.Resolve(ctx, org)
	if err != nil {
		return err
	}
	if !snap.Writable {
		return &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: key}
	}
	v, ok := snap.Values[key]
	if !ok || v.Kind != "boolean" || !v.Enabled {
		return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: key}
	}
	return nil
}

// CheckLimit is a preflight check. Resource mutations must serialize on the organization row
// and re-read usage in their transaction; metered events should call Consume instead.
func (s *Service) CheckLimit(ctx context.Context, org int64, key string, used, delta int64) (entitlement.Decision, error) {
	snap, err := s.Resolve(ctx, org)
	if err != nil {
		return entitlement.Decision{}, err
	}
	if !snap.Writable {
		return entitlement.Decision{}, &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: key}
	}
	v, ok := snap.Values[key]
	if !ok {
		return entitlement.Decision{}, &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: key}
	}
	return entitlement.Check(v, used, delta)
}

// Consume atomically checks and counts positive monthly usage, with idempotent event keys.
// A failed business operation must not call this; reservations belong in that operation's transaction.
func (s *Service) Consume(ctx context.Context, org int64, key string, n int64, eventKey string) (entitlement.Decision, error) {
	return s.ConsumeAt(ctx, org, key, n, eventKey, time.Now().UTC())
}

// ConsumeAt preserves the source event time and rejects closed accounting periods.
func (s *Service) ConsumeAt(ctx context.Context, org int64, key string, n int64, eventKey string, occurred time.Time) (entitlement.Decision, error) {
	var out entitlement.Decision
	if occurred.IsZero() || occurred.After(time.Now().Add(time.Minute)) {
		return out, model.ErrInvalid
	}
	if n <= 0 || strings.TrimSpace(eventKey) == "" || len(eventKey) > 200 {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		snap, err := resolve(ctx, r, org)
		if err != nil {
			return err
		}
		v, ok := snap.Values[key]
		if !ok || v.Kind != "limit" || v.Aggregation != "sum" || v.Period != "month" {
			return model.ErrInvalid
		}
		loc := time.FixedZone("TRT", 3*60*60)
		at := occurred.In(loc)
		start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, loc)
		period, err := r.Q.BillingEnsureUsagePeriod(ctx, db.BillingEnsureUsagePeriodParams{OrganizationID: org, StartsAt: repository.TS(start), EndsAt: repository.TS(start.AddDate(0, 1, 0))})
		if err != nil {
			return err
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		used, err := r.Q.BillingUsageInPeriod(ctx, db.BillingUsageInPeriodParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[key].ID})
		if err != nil {
			return err
		}
		exists, err := r.Q.BillingUsageExists(ctx, db.BillingUsageExistsParams{OrganizationID: org, IdempotencyKey: eventKey})
		if err != nil {
			return err
		}
		if exists {
			record, err := r.Q.BillingGetUsageEvent(ctx, db.BillingGetUsageEventParams{OrganizationID: org, IdempotencyKey: eventKey})
			if err != nil {
				return err
			}
			if record.Key != key || record.Quantity != n {
				return model.ErrConflict
			}
			out = entitlement.Decision{Allowed: true, Used: used, Limit: v.Limit, Unlimited: v.Unlimited, UpgradePath: entitlement.UpgradePath}
			return nil
		}
		if !snap.Writable {
			return &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: key}
		}
		out, err = entitlement.Check(v, used, n)
		if err != nil {
			return err
		}
		if period.ClosedAt.Valid {
			return model.ErrConflict
		}
		if err = r.Q.BillingRecordUsageAt(ctx, db.BillingRecordUsageAtParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[key].ID, IdempotencyKey: eventKey, Quantity: n, OccurredAt: repository.TS(occurred)}); err != nil {
			return err
		}
		if err = r.Q.BillingIncrementUsage(ctx, db.BillingIncrementUsageParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[key].ID, Quantity: n}); err != nil {
			return err
		}
		before, _ := entitlement.Check(v, used, 0)
		if out.WarningPercent > before.WarningPercent {
			if err = r.Emit(ctx, events.New(events.EntitlementsQuotaWarning).WithTenant(org).WithPayload(map[string]any{"organization_id": org, "feature": key, "percent": out.WarningPercent, "used": used + n, "limit": v.Limit})); err != nil {
				return err
			}
		}
		out.Used = used + n
		return nil
	})
	return out, err
}
func (s *Service) SetOverride(ctx context.Context, org int64, key string, in *model.Override, actor int64) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		f, ok := fs[key]
		if !ok {
			return model.ErrNotFound
		}
		if in == nil {
			err = r.Q.BillingDeleteOverride(ctx, db.BillingDeleteOverrideParams{OrganizationID: org, FeatureID: f.ID})
		} else {
			in.Feature = key
			if err = model.ValidateGrant(in.Grant, f.Kind); err != nil {
				return err
			}
			if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 || (in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now())) {
				return model.ErrInvalid
			}
			var expiry time.Time
			if in.ExpiresAt != nil {
				expiry = *in.ExpiresAt
			}
			err = r.Q.BillingUpsertOverride(ctx, db.BillingUpsertOverrideParams{OrganizationID: org, FeatureID: f.ID, Kind: f.Kind, ValueBool: repository.Bool(in.Enabled), ValueLimit: repository.Int(in.Limit), Unlimited: in.Unlimited, Reason: in.Reason, ExpiresAt: repository.TS(expiry), CreatedBy: repository.Int(&actor)})
		}
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New(events.EntitlementsOverrideChanged).WithTenant(org).WithActor(actor).WithPayload(map[string]any{"organization_id": org, "feature": key, "removed": in == nil}))
	})
}

// WithinLimit checks a resource count and performs the mutation in one transaction.
// All writers for that resource must use this path (organization lock first).
// count and mutate must use the supplied queries, never the pool or cached counts.
func (s *Service) WithinLimit(ctx context.Context, org int64, key string, delta int64,
	count func(context.Context, *db.Queries) (int64, error), mutate func(context.Context, *db.Queries) error) error {
	if count == nil || mutate == nil || delta <= 0 {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		snap, err := resolve(ctx, r, org)
		if err != nil {
			return err
		}
		if !snap.Writable {
			return &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: key}
		}
		v, ok := snap.Values[key]
		if !ok || v.Kind != "limit" {
			return model.ErrInvalid
		}
		used, err := count(ctx, r.Q)
		if err != nil {
			return err
		}
		if _, err = entitlement.Check(v, used, delta); err != nil {
			return err
		}
		return mutate(ctx, r.Q)
	})
}

// WithAIReservation serializes AI policy, monthly quota and credit reservation on the
// billing organization lock. reserve must use q for every read/write and must not call
// the provider while the transaction is open. The AI run ledger owns pending usage.
func (s *Service) WithAIReservation(ctx context.Context, org int64,
	reserve func(*db.Queries, map[string]entitlement.Value) error) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		snap, err := resolve(ctx, r, org)
		if err != nil {
			return err
		}
		if !snap.Writable {
			return &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: "ai.enabled"}
		}
		v, ok := snap.Values["ai.enabled"]
		if !ok || !v.Enabled {
			return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: "ai.enabled"}
		}
		return reserve(r.Q, snap.Values)
	})
}

package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
)

// Platform features for the daily AI limits (CF-229). The platform admin sets
// them on the plan or as an organization override, unlimited included; the
// tenant ai_policies row may only lower them (NULL = follow the platform).
const (
	FeatureDailyCalls  = "ai.daily_calls"
	FeatureDailyBudget = "ai.daily_budget_micros"
	FeatureConcurrency = "ai.concurrency"
)

// Cap is one limit: Unlimited, or at most Limit.
type Cap struct {
	Limit     *int64 `json:"limit"`
	Unlimited bool   `json:"unlimited"`
}

// Allows reports whether used+delta stays within the cap.
func (c Cap) Allows(used, delta int64) bool {
	return c.Unlimited || (c.Limit != nil && delta <= *c.Limit-used)
}

// Limits is the set of daily AI limits for one organization.
type Limits struct {
	DailyCalls        Cap `json:"daily_calls"`
	Concurrency       Cap `json:"concurrency"`
	DailyBudgetMicros Cap `json:"daily_budget_micros"`
}

// PolicyLimits is what the tenant policy endpoint shows next to the policy.
type PolicyLimits struct {
	Platform  Limits `json:"platform_limits"`
	Effective Limits `json:"effective_limits"`
}

// platformCap reads a resolved entitlement. A soft limit only warns, so it
// does not cap; a missing feature fails closed (limit 0).
func platformCap(values map[string]entitlement.Value, key string) Cap {
	v, ok := values[key]
	if !ok || v.Kind != "limit" {
		zero := int64(0)
		return Cap{Limit: &zero}
	}
	if v.Unlimited || v.Enforcement == "soft" {
		return Cap{Unlimited: true}
	}
	limit := v.Limit
	return Cap{Limit: &limit}
}

// lower applies an optional tenant limit: the lower of the two wins, and the
// tenant can never raise the platform's.
func lower(platform Cap, tenant *int64) Cap {
	if tenant == nil {
		return platform
	}
	if platform.Unlimited || *tenant < *platform.Limit {
		t := *tenant
		return Cap{Limit: &t}
	}
	return platform
}

// Platform returns the platform-side daily AI limits.
func Platform(values map[string]entitlement.Value) Limits {
	return Limits{
		DailyCalls:        platformCap(values, FeatureDailyCalls),
		Concurrency:       platformCap(values, FeatureConcurrency),
		DailyBudgetMicros: platformCap(values, FeatureDailyBudget),
	}
}

// Effective combines the platform limits with the tenant policy.
func Effective(values map[string]entitlement.Value, p db.AiPolicy) Limits {
	pl := Platform(values)
	return Limits{
		DailyCalls:        lower(pl.DailyCalls, int4(p.DailyCalls.Int32, p.DailyCalls.Valid)),
		Concurrency:       lower(pl.Concurrency, int4(p.Concurrency.Int32, p.Concurrency.Valid)),
		DailyBudgetMicros: lower(pl.DailyBudgetMicros, int8(p.DailyBudgetMicros.Int64, p.DailyBudgetMicros.Valid)),
	}
}

func int4(v int32, ok bool) *int64 {
	if !ok {
		return nil
	}
	n := int64(v)
	return &n
}

func int8(v int64, ok bool) *int64 {
	if !ok {
		return nil
	}
	return &v
}

// Limits resolves the platform and effective daily AI limits for display and
// for checks outside a reservation (prefetch). Reserve re-reads them under
// the organization lock.
func (s *Store) Limits(ctx context.Context, p db.AiPolicy) (PolicyLimits, error) {
	snap, err := s.Entitlements.Resolve(ctx, p.OrganizationID)
	if err != nil {
		return PolicyLimits{}, err
	}
	return PolicyLimits{Platform: Platform(snap.Values), Effective: Effective(snap.Values, p)}, nil
}

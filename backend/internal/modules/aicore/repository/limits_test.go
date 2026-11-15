package repository

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/jackc/pgx/v5/pgtype"
)

func limitValue(key string, limit int64, unlimited bool) entitlement.Value {
	return entitlement.Value{Feature: key, Kind: "limit", Limit: limit, Unlimited: unlimited, Enforcement: "hard"}
}

func TestEffectiveLimits(t *testing.T) {
	unlimited := map[string]entitlement.Value{
		FeatureDailyCalls:  limitValue(FeatureDailyCalls, 0, true),
		FeatureDailyBudget: limitValue(FeatureDailyBudget, 0, true),
		FeatureConcurrency: limitValue(FeatureConcurrency, 0, true),
	}
	capped := map[string]entitlement.Value{
		FeatureDailyCalls:  limitValue(FeatureDailyCalls, 500, false),
		FeatureDailyBudget: limitValue(FeatureDailyBudget, 1000, false),
		FeatureConcurrency: limitValue(FeatureConcurrency, 5, false),
	}
	follow := db.AiPolicy{}
	tenant := func(calls int32, conc int32, budget int64) db.AiPolicy {
		return db.AiPolicy{DailyCalls: pgtype.Int4{Int32: calls, Valid: true}, Concurrency: pgtype.Int4{Int32: conc, Valid: true}, DailyBudgetMicros: pgtype.Int8{Int64: budget, Valid: true}}
	}

	t.Run("platform unlimited, tenant follows", func(t *testing.T) {
		l := Effective(unlimited, follow)
		if !l.DailyCalls.Unlimited || !l.Concurrency.Unlimited || !l.DailyBudgetMicros.Unlimited {
			t.Fatalf("%+v", l)
		}
		if !l.DailyCalls.Allows(1_000_000, 1) {
			t.Fatal("unlimited blocked")
		}
	})
	t.Run("tenant lowers an unlimited platform", func(t *testing.T) {
		l := Effective(unlimited, tenant(100, 2, 50))
		if l.DailyCalls.Allows(100, 1) || !l.DailyCalls.Allows(99, 1) || l.Concurrency.Allows(2, 1) || l.DailyBudgetMicros.Allows(40, 11) {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("tenant cannot exceed the platform", func(t *testing.T) {
		l := Effective(capped, tenant(100000, 20, 1_000_000))
		if *l.DailyCalls.Limit != 500 || *l.Concurrency.Limit != 5 || *l.DailyBudgetMicros.Limit != 1000 {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("tenant below the platform wins", func(t *testing.T) {
		l := Effective(capped, tenant(10, 1, 7))
		if *l.DailyCalls.Limit != 10 || *l.Concurrency.Limit != 1 || *l.DailyBudgetMicros.Limit != 7 {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("soft platform limit does not cap", func(t *testing.T) {
		v := capped[FeatureDailyCalls]
		v.Enforcement = "soft"
		l := Effective(map[string]entitlement.Value{FeatureDailyCalls: v}, follow)
		if !l.DailyCalls.Unlimited {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("missing feature fails closed", func(t *testing.T) {
		l := Effective(map[string]entitlement.Value{}, tenant(100, 3, 10))
		if l.DailyCalls.Allows(0, 1) || l.Concurrency.Allows(0, 1) || l.DailyBudgetMicros.Allows(0, 1) {
			t.Fatalf("%+v", l)
		}
	})
}

// CF-228: batch jobs get a share of the effective daily limits.
func TestBatchShare(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	eff := Limits{DailyCalls: Cap{Limit: n(100)}, Concurrency: Cap{Limit: n(3)}, DailyBudgetMicros: Cap{Limit: n(10_000_000)}}
	got := BatchShare(eff, db.AiPolicy{BatchSharePercent: 30})
	if *got.DailyCalls.Limit != 30 || *got.DailyBudgetMicros.Limit != 3_000_000 || *got.Concurrency.Limit != 3 {
		t.Fatalf("%+v", got)
	}
	// A policy that never stored a share uses the default.
	if got := BatchShare(eff, db.AiPolicy{}); *got.DailyCalls.Limit != DefaultBatchSharePercent {
		t.Fatalf("default share %+v", got.DailyCalls)
	}
	if got := BatchShare(Limits{DailyCalls: Cap{Unlimited: true}, DailyBudgetMicros: Cap{Unlimited: true}}, db.AiPolicy{BatchSharePercent: 10}); !got.DailyCalls.Unlimited || !got.DailyBudgetMicros.Unlimited {
		t.Fatalf("unlimited lost: %+v", got)
	}
}

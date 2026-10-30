package entitlement_test

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"math"
	"testing"
)

func TestQuotaBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		limit, used, delta     int64
		soft, unlimited, allow bool
		warning                int
	}{
		{"below warning", 100, 78, 1, false, false, true, 0},
		{"warn at 80", 100, 79, 1, false, false, true, 80},
		{"100 allowed", 100, 99, 1, false, false, true, 100},
		{"hard overflow", 100, 100, 1, false, false, false, 100},
		{"soft overflow", 100, 100, 1, true, false, true, 100},
		{"unlimited", 0, 10000, 1, false, true, true, 0},
		{"zero denies", 0, 0, 1, false, false, false, 100},
		{"big limit no overflow", math.MaxInt64, math.MaxInt64 - 1, 1, false, false, true, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enforcement := "hard"
			if tc.soft {
				enforcement = "soft"
			}
			d, err := entitlement.Check(entitlement.Value{Kind: "limit", Limit: tc.limit, Enforcement: enforcement, Unlimited: tc.unlimited, WarnPercent: 80}, tc.used, tc.delta)
			if d.Allowed != tc.allow || (err == nil) != tc.allow || d.WarningPercent != tc.warning {
				t.Fatalf("%+v %v", d, err)
			}
		})
	}
	for _, pair := range [][2]int64{{-1, 1}, {1, -1}, {math.MaxInt64, 1}} {
		if _, err := entitlement.Check(entitlement.Value{Kind: "limit", Unlimited: true}, pair[0], pair[1]); err == nil {
			t.Fatal("invalid/overflow accepted")
		}
	}
}

package model_test

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"testing"
)

func TestAllLifecycleEdges(t *testing.T) {
	// Independent specification, including forbidden reverse paths and terminal states.
	allowed := map[string]map[string]bool{
		"pending_payment": {"active": true, "trialing": true, "canceled": true, "expired": true},
		"trialing":        {"active": true, "past_due": true, "canceled": true, "expired": true},
		"active":          {"past_due": true, "paused": true, "canceled": true, "expired": true},
		"past_due":        {"active": true, "grace": true, "canceled": true},
		"grace":           {"active": true, "suspended": true, "canceled": true, "expired": true},
		"suspended":       {"active": true, "canceled": true, "expired": true},
		"paused":          {"active": true, "canceled": true, "expired": true},
	}
	states := []string{"pending_payment", "trialing", "active", "past_due", "grace", "suspended", "paused", "canceled", "expired", "unknown"}
	for _, from := range states {
		for _, to := range states {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				err := model.ValidateTransition(from, to)
				if (err == nil) != allowed[from][to] {
					t.Fatalf("allowed=%v err=%v", allowed[from][to], err)
				}
			})
		}
	}
}

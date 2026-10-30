package model

import "fmt"

// LegalTransitions is deliberately explicit: terminal subscriptions cannot be resurrected.
// A new purchase creates a new subscription, preserving the old audit trail.
func LegalTransitions() map[string][]string {
	return map[string][]string{
		"pending_payment": {"active", "trialing", "canceled", "expired"},
		"trialing":        {"active", "past_due", "canceled", "expired"},
		"active":          {"past_due", "paused", "canceled", "expired"},
		"past_due":        {"active", "grace", "canceled"},
		"grace":           {"active", "suspended", "canceled", "expired"},
		"suspended":       {"active", "canceled", "expired"},
		"paused":          {"active", "canceled", "expired"},
		"canceled":        {}, "expired": {},
	}
}
func ValidateTransition(from, to string) error {
	for _, s := range LegalTransitions()[from] {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: transition %s -> %s", ErrConflict, from, to)
}

// CanWrite is separate from organization suspension: inbound messages and reads continue.
func CanWrite(status string) bool {
	return status == "active" || status == "trialing" || status == "past_due" || status == "grace"
}

package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/apierror"
)

// Availability says whether AI actions (copilot, AI runs) can run for an
// organization and, when not, why. Reason is the error code a run would get.
type Availability struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
	Entitled   bool   `json:"entitled"`
}

// Configured reports whether the platform has a priced provider for task.
func (s *Service) Configured(task string) bool {
	route, ok := s.Routes[task]
	return ok && route.Provider != nil && route.InputPrice > 0 && route.OutputPrice > 0
}

// Availability checks the static preconditions of a reply run: platform
// configuration, the organization policy and the plan. Credits and daily
// limits are checked when a run is reserved (429 AI_CREDITS_EXHAUSTED /
// AI_LIMIT_REACHED).
func (s *Service) Availability(ctx context.Context, org int64) (Availability, error) {
	a := Availability{Configured: s.Configured("reply")}
	policy, err := s.Repo.Policy(ctx, org)
	if err != nil {
		return a, err
	}
	a.Enabled = policy.Enabled
	a.Entitled = true
	if s.Repo.Entitlements != nil {
		snap, err := s.Repo.Entitlements.Resolve(ctx, org)
		if err != nil {
			return a, err
		}
		_, tokens := snap.Values["ai.tokens"]
		_, credits := snap.Values["ai.credits"]
		a.Entitled = tokens && credits
	}
	switch {
	case !a.Configured:
		a.Reason = apierror.CodeNotConfigured
	case !a.Enabled:
		a.Reason = apierror.CodeDisabled
	case !a.Entitled:
		a.Reason = apierror.CodeNotEntitled
	default:
		a.Available = true
	}
	return a, nil
}

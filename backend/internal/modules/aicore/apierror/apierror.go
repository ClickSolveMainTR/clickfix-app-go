// Package apierror maps AI execution errors to API responses. Business
// conditions (AI off, not configured, plan, credits, limits) are 4xx with a
// specific code so clients can explain them and the error tracker ignores
// them; only a provider that failed to answer is a 503.
package apierror

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// API error codes.
const (
	CodeDisabled            = "AI_DISABLED"
	CodeNotConfigured       = "AI_NOT_CONFIGURED"
	CodeNotEntitled         = "AI_NOT_ENTITLED"
	CodeCreditsExhausted    = "AI_CREDITS_EXHAUSTED" // #nosec G101 -- skip reason / error code, not a credential
	CodeLimitReached        = "AI_LIMIT_REACHED"
	CodeGuardRejected       = "AI_GUARD_REJECTED"
	CodeProviderUnavailable = "AI_PROVIDER_UNAVAILABLE"
)

// Code returns the API error code for err ("" when err is not an AI error).
func Code(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ai.ErrInjection), errors.Is(err, ai.ErrRefused):
		return CodeGuardRejected
	case errors.Is(err, model.ErrDisabled):
		return CodeDisabled
	case errors.Is(err, entitlement.ErrDenied):
		return CodeNotEntitled
	case errors.Is(err, model.ErrCredits):
		return CodeCreditsExhausted
	case errors.Is(err, model.ErrBudget), errors.Is(err, model.ErrConcurrency), errors.Is(err, model.ErrDaily):
		return CodeLimitReached
	case errors.Is(err, ai.ErrNotConfigured):
		return CodeNotConfigured
	case errors.Is(err, ai.ErrUnavailable), errors.Is(err, ai.ErrTruncated), errors.Is(err, ai.ErrInvalidOutput):
		return CodeProviderUnavailable
	}
	return ""
}

// Write writes the response for an AI error and reports whether err was one.
func Write(w http.ResponseWriter, r *http.Request, err error) bool {
	code := Code(err)
	switch code {
	case "":
		return false
	case CodeGuardRejected:
		response.Error(w, r, http.StatusUnprocessableEntity, code, "AI guard rejected the request or output")
	case CodeDisabled:
		response.Error(w, r, http.StatusForbidden, code, "AI is turned off for this organization")
	case CodeNotEntitled:
		response.Error(w, r, http.StatusForbidden, code, "The current plan does not include AI")
	case CodeCreditsExhausted:
		response.Error(w, r, http.StatusTooManyRequests, code, "AI credits exhausted: stop_and_handoff")
	case CodeLimitReached:
		response.Error(w, r, http.StatusTooManyRequests, code, "AI budget, concurrency or daily call limit reached")
	case CodeNotConfigured:
		response.Conflict(w, r, code, "AI is not configured on this platform")
	default:
		response.ServiceUnavailable(w, r, code, "AI provider did not return a usable answer")
	}
	return true
}

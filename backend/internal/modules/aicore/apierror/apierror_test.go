package apierror

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
)

// Business conditions are 4xx with a specific code; only a provider that
// did not answer is a 503.
func TestWriteMapsBusinessConditionsTo4xx(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("run: %w", model.ErrDisabled), http.StatusForbidden, CodeDisabled},
		{&entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: "ai.tokens"}, http.StatusForbidden, CodeNotEntitled},
		{ai.ErrNotConfigured, http.StatusConflict, CodeNotConfigured},
		{model.ErrCredits, http.StatusTooManyRequests, CodeCreditsExhausted},
		{model.ErrDaily, http.StatusTooManyRequests, CodeLimitReached},
		{ai.ErrInjection, http.StatusUnprocessableEntity, CodeGuardRejected},
		{ai.ErrUnavailable, http.StatusServiceUnavailable, CodeProviderUnavailable},
	} {
		rr := httptest.NewRecorder()
		if !Write(rr, httptest.NewRequest(http.MethodPost, "/v1/tenant/ai/runs", nil), c.err) {
			t.Fatalf("%v not handled", c.err)
		}
		if rr.Code != c.status || Code(c.err) != c.code {
			t.Fatalf("%v: status %d code %s, want %d %s", c.err, rr.Code, Code(c.err), c.status, c.code)
		}
	}
	if Write(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), errors.New("db down")) {
		t.Fatal("unrelated errors must fall through to the 500 path")
	}
}

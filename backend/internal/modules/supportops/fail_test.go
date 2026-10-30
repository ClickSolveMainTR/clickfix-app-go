package supportops

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
)

func TestFailWritesPlanGateNotServerError(t *testing.T) {
	for _, c := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"plan gate", fmt.Errorf("save: %w", &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: "security.sso"}), 403, entitlement.CodeFeatureRequired},
		{"denied", ErrDenied, 403, ""},
		{"conflict", ErrConflict, 409, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			Fail(w, httptest.NewRequest("PUT", "/v1/tenant/security/sso", nil), c.err)
			if w.Code != c.status || !strings.Contains(w.Body.String(), c.body) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

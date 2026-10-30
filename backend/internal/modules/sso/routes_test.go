package sso

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
)

// Usecase sentinels must reach the router as 4xx, never as a 500.
func TestMapErrorKeepsUsecaseErrorsOutOf5xx(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("wrapped: %w", usecase.ErrInvalid), http.StatusBadRequest},
		{usecase.ErrDenied, http.StatusForbidden},
	} {
		mapped := mapError(c.err)
		if !errors.Is(mapped, supportops.ErrInvalid) && !errors.Is(mapped, supportops.ErrDenied) {
			t.Fatalf("%v not mapped", c.err)
		}
		rr := httptest.NewRecorder()
		supportops.Fail(rr, httptest.NewRequest(http.MethodPost, "/v1/tenant/security/scim-token", nil), mapped)
		if rr.Code != c.want {
			t.Fatalf("%v: status %d, want %d", c.err, rr.Code, c.want)
		}
	}
	if mapError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}

package attachments

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/handler"
)

func TestRegisterRoutesWithoutTenancyMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(nil, nil), nil, nil, nil)
	for _, path := range []string{"/v1/tenant/attachments", "/v1/public/attachments/abc"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 when tenancy is off", path, rec.Code)
		}
	}
}

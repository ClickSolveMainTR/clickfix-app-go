package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/google/uuid"
)

// TestExecuteRejectsOversizedBody: the body is read through a 1 MiB limit
// before the service is touched (svc is nil), CF-153.
func TestExecuteRejectsOversizedBody(t *testing.T) {
	h := New(nil)
	body := append([]byte(`{"action":"x","ids":["`), bytes.Repeat([]byte("a"), maxBulkBodyBytes+1)...)
	body = append(body, []byte(`"]}`)...)
	r := httptest.NewRequest(http.MethodPost, "/v1/platform/users/bulk", bytes.NewReader(body))
	r = r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserID: uuid.New(), UserInternal: 1}))
	w := httptest.NewRecorder()
	h.ExecuteUsers(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
}

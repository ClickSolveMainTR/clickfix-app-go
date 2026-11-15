package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// KnowledgeHandler serves the AI knowledge panel (CF-220).
type KnowledgeHandler struct{ Service *usecase.KnowledgeService }

// Panel is GET /v1/tenant/analytics/ai-knowledge.
func (h *KnowledgeHandler) Panel(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, until time.Time
	var department *uuid.UUID
	for key, dst := range map[string]*time.Time{"from": &from, "until": &until} {
		if v := q.Get(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				response.Error(w, r, 400, "VALIDATION_ERROR", key+" must be RFC3339")
				return
			}
			*dst = t
		}
	}
	if v := q.Get("department_uuid"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid department_uuid")
			return
		}
		department = &id
	}
	out, err := h.Service.Panel(r.Context(), orgctx.Must(r.Context()).ID, department, from, until)
	switch {
	case errors.Is(err, model.ErrDepartmentsUnavailable):
		response.Error(w, r, 400, "DEPARTMENTS_UNAVAILABLE", "Departments are not available yet")
	case errors.Is(err, usecase.ErrInvalid):
		response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid panel filter")
	case err != nil:
		response.Internal(w, r, "AI knowledge panel unavailable")
	default:
		response.JSON(w, r, 200, out)
	}
}

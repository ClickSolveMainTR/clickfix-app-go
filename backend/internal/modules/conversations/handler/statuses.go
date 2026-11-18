package handler

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// CodeStatusExists: another custom status has the name (CF-249).
const CodeStatusExists = "STATUS_EXISTS"

type customStatusRequest struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	Category  *string `json:"category"`
	SortOrder *int32  `json:"sort_order"`
}

func (c customStatusRequest) input() usecase.CustomStatusInput {
	return usecase.CustomStatusInput{Name: c.Name, Color: c.Color, Category: c.Category, SortOrder: c.SortOrder}
}

// CustomStatuses lists the organization's custom conversation statuses.
func (h *Handler) CustomStatuses(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := who(w, r)
	if !ok {
		return
	}
	items, err := h.svc.CustomStatuses(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateCustomStatus adds a custom status.
func (h *Handler) CreateCustomStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	var req customStatusRequest
	if !decode(w, r, &req) {
		return
	}
	st, err := h.svc.CreateCustomStatus(r.Context(), scope.ID, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversation_statuses.created", st.UUID, map[string]any{"name": st.Name, "category": st.Category})
	response.JSON(w, r, http.StatusCreated, st)
}

// UpdateCustomStatus changes a custom status.
func (h *Handler) UpdateCustomStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req customStatusRequest
	if !decode(w, r, &req) {
		return
	}
	st, err := h.svc.UpdateCustomStatus(r.Context(), scope.ID, id, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversation_statuses.updated", st.UUID, map[string]any{"name": st.Name, "category": st.Category})
	response.JSON(w, r, http.StatusOK, st)
}

// DeleteCustomStatus removes a custom status.
func (h *Handler) DeleteCustomStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCustomStatus(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversation_statuses.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

package handler

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"net/http"
)

func (h *Handler) Diagnostics(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Diagnostics(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

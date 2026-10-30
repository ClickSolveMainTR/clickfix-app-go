package handler

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	in, err := supportops.Read[usecase.CreateInput](r)
	if err != nil {
		invalid(w, r, "Invalid conversation")
		return
	}
	out, err := h.svc.Create(r.Context(), scope.ID, actor, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "created", out.Uuid, nil)
	response.JSON(w, r, 201, map[string]any{"uuid": out.Uuid, "number": out.Number, "status": out.Status})
}

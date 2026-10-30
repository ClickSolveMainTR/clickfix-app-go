package handler

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"net/http"
)

// VoiceCalls lists call logs and references to recording messages; attachments
// are accessed using the normal tenant attachment permission checks and API URLs.
func (h *Handler) VoiceCalls(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	query := apiquery.Parse(r.URL.Query())
	rows, total, err := h.svc.VoiceCalls(r.Context(), scope.ID, id, query.Limit, query.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": rows, "total": total, "limit": query.Limit, "offset": query.Offset})
}

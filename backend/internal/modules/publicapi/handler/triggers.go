package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) Triggers(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	if !org.Has(rbac.PermTenantContactsRead) || !org.Has(rbac.PermTenantConversationsRead) {
		response.Forbidden(w, r, "Conversation and contact read permissions required")
		return
	}
	event := r.URL.Query().Get("event")
	if !slices.Contains([]string{"conversation.created", "conversation.resolved", "contact.created", "csat.submitted", "conversation.tag_added"}, event) {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	cursor := int64(0)
	if v := r.URL.Query().Get("cursor"); v != "" {
		var err error
		cursor, err = strconv.ParseInt(v, 10, 64)
		if err != nil || cursor < 0 {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
	}
	limit := int32(100)
	oldest, err := h.Service.Store.IntegrationTriggerOldest(r.Context(), org.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, r, err)
		return
	}
	if cursor > 0 && oldest > cursor {
		response.Error(w, r, 410, "CURSOR_EXPIRED", "Cursor history expired; resynchronize with cursor=0")
		return
	}
	rows, err := h.Service.Store.IntegrationPollTriggers(r.Context(), db.IntegrationPollTriggersParams{OrganizationID: org.ID, EventName: event, AfterSequence: cursor, RowLimit: limit})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		cursor = row.Sequence
		items = append(items, json.RawMessage(row.Payload))
	}
	// Empty pages retain the last seen cursor so a concurrent insert is never skipped.
	response.JSON(w, r, 200, map[string]any{"items": items, "next_cursor": strconv.FormatInt(cursor, 10), "has_more": len(rows) == int(limit), "retention_days": 30})
}

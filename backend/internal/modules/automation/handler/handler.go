package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/integrationhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func (h *Handler) Triggers(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, usecase.TriggerNames())
}
func (h *Handler) CRUD(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	id := uuid.Nil
	var err error
	if raw := r.PathValue("uuid"); raw != "" {
		id, err = uuid.Parse(raw)
		if err != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var in model.RuleInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&in) != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
		actor := authctx.MustPrincipal(r.Context())
		out, err := h.Service.Save(r.Context(), org.ID, actor.UserInternal, id, in)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.Activity.Record(r.Context(), &actor.UserInternal, "updated", "automation_rules", &out.Uuid, map[string]any{"name": in.Name}, r)
		status := 200
		if id == uuid.Nil {
			status = 201
		}
		response.JSON(w, r, status, out)
	case http.MethodDelete:
		n, err := h.Service.Store.DeleteAutomationRule(r.Context(), db.DeleteAutomationRuleParams{OrganizationID: org.ID, Uuid: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if n == 0 {
			h.fail(w, r, pgx.ErrNoRows)
			return
		}
		response.JSON(w, r, 200, map[string]bool{"deleted": true})
	default:
		if id != uuid.Nil {
			row, err := h.Service.Store.GetAutomationRule(r.Context(), db.GetAutomationRuleParams{OrganizationID: org.ID, Uuid: id})
			if err != nil {
				h.fail(w, r, err)
				return
			}
			response.JSON(w, r, 200, row)
			return
		}
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, err := h.Service.Store.ListAutomationRules(r.Context(), db.ListAutomationRulesParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		n, err := h.Service.Store.CountAutomationRules(r.Context(), org.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
	}
}
func (h *Handler) DryRun(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	var in struct {
		ConversationUUID uuid.UUID `json:"conversation_uuid"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.ConversationUUID == uuid.Nil {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	rule, err := h.Service.Store.GetAutomationRule(r.Context(), db.GetAutomationRuleParams{OrganizationID: org.ID, Uuid: id})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rule.CreatedBy = authctx.MustPrincipal(r.Context()).UserInternal
	matched, err := h.Service.DryRun(r.Context(), rule, in.ConversationUUID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"matched": matched, "actions": json.RawMessage(rule.Actions), "dry_run": true})
}
func (h *Handler) Executions(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	q, ok := integrationhttp.Query(w, r, false)
	if !ok {
		return
	}
	rows, err := h.Service.Store.ListAutomationExecutions(r.Context(), db.ListAutomationExecutionsParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.Service.Store.CountAutomationExecutions(r.Context(), org.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Automation rule not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}

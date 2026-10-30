package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/integrationhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) Workflows(w http.ResponseWriter, r *http.Request) {
	s := h.Service.Workflows
	org := orgctx.Must(r.Context()).ID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	id := uuid.Nil
	if raw := r.PathValue("uuid"); raw != "" {
		var err error
		id, err = uuid.Parse(raw)
		if err != nil {
			h.workflowError(w, r, usecase.ErrInvalid)
			return
		}
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var in model.WorkflowInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&in) != nil {
			h.workflowError(w, r, usecase.ErrInvalid)
			return
		}
		row, err := s.Save(r.Context(), org, actor, id, in)
		if err != nil {
			h.workflowError(w, r, err)
			return
		}
		h.Activity.Record(r.Context(), &actor, "updated", "automation_rules", &row.Uuid, map[string]any{"workflow": true, "revision": row.Revision}, r)
		status := 200
		if id == uuid.Nil {
			status = 201
		}
		response.JSON(w, r, status, workflowView(row))
	default:
		if id != uuid.Nil {
			row, err := s.Store.Q.WorkflowGet(r.Context(), db.WorkflowGetParams{OrganizationID: org, Uuid: id})
			if err != nil {
				h.workflowError(w, r, err)
				return
			}
			response.JSON(w, r, 200, workflowView(row))
			return
		}
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, err := s.Store.Q.WorkflowList(r.Context(), db.WorkflowListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			h.workflowError(w, r, err)
			return
		}
		n, err := s.Store.Q.WorkflowCount(r.Context(), org)
		if err != nil {
			h.workflowError(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(workflowViews(rows), n, q.Limit, q.Offset))
	}
}
func (h *Handler) WorkflowCommand(w http.ResponseWriter, r *http.Request) {
	s := h.Service.Workflows
	org := orgctx.Must(r.Context()).ID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.workflowError(w, r, usecase.ErrInvalid)
		return
	}
	row, err := s.Store.Q.WorkflowGet(r.Context(), db.WorkflowGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	cmd := r.PathValue("command")
	if cmd == "versions" {
		rows, e := s.Store.Q.WorkflowVersions(r.Context(), db.WorkflowVersionsParams{OrganizationID: org, WorkflowID: row.ID})
		if e != nil {
			h.workflowError(w, r, e)
			return
		}
		response.JSON(w, r, 200, versionViews(rows))
		return
	}
	if cmd == "runs" {
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, e := s.Store.Q.WorkflowRunList(r.Context(), db.WorkflowRunListParams{OrganizationID: org, WorkflowID: row.ID, Limit: q.Limit, Offset: q.Offset})
		if e != nil {
			h.workflowError(w, r, e)
			return
		}
		n, e := s.Store.Q.WorkflowRunCount(r.Context(), db.WorkflowRunCountParams{OrganizationID: org, WorkflowID: row.ID})
		if e != nil {
			h.workflowError(w, r, e)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(runViews(rows), n, q.Limit, q.Offset))
		return
	}
	var in struct {
		Revision     int32     `json:"revision"`
		Enabled      bool      `json:"enabled"`
		Conversation uuid.UUID `json:"conversation_uuid"`
		Event        uuid.UUID `json:"event_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		h.workflowError(w, r, usecase.ErrInvalid)
		return
	}
	var out any
	switch cmd {
	case "publish":
		out, err = s.Publish(r.Context(), org, actor, id, in.Revision)
	case "enable":
		out, err = s.Enable(r.Context(), org, id, in.Enabled)
	case "test":
		if in.Conversation == uuid.Nil {
			err = usecase.ErrInvalid
			break
		}
		var trace []model.TraceStep
		trace, err = s.DryRun(r.Context(), row, actor, in.Conversation)
		out = map[string]any{"dry_run": true, "trace": trace}
	case "webhook":
		// This is an authenticated ingress: API keys retain tenant permission checks.
		// Stable caller event ids, scoped by workflow, make webhook retries idempotent.
		if in.Conversation == uuid.Nil || in.Event == uuid.Nil {
			err = usecase.ErrInvalid
			break
		}
		_, err = h.Service.Conversations.Get(r.Context(), org, actor, in.Conversation)
		if err != nil {
			break
		}
		versions, e := s.Store.Q.WorkflowMatching(r.Context(), db.WorkflowMatchingParams{OrganizationID: org, TriggerEvent: "webhook"})
		if e != nil {
			err = e
			break
		}
		for _, v := range versions {
			if v.WorkflowID == row.ID {
				out, err = s.StartWebhook(r.Context(), v, in.Event, in.Conversation)
				break
			}
		}
		if out == nil && err == nil {
			err = usecase.ErrInvalid
		}
	default:
		err = usecase.ErrInvalid
	}
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	if cmd == "publish" || cmd == "enable" {
		h.Activity.Record(r.Context(), &actor, "updated", "automation_rules", &id, map[string]any{"workflow": true, "command": cmd}, r)
	}
	switch v := out.(type) {
	case db.Workflow:
		out = workflowView(v)
	case db.WorkflowRun:
		out = runView(v)
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) WorkflowRun(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	id, err := uuid.Parse(r.PathValue("run"))
	if err != nil {
		h.workflowError(w, r, usecase.ErrInvalid)
		return
	}
	row, err := h.Service.Workflows.Store.Q.WorkflowRunGet(r.Context(), db.WorkflowRunGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	scope := orgctx.Must(r.Context())
	if !scope.Has(rbac.PermTenantConversationsRead) {
		response.Forbidden(w, r, "")
		return
	}
	if _, err = h.Service.Conversations.Get(r.Context(), org, authctx.MustPrincipal(r.Context()).UserInternal, row.ConversationUuid); err != nil {
		response.Forbidden(w, r, "")
		return
	}
	steps, err := h.Service.Workflows.Store.Q.WorkflowSteps(r.Context(), db.WorkflowStepsParams{OrganizationID: org, RunID: row.ID})
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	version, err := h.Service.Workflows.Store.Q.WorkflowVersionGet(r.Context(), db.WorkflowVersionGetParams{OrganizationID: org, ID: row.VersionID})
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"run": runView(row), "steps": stepViews(steps), "version": versionViews([]db.WorkflowVersion{version})[0]})
}
func (h *Handler) WorkflowImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Rule uuid.UUID `json:"routing_rule_uuid"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.Rule == uuid.Nil {
		h.workflowError(w, r, usecase.ErrInvalid)
		return
	}
	org := orgctx.Must(r.Context()).ID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	row, err := h.Service.Workflows.ImportRouting(r.Context(), org, actor, in.Rule)
	if err != nil {
		h.workflowError(w, r, err)
		return
	}
	h.Activity.Record(r.Context(), &actor, "updated", "automation_rules", &row.Uuid, map[string]any{"imported_routing_rule": in.Rule}, r)
	response.JSON(w, r, 201, workflowView(row))
}
func (h *Handler) workflowError(w http.ResponseWriter, r *http.Request, err error) {
	if entitlement.WriteError(w, r, err) {
		return
	}
	if errors.Is(err, usecase.ErrWorkflowConflict) {
		response.Conflict(w, r, "CONFLICT", "Workflow revision changed")
		return
	}
	h.fail(w, r, err)
}

func (h *Handler) WorkflowMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "tenant.workflows", DefaultFields: []string{"uuid", "name", "revision", "published_version", "enabled"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true}, SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{}, Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{}})
}

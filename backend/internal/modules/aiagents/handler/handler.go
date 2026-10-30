// Package handler exposes AI configuration, analysis history and copilot.
package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/apierror"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "")
	case errors.Is(err, model.ErrInvalid), errors.Is(err, coremodel.ErrInvalid):
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid AI agent input")
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "CONFLICT", "AI agent state changed")
	case apierror.Write(w, r, err):
	default:
		response.InternalErr(w, r, err, "")
	}
}
func (h *Handler) audit(r *http.Request, operation string) {
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.Activity.Record(r.Context(), &actor, "organizations.updated", "organizations", nil, map[string]any{"area": "aiagents", "operation": operation}, r)
}
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodPut {
		var in model.Config
		if !kbhandler.Decode(w, r, &in) {
			return
		}
		revision, err := h.Service.Repo.SaveConfig(r.Context(), org, authctx.MustPrincipal(r.Context()).UserInternal, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "config.updated")
		response.JSON(w, r, 200, map[string]any{"config": in, "revision": revision})
		return
	}
	cfg, revision, err := h.Service.Repo.Config(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"config": cfg, "revision": revision})
}
func conversationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return uuid.Nil, false
	}
	return id, true
}
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	c, err := h.Service.Repo.Q.GetConversationByUUID(r.Context(), db.GetConversationByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		fail(w, r, err)
		return
	}
	if err = h.Service.Repo.CheckConversation(r.Context(), org, c.ID); err != nil {
		fail(w, r, err)
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.AIAgentJobs(r.Context(), db.AIAgentJobsParams{OrganizationID: org, Uuid: id, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	scope := orgctx.Must(r.Context())
	if !scope.CanSeeAll(access.Resource) {
		viewer, e := access.For(r.Context(), h.Service.Repo.Q, scope, authctx.MustPrincipal(r.Context()).UserInternal)
		if e != nil {
			fail(w, r, e)
			return
		}
		for i := range rows {
			if rows[i].Task != "triage" {
				continue
			}
			var triage model.Triage
			if e = json.Unmarshal(rows[i].Result, &triage); e != nil {
				fail(w, r, e)
				return
			}
			ids := []uuid.UUID{}
			for _, match := range triage.SimilarConversations {
				ids = append(ids, match.UUID)
			}
			visible, e := viewer.Visible(r.Context(), h.Service.Repo.Q, org, ids)
			if e != nil {
				fail(w, r, e)
				return
			}
			filtered := []model.ConversationMatch{}
			for _, match := range triage.SimilarConversations {
				if visible[match.UUID] {
					filtered = append(filtered, match)
				}
			}
			triage.SimilarConversations = filtered
			rows[i].Result, e = json.Marshal(triage)
			if e != nil {
				fail(w, r, e)
				return
			}
		}
	}
	count, err := h.Service.Repo.Q.AIAgentJobCount(r.Context(), db.AIAgentJobCountParams{OrganizationID: org, Uuid: id})
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": rows, "total": count, "limit": q.Limit, "offset": q.Offset})
}
func (h *Handler) Copilot(w http.ResponseWriter, r *http.Request) {
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in model.CopilotInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.Copilot(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "copilot.generated")
	response.JSON(w, r, 200, out)
}

func (h *Handler) Tools(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if h.Service.Tools == nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	if r.Method == http.MethodPut {
		var in usecase.ToolInput
		if !kbhandler.Decode(w, r, &in) {
			return
		}
		id, err := h.Service.Tools.Save(r.Context(), org, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "tool.saved")
		response.JSON(w, r, 200, map[string]any{"uuid": id})
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.AITools(r.Context(), db.AIToolsParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	count, err := h.Service.Repo.Q.AIToolCount(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	items := make([]toolView, len(rows))
	for i, row := range rows {
		items[i] = toolView{AIToolsRow: row, Schema: rawJSON(row.Schema)}
	}
	response.JSON(w, r, 200, map[string]any{"items": items, "total": count, "limit": q.Limit, "offset": q.Offset})
}
func (h *Handler) ToolCalls(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.AIToolCalls(r.Context(), db.AIToolCallsParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	count, err := h.Service.Repo.Q.AIToolCallsCount(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": rows, "total": count, "limit": q.Limit, "offset": q.Offset})
}
func (h *Handler) InvokeTool(w http.ResponseWriter, r *http.Request) {
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	if h.Service.Tools == nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	var in usecase.Invocation
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	value, err := h.Service.Tools.Invoke(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "tool.invoked")
	response.JSON(w, r, 200, map[string]any{"result": value, "dry_run": in.DryRun, "invocation_key": in.Key})
}

func (h *Handler) Procedures(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		q, ok := listQuery(w, r)
		if !ok {
			return
		}
		rows, err := h.Service.Repo.Q.AIProcedures(r.Context(), db.AIProceduresParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.Service.Repo.Q.AIProcedureCount(r.Context(), org)
		if err != nil {
			fail(w, r, err)
			return
		}
		items := make([]procedureView, len(rows))
		for i, row := range rows {
			items[i] = viewProcedure(row)
		}
		response.JSON(w, r, 200, map[string]any{"items": items, "total": total, "limit": q.Limit, "offset": q.Offset})
		return
	}
	var id uuid.UUID
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = conversationID(w, r)
		if !ok {
			return
		}
	}
	var in usecase.Procedure
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	row, err := h.Service.SaveProcedure(r.Context(), org, id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "procedure.saved")
	response.JSON(w, r, 200, viewProcedure(row))
}
func (h *Handler) PublishProcedure(w http.ResponseWriter, r *http.Request) {
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Revision int32 `json:"revision"`
		Publish  bool  `json:"publish"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	if in.Revision < 1 {
		fail(w, r, model.ErrInvalid)
		return
	}
	row, err := h.Service.Repo.Q.AIProcedurePublish(r.Context(), db.AIProcedurePublishParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: id, ExpectedRevision: in.Revision, Publish: in.Publish})
	if errors.Is(err, pgx.ErrNoRows) {
		err = model.ErrConflict
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "procedure.published")
	response.JSON(w, r, 200, viewProcedure(row))
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	resource := "tenant.ai.procedures"
	fields := []string{"uuid", "intent", "locale", "title", "revision", "published_revision"}
	if strings.Contains(r.URL.Path, "/tools/") {
		resource = "tenant.ai.tools"
		fields = []string{"uuid", "name", "description", "enabled", "requires_approval"}
	}
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: resource, DefaultSort: "-id", DefaultFields: fields,
		Capabilities:     resourcemeta.Capabilities{Read: true, Create: true, Update: true},
		SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{},
		Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{},
	})
}

func listQuery(w http.ResponseWriter, r *http.Request) (apiquery.Query, bool) {
	q := apiquery.Parse(r.URL.Query())
	if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 || q.Q != "" {
		response.ValidationError(w, r, []response.Detail{{Field: "query", Code: "unsupported", Message: "Only limit and offset are supported on this list"}})
		return q, false
	}
	return q, true
}

// JSONB columns are []byte in sqlc rows, which encoding/json writes as base64.
// These views keep them as the JSON objects documented in OpenAPI.
type procedureView struct {
	db.AiProcedure
	Draft     json.RawMessage `json:"draft"`
	Published json.RawMessage `json:"published"`
}

type toolView struct {
	db.AIToolsRow
	Schema json.RawMessage `json:"schema"`
}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

func viewProcedure(row db.AiProcedure) procedureView {
	return procedureView{AiProcedure: row, Draft: rawJSON(row.Draft), Published: rawJSON(row.Published)}
}

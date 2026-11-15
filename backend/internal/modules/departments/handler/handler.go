// Package handler serves /v1/tenant/departments and the department of a
// conversation (CF-215).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the department routes.
type Handler struct {
	svc      *usecase.Service
	q        *db.Queries
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, q *db.Queries, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, q: q, activity: rec}
}

// Meta is the table contract of GET /v1/tenant/departments.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:         model.Resource,
		DefaultSort:      "sort_order",
		DefaultFields:    []string{"name", "teams", "leads", "sla_policy_uuid", "ai"},
		Capabilities:     resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true},
		SearchableFields: []string{"name", "description"},
		SortableFields:   []string{},
		FilterableFields: []string{"brand", "mine"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "departments.columns.name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "teams", LabelKey: "departments.columns.teams", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "leads", LabelKey: "departments.columns.leads", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "sla_policy_uuid", LabelKey: "departments.columns.sla", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "ai", LabelKey: "departments.columns.ai", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "created_at", LabelKey: "departments.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, DefaultVisible: false},
		},
		Filters:     []resourcemeta.Filter{},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// MetaH serves GET /v1/tenant/departments/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

func (h *Handler) viewer(w http.ResponseWriter, r *http.Request) (usecase.Viewer, bool) {
	p := authctx.MustPrincipal(r.Context())
	v, err := h.svc.ViewerFor(r.Context(), orgctx.Must(r.Context()), p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return v, false
	}
	return v, true
}

// List serves GET /v1/tenant/departments.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q", Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if len(q.Sort) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: "departments are listed in their sort order", Code: "forbidden"}})
		return
	}
	f := model.ListFilter{Limit: q.Limit, Offset: q.Offset, Q: q.Q, Mine: values.Get("mine") == "true"}
	if raw := strings.TrimSpace(values.Get("brand")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "brand", Message: "brand must be a uuid", Code: "invalid"}})
			return
		}
		f.BrandUUID = &id
	}
	v, ok := h.viewer(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, v, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get serves GET /v1/tenant/departments/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// Create serves POST /v1/tenant/departments.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.Input
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "departments.created", d.UUID, map[string]any{"name": d.Name})
	response.JSON(w, r, http.StatusCreated, d)
}

// Patch serves PATCH /v1/tenant/departments/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Input
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.Update(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "departments.updated", d.UUID, map[string]any{"name": d.Name})
	response.JSON(w, r, http.StatusOK, d)
}

// Delete serves DELETE /v1/tenant/departments/{uuid}?move_to=none|<uuid>.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	move := deptscope.Optional{}
	switch raw := strings.TrimSpace(r.URL.Query().Get("move_to")); raw {
	case "":
	case "none":
		move.Set = true
	default:
		to, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "move_to", Message: "move_to must be none or a department uuid", Code: "invalid"}})
			return
		}
		move = deptscope.Optional{Set: true, Value: &to}
	}
	if err := h.svc.Delete(r.Context(), orgctx.Must(r.Context()).ID, id, move); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "departments.deleted", id, map[string]any{"move_to": r.URL.Query().Get("move_to")})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Shares serves GET /v1/tenant/departments/{uuid}/shares.
func (h *Handler) Shares(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.Shares(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Share serves POST /v1/tenant/departments/{uuid}/shares: a lead of the
// item's department shares it with this department.
func (h *Handler) Share(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ShareInput
	if !decode(w, r, &in) {
		return
	}
	v, ok := h.viewer(w, r)
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	sh, err := h.svc.Share(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, v, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "departments.shared", id, map[string]any{"resource_kind": sh.ResourceKind, "resource_uuid": sh.ResourceUUID.String()})
	response.JSON(w, r, http.StatusCreated, sh)
}

// Unshare serves DELETE /v1/tenant/departments/{uuid}/shares/{shareUuid}.
func (h *Handler) Unshare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	share, ok := pathUUID(w, r, "shareUuid")
	if !ok {
		return
	}
	v, ok := h.viewer(w, r)
	if !ok {
		return
	}
	if err := h.svc.Unshare(r.Context(), orgctx.Must(r.Context()).ID, v, id, share); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "departments.unshared", id, map[string]any{"share_uuid": share.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// conversation checks that the caller sees the conversation (team
// visibility of the inbox).
func (h *Handler) conversation(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return id, false
	}
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	v, err := access.For(r.Context(), h.q, scope, p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return id, false
	}
	conv, err := h.q.GetConversationDepartment(r.Context(), db.GetConversationDepartmentParams{OrganizationID: scope.ID, Uuid: id})
	if err != nil {
		writeError(w, r, model.ErrConversationNotFound)
		return id, false
	}
	if !v.All {
		visible, err := v.CanSee(r.Context(), h.q, scope.ID, conv.ID)
		if err != nil {
			writeError(w, r, err)
			return id, false
		}
		if !visible {
			writeError(w, r, model.ErrConversationNotFound)
			return id, false
		}
	}
	return id, true
}

// ConversationDepartment serves GET /v1/tenant/conversations/{uuid}/department.
func (h *Handler) ConversationDepartment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.conversation(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Conversation(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Transfer serves POST /v1/tenant/conversations/{uuid}/department-transfer:
// an agent hands the conversation to another department with its context.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	if !scope.Has(rbac.PermTenantConversationsAssign) {
		response.Forbidden(w, r, "Transferring a conversation needs the assign permission")
		return
	}
	id, ok := h.conversation(w, r)
	if !ok {
		return
	}
	var in model.TransferInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.Transfer(access.WithViewer(r.Context(), access.Viewer{UserID: p.UserInternal, All: true, TeamIDs: []int64{}}),
		scope.ID, id, usecase.Actor{Kind: model.SourceAgent, UserID: p.UserInternal}, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"reason": in.Reason}
	if in.DepartmentUUID != nil {
		payload["department_uuid"] = in.DepartmentUUID.String()
	}
	h.recordAs(r, "departments.conversation_transferred", "tenant.conversations", id, payload)
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	h.recordAs(r, action, model.Resource, id, payload)
}

func (h *Handler) recordAs(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is not a valid uuid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var inUse *model.InUseError
	if errors.As(err, &inUse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, "DEPARTMENT_IN_USE",
			"The department still owns knowledge; delete with move_to=none or move_to=<department uuid>", []response.Detail{
				{Field: "articles", Code: "count", Message: itoa(inUse.Usage.Articles)},
				{Field: "sources", Code: "count", Message: itoa(inUse.Usage.Sources)},
				{Field: "context_sections", Code: "count", Message: itoa(inUse.Usage.ContextSections)},
				{Field: "suggestions", Code: "count", Message: itoa(inUse.Usage.Suggestions)},
			})
		return
	}
	switch {
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrConversationNotFound), errors.Is(err, model.ErrResourceNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, model.ErrForbidden):
		response.Forbidden(w, r, "Only a lead of the department (or a manager) can do this")
	case errors.Is(err, model.ErrNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "name", Message: err.Error(), Code: "taken"}})
	case errors.Is(err, model.ErrInUse):
		response.Conflict(w, r, "DEPARTMENT_IN_USE", err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

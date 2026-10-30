// Package handler serves /v1/tenant/teams (CF-5).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the team routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Meta is the table contract of GET /v1/tenant/teams.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "name",
		DefaultFields: []string{"name", "leads", "member_count", "skills", "working_hours"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "description"},
		SortableFields:   []string{"name", "created_at"},
		FilterableFields: []string{"skill", "member"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "teams.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "leads", LabelKey: "teams.columns.leads", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "member_count", LabelKey: "teams.columns.member_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "skills", LabelKey: "teams.columns.skills", Type: resourcemeta.ColumnTypeString, Filterable: true, DefaultVisible: true},
			{Key: "working_hours", LabelKey: "teams.columns.working_hours", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "created_at", LabelKey: "teams.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters:     []resourcemeta.Filter{},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// MetaH serves GET /v1/tenant/teams/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// List serves GET /v1/tenant/teams.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q", Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f := model.ListFilter{Limit: q.Limit, Offset: q.Offset, Q: q.Q, Skill: strings.TrimSpace(values.Get("skill")), SortField: "name"}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	if raw := strings.TrimSpace(values.Get("member")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "member", Message: "member must be a user uuid", Code: "invalid"}})
			return
		}
		f.MemberUUID = id
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get serves GET /v1/tenant/teams/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	team, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, team)
}

// Create serves POST /v1/tenant/teams.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.TeamInput
	if !decode(w, r, &in) {
		return
	}
	team, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "teams.created", team.UUID, map[string]any{"name": team.Name, "members": len(team.Members)})
	response.JSON(w, r, http.StatusCreated, team)
}

// Patch serves PATCH /v1/tenant/teams/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.TeamInput
	if !decode(w, r, &in) {
		return
	}
	if in.Members != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "members", Message: "use the team member routes", Code: "forbidden"}})
		return
	}
	team, err := h.svc.Update(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "teams.updated", team.UUID, map[string]any{"name": team.Name})
	response.JSON(w, r, http.StatusOK, team)
}

// Delete serves DELETE /v1/tenant/teams/{uuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "teams.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// PutMember serves PUT /v1/tenant/teams/{uuid}/members/{userUuid} (add or
// change the team role: {"role": "lead" | "member"}).
func (h *Handler) PutMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	userID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	team, err := h.svc.SetMember(r.Context(), scope.ID, id, model.MemberInput{UserUUID: userID, Role: in.Role})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "teams.member_set", team.UUID, map[string]any{"user_uuid": userID.String(), "role": in.Role})
	response.JSON(w, r, http.StatusOK, team)
}

// DeleteMember serves DELETE /v1/tenant/teams/{uuid}/members/{userUuid}.
func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	userID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	team, err := h.svc.RemoveMember(r.Context(), scope.ID, id, userID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "teams.member_removed", team.UUID, map[string]any{"user_uuid": userID.String()})
	response.JSON(w, r, http.StatusOK, team)
}

// PutMemberSkills serves PUT /v1/tenant/members/{userUuid}/skills.
func (h *Handler) PutMemberSkills(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	userID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	var in struct {
		Skills []string `json:"skills"`
	}
	if !decode(w, r, &in) {
		return
	}
	skills, err := h.svc.SetMemberSkills(r.Context(), scope.ID, userID, in.Skills)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "members.skills_updated", userID, map[string]any{"skills": skills})
	response.JSON(w, r, http.StatusOK, map[string]any{"skills": skills})
}

// Skills serves GET /v1/tenant/skills (every tag in use).
func (h *Handler) Skills(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	items, err := h.svc.Skills(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	resource := model.Resource
	if strings.HasPrefix(action, "members.") {
		resource = "tenant.members"
	}
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
	var qe *apiquery.ValidationError
	if errors.As(err, &qe) {
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, usecase.ErrMemberNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

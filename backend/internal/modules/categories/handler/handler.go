// Package handler serves /v1/tenant/categories and
// /v1/tenant/category-assignments (CF-218).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/classify"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the category routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Meta is the table contract of GET /v1/tenant/categories.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "position",
		DefaultFields: []string{"name", "slug", "department_uuid", "examples"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Search: false, Filter: true, Sort: false,
		},
		SearchableFields: []string{},
		SortableFields:   []string{},
		FilterableFields: []string{"department", "include_archived"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "categories.columns.name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "slug", LabelKey: "categories.columns.slug", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "department_uuid", LabelKey: "categories.columns.department", Type: resourcemeta.ColumnTypeString, Filterable: true, DefaultVisible: true},
			{Key: "examples", LabelKey: "categories.columns.examples", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "updated_at", LabelKey: "categories.columns.updated_at", Type: resourcemeta.ColumnTypeDatetime, DefaultVisible: false},
		},
		Filters:     []resourcemeta.Filter{},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// MetaH serves GET /v1/tenant/categories/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// List serves GET /v1/tenant/categories (the whole tree, parents first).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	dept := uuid.Nil
	if raw := strings.TrimSpace(values.Get("department")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "department", Message: "department must be a uuid", Code: "invalid"}})
			return
		}
		dept = id
	}
	archived := values.Get("include_archived") == "true"
	items, err := h.svc.List(r.Context(), scope.ID, dept, archived)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get serves GET /v1/tenant/categories/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Create serves POST /v1/tenant/categories.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.CategoryInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "categories.created", c.UUID, map[string]any{"name": c.Name, "slug": c.Slug})
	response.JSON(w, r, http.StatusCreated, c)
}

// Patch serves PATCH /v1/tenant/categories/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CategoryInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.Update(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "categories.updated", c.UUID, map[string]any{"name": c.Name, "archived": c.Archived})
	response.JSON(w, r, http.StatusOK, c)
}

// Delete serves DELETE /v1/tenant/categories/{uuid}.
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
	h.record(r, "categories.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Review serves GET /v1/tenant/categories/review (records to confirm).
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset", Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	items, total, err := h.svc.ReviewQueue(r.Context(), scope.ID, model.ReviewFilter{
		SubjectType: strings.TrimSpace(values.Get("subject_type")), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ExportLabels serves GET /v1/tenant/categories/labels/export: the label
// store as JSON Lines (one LabelExport per line), the CF-219 training set.
func (h *Handler) ExportLabels(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "since", Message: "since must be an RFC 3339 time", Code: "invalid"}})
			return
		}
		since = t
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="category-labels-`+strconv.FormatInt(time.Now().Unix(), 10)+`.jsonl"`)
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	count := 0
	err := h.svc.ExportLabels(r.Context(), scope.ID, since, func(l model.LabelExport) error {
		count++
		return enc.Encode(l)
	})
	if err != nil {
		if count == 0 {
			w.Header().Del("Content-Disposition")
			writeError(w, r, err)
		}
		return
	}
	h.record(r, "categories.labels_exported", scope.UUID, map[string]any{"count": count})
}

func subject(w http.ResponseWriter, r *http.Request) (string, uuid.UUID, bool) {
	kind := r.PathValue("subjectType")
	if !classify.ValidSubjectType(kind) {
		response.NotFound(w, r, usecase.ErrUnknownSubject.Error())
		return "", uuid.Nil, false
	}
	id, ok := pathUUID(w, r, "subjectUuid")
	return kind, id, ok
}

// GetAssignment serves GET /v1/tenant/category-assignments/{subjectType}/{subjectUuid}.
func (h *Handler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	kind, id, ok := subject(w, r)
	if !ok {
		return
	}
	a, err := h.svc.Assignment(r.Context(), scope.ID, actor(r), kind, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"assignment": a})
}

// Classify serves POST /v1/tenant/category-assignments/{subjectType}/{subjectUuid}/classify.
func (h *Handler) Classify(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	kind, id, ok := subject(w, r)
	if !ok {
		return
	}
	a, err := h.svc.ClassifyRecord(r.Context(), scope.ID, actor(r), kind, id, r.URL.Query().Get("force") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"assignment": a})
}

// PutAssignment serves PUT /v1/tenant/category-assignments/{subjectType}/{subjectUuid}:
// an agent confirms or corrects the category (stored as a label).
func (h *Handler) PutAssignment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	kind, id, ok := subject(w, r)
	if !ok {
		return
	}
	var in model.LabelInput
	if !decode(w, r, &in) {
		return
	}
	if in.CategoryUUID == uuid.Nil {
		response.ValidationError(w, r, []response.Detail{{Field: "category_uuid", Message: "category_uuid is required", Code: "required"}})
		return
	}
	a, err := h.svc.Label(r.Context(), scope.ID, actor(r), kind, id, in.CategoryUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "categories.labeled", id, map[string]any{"subject_type": kind, "category_uuid": in.CategoryUUID.String()})
	response.JSON(w, r, http.StatusOK, map[string]any{"assignment": a})
}

// DeleteAssignment serves DELETE /v1/tenant/category-assignments/{subjectType}/{subjectUuid}.
func (h *Handler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	kind, id, ok := subject(w, r)
	if !ok {
		return
	}
	if err := h.svc.Clear(r.Context(), scope.ID, actor(r), kind, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func actor(r *http.Request) int64 {
	return authctx.MustPrincipal(r.Context()).UserInternal
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
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
	switch {
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, usecase.ErrUnknownSubject):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrSlugTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "slug", Message: err.Error(), Code: "taken"}})
	case errors.Is(err, usecase.ErrInUse), errors.Is(err, usecase.ErrNoCategories), errors.Is(err, usecase.ErrNoText):
		response.Error(w, r, http.StatusConflict, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrDepartmentsUnavailable):
		response.ValidationError(w, r, []response.Detail{{Field: "department", Message: err.Error(), Code: "unavailable"}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

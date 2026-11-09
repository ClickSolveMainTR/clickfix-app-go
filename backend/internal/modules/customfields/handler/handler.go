// Package handler exposes custom fields and custom objects over HTTP (CF-7).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to custom fields.
const (
	CodeKeyTaken     = "CUSTOM_FIELD_KEY_TAKEN"
	CodeLimitReached = "CUSTOM_FIELD_LIMIT_REACHED"
	maxBody          = 256 << 10
)

// RecordSort lists the built-in sortable record columns; `cf.<key>` is
// accepted on top of these.
var RecordSort = apiquery.SortColumns{"display_name": "display_name", "created_at": "created_at", "updated_at": "updated_at"}

// Handler serves the custom field endpoints.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Meta lists entity and field types.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, h.svc.Meta())
}

// ListDefinitions lists the fields of ?entity_type= (and ?object_type_uuid=).
func (h *Handler) ListDefinitions(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	ds, ok := definitionScope(w, r)
	if !ok {
		return
	}
	includeArchived, _ := strconv.ParseBool(r.URL.Query().Get("include_archived"))
	items, err := h.svc.ListDefinitions(r.Context(), scope.ID, ds, includeArchived)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateDefinition adds a field.
func (h *Handler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.CreateDefinitionInput
	if !decode(w, r, &in) {
		return
	}
	def, err := h.svc.CreateDefinition(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// The field is live now: close its unknown-key warning and apply the
	// values contacts already sent under that key.
	adopted, err := h.svc.Adopt(r.Context(), scope.ID, def, in.FromUnknownKey)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_fields.created", "custom_fields", def.UUID, map[string]any{
		"key": def.Key, "entity_type": def.EntityType, "type": def.Type, "visibility": def.Visibility,
		"sensitive": def.Sensitive, "widget_access": def.WidgetAccess, "adopted_values": adopted,
	})
	response.JSON(w, r, http.StatusCreated, def)
}

// GetDefinition returns one field.
func (h *Handler) GetDefinition(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	def, err := h.svc.GetDefinition(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, def)
}

// UpdateDefinition patches a field.
func (h *Handler) UpdateDefinition(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.UpdateDefinitionInput
	if !decode(w, r, &in) {
		return
	}
	def, changes, err := h.svc.UpdateDefinitionAudited(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_fields.updated", "custom_fields", def.UUID, changes)
	response.JSON(w, r, http.StatusOK, def)
}

// ArchiveDefinition hides a field (DELETE); values stay stored.
func (h *Handler) ArchiveDefinition(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, true)
}

// RestoreDefinition brings an archived field back.
func (h *Handler) RestoreDefinition(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, false)
}

func (h *Handler) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	def, err := h.svc.SetArchived(r.Context(), scope.ID, id, archived)
	if err != nil {
		writeError(w, r, err)
		return
	}
	action := "custom_fields.restored"
	if archived {
		action = "custom_fields.archived"
	}
	h.record(r, action, "custom_fields", def.UUID, map[string]any{"key": def.Key, "entity_type": def.EntityType})
	response.JSON(w, r, http.StatusOK, def)
}

// Reorder sets the display order of one entity's fields.
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.ReorderInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.Reorder(r.Context(), scope.ID, in); err != nil {
		writeError(w, r, err)
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, "custom_fields.reordered", "custom_fields", nil, map[string]any{"entity_type": in.EntityType}, r)
	items, err := h.svc.ListDefinitions(r.Context(), scope.ID, model.DefinitionScope{EntityType: in.EntityType, ObjectTypeUUID: in.ObjectTypeUUID}, true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ListObjectTypes lists custom object types.
func (h *Handler) ListObjectTypes(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	includeArchived, _ := strconv.ParseBool(r.URL.Query().Get("include_archived"))
	items, err := h.svc.ListObjectTypes(r.Context(), scope.ID, includeArchived)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateObjectType adds a custom object type.
func (h *Handler) CreateObjectType(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.ObjectTypeInput
	if !decode(w, r, &in) {
		return
	}
	ot, err := h.svc.CreateObjectType(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_objects.created", "custom_objects", ot.UUID, map[string]any{"key": ot.Key})
	response.JSON(w, r, http.StatusCreated, ot)
}

// GetObjectType returns one object type.
func (h *Handler) GetObjectType(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	ot, err := h.svc.GetObjectType(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, ot)
}

// UpdateObjectType changes labels or archives an object type.
func (h *Handler) UpdateObjectType(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ObjectTypeInput
	if !decode(w, r, &in) {
		return
	}
	ot, err := h.svc.UpdateObjectType(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_objects.updated", "custom_objects", ot.UUID, map[string]any{"key": ot.Key, "archived": ot.Archived})
	response.JSON(w, r, http.StatusOK, ot)
}

// ListRecords lists records with q, sort (incl. cf.<key>) and cf.* filters.
func (h *Handler) ListRecords(w http.ResponseWriter, r *http.Request) {
	rs, ok := h.recordScope(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	list := model.RecordList{Q: q.Q, Sort: "created_at", SortDesc: true, Limit: q.Limit, Offset: q.Offset}
	if len(q.Sort) > 0 {
		field := q.Sort[0].Field
		_, builtin := RecordSort[field]
		_, isCF := customfields.SortExpr("x", field, rs.Defs)
		if !builtin && !isCF {
			response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: "unknown sort field " + field, Code: "unknown"}})
			return
		}
		list.Sort, list.SortDesc = field, q.Sort[0].Desc
	}
	filters, err := h.svc.ParseFilters(values, rs.Defs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	list.Filters = filters
	items, total, err := h.svc.ListRecords(r.Context(), rs, list)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateRecord adds a record.
func (h *Handler) CreateRecord(w http.ResponseWriter, r *http.Request) {
	rs, ok := h.recordScope(w, r)
	if !ok {
		return
	}
	var in model.RecordInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	rec, err := h.svc.CreateRecord(r.Context(), rs, actor, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_objects.record_created", "custom_objects", rec.UUID, map[string]any{"object_type": rs.Type.Key, "display_name": rec.DisplayName})
	response.JSON(w, r, http.StatusCreated, rec)
}

// GetRecord returns one record.
func (h *Handler) GetRecord(w http.ResponseWriter, r *http.Request) {
	rs, ok := h.recordScope(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "record_uuid")
	if !ok {
		return
	}
	rec, err := h.svc.GetRecord(r.Context(), rs, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rec)
}

// UpdateRecord patches a record.
func (h *Handler) UpdateRecord(w http.ResponseWriter, r *http.Request) {
	rs, ok := h.recordScope(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "record_uuid")
	if !ok {
		return
	}
	var in model.RecordInput
	if !decode(w, r, &in) {
		return
	}
	rec, err := h.svc.UpdateRecord(r.Context(), rs, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_objects.record_updated", "custom_objects", rec.UUID, map[string]any{"object_type": rs.Type.Key})
	response.JSON(w, r, http.StatusOK, rec)
}

// DeleteRecord removes a record.
func (h *Handler) DeleteRecord(w http.ResponseWriter, r *http.Request) {
	rs, ok := h.recordScope(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "record_uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteRecord(r.Context(), rs, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_objects.record_deleted", "custom_objects", id, map[string]any{"object_type": rs.Type.Key})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ListUnknownKeys lists the open "tanımsız alan geldi" warnings
// (?channel_uuid= narrows them to one widget).
func (h *Handler) ListUnknownKeys(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var channel *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("channel_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "channel_uuid must be a UUID")
			return
		}
		channel = &id
	}
	items, err := h.svc.ListUnknownKeys(r.Context(), scope.ID, channel)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// DismissUnknownKey hides one warning until the key arrives again.
func (h *Handler) DismissUnknownKey(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DismissUnknownKey(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "custom_fields.unknown_dismissed", "custom_fields", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"dismissed": true})
}

// Values returns the agent view of one contact or company
// (?entity_type=contact|company&entity_uuid=).
func (h *Handler) Values(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	entity := r.URL.Query().Get("entity_type")
	id, err := uuid.Parse(r.URL.Query().Get("entity_uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "entity_uuid must be a UUID")
		return
	}
	out, err := h.svc.EntityValues(r.Context(), scope.ID, entity, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) recordScope(w http.ResponseWriter, r *http.Request) (usecase.RecordScope, bool) {
	scope, typeID, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return usecase.RecordScope{}, false
	}
	rs, err := h.svc.ResolveRecordScope(r.Context(), scope.ID, typeID)
	if err != nil {
		writeError(w, r, err)
		return usecase.RecordScope{}, false
	}
	return rs, true
}

func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, action, resource, &id, payload, r)
}

func definitionScope(w http.ResponseWriter, r *http.Request) (model.DefinitionScope, bool) {
	ds := model.DefinitionScope{EntityType: strings.TrimSpace(r.URL.Query().Get("entity_type"))}
	if raw := strings.TrimSpace(r.URL.Query().Get("object_type_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "object_type_uuid is invalid")
			return ds, false
		}
		ds.ObjectTypeUUID = &id
	}
	return ds, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func scopeOf(w http.ResponseWriter, r *http.Request) (orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("customfields: missing organization scope"), "")
		return orgctx.Scope{}, false
	}
	return scope, true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func scopeAndID(w http.ResponseWriter, r *http.Request, name string) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return orgctx.Scope{}, uuid.Nil, false
	}
	id, ok := pathUUID(w, r, name)
	return scope, id, ok
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *customfields.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Errors))
		for _, e := range ve.Errors {
			details = append(details, response.Detail{Field: e.Field, Message: e.Message, Code: e.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "))
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, CodeKeyTaken, strings.TrimPrefix(err.Error(), usecase.ErrConflict.Error()+": "))
	case errors.Is(err, usecase.ErrLimit):
		response.Conflict(w, r, CodeLimitReached, strings.TrimPrefix(err.Error(), usecase.ErrLimit.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

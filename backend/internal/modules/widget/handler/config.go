package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the editor API.
const (
	CodeSettingsInvalid = "WIDGET_SETTINGS_INVALID"
	CodeVersionConflict = "WIDGET_VERSION_CONFLICT"
	maxDraftBodyBytes   = 300 << 10
)

// ConfigHandler serves the widget editor routes (CF-150) under
// /v1/tenant/widgets/{uuid}/...
type ConfigHandler struct {
	svc      *usecase.Configs
	activity *activity.Recorder
}

// NewConfig builds the editor handler; rec may be nil.
func NewConfig(svc *usecase.Configs, rec *activity.Recorder) *ConfigHandler {
	return &ConfigHandler{svc: svc, activity: rec}
}

func (h *ConfigHandler) scope(w http.ResponseWriter, r *http.Request) (orgctx.Scope, int64, uuid.UUID, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "")
		return scope, 0, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Widget not found")
		return scope, 0, uuid.Nil, false
	}
	var actor int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		actor = p.UserInternal
	}
	return scope, actor, id, true
}

func (h *ConfigHandler) record(r *http.Request, actor int64, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	h.activity.Record(r.Context(), &actor, action, "channel", &id, payload, r)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDraftBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func (h *ConfigHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if entitlement.WriteError(w, r, err) {
		return
	}
	var se *usecase.SettingsError
	switch {
	case errors.As(err, &se):
		details := make([]response.Detail, 0, len(se.Issues))
		for _, i := range se.Issues {
			details = append(details, response.Detail{Field: i.Path, Code: i.Code, Message: i.Message})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeSettingsInvalid, "Fix the highlighted settings before publishing", details)
	case errors.Is(err, usecase.ErrVersionConflict):
		response.Conflict(w, r, CodeVersionConflict, "Someone published a newer version; reload to continue")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Widget not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, channelsusecase.ErrInvalid):
		// Validation of a duplicate comes from the channels service.
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Widget request failed")
	}
}

// Config is GET /v1/tenant/widgets/{uuid}/config.
func (h *ConfigHandler) Config(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Config(r.Context(), scope.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SaveDraft is PUT /v1/tenant/widgets/{uuid}/draft.
func (h *ConfigHandler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var body struct {
		Settings    map[string]any `json:"settings"`
		BaseVersion int            `json:"base_version"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	out, err := h.svc.SaveDraft(r.Context(), scope.ID, actor, id, body.Settings, body.BaseVersion)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DiscardDraft is DELETE /v1/tenant/widgets/{uuid}/draft.
func (h *ConfigHandler) DiscardDraft(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	if err := h.svc.DiscardDraft(r.Context(), scope.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"discarded": true})
}

// Publish is POST /v1/tenant/widgets/{uuid}/publish.
func (h *ConfigHandler) Publish(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var in usecase.PublishInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Publish(r.Context(), scope.ID, actor, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, actor, "widgets.published", id, map[string]any{"organization_uuid": scope.UUID, "version": out.Version})
	response.JSON(w, r, http.StatusOK, out)
}

// Versions is GET /v1/tenant/widgets/{uuid}/versions.
func (h *ConfigHandler) Versions(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Versions(r.Context(), scope.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func versionParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || n < 1 {
		response.NotFound(w, r, "Version not found")
		return 0, false
	}
	return n, true
}

// Version is GET /v1/tenant/widgets/{uuid}/versions/{version}.
func (h *ConfigHandler) Version(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	n, ok := versionParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Version(r.Context(), scope.ID, id, n)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Rollback is POST /v1/tenant/widgets/{uuid}/versions/{version}/rollback.
func (h *ConfigHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	n, ok := versionParam(w, r)
	if !ok {
		return
	}
	var body struct {
		BaseVersion int `json:"base_version"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	out, err := h.svc.Rollback(r.Context(), scope.ID, actor, id, n, body.BaseVersion)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, actor, "widgets.rolled_back", id, map[string]any{"organization_uuid": scope.UUID, "version": out.Version, "rollback_of": n})
	response.JSON(w, r, http.StatusOK, out)
}

// Duplicate is POST /v1/tenant/widgets/{uuid}/duplicate.
func (h *ConfigHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	out, err := h.svc.Duplicate(r.Context(), scope.ID, actor, id, body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, actor, "widgets.duplicated", out.UUID, map[string]any{"organization_uuid": scope.UUID, "source_uuid": id})
	response.JSON(w, r, http.StatusCreated, out)
}

// Installs is GET /v1/tenant/widgets/{uuid}/installs.
func (h *ConfigHandler) Installs(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Installs(r.Context(), scope.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rejection, err := h.svc.IdentityRejection(r.Context(), scope.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "identity_rejection": rejection})
}

// AppKeys is GET /v1/tenant/widgets/{uuid}/app-keys (CF-201).
func (h *ConfigHandler) AppKeys(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListAppKeys(r.Context(), scope.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateAppKey is POST /v1/tenant/widgets/{uuid}/app-keys: the response
// carries the key exactly once.
func (h *ConfigHandler) CreateAppKey(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var in usecase.CreateAppKeyInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.svc.CreateAppKey(r.Context(), scope.ID, actor, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, actor, "widgets.app_key_created", id, map[string]any{
		"organization_uuid": scope.UUID, "app_key_uuid": out.UUID, "platform": out.Platform, "package_name": out.PackageName,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// RevokeAppKey is DELETE /v1/tenant/widgets/{uuid}/app-keys/{key}.
func (h *ConfigHandler) RevokeAppKey(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	key, err := uuid.Parse(r.PathValue("key"))
	if err != nil {
		response.NotFound(w, r, "App key not found")
		return
	}
	out, err := h.svc.RevokeAppKey(r.Context(), scope.ID, id, key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.record(r, actor, "widgets.app_key_revoked", id, map[string]any{
		"organization_uuid": scope.UUID, "app_key_uuid": out.UUID, "platform": out.Platform, "package_name": out.PackageName,
	})
	response.JSON(w, r, http.StatusOK, out)
}

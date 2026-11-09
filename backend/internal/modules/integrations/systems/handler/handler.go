// Package handler is the HTTP surface of the connected system (CF-197).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

const maxBody = 64 << 10

// Error codes of this surface.
const (
	CodeNotConnected  = "SYSTEM_NOT_CONNECTED"
	CodeNotAllowed    = "ACTION_NOT_ALLOWED"
	CodeNeedsApproval = "APPROVAL_REQUIRED"
	CodeDisabled      = "ACTION_DISABLED"
)

// Limit gates the endpoints that call out ("Dene", lookup, run, preview).
type Limit func(r *http.Request, org int64) bool

type Handler struct {
	Svc      *usecase.Service
	Activity *activity.Recorder
	Limit    Limit
}

func New(svc *usecase.Service, rec *activity.Recorder, limit Limit) *Handler {
	return &Handler{Svc: svc, Activity: rec, Limit: limit}
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	out, err := h.Svc.Overview(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SaveConnection(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.ConnectionInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	out, err := h.Svc.SaveConnection(r.Context(), scope.ID, actor.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, out.UUID, map[string]any{"connected_system": "connection", "auth_type": out.AuthType, "secret_changed": in.Secret != ""})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SaveSource(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SourceInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Svc.SaveSource(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, out.UUID, map[string]any{"connected_system": "source", "object_type_uuid": id})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.Svc.DeleteSource(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, id, map[string]any{"connected_system": "source_deleted"})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) TestSource(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r, "uuid")
	if !ok || !h.allowed(w, r, scope.ID) {
		return
	}
	var in model.TestInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	out, err := h.Svc.TestSource(r.Context(), scope.ID, actor.UserInternal, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SaveAction(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.ActionInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Svc.SaveAction(r.Context(), scope.ID, r.PathValue("key"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, out.UUID, map[string]any{"connected_system": "action", "key": out.Key, "allowed_for": out.AllowedFor, "requires_approval": out.RequiresApproval})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	if err := h.Svc.DeleteAction(r.Context(), scope.ID, key); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope.UUID, map[string]any{"connected_system": "action_deleted", "key": key})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) RunAction(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok || !h.allowed(w, r, scope.ID) {
		return
	}
	var in model.RunInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	key := r.PathValue("key")
	channel := model.ChannelFrom(r.Context())
	out, err := h.Svc.Run(r.Context(), scope.ID, actor.UserInternal, key, channel, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if !out.DryRun {
		h.record(r, scope.UUID, map[string]any{"connected_system": "action_run", "key": key, "channel": channel, "ok": out.OK, "status_code": out.StatusCode})
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SavePlaybook(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in model.PlaybookInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Svc.SavePlaybook(r.Context(), scope.ID, r.PathValue("intent"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, out.UUID, map[string]any{"connected_system": "playbook", "intent": out.Intent})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeletePlaybook(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	intent := r.PathValue("intent")
	if err := h.Svc.DeletePlaybook(r.Context(), scope.ID, intent); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope.UUID, map[string]any{"connected_system": "playbook_deleted", "intent": intent})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

// Lookup reads one concept for a customer (?contact_uuid= or ?external_id=).
func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok || !h.allowed(w, r, scope.ID) {
		return
	}
	var contact *uuid.UUID
	if raw := r.URL.Query().Get("contact_uuid"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "contact_uuid is invalid")
			return
		}
		contact = &id
	}
	ext := strings.TrimSpace(r.URL.Query().Get("external_id"))
	if contact == nil && ext == "" {
		response.BadRequest(w, r, response.CodeValidationError, "contact_uuid or external_id is required")
		return
	}
	id, err := h.Svc.ResolveIdentity(r.Context(), scope.ID, contact, ext)
	if err != nil {
		writeError(w, r, err)
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	out, err := h.Svc.Lookup(r.Context(), scope.ID, actor.UserInternal, r.PathValue("key"), id, model.ChannelFrom(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Preview is "bu müşteri olarak dene".
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok || !h.allowed(w, r, scope.ID) {
		return
	}
	var in model.PreviewInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Svc.Preview(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) allowed(w http.ResponseWriter, r *http.Request, org int64) bool {
	if h.Limit != nil && !h.Limit(r, org) {
		response.TooManyRequests(w, r, "Too many connected system requests; try again in a minute")
		return false
	}
	return true
}

func (h *Handler) record(r *http.Request, id uuid.UUID, payload map[string]any) {
	if h.Activity == nil {
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	h.Activity.Record(r.Context(), &actor.UserInternal, "updated", "settings", &id, payload, r)
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
		response.InternalErr(w, r, errors.New("systems: missing organization scope"), "")
		return orgctx.Scope{}, false
	}
	return scope, true
}

func scopeAndID(w http.ResponseWriter, r *http.Request, name string) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return orgctx.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return orgctx.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, model.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, model.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, strings.TrimPrefix(err.Error(), model.ErrInvalid.Error()+": "))
	case errors.Is(err, model.ErrNotConnected):
		response.Conflict(w, r, CodeNotConnected, "Connect your system first (base URL and authentication)")
	case errors.Is(err, model.ErrNotAllowed):
		response.Forbidden(w, r, "This action is not allowed for this caller")
	case errors.Is(err, model.ErrNeedsApproval):
		response.Conflict(w, r, CodeNeedsApproval, "This action needs a member's approval")
	case errors.Is(err, model.ErrDisabled):
		response.Conflict(w, r, CodeDisabled, "This action is turned off")
	default:
		response.InternalErr(w, r, err, "")
	}
}

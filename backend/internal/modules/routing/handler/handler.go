// Package handler exposes routing (/v1/tenant/routing/*) and agent
// capacity (/v1/tenant/capacity*).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	maxBodyBytes     = 64 << 10
	activityResource = "routing"
)

// Handler serves routing and capacity.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Settings returns the routing settings.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Settings(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SaveSettings replaces the routing settings.
func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.Settings
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SaveSettings(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "routing.settings_updated", scope.UUID, map[string]any{"enabled": out.Enabled, "strategy": out.Strategy})
	response.JSON(w, r, http.StatusOK, out)
}

// Rules lists routing rules.
func (h *Handler) Rules(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Rules(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateRule adds a rule.
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.RuleInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateRule(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "routing.rule_created", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateRule replaces a rule.
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in model.RuleInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateRule(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "routing.rule_updated", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteRule removes a rule.
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "routing.rule_deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// Events lists the routing audit trail (?conversation=uuid).
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var conv *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("conversation")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "conversation", Message: "must be a uuid"}})
			return
		}
		conv = &id
	}
	items, err := h.svc.Events(r.Context(), scope.ID, conv, 100)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Capacity is the supervisor view.
func (h *Handler) Capacity(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Capacity(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Limits returns capacity limits.
func (h *Handler) Limits(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CapacityLimits(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SetLimits replaces the limits of one agent or the defaults.
func (h *Handler) SetLimits(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.CapacityInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SetCapacityLimits(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	target := scope.UUID
	if in.UserUUID != nil {
		target = *in.UserUUID
	}
	h.record(r, scope, "routing.capacity_updated", target, map[string]any{"limits": in.Limits})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	payload["organization_uuid"] = scope.UUID
	h.activity.Record(r.Context(), &p.UserInternal, action, activityResource, &id, payload, r)
}

func org(w http.ResponseWriter, r *http.Request) (orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("routing: missing organization scope"), "")
		return orgctx.Scope{}, false
	}
	return scope, true
}

func orgAndID(w http.ResponseWriter, r *http.Request) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := org(w, r)
	if !ok {
		return scope, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return scope, uuid.Nil, false
	}
	return scope, id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": ")}})
	default:
		response.InternalErr(w, r, err, "")
	}
}

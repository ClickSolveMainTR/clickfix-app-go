// Package handler exposes presence and typing over HTTP (CF-10).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/presence and /v1/tenant/realtime/typing.
type Handler struct {
	svc *usecase.Service
	q   *db.Queries
}

// New builds the handler.
func New(svc *usecase.Service, q *db.Queries) *Handler {
	return &Handler{svc: svc, q: q}
}

func actor(w http.ResponseWriter, r *http.Request) (usecase.Actor, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("presence: missing organization scope"), "")
		return usecase.Actor{}, false
	}
	p := authctx.MustPrincipal(r.Context())
	return usecase.Actor{OrganizationID: scope.ID, OrganizationUUID: scope.UUID, UserID: p.UserInternal, UserUUID: p.UserID}, true
}

// List returns the members of the organization with their presence.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), a.OrganizationID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetStatus stores the caller's status.
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	me, err := h.svc.SetStatus(r.Context(), a, in.Status)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

// Idle reports the caller inactive or back (automatic away, CF-54).
func (h *Handler) Idle(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	var in struct {
		Idle bool `json:"idle"`
	}
	if !decode(w, r, &in) {
		return
	}
	me, err := h.svc.Idle(r.Context(), a, in.Idle)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

// Heartbeat keeps the caller present.
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	me, err := h.svc.Heartbeat(r.Context(), a)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

// Typing publishes a typing indicator to a conversation channel.
func (h *Handler) Typing(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	var in struct {
		ConversationUUID uuid.UUID `json:"conversation_uuid"`
		State            string    `json:"state"`
	}
	if !decode(w, r, &in) {
		return
	}
	name := ""
	if u, err := h.q.GetUserByID(r.Context(), a.UserID); err == nil {
		name = u.Name
	}
	v, err := access.For(r.Context(), h.q, orgctx.Must(r.Context()), a.UserID)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	if err := h.svc.Typing(access.WithViewer(r.Context(), v), a, name, in.ConversationUUID, in.State); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"sent": true})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Conversation was not found")
	case errors.Is(err, usecase.ErrRateLimited):
		response.TooManyRequests(w, r, "Typing events are rate limited")
	default:
		response.InternalErr(w, r, err, "")
	}
}

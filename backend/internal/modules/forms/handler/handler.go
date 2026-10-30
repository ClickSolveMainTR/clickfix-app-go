// Package handler exposes dynamic request forms (/v1/tenant/request-forms)
// and the form values of a conversation.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	maxBodyBytes     = 64 << 10
	activityResource = "request_forms"
)

// Handler serves request forms.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// List lists forms.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), scope.ID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get returns one form.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Create adds a form.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.FormInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.record(r, scope, "request_forms.created", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusCreated, out)
}

// Update replaces a form.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in model.FormInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Update(r.Context(), scope.ID, id, in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.record(r, scope, "request_forms.updated", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusOK, out)
}

// Delete removes a form.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), scope.ID, id); err != nil {
		WriteError(w, r, err)
		return
	}
	h.record(r, scope, "request_forms.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ConversationForm returns a conversation's form and values.
func (h *Handler) ConversationForm(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ConversationForm(r.Context(), scope.ID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateConversationForm changes a conversation's form values.
func (h *Handler) UpdateConversationForm(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in struct {
		Values map[string]any `json:"values"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateConversationValues(r.Context(), scope.ID, id, in.Values)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	keys := make([]string, 0, len(in.Values))
	for k := range in.Values {
		keys = append(keys, k)
	}
	h.record(r, scope, "request_forms.values_updated", id, map[string]any{"fields": keys})
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
		response.InternalErr(w, r, errors.New("forms: missing organization scope"), "")
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

// WriteError maps service errors; field validation failures become 422
// details keyed by form key (the widget uses the same mapping).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *customfields.ValidationError
	switch {
	case errors.As(err, &ve):
		details := make([]response.Detail, 0, len(ve.Errors))
		for _, e := range ve.Errors {
			details = append(details, response.Detail{Field: e.Field, Message: e.Message, Code: e.Code})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": ")}})
	default:
		response.InternalErr(w, r, err, "")
	}
}

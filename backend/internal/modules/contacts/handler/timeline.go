package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// GetCompanyOverview returns the account screen of a company (CF-85).
func (h *Handler) GetCompanyOverview(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Overview(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetCompanyTimeline returns one page of the unified company timeline:
// ?kinds=note,event&before=<next_before cursor>&limit=30.
func (h *Handler) GetCompanyTimeline(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	q := usecase.TimelineQuery{Limit: atoi(v.Get("limit"))}
	for _, k := range strings.Split(v.Get("kinds"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			q.Kinds = append(q.Kinds, k)
		}
	}
	q.Before = strings.TrimSpace(v.Get("before"))
	page, err := h.svc.Timeline(r.Context(), scope.ID, id, q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// CreateCompanyNote adds an account note to a company.
func (h *Handler) CreateCompanyNote(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	companyUUID, note, err := h.svc.AddNote(r.Context(), scope.ID, id, actor, req.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.note_added", "companies", companyUUID, map[string]any{"note": note.UUID})
	response.JSON(w, r, http.StatusCreated, note)
}

// DeleteCompanyNote removes an account note: its author, or anyone holding
// the contacts delete permission.
func (h *Handler) DeleteCompanyNote(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	err := h.svc.DeleteNote(r.Context(), scope.ID, id, actor, scope.Has(rbac.PermTenantContactsDelete))
	if errors.Is(err, usecase.ErrForbidden) {
		response.Forbidden(w, r, "Only the author can delete this note")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.note_deleted", "company_notes", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

type companyEventRequest struct {
	CompanyUUID    *uuid.UUID     `json:"company_uuid"`
	ExternalID     string         `json:"external_id"`
	Domain         string         `json:"domain"`
	ContactEmail   string         `json:"contact_email"`
	Name           string         `json:"name"`
	Properties     map[string]any `json:"properties"`
	Source         string         `json:"source"`
	IdempotencyKey string         `json:"idempotency_key"`
	OccurredAt     string         `json:"occurred_at"`
}

// RecordCompanyEvent is the integration endpoint for in-product events
// (signed up, invited a teammate, hit a plan limit, ...). Reached directly
// or through the public API (/v1/public-api/company-events, key scope
// events:write).
func (h *Handler) RecordCompanyEvent(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req companyEventRequest
	if !decode(w, r, &req) {
		return
	}
	in := usecase.CompanyEventInput{
		Company:      usecase.CompanyKey{UUID: req.CompanyUUID, ExternalID: req.ExternalID, Domain: req.Domain},
		ContactEmail: req.ContactEmail, Name: req.Name, Properties: req.Properties, Source: req.Source,
		IdempotencyKey: req.IdempotencyKey,
	}
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); in.IdempotencyKey == "" && key != "" {
		in.IdempotencyKey = key
	}
	if raw := strings.TrimSpace(req.OccurredAt); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			invalid(w, r, "occurred_at must be an RFC 3339 time")
			return
		}
		in.OccurredAt = &t
	}
	ev, err := h.svc.RecordEvent(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if ev.Duplicate {
		status = http.StatusOK
	}
	response.JSON(w, r, status, ev)
}

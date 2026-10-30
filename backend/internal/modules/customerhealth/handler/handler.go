// Package handler exposes customer health scores (CF-87).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves health settings, scores and usage reports.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// GetSettings returns the organization's health score settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.GetSettings(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings edits the settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in usecase.SettingsInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "customer_health.settings_updated", "customer_health", nil, map[string]any{
		"enabled": out.Enabled, "threshold": out.Threshold,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// GetHealth returns one company's health.
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetHealth(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Recompute scores one company now.
func (h *Handler) Recompute(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Recompute(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type usageRequest struct {
	CompanyUUID *uuid.UUID `json:"company_uuid"`
	ExternalID  string     `json:"external_id"`
	Domain      string     `json:"domain"`
	Score       *int       `json:"score"`
}

// ReportUsage stores the product usage signal of a company. Reached
// directly or through the public API (/v1/public-api/company-usage).
func (h *Handler) ReportUsage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var req usageRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Score == nil {
		invalid(w, r, "score is required")
		return
	}
	id, err := h.svc.ReportUsage(r.Context(), scope.ID, usecase.UsageKey{UUID: req.CompanyUUID, ExternalID: req.ExternalID, Domain: req.Domain}, *req.Score)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"company_uuid": id, "usage_score": *req.Score})
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, action, resource string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	payload["organization_uuid"] = scope.UUID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, action, resource, id, payload, r)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func invalid(w http.ResponseWriter, r *http.Request, msg string) {
	response.ValidationError(w, r, []response.Detail{{Message: msg}})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrInvalid):
		invalid(w, r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

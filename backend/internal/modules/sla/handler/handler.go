// Package handler exposes the SLA module (/v1/tenant/sla/*) and the SLA
// clocks of a conversation.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	maxBodyBytes     = 64 << 10
	activityResource = "sla"
	// CodeConflict: a team already has a schedule.
	CodeConflict = "SLA_CONFLICT"
)

// Handler serves the SLA endpoints.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// ListBusinessHours lists schedules.
func (h *Handler) ListBusinessHours(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListBusinessHours(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetBusinessHours returns one schedule.
func (h *Handler) GetBusinessHours(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetBusinessHours(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateBusinessHours adds a schedule.
func (h *Handler) CreateBusinessHours(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.BusinessHoursInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateBusinessHours(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.business_hours_created", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateBusinessHours replaces a schedule.
func (h *Handler) UpdateBusinessHours(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in model.BusinessHoursInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateBusinessHours(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.business_hours_updated", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteBusinessHours removes a schedule.
func (h *Handler) DeleteBusinessHours(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteBusinessHours(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.business_hours_deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ListHolidays lists organization holidays (default: from 30 days ago).
func (h *Handler) ListHolidays(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	from := time.Now().UTC().AddDate(0, 0, -30)
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		t, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			invalid(w, r, "from must be YYYY-MM-DD")
			return
		}
		from = t
	}
	items, err := h.svc.ListHolidays(r.Context(), scope.ID, from)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateHoliday adds an organization holiday.
func (h *Handler) CreateHoliday(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.HolidayInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateHoliday(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.holiday_created", out.UUID, map[string]any{"name": out.Name, "date": out.Date})
	response.JSON(w, r, http.StatusCreated, out)
}

// DeleteHoliday removes an organization holiday.
func (h *Handler) DeleteHoliday(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteHoliday(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.holiday_deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// PublicHolidays lists national holidays (?country=TR&year=2026).
func (h *Handler) PublicHolidays(w http.ResponseWriter, r *http.Request) {
	if _, ok := org(w, r); !ok {
		return
	}
	v := r.URL.Query()
	year := time.Now().UTC().Year()
	if raw := strings.TrimSpace(v.Get("year")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			invalid(w, r, "year must be a number")
			return
		}
		year = n
	}
	items, err := h.svc.PublicHolidays(r.Context(), v.Get("country"), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ListPolicies lists SLA policies in evaluation order.
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListPolicies(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetPolicy returns one policy.
func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetPolicy(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreatePolicy adds a policy.
func (h *Handler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	var in model.PolicyInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreatePolicy(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.policy_created", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdatePolicy replaces a policy.
func (h *Handler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in model.PolicyInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdatePolicy(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.policy_updated", out.UUID, map[string]any{"name": out.Name})
	response.JSON(w, r, http.StatusOK, out)
}

// DeletePolicy removes a policy.
func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeletePolicy(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.policy_deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ContractedCompanies lists companies with a contracted SLA.
func (h *Handler) ContractedCompanies(w http.ResponseWriter, r *http.Request) {
	scope, ok := org(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ContractedCompanies(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetCompanyPolicy sets a company's contracted SLA.
func (h *Handler) SetCompanyPolicy(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	var in model.CompanyPolicyInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SetCompanyPolicy(r.Context(), scope.ID, id, in.PolicyUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "sla.company_policy_set", id, map[string]any{"policy_uuid": in.PolicyUUID})
	response.JSON(w, r, http.StatusOK, out)
}

// ConversationSLA returns the SLA clocks of a conversation.
func (h *Handler) ConversationSLA(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := orgAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ConversationSLA(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
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
		response.InternalErr(w, r, errors.New("sla: missing organization scope"), "")
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

func invalid(w http.ResponseWriter, r *http.Request, msg string) {
	response.ValidationError(w, r, []response.Detail{{Message: msg}})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, CodeConflict, strings.TrimPrefix(err.Error(), usecase.ErrConflict.Error()+": "))
	case errors.Is(err, usecase.ErrInvalid):
		invalid(w, r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

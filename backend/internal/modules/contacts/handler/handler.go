// Package handler exposes contacts and companies (/v1/tenant/contacts,
// /v1/tenant/companies).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes.
const (
	CodeDomainExists = "COMPANY_DOMAIN_EXISTS"
	maxBodyBytes     = 64 << 10
)

// Handler serves contacts and companies.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// ListContacts returns one page of contacts.
func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	limit, offset := apiquery.Clamp(atoi(v.Get("limit")), atoi(v.Get("offset")), 50, 100)
	f := usecase.ContactFilter{Q: v.Get("q"), Limit: limit, Offset: offset, Sort: "created_at", Desc: true}
	if raw := strings.TrimSpace(v.Get("sort")); raw != "" {
		f.Desc = strings.HasPrefix(raw, "-")
		f.Sort = strings.TrimPrefix(raw, "-")
	}
	if raw := strings.TrimSpace(v.Get("company")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			invalid(w, r, "company must be a uuid")
			return
		}
		f.Company = &id
	}
	items, total, err := h.svc.ListContacts(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// GetContact returns one contact with identities.
func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.GetContact(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

type contactRequest struct {
	Name         *string          `json:"name"`
	Email        *string          `json:"email"`
	Phone        *string          `json:"phone"`
	JobTitle     *string          `json:"job_title"`
	Company      *json.RawMessage `json:"company"`
	Locale       *string          `json:"locale"`
	Timezone     *string          `json:"timezone"`
	Notes        *string          `json:"notes"`
	CustomFields map[string]any   `json:"custom_fields"`
	Blocked      *bool            `json:"blocked"`
}

func (req contactRequest) input() (usecase.ContactInput, error) {
	in := usecase.ContactInput{
		Name: req.Name, Email: req.Email, Phone: req.Phone, JobTitle: req.JobTitle, Locale: req.Locale,
		Timezone: req.Timezone, Notes: req.Notes, CustomFields: req.CustomFields, Blocked: req.Blocked,
	}
	ref, err := optionalRef(req.Company)
	if err != nil {
		return in, err
	}
	in.Company = ref
	return in, nil
}

// CreateContact creates a contact.
func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req contactRequest
	if !decode(w, r, &req) {
		return
	}
	in, err := req.input()
	if err != nil {
		invalid(w, r, "company must be a uuid or null")
		return
	}
	c, err := h.svc.CreateContact(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.created", "contacts", c.UUID, map[string]any{"name": c.Name})
	response.JSON(w, r, http.StatusCreated, c)
}

// UpdateContact edits a contact.
func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req contactRequest
	if !decode(w, r, &req) {
		return
	}
	in, err := req.input()
	if err != nil {
		invalid(w, r, "company must be a uuid or null")
		return
	}
	c, err := h.svc.UpdateContact(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.updated", "contacts", c.UUID, map[string]any{"name": c.Name})
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteContact deletes a contact.
func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteContact(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.deleted", "contacts", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// MergeContact folds another contact into this one.
func (h *Handler) MergeContact(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req struct {
		From uuid.UUID `json:"from"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Merge(r.Context(), scope.ID, id, req.From)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.merged", "contacts", id, map[string]any{
		"from": req.From, "identities": res.Identities, "conversations": res.Conversations,
	})
	response.JSON(w, r, http.StatusOK, res)
}

// ListCompanies returns one page of companies.
func (h *Handler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	limit, offset := apiquery.Clamp(atoi(v.Get("limit")), atoi(v.Get("offset")), 50, 100)
	items, total, err := h.svc.ListCompanies(r.Context(), scope.ID, v.Get("q"), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// GetCompany returns one company.
func (h *Handler) GetCompany(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.GetCompany(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

type companyRequest struct {
	Name         *string          `json:"name"`
	Domain       *string          `json:"domain"`
	ExternalID   *string          `json:"external_id"`
	Tier         *string          `json:"tier"`
	AccountOwner *json.RawMessage `json:"account_owner"`
	Notes        *string          `json:"notes"`
	CustomFields map[string]any   `json:"custom_fields"`
}

func (req companyRequest) input() (usecase.CompanyInput, error) {
	in := usecase.CompanyInput{Name: req.Name, Domain: req.Domain, ExternalID: req.ExternalID, Tier: req.Tier, Notes: req.Notes, CustomFields: req.CustomFields}
	ref, err := optionalRef(req.AccountOwner)
	in.AccountOwner = ref
	return in, err
}

// CreateCompany creates a company.
func (h *Handler) CreateCompany(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req companyRequest
	if !decode(w, r, &req) {
		return
	}
	in, err := req.input()
	if err != nil {
		invalid(w, r, "account_owner must be a user uuid or null")
		return
	}
	c, err := h.svc.CreateCompany(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.created", "companies", c.UUID, map[string]any{"name": c.Name})
	response.JSON(w, r, http.StatusCreated, c)
}

// UpdateCompany edits a company.
func (h *Handler) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req companyRequest
	if !decode(w, r, &req) {
		return
	}
	in, err := req.input()
	if err != nil {
		invalid(w, r, "account_owner must be a user uuid or null")
		return
	}
	c, err := h.svc.UpdateCompany(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.updated", "companies", c.UUID, map[string]any{"name": c.Name})
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteCompany deletes a company.
func (h *Handler) DeleteCompany(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCompany(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.deleted", "companies", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	payload["organization_uuid"] = scope.UUID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, action, resource, &id, payload, r)
}

// optionalRef: absent → nil (keep), null → pointer to nil (clear), uuid → set.
func optionalRef(raw *json.RawMessage) (**uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	var target *uuid.UUID
	if string(*raw) != "null" {
		var u uuid.UUID
		if err := json.Unmarshal(*raw, &u); err != nil {
			return nil, err
		}
		target = &u
	}
	return &target, nil
}

func scopeOf(w http.ResponseWriter, r *http.Request) (orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("contacts: missing organization scope"), "")
	}
	return scope, ok
}

func scopeAndID(w http.ResponseWriter, r *http.Request) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := scopeOf(w, r)
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
	case errors.Is(err, usecase.ErrDomainExists):
		response.Conflict(w, r, CodeDomainExists, "Another company already uses this domain")
	case errors.Is(err, usecase.ErrInvalid):
		invalid(w, r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

func atoi(s string) int32 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

type revenueRequest struct {
	MRRMinor           *int64           `json:"mrr_minor"`
	Currency           *string          `json:"currency"`
	Plan               *string          `json:"plan"`
	BillingStatus      *string          `json:"billing_status"`
	RenewalAt          *json.RawMessage `json:"renewal_at"`
	LifetimeValueMinor *int64           `json:"lifetime_value_minor"`
	ExternalRef        *string          `json:"external_ref"`
}

// integrationRevenueRequest names the company by uuid, external id or
// domain, so a billing system can push without knowing ClickFix ids.
type integrationRevenueRequest struct {
	revenueRequest
	CompanyUUID *uuid.UUID `json:"company_uuid"`
	ExternalID  string     `json:"external_id"`
	Domain      string     `json:"domain"`
	Source      string     `json:"source"`
}

func (req revenueRequest) input(source string, actor int64) (usecase.RevenueInput, error) {
	in := usecase.RevenueInput{
		MRRMinor: req.MRRMinor, Currency: req.Currency, Plan: req.Plan, BillingStatus: req.BillingStatus,
		LifetimeValueMinor: req.LifetimeValueMinor, ExternalRef: req.ExternalRef, Source: source, ActorID: &actor,
	}
	if req.RenewalAt != nil {
		var target *time.Time
		if string(*req.RenewalAt) != "null" {
			var raw string
			if err := json.Unmarshal(*req.RenewalAt, &raw); err != nil {
				return in, err
			}
			t, err := parseDateTime(raw)
			if err != nil {
				return in, err
			}
			target = &t
		}
		in.RenewalAt = &target
	}
	return in, nil
}

// parseDateTime accepts RFC 3339 or a bare date (YYYY-MM-DD, midnight UTC).
func parseDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.DateOnly, raw)
}

// GetCompanyRevenue returns a company's revenue and its history.
func (h *Handler) GetCompanyRevenue(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	rev, hist, err := h.svc.GetRevenue(r.Context(), scope.ID, id, 50)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"revenue": rev, "history": hist})
}

// PutCompanyRevenue sets a company's revenue by hand (source manual).
func (h *Handler) PutCompanyRevenue(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req revenueRequest
	if !decode(w, r, &req) {
		return
	}
	in, err := req.input(usecase.RevenueSourceManual, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		invalid(w, r, "renewal_at must be an RFC 3339 time, a YYYY-MM-DD date or null")
		return
	}
	companyUUID, rev, err := h.svc.SetRevenue(r.Context(), scope.ID, usecase.CompanyKey{UUID: &id}, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.revenue_updated", "companies", companyUUID, map[string]any{
		"source": rev.Source, "mrr_minor": rev.MRRMinor, "currency": rev.Currency, "plan": rev.Plan,
	})
	response.JSON(w, r, http.StatusOK, rev)
}

// DeleteCompanyRevenue clears a company's current revenue.
func (h *Handler) DeleteCompanyRevenue(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.ClearRevenue(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.revenue_cleared", "companies", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// UpsertCompanyRevenue is the integration endpoint: a billing system pushes
// a customer's revenue, naming the company by uuid, external id or domain.
// Reached directly or through the public API (/v1/public-api/company-revenue,
// key scope revenue:write, Idempotency-Key required).
func (h *Handler) UpsertCompanyRevenue(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req integrationRevenueRequest
	if !decode(w, r, &req) {
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = usecase.RevenueSourceAPI
	}
	if source == usecase.RevenueSourceManual {
		invalid(w, r, "source must be api, stripe or iyzico")
		return
	}
	in, err := req.input(source, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		invalid(w, r, "renewal_at must be an RFC 3339 time, a YYYY-MM-DD date or null")
		return
	}
	key := usecase.CompanyKey{UUID: req.CompanyUUID, ExternalID: req.ExternalID, Domain: req.Domain}
	companyUUID, rev, err := h.svc.SetRevenue(r.Context(), scope.ID, key, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "companies.revenue_updated", "companies", companyUUID, map[string]any{
		"source": rev.Source, "mrr_minor": rev.MRRMinor, "currency": rev.Currency, "plan": rev.Plan,
	})
	response.JSON(w, r, http.StatusOK, map[string]any{"company_uuid": companyUUID, "revenue": rev})
}

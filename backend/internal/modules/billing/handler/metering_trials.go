package handler

import (
	"github.com/google/uuid"
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.ent.UsageView(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) DailyUsage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	start, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("starts_at"))
	end, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("ends_at"))
	if e1 != nil || e2 != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "starts_at/ends_at", Code: "invalid", Message: "RFC3339 boundaries required"}})
		return
	}
	out, err := h.ent.Daily(r.Context(), org, start, end)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) Meter(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in entitlements.MeterEvent
	if !decode(w, r, &in) {
		return
	}
	out, err := h.ent.Meter(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) CloseUsage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "period")
	if !ok {
		return
	}
	if err := h.ent.ClosePeriod(r.Context(), org, id, time.Now().UTC()); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.usage_closed", "billing_usage_periods", id)
	response.JSON(w, r, 200, map[string]string{"status": "closed"})
}
func (h *Handler) ExtendTrial(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "subscription")
	if !ok {
		return
	}
	var in struct {
		EndsAt time.Time `json:"ends_at"`
		Reason string    `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ExtendTrial(r.Context(), org, id, in.EndsAt, in.Reason, actor(r)); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.trial_extended", "billing_subscriptions", id)
	response.JSON(w, r, 200, map[string]string{"status": "extended"})
}
func (h *Handler) GatewaySettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var in usecase.GatewaySettings
		if !decode(w, r, &in) {
			return
		}
		if err := h.svc.SaveGatewaySettings(r.Context(), in); err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "billing.gateway_settings_changed", "billing_gateway_settings", uuid.Nil)
	}
	out, err := h.svc.GatewaySettings(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}

package handler

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// adminFilter reads limit/offset/q plus `status` (and `live=true` for
// subscriptions). Sorting is fixed: newest first.
func adminFilter(w http.ResponseWriter, r *http.Request) (usecase.AdminFilter, bool) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	if len(apiquery.ForbiddenParams(values)) > 0 || len(q.Sort) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "query", Code: "invalid", Message: "Use limit, offset, q and status; order is fixed."}})
		return usecase.AdminFilter{}, false
	}
	live := values.Get("live")
	if live != "" && live != "true" && live != "false" {
		response.ValidationError(w, r, []response.Detail{{Field: "live", Code: "invalid", Message: "live must be true or false"}})
		return usecase.AdminFilter{}, false
	}
	return usecase.AdminFilter{Status: values.Get("status"), LiveOnly: live == "true", Search: q.Q, Limit: q.Limit, Offset: q.Offset}, true
}

// AdminSubscriptions lists subscriptions of every organization (CF-39).
func (h *Handler) AdminSubscriptions(w http.ResponseWriter, r *http.Request) {
	f, ok := adminFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.AdminSubscriptions(r.Context(), f)
	if err != nil {
		fail(w, r, err)
		return
	}
	limit, offset := apiquery.Clamp(f.Limit, f.Offset, 20, 100)
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// AdminSubscriptionStats returns live subscription counts and MRR (CF-39).
func (h *Handler) AdminSubscriptionStats(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.AdminSubscriptionStats(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AdminPayments lists payment attempts across organizations, e.g. failed ones.
func (h *Handler) AdminPayments(w http.ResponseWriter, r *http.Request) {
	f, ok := adminFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.AdminPayments(r.Context(), f)
	if err != nil {
		fail(w, r, err)
		return
	}
	limit, offset := apiquery.Clamp(f.Limit, f.Offset, 20, 100)
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// AdminTransferQueue lists bank transfer reports with organization and invoice
// context; reported (pending review) rows come first.
func (h *Handler) AdminTransferQueue(w http.ResponseWriter, r *http.Request) {
	f, ok := adminFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.AdminTransfers(r.Context(), f)
	if err != nil {
		fail(w, r, err)
		return
	}
	limit, offset := apiquery.Clamp(f.Limit, f.Offset, 20, 100)
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// TrialInvoice returns (creating if needed) the first paid-period invoice of
// a trialing subscription so the customer can pay before the trial ends.
func (h *Handler) TrialInvoice(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "subscription")
	if !ok {
		return
	}
	out, err := h.svc.TrialInvoice(r.Context(), org, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.trial_invoice_created", "billing_subscriptions", id)
	response.JSON(w, r, http.StatusOK, out)
}

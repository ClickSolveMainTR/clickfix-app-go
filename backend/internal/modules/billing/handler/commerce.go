package handler

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

func (h *Handler) BillingProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var out usecase.ProfileInput
	var err error
	if r.Method == http.MethodPut {
		if !decode(w, r, &out) {
			return
		}
		out, err = h.svc.SaveProfile(r.Context(), org, out)
	} else {
		out, err = h.svc.Profile(r.Context(), org)
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) InvoiceDetail(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "invoice")
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		if err := h.svc.IssueInvoice(r.Context(), org, id); err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, map[string]string{"status": "processed"})
		return
	}
	format := r.PathValue("format")
	if format != "" {
		doc, err := h.svc.PrepareInvoice(r.Context(), org, id)
		if err != nil {
			fail(w, r, err)
			return
		}
		var data []byte
		mime := ""
		switch format {
		case "xml":
			data = doc.Xml
			mime = "application/xml"
		case "pdf":
			data = doc.Pdf
			mime = "application/pdf"
		case "html":
			data = doc.Html
			mime = "text/html; charset=utf-8"
		default:
			response.NotFound(w, r, "")
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		w.Header().Set("Content-Disposition", "inline; filename=\""+id.String()+"."+format+"\"")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	invoice, lines, err := h.svc.InvoiceDetail(r.Context(), org, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"invoice": invoiceResponse(invoice), "lines": lines})
}
func (h *Handler) GenerateInvoice(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.InvoiceInput
	if !decode(w, r, &in) {
		return
	}
	if in.Kind != "" && in.Kind != "renewal" && in.Kind != "manual" {
		response.BadRequest(w, r, "INVALID_INVOICE_KIND", "Invalid invoice kind")
		return
	}
	out, err := h.svc.GenerateInvoice(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 201, invoiceResponse(out))
}
func (h *Handler) PlanChange(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.PlanChangeInput
	if !decode(w, r, &in) {
		return
	}
	if r.PathValue("operation") == "preview" {
		out, err := h.svc.PreviewPlanChange(r.Context(), org, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, out)
		return
	}
	if r.PathValue("operation") != "apply" {
		response.NotFound(w, r, "")
		return
	}
	out, err := h.svc.ApplyPlanChange(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) Addons(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in usecase.AddonInput
		if !decode(w, r, &in) {
			return
		}
		out, err := h.svc.CreateAddon(r.Context(), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 201, out)
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.AddonCatalog(r.Context(), q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.commercePage(w, r, out, 0, "addons", q.Limit, q.Offset)
}
func (h *Handler) ChangeAddon(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.AddonChangeInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.ChangeAddon(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"invoice": optionalInvoiceResponse(out)})
}
func (h *Handler) Coupons(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in usecase.CouponInput
		if !decode(w, r, &in) {
			return
		}
		out, err := h.svc.CreateCoupon(r.Context(), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 201, out)
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CouponCatalog(r.Context(), q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.commercePage(w, r, out, 0, "coupons", q.Limit, q.Offset)
}
func (h *Handler) AttachCoupon(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.AttachCoupon(r.Context(), org, in.Code); err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]string{"status": "attached"})
}
func (h *Handler) Wallet(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Wallet(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	counts, err := h.svc.CommerceCounts(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	settings, err := h.svc.WalletSettings(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"try_minor": out.TRYMinor, "ai_credits": out.AICredits, "video_minutes": out.VideoMinutes, "ai_action": out.AIAction, "settings": settings, "entries": map[string]any{"items": out.Entries, "total": counts.Ledger, "limit": q.Limit, "offset": q.Offset}})
}
func (h *Handler) WalletCommand(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.WalletInput
	if !decode(w, r, &in) {
		return
	}
	in.Actor = actor(r)
	out, err := h.svc.WalletCommand(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	// The ledger keeps who and why; the activity log makes it searchable.
	a := actor(r)
	h.activity.Record(r.Context(), &a, "billing.wallet_adjusted", "billing_credit_ledger", &out.Uuid, map[string]any{"organization_uuid": r.PathValue("organization"), "unit": out.Unit, "delta": out.Delta, "reason": out.Reason, "category": in.Category}, r)
	response.JSON(w, r, 201, out)
}
func (h *Handler) CreditPackages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in usecase.CreditPackageInput
		if !decode(w, r, &in) {
			return
		}
		out, err := h.svc.CreateCreditPackage(r.Context(), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 201, out)
		return
	}
	if r.Method == http.MethodPut {
		id, ok := path(w, r, "package")
		if !ok {
			return
		}
		var in usecase.CreditPackageInput
		if !decode(w, r, &in) {
			return
		}
		out, err := h.svc.UpdateCreditPackage(r.Context(), id, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, out)
		return
	}
	out, err := h.svc.CreditPackages(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": out})
}

// TenantCreditPackages lists the packs the organization in scope may buy.
func (h *Handler) TenantCreditPackages(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.svc.OrganizationCreditPackages(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": out})
}

// AdminChangePlan switches or schedules an organization's plan (platform).
func (h *Handler) AdminChangePlan(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.AdminPlanChange
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.AdminChangePlan(r.Context(), org, in, actor(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	a := actor(r)
	h.activity.Record(r.Context(), &a, "billing.plan_changed", "billing_subscriptions", &out.UUID, map[string]any{"organization_uuid": r.PathValue("organization"), "price_uuid": in.PriceUUID, "timing": in.Timing, "reason": in.Reason}, r)
	response.JSON(w, r, 200, out)
}

func (h *Handler) PurchaseCredits(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.CreditPurchaseInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.PurchaseCredits(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 201, invoiceResponse(out))
}
func (h *Handler) WalletSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.WalletSettingsInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.SaveWalletSettings(r.Context(), org, in); err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]string{"status": "saved"})
}
func (h *Handler) Statements(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in struct {
			Rows []usecase.StatementInput `json:"rows"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.svc.ImportStatement(r.Context(), in.Rows); err != nil {
			fail(w, r, err)
			return
		}
		if err := h.svc.ReconcileStatements(r.Context()); err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, map[string]string{"status": "reconciled"})
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.StatementReport(r.Context(), q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.commercePage(w, r, out, 0, "statements", q.Limit, q.Offset)
}
func (h *Handler) Referral(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		var in struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.svc.RedeemReferral(r.Context(), org, in.Code); err != nil {
			fail(w, r, err)
			return
		}
	}
	code, err := h.svc.ReferralCode(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]string{"code": code})
}
func (h *Handler) commercePage(w http.ResponseWriter, r *http.Request, items any, org int64, kind string, limit, offset int32) {
	counts, err := h.svc.CommerceCounts(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	total := map[string]int64{"addons": counts.Addons, "coupons": counts.Coupons, "statements": counts.Statements}[kind]
	response.JSON(w, r, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// CommerceAudit records successful mutations through the shared recorder. The
// existing settings action keeps backend-only additions compatible with the
// current activity labels; the exact operation is retained in the payload.
func (h *Handler) CommerceAudit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recorder := &commerceResponse{ResponseWriter: w, status: http.StatusOK}
		next(recorder, r)
		if r.Method != http.MethodGet && recorder.status < 400 {
			user := actor(r)
			h.activity.Record(r.Context(), &user, "settings.updated", "platform.settings", nil, map[string]any{"operation": r.Pattern, "organization_uuid": r.PathValue("organization"), "invoice_uuid": r.PathValue("invoice")}, r)
		}
	}
}

type commerceResponse struct {
	http.ResponseWriter
	status int
}

func (w *commerceResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (h *Handler) WalletPayment(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.WalletPaymentInput
	if !decode(w, r, &in) {
		return
	}
	id, err := h.svc.PayFromWallet(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"payment_uuid": id})
}

type invoiceView struct {
	db.BillingInvoice
	Buyer  json.RawMessage `json:"buyer_snapshot"`
	Seller json.RawMessage `json:"seller_snapshot"`
}

func invoiceResponse(in db.BillingInvoice) invoiceView {
	return invoiceView{BillingInvoice: in, Buyer: json.RawMessage(in.BuyerSnapshot), Seller: json.RawMessage(in.SellerSnapshot)}
}
func optionalInvoiceResponse(in *db.BillingInvoice) any {
	if in == nil {
		return nil
	}
	return invoiceResponse(*in)
}

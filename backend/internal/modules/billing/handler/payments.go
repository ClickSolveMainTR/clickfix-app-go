package handler

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Webhook limits: provider bodies are small forms/JSON; one source may not
// flood the endpoint (signature checks and provider API calls are not free).
const (
	notifyBodyLimit  = 64 << 10
	returnBodyLimit  = 8 << 10
	notifyRateLimit  = 300
	notifyRateWindow = time.Minute
)

// defaultTrustedProxies applies when PAYMENTS_TRUSTED_PROXY_CIDRS is empty:
// the API is only reachable through Traefik and the BFF on the private
// Docker network, so private and loopback peers are proxies. Without this
// every provider shares the proxy's address and one rate-limit bucket.
const defaultTrustedProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"

// webhookSource is the transport peer, or, when the peer is a trusted proxy
// (PAYMENTS_TRUSTED_PROXY_CIDRS, default the private ranges), the right-most
// X-Forwarded-For hop that is not itself a trusted proxy.
func webhookSource(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	list := os.Getenv("PAYMENTS_TRUSTED_PROXY_CIDRS")
	if strings.TrimSpace(list) == "" {
		list = defaultTrustedProxies
	}
	trusted := parseCIDRs(list)
	if !contains(trusted, peer) {
		return peer
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip, e := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if e != nil {
			return netip.Addr{}
		}
		peer = ip.Unmap()
		if !contains(trusted, peer) {
			break
		}
	}
	return peer
}

func parseCIDRs(list string) []netip.Prefix {
	out := []netip.Prefix{}
	for _, entry := range strings.Split(list, ",") {
		if p, e := netip.ParsePrefix(strings.TrimSpace(entry)); e == nil {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []netip.Prefix, ip netip.Addr) bool {
	for _, p := range list {
		if ip.IsValid() && p.Contains(ip) {
			return true
		}
	}
	return false
}

// webhookSourceAllowed applies PAYMENTS_<PROVIDER>_WEBHOOK_CIDRS. Empty list
// (the default) allows every source: the HMAC signature is the security
// boundary and provider IP ranges change without notice. A set list is
// enforced in addition to the signature.
func webhookSourceAllowed(provider string, source netip.Addr) bool {
	list := strings.TrimSpace(os.Getenv("PAYMENTS_" + strings.ToUpper(provider) + "_WEBHOOK_CIDRS"))
	if list == "" {
		return true
	}
	return contains(parseCIDRs(list), source)
}

// notifyOutcome maps a Notify result onto the provider answer. Providers
// retry on anything but their acknowledgement, so only transient failures
// (database, provider API timeouts) answer 5xx.
func notifyOutcome(err error) (int, string) {
	var refusal *payments.ProviderError
	switch {
	case err == nil:
		return http.StatusOK, "processed"
	case errors.Is(err, usecase.ErrUnknownOrder):
		return http.StatusOK, "unknown_order"
	case errors.Is(err, payments.ErrProviderDisabled):
		return http.StatusNotFound, "provider_disabled"
	case errors.Is(err, payments.ErrInvalidSignature):
		return http.StatusBadRequest, "invalid_signature"
	case errors.Is(err, payments.ErrMalformed):
		return http.StatusBadRequest, "malformed"
	case errors.Is(err, usecase.ErrAmountMismatch):
		return http.StatusBadRequest, "amount_mismatch"
	case errors.As(err, &refusal) && !refusal.Temporary:
		return http.StatusBadRequest, "provider_refused"
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrInvalid), errors.Is(err, model.ErrNotFound), errors.Is(err, payments.ErrUnsupported):
		return http.StatusBadRequest, "rejected"
	default:
		return http.StatusServiceUnavailable, "transient_error"
	}
}

// PaymentNotify is the provider server-to-server notification (webhook):
// 200 processed or duplicate (replays are absorbed by billing_webhook_receipts),
// 400 bad signature / malformed / permanently rejected, 403 source not
// allowed, 404 provider not enabled, 413 body too large, 429 rate limited,
// 503 transient internal failure (the provider retries).
func (h *Handler) PaymentNotify(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	source := webhookSource(r)
	audit := func(status int, outcome string, err error) {
		if h.log == nil {
			return
		}
		attrs := []any{"provider", provider, "status", status, "outcome", outcome, "source", source.String()}
		if err != nil && status >= 500 {
			attrs = append(attrs, "error", err.Error())
		}
		if status >= 500 {
			h.log.Error("billing_payment_notify", attrs...)
			return
		}
		h.log.Info("billing_payment_notify", attrs...)
	}
	if provider != "iyzico" && provider != "paytr" && provider != "fake" {
		audit(http.StatusNotFound, "unknown_provider", nil)
		http.Error(w, "FAIL", http.StatusNotFound)
		return
	}
	if h.limit != nil {
		if ok, _ := h.limit(r, "billing_notify:"+provider, source.String(), notifyRateLimit, notifyRateWindow); !ok {
			audit(http.StatusTooManyRequests, "rate_limited", nil)
			http.Error(w, "FAIL", http.StatusTooManyRequests)
			return
		}
	}
	if !webhookSourceAllowed(provider, source) {
		audit(http.StatusForbidden, "source_not_allowed", nil)
		http.Error(w, "FAIL", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, notifyBodyLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		audit(status, "unreadable_body", nil)
		http.Error(w, "FAIL", status)
		return
	}
	p, err := h.svc.Notify(r.Context(), provider, r.Header, body)
	status, outcome := notifyOutcome(err)
	audit(status, outcome, err)
	if status == http.StatusOK && p != nil {
		p.Acknowledge(w)
		return
	}
	http.Error(w, "FAIL", status)
}

// IyzicoReturn is the browser coming back from the iyzico Checkout Form
// (a cross-site form POST). The result is read from iyzico's API, then the
// browser is sent to the billing page, which polls the verified status.
func (h *Handler) IyzicoReturn(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("reference")
	status := "error"
	// Over the limit the payer still lands on the billing page: it polls the
	// status the webhook and reconciliation settle.
	limited := false
	if h.limit != nil {
		if ok, _ := h.limit(r, "billing_return", webhookSource(r).String(), 60, time.Minute); !ok {
			limited, status = true, "processing"
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, returnBodyLimit)
	if err := r.ParseForm(); err == nil && !limited {
		got, e := h.svc.ReturnCheckout(r.Context(), ref, r.PostForm.Get("token"))
		switch {
		case e == nil && (got == "succeeded" || got == "failed" || got == "expired"):
			status = got
		case e == nil:
			status = "processing"
		default:
			if code, _ := notifyOutcome(e); code >= 500 {
				// iyzico or the database is slow: the webhook and the
				// reconciliation job settle it; the page keeps polling.
				status = "processing"
			}
			if h.log != nil {
				h.log.Warn("billing_payment_return", "provider", "iyzico", "outcome", status, "error_kind", fmt.Sprintf("%T", e))
			}
		}
	}
	target := h.svc.PaymentFrontend() + "/billing?" + url.Values{"payment": {status}, "payment_ref": {ref}}.Encode()
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// PaymentAvailability tells the billing UI which purchase paths work now.
func (h *Handler) PaymentAvailability(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.svc.PaymentAvailability(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}

// CheckoutStatus is polled by the billing page after a provider return.
func (h *Handler) CheckoutStatus(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CheckoutStatus(r.Context(), org, r.PathValue("reference"))
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}

// buyerIP is the payer's address for provider fraud checks: the left-most
// X-Forwarded-For hop the BFF sets, else the peer.
func buyerIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip, err := netip.ParseAddr(strings.TrimSpace(strings.Split(xff, ",")[0])); err == nil {
			return ip.Unmap().String()
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in usecase.CheckoutInput
	if !decode(w, r, &in) {
		return
	}
	buyer, err := h.svc.CheckoutBuyer(r.Context(), org, buyerIP(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	out, err := h.svc.Checkout(r.Context(), org, in, buyer)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.checkout_created", "billing_payments", out.PaymentUUID)
	response.JSON(w, r, 201, out)
}
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.InvoiceList(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.paymentPage(w, r, out, org, false, "invoices", q.Limit, q.Offset)
}
func (h *Handler) BankAccounts(w http.ResponseWriter, r *http.Request) {
	admin := strings.HasPrefix(r.URL.Path, "/v1/platform/")
	if r.Method == http.MethodPut {
		var in usecase.BankAccountInput
		if !decode(w, r, &in) {
			return
		}
		out, err := h.svc.SaveBankAccount(r.Context(), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "billing.bank_account_saved", "billing_bank_accounts", out.UUID)
		response.JSON(w, r, 200, out)
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.BankAccounts(r.Context(), admin, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.paymentPage(w, r, out, 0, admin, "bank_accounts", q.Limit, q.Offset)
}
func (h *Handler) Transfers(w http.ResponseWriter, r *http.Request) {
	org := int64(0)
	if r.PathValue("organization") != "" || !strings.HasPrefix(r.URL.Path, "/v1/platform/") {
		var ok bool
		org, ok = h.organization(w, r)
		if !ok {
			return
		}
	}
	if r.Method == http.MethodPost {
		var in usecase.TransferInput
		if !decode(w, r, &in) {
			return
		}
		id, err := h.svc.ReportTransfer(r.Context(), org, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "billing.transfer_reported", "billing_bank_transfers", id)
		response.JSON(w, r, 201, map[string]uuid.UUID{"uuid": id})
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Transfers(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.paymentPage(w, r, out, org, false, "transfers", q.Limit, q.Offset)
}
func (h *Handler) ReviewTransfer(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "transfer")
	if !ok {
		return
	}
	var in usecase.TransferReview
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ReviewTransfer(r.Context(), org, id, in, actor(r)); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.transfer_reviewed", "billing_bank_transfers", id)
	response.JSON(w, r, 200, map[string]string{"status": "reviewed"})
}
func (h *Handler) PaymentCards(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		id, ok := path(w, r, "card")
		if !ok {
			return
		}
		if err := h.svc.RemovePaymentCard(r.Context(), org, id); err != nil {
			fail(w, r, err)
			return
		}
		h.audit(r, "billing.card_removed", "billing_payment_methods", id)
		response.JSON(w, r, 200, map[string]string{"status": "removed"})
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Cards(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.paymentPage(w, r, out, org, false, "cards", q.Limit, q.Offset)
}
func (h *Handler) RefundPayment(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "payment")
	if !ok {
		return
	}
	var in usecase.RefundInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.RefundPayment(r.Context(), org, id, in); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.payment_refunded", "billing_payments", id)
	response.JSON(w, r, 200, map[string]string{"status": "refunded"})
}
func (h *Handler) ReconciliationReport(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	q, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ReconciliationReport(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.paymentPage(w, r, out, org, false, "mismatches", q.Limit, q.Offset)
}

func paymentQuery(w http.ResponseWriter, r *http.Request) (apiquery.Query, bool) {
	q := apiquery.Parse(r.URL.Query())
	if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || q.Q != "" || len(q.Sort) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "query", Code: "invalid", Message: "Use limit and offset; order is fixed."}})
		return q, false
	}
	return q, true
}
func (h *Handler) paymentPage(w http.ResponseWriter, r *http.Request, items any, org int64, admin bool, kind string, limit, offset int32) {
	counts, err := h.svc.PaymentListCounts(r.Context(), org, admin)
	if err != nil {
		fail(w, r, err)
		return
	}
	totals := map[string]int64{"invoices": counts.Invoices, "cards": counts.Cards, "bank_accounts": counts.BankAccounts, "transfers": counts.Transfers, "mismatches": counts.Mismatches}
	response.JSON(w, r, 200, map[string]any{"items": items, "total": totals[kind], "limit": limit, "offset": offset})
}

func (h *Handler) TransferReceipt(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "transfer")
	if !ok {
		return
	}
	attachment, err := h.svc.TransferReceipt(r.Context(), org, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.attachments == nil {
		response.Internal(w, r, "")
		return
	}
	file, err := h.attachments.Open(r.Context(), org, attachment, "")
	if err != nil {
		response.NotFound(w, r, "")
		return
	}
	defer func() { _ = file.Body.Close() }()
	h.audit(r, "billing.receipt_downloaded", "billing_bank_transfers", id)
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Attachment.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	if file.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file.Body)
}

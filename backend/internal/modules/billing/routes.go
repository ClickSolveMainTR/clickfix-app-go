package billing

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the billing API. stepUp (optional) wraps the manual
// platform operations on subscriptions and entitlements (CF-39): transitions,
// trial extension, overrides, transfer review and refunds require a fresh
// password/passkey confirmation.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver, stepUp ...func(http.Handler) http.Handler) {
	mux.HandleFunc("GET /v1/public/pricing", h.Pricing)
	mux.HandleFunc("POST /v1/public/billing/payments/{provider}/notify", h.PaymentNotify)
	mux.HandleFunc("POST /v1/public/billing/payments/iyzico/return/{reference}", h.IyzicoReturn)
	authn := middleware.Authenticate(tokens, loader)
	for _, r := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"GET /v1/platform/organizations/{organization}/billing/bank-statements/meta", h.BankStatementMeta, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/organizations/{organization}/billing/bank-statements", h.CommerceAudit(h.BankStatementLines), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/organizations/{organization}/billing/bank-statements/import", h.CommerceAudit(h.ImportBankStatement), rbac.PermPlatformSubscriptionsWrite},
		{"POST /v1/platform/organizations/{organization}/billing/bank-statements/{line}/review", h.CommerceAudit(h.ReviewBankStatement), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/billing/addons", h.CommerceAudit(h.Addons), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/billing/addons", h.CommerceAudit(h.Addons), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/billing/coupons", h.CommerceAudit(h.Coupons), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/billing/coupons", h.CommerceAudit(h.Coupons), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/billing/credit-packages", h.CommerceAudit(h.CreditPackages), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/billing/credit-packages", h.CommerceAudit(h.CreditPackages), rbac.PermPlatformSubscriptionsWrite},
		{"PUT /v1/platform/billing/credit-packages/{package}", h.CommerceAudit(h.CreditPackages), rbac.PermPlatformSubscriptionsWrite},
		{"POST /v1/platform/organizations/{organization}/subscription/change-plan", h.AdminChangePlan, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/billing/statements", h.CommerceAudit(h.Statements), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/billing/statements", h.CommerceAudit(h.Statements), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/profile", h.CommerceAudit(h.BillingProfile), rbac.PermPlatformSubscriptionsRead},
		{"PUT /v1/platform/organizations/{organization}/billing/profile", h.CommerceAudit(h.BillingProfile), rbac.PermPlatformSubscriptionsWrite},
		{"POST /v1/platform/organizations/{organization}/billing/invoices", h.CommerceAudit(h.GenerateInvoice), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/invoices/{invoice}", h.CommerceAudit(h.InvoiceDetail), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/organizations/{organization}/billing/invoices/{invoice}/issue", h.CommerceAudit(h.InvoiceDetail), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/invoices/{invoice}/{format}", h.CommerceAudit(h.InvoiceDetail), rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/organizations/{organization}/billing/wallet/entries", h.CommerceAudit(h.WalletCommand), rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/wallet", h.CommerceAudit(h.Wallet), rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/organizations/{organization}/billing/bank-transfers/{transfer}/receipt", h.TransferReceipt, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/bank-accounts", h.BankAccounts, rbac.PermPlatformSubscriptionsRead},
		{"PUT /v1/platform/billing/bank-accounts", h.BankAccounts, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/billing/bank-transfers", h.Transfers, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/bank-transfers/queue", h.AdminTransferQueue, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/subscriptions", h.AdminSubscriptions, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/subscriptions/stats", h.AdminSubscriptionStats, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/payments", h.AdminPayments, rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/organizations/{organization}/billing/bank-transfers/{transfer}/review", h.ReviewTransfer, rbac.PermPlatformSubscriptionsWrite},
		{"POST /v1/platform/organizations/{organization}/billing/payments/{payment}/refund", h.RefundPayment, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/reconciliation", h.ReconciliationReport, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/billing/gateway-settings", h.GatewaySettings, rbac.PermPlatformSubscriptionsRead},
		{"PUT /v1/platform/billing/gateway-settings", h.GatewaySettings, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/billing/usage", h.Usage, rbac.PermPlatformEntitlementsRead},
		{"GET /v1/platform/organizations/{organization}/billing/usage/daily", h.DailyUsage, rbac.PermPlatformEntitlementsRead},
		{"POST /v1/platform/organizations/{organization}/billing/usage", h.Meter, rbac.PermPlatformEntitlementsWrite},
		{"POST /v1/platform/organizations/{organization}/billing/usage/{period}/close", h.CloseUsage, rbac.PermPlatformEntitlementsWrite},
		{"POST /v1/platform/organizations/{organization}/subscriptions/{subscription}/extend-trial", h.ExtendTrial, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/plans/meta", h.Meta, rbac.PermPlatformPlansRead},
		{"GET /v1/platform/plans", h.List, rbac.PermPlatformPlansRead},
		{"POST /v1/platform/plans", h.Create, rbac.PermPlatformPlansWrite},
		{"GET /v1/platform/plans/{uuid}", h.Get, rbac.PermPlatformPlansRead},
		{"PUT /v1/platform/plans/{uuid}", h.Update, rbac.PermPlatformPlansWrite},
		{"DELETE /v1/platform/plans/{uuid}", h.Archive, rbac.PermPlatformPlansWrite},
		{"POST /v1/platform/plans/{uuid}/versions", h.SaveVersion, rbac.PermPlatformPlansWrite},
		{"PUT /v1/platform/plans/{uuid}/versions/{version}", h.SaveVersion, rbac.PermPlatformPlansWrite},
		{"POST /v1/platform/plans/{uuid}/versions/{version}/publish", h.Publish, rbac.PermPlatformPlansWrite},
		{"POST /v1/platform/plan-migrations", h.Migrate, rbac.PermPlatformPlansWrite},
		{"GET /v1/platform/billing/features", h.Features, rbac.PermPlatformPlansRead},
		{"POST /v1/platform/subscriptions", h.CreateSubscription, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/subscription", h.Subscription, rbac.PermPlatformSubscriptionsRead},
		{"POST /v1/platform/organizations/{organization}/subscriptions/{subscription}/transitions", h.Transition, rbac.PermPlatformSubscriptionsWrite},
		{"GET /v1/platform/organizations/{organization}/subscriptions/{subscription}/history", h.History, rbac.PermPlatformSubscriptionsRead},
		{"GET /v1/platform/organizations/{organization}/entitlements", h.Entitlements, rbac.PermPlatformEntitlementsRead},
		{"PUT /v1/platform/organizations/{organization}/entitlements/{feature}", h.Override, rbac.PermPlatformEntitlementsWrite},
		{"DELETE /v1/platform/organizations/{organization}/entitlements/{feature}", h.Override, rbac.PermPlatformEntitlementsWrite},
	} {
		chain := []func(http.Handler) http.Handler{authn, middleware.RequirePermission(r.permission)}
		if r.permission == rbac.PermPlatformSubscriptionsWrite || r.permission == rbac.PermPlatformEntitlementsWrite {
			chain = append(chain, stepUp...)
		}
		mux.Handle(r.pattern, middleware.Chain(r.fn, chain...))
	}
	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	for _, route := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"GET /v1/tenant/billing/profile", h.CommerceAudit(h.BillingProfile), rbac.PermTenantBillingRead},
		{"PUT /v1/tenant/billing/profile", h.CommerceAudit(h.BillingProfile), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/invoices/{invoice}", h.CommerceAudit(h.InvoiceDetail), rbac.PermTenantBillingRead},
		{"GET /v1/tenant/billing/invoices/{invoice}/{format}", h.CommerceAudit(h.InvoiceDetail), rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/plan-change/{operation}", h.CommerceAudit(h.PlanChange), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/addons", h.CommerceAudit(h.Addons), rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/addons", h.CommerceAudit(h.ChangeAddon), rbac.PermTenantBillingWrite},
		{"POST /v1/tenant/billing/coupon", h.CommerceAudit(h.AttachCoupon), rbac.PermTenantBillingWrite},
		{"POST /v1/tenant/billing/wallet/pay", h.CommerceAudit(h.WalletPayment), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/wallet", h.CommerceAudit(h.Wallet), rbac.PermTenantBillingRead},
		{"PUT /v1/tenant/billing/wallet/settings", h.CommerceAudit(h.WalletSettings), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/credit-packages", h.TenantCreditPackages, rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/credit-purchases", h.CommerceAudit(h.PurchaseCredits), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/referral", h.CommerceAudit(h.Referral), rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/referral", h.CommerceAudit(h.Referral), rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/invoices", h.Invoices, rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/checkout", h.Checkout, rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/checkout/{reference}", h.CheckoutStatus, rbac.PermTenantBillingRead},
		{"GET /v1/tenant/billing/payment-availability", h.PaymentAvailability, rbac.PermTenantBillingRead},
		{"GET /v1/tenant/billing/bank-accounts", h.BankAccounts, rbac.PermTenantBillingRead},
		{"GET /v1/tenant/billing/bank-transfers", h.Transfers, rbac.PermTenantBillingRead},
		{"POST /v1/tenant/billing/bank-transfers", h.Transfers, rbac.PermTenantBillingWrite},
		{"GET /v1/tenant/billing/cards", h.PaymentCards, rbac.PermTenantBillingRead},
		{"DELETE /v1/tenant/billing/cards/{card}", h.PaymentCards, rbac.PermTenantBillingWrite},
	} {
		mux.Handle(route.pattern, middleware.Chain(route.fn, authn, org, middleware.RequireTenantPermission(route.permission)))
	}
	mux.Handle("GET /v1/tenant/billing/usage", middleware.Chain(http.HandlerFunc(h.Usage), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingRead)))
	mux.Handle("GET /v1/tenant/billing/usage/daily", middleware.Chain(http.HandlerFunc(h.DailyUsage), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingRead)))
	mux.Handle("GET /v1/tenant/billing/subscription", middleware.Chain(http.HandlerFunc(h.Subscription), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingRead)))
	mux.Handle("POST /v1/tenant/billing/subscriptions/{subscription}/trial-invoice", middleware.Chain(http.HandlerFunc(h.TrialInvoice), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingWrite)))
	mux.Handle("POST /v1/tenant/billing/subscriptions/{subscription}/cancel", middleware.Chain(http.HandlerFunc(h.Cancel), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingWrite)))
	mux.Handle("GET /v1/tenant/entitlements", middleware.Chain(http.HandlerFunc(h.Entitlements), authn, org, middleware.RequireTenantPermission(rbac.PermTenantBillingRead)))
}

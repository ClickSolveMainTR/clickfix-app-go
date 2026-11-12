package billing

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of billing (CF-211): read-only. Plan changes, payments and
// cancellation stay on the billing page (payment forms, step-up); the
// assistant sends the user there with navigation_open_route.

// Capabilities are the billing capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/billing"}
	return []*capability.Capability{
		{
			Name: "billing.subscription", Module: "billing", Kind: capability.Read, Permission: rbac.PermTenantBillingRead, Surfaces: s, Routes: routes,
			Title: capability.T("Abonelik", "Subscription"), Description: "Current subscription: status, period, trial and cancellation.",
			Run: capability.Get("/v1/tenant/billing/subscription"),
		},
		{
			Name: "billing.usage", Module: "billing", Kind: capability.Read, Permission: rbac.PermTenantBillingRead, Surfaces: s, Routes: routes,
			Title: capability.T("Kullanım", "Usage"), Description: "Metered usage per period (current period first).",
			Run: capability.Get("/v1/tenant/billing/usage"),
		},
		{
			Name: "billing.invoices", Module: "billing", Kind: capability.Read, Permission: rbac.PermTenantBillingRead, Surfaces: s, Routes: routes,
			Title: capability.T("Faturalar", "Invoices"), Description: "Invoices with status, total (minor units) and due date.",
			Input: capability.Object(nil, capability.Paged(map[string]any{})),
			Run:   capability.Get("/v1/tenant/billing/invoices", "limit", "offset"),
		},
		{
			Name: "billing.wallet", Module: "billing", Kind: capability.Read, Permission: rbac.PermTenantBillingRead, Surfaces: s, Routes: routes,
			Title: capability.T("Cüzdan ve krediler", "Wallet and credits"), Description: "Wallet balance, AI credits, video minutes and recent ledger entries.",
			Input: capability.Object(nil, capability.Paged(map[string]any{})),
			Run:   capability.Get("/v1/tenant/billing/wallet", "limit", "offset"),
		},
	}
}

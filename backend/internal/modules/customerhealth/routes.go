// Package customerhealth mounts customer health scores (CF-87).
package customerhealth

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the health routes.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/customer-health/settings", h.GetSettings, rbac.PermTenantContactsRead)
	route("PUT /v1/tenant/customer-health/settings", h.PutSettings, rbac.PermTenantSettingsWrite)
	route("GET /v1/tenant/companies/{uuid}/health", h.GetHealth, rbac.PermTenantContactsRead)
	route("POST /v1/tenant/companies/{uuid}/health/recompute", h.Recompute, rbac.PermTenantContactsWrite)
	route("PUT /v1/tenant/company-usage", h.ReportUsage, rbac.PermTenantContactsWrite)
}

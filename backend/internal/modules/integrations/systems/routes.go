// Package systems is the connected system, "Sistemini bağla" (CF-197):
// concepts (custom object types), where each is read from, actions and
// AI playbooks on one screen.
package systems

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Base is the route prefix.
const Base = "/v1/tenant/integrations/system"

// RegisterRoutes mounts /v1/tenant/integrations/system/*. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm)))
	}
	read, manage, use := rbac.PermTenantIntegrationsRead, rbac.PermTenantIntegrationsManage, rbac.PermTenantIntegrationsUse
	route("GET "+Base, h.Overview, read)
	route("PUT "+Base+"/connection", h.SaveConnection, manage)
	route("PUT "+Base+"/sources/{uuid}", h.SaveSource, manage)
	route("DELETE "+Base+"/sources/{uuid}", h.DeleteSource, manage)
	route("POST "+Base+"/sources/{uuid}/test", h.TestSource, manage)
	route("PUT "+Base+"/actions/{key}", h.SaveAction, manage)
	route("DELETE "+Base+"/actions/{key}", h.DeleteAction, manage)
	route("POST "+Base+"/actions/{key}/run", h.RunAction, use)
	route("PUT "+Base+"/playbooks/{intent}", h.SavePlaybook, manage)
	route("DELETE "+Base+"/playbooks/{intent}", h.DeletePlaybook, manage)
	route("GET "+Base+"/concepts/{key}/lookup", h.Lookup, use)
	route("POST "+Base+"/preview", h.Preview, manage)
}

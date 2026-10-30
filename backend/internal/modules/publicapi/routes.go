package publicapi

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	auth := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		mux.Handle(pattern, middleware.Chain(fn, auth, org, middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/api-keys/meta", h.Meta, rbac.PermTenantAPIKeysManage)
	route("GET /v1/tenant/api-keys", h.Keys, rbac.PermTenantAPIKeysManage)
	route("POST /v1/tenant/api-keys", h.Keys, rbac.PermTenantAPIKeysManage)
	route("DELETE /v1/tenant/api-keys/{uuid}", h.Keys, rbac.PermTenantAPIKeysManage)
	route("GET /v1/tenant/integration-triggers", h.Triggers, rbac.PermTenantConversationsReadAll)
	route("GET /v1/tenant/integration-contacts", h.Contacts, rbac.PermTenantContactsRead)
	route("GET /v1/tenant/integration-companies", h.Companies, rbac.PermTenantContactsRead)
	mux.HandleFunc("/v1/public-api/{path...}", h.Public)
}

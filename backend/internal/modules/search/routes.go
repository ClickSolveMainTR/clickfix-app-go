package search

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	searchhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/search/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts command-palette search endpoints. The organization
// search (/v1/tenant/search) is mounted only when tenancy is on (resolver
// non-nil); per-spec tenant permissions are checked by the service.
func RegisterRoutes(
	mux *http.ServeMux,
	h *searchhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	authn := middleware.Authenticate(tokens, loader)
	session := middleware.RequirePermission(rbac.PermAuthSession)

	mux.Handle("GET /v1/search/specs", middleware.Chain(
		http.HandlerFunc(h.Specs), authn, session,
	))
	mux.Handle("GET /v1/search", middleware.Chain(
		http.HandlerFunc(h.Search), authn, session,
	))

	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	mux.Handle("GET /v1/tenant/search/specs", middleware.Chain(
		http.HandlerFunc(h.TenantSpecs), authn, org,
	))
	mux.Handle("GET /v1/tenant/search", middleware.Chain(
		http.HandlerFunc(h.TenantSearch), authn, org,
	))
}

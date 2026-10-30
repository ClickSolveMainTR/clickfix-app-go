package apps

import (
	"net/http"
	"slices"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, t *jwt.Manager, l middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/app-context/jwks", h.JWKS)
	if resolver == nil {
		return
	}
	auth := middleware.Authenticate(t, l)
	org := middleware.RequireOrganization(resolver)
	admin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := orgctx.Must(r.Context())
			if s.MemberRole != orgctx.RoleOwner && s.MemberRole != orgctx.RoleAdmin && !slices.Contains(s.MemberRoles, orgctx.RoleOwner) && !slices.Contains(s.MemberRoles, orgctx.RoleAdmin) {
				response.Forbidden(w, r, "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle("GET /v1/tenant/apps/meta", middleware.Chain(http.HandlerFunc(h.Meta), auth, org, middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)))
	for _, p := range []string{"GET /v1/tenant/apps", "GET /v1/tenant/apps/{uuid}"} {
		mux.Handle(p, middleware.Chain(http.HandlerFunc(h.CRUD), auth, org, middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)))
	}
	for _, p := range []string{"POST /v1/tenant/apps", "PUT /v1/tenant/apps/{uuid}", "DELETE /v1/tenant/apps/{uuid}"} {
		mux.Handle(p, middleware.Chain(http.HandlerFunc(h.CRUD), auth, org, admin, middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)))
	}
	mux.Handle("POST /v1/tenant/apps/{uuid}/launch", middleware.Chain(http.HandlerFunc(h.Launch), auth, org, middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)))
	mux.Handle("POST /v1/tenant/apps/{uuid}/actions", middleware.Chain(http.HandlerFunc(h.Action), auth, org, middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)))
}

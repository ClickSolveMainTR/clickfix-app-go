// Package macros mounts canned responses and macros (CF-48).
package macros

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes. Reading needs tenant.macros.read;
// personal items are managed with read, organization-wide ones need
// tenant.macros.write (checked by the usecase).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantMacrosRead)
	convRead := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/canned-responses", h.ListCanned, read)
	route("POST /v1/tenant/canned-responses", h.CreateCanned, read)
	route("PATCH /v1/tenant/canned-responses/{uuid}", h.UpdateCanned, read)
	route("DELETE /v1/tenant/canned-responses/{uuid}", h.DeleteCanned, read)
	route("POST /v1/tenant/canned-responses/{uuid}/render", h.RenderCanned, read, convRead)

	route("GET /v1/tenant/macros", h.ListMacros, read)
	route("POST /v1/tenant/macros", h.CreateMacro, read)
	route("PATCH /v1/tenant/macros/{uuid}", h.UpdateMacro, read)
	route("DELETE /v1/tenant/macros/{uuid}", h.DeleteMacro, read)
	route("POST /v1/tenant/conversations/{uuid}/macros/{macro}", h.ApplyMacro, read, convRead)
}

// Package aicore mounts guarded AI execution and masked run audit APIs.
package aicore

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("GET /v1/platform/organizations/{organization}/ai/runs", middleware.Chain(http.HandlerFunc(h.Runs), authn, middleware.RequirePermission(rbac.PermPlatformLogsRead)))
	mux.Handle("GET /v1/platform/organizations/{organization}/ai/runs/{uuid}", middleware.Chain(http.HandlerFunc(h.Get), authn, middleware.RequirePermission(rbac.PermPlatformLogsRead)))
	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	for _, route := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"GET /v1/tenant/ai/availability", h.Availability, rbac.PermTenantConversationsRead},
		{"GET /v1/tenant/ai/policy", h.Policy, rbac.PermTenantSettingsRead},
		{"PUT /v1/tenant/ai/policy", h.Policy, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/runs/meta", h.Meta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/runs", h.Runs, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/runs/{uuid}", h.Get, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/ai/runs", h.Run, rbac.PermTenantConversationsReply},
	} {
		mux.Handle(route.pattern, middleware.Chain(route.fn, authn, org, middleware.RequireTenantPermission(route.permission)))
	}
}

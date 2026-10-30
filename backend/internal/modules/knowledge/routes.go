// Package knowledge registers knowledge authoring and retrieval routes (CF-73/63).
package knowledge

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	for _, route := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"GET /v1/tenant/knowledge/articles/meta", h.Meta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/knowledge/articles", h.Articles, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/knowledge/articles", h.Articles, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/knowledge/articles/{uuid}", h.Article, rbac.PermTenantSettingsRead},
		{"PUT /v1/tenant/knowledge/articles/{uuid}", h.Article, rbac.PermTenantSettingsWrite},
		{"DELETE /v1/tenant/knowledge/articles/{uuid}", h.Article, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/knowledge/articles/{uuid}/versions", h.Versions, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/knowledge/articles/{uuid}/publish", h.Publish, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/knowledge/collections", h.Collections, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/knowledge/collections", h.Collections, rbac.PermTenantSettingsWrite},
		{"PUT /v1/tenant/knowledge/collections/{uuid}", h.Collections, rbac.PermTenantSettingsWrite},
		{"DELETE /v1/tenant/knowledge/collections/{uuid}", h.Collections, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/knowledge/search", h.Search, rbac.PermTenantSettingsRead},
	} {
		mux.Handle(route.pattern, middleware.Chain(route.fn, authn, org, middleware.RequireTenantPermission(route.permission)))
	}
}

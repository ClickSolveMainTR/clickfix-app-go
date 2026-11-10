package widget

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterConfigRoutes mounts the widget editor (CF-150): reading needs
// tenant.channels.read, editing the draft, duplicating and managing mobile
// app keys (CF-201) tenant.channels.manage, publishing and rollback
// tenant.widgets.publish.
func RegisterConfigRoutes(mux *http.ServeMux, h *handler.ConfigHandler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver, ent entitlement.Engine) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantChannelsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantChannelsManage)
	publish := middleware.RequireTenantPermission(rbac.PermTenantWidgetsPublish)
	writable := entitlement.Writable(ent)
	get := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, read))
	}
	write := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, read}, append(guards, writable)...)...))
	}
	get("GET /v1/tenant/widgets/{uuid}/config", h.Config)
	get("GET /v1/tenant/widgets/{uuid}/versions", h.Versions)
	get("GET /v1/tenant/widgets/{uuid}/versions/{version}", h.Version)
	get("GET /v1/tenant/widgets/{uuid}/installs", h.Installs)
	get("GET /v1/tenant/widgets/{uuid}/app-keys", h.AppKeys)
	write("POST /v1/tenant/widgets/{uuid}/app-keys", h.CreateAppKey, manage)
	write("DELETE /v1/tenant/widgets/{uuid}/app-keys/{key}", h.RevokeAppKey, manage)
	write("PUT /v1/tenant/widgets/{uuid}/draft", h.SaveDraft, manage)
	write("DELETE /v1/tenant/widgets/{uuid}/draft", h.DiscardDraft, manage)
	write("POST /v1/tenant/widgets/{uuid}/duplicate", h.Duplicate, manage)
	write("POST /v1/tenant/widgets/{uuid}/publish", h.Publish, manage, publish)
	write("POST /v1/tenant/widgets/{uuid}/versions/{version}/rollback", h.Rollback, manage, publish)
}

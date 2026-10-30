// Package mobileext mounts the mobile OS surface routes (CF-152): widget
// tokens and the widget API, Live Activity push tokens and settings.
// Contract: docs/mobile/widgets.md.
package mobileext

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// WidgetHandlers are existing tenant handlers reused under the widget
// authentication. Presence is presence handler.SetStatus; Counts is the
// conversations Counts handler already wrapped with its viewer middleware.
// A nil handler is not mounted.
type WidgetHandlers struct {
	Presence http.Handler
	Counts   http.Handler
}

// RegisterRoutes mounts /v1/mobile/*. Without a resolver (tenancy off) the
// organization-scoped widget routes are skipped.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, svc *usecase.Service, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver, widget WidgetHandlers) {
	authn := middleware.Authenticate(tokens, loader)

	// User-level, like /v1/push-devices: no organization header.
	mux.Handle("DELETE /v1/mobile/widget-token", middleware.Chain(http.HandlerFunc(h.RevokeWidgetToken), authn))
	mux.Handle("POST /v1/mobile/live-activity-tokens", middleware.Chain(http.HandlerFunc(h.RegisterLiveActivityToken), authn))
	mux.Handle("DELETE /v1/mobile/live-activity-tokens/{token}", middleware.Chain(http.HandlerFunc(h.DeleteLiveActivityToken), authn))
	mux.Handle("GET /v1/mobile/live-activity-settings", middleware.Chain(http.HandlerFunc(h.GetLiveActivitySettings), authn))
	mux.Handle("PUT /v1/mobile/live-activity-settings", middleware.Chain(http.HandlerFunc(h.PutLiveActivitySettings), authn))

	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	mux.Handle("POST /v1/mobile/widget-token", middleware.Chain(http.HandlerFunc(h.IssueWidgetToken), authn, org))

	// Widget API: only `Authorization: Widget <token>`; membership and
	// permissions are resolved again on every request.
	widgetChain := []func(http.Handler) http.Handler{
		handler.AuthenticateWidget(svc, loader), h.RateLimitWidget, org, handler.RequireWidgetOrganization,
	}
	if widget.Presence != nil {
		mux.Handle("PUT /v1/mobile/widget/presence", middleware.Chain(widget.Presence, widgetChain...))
	}
	if widget.Counts != nil {
		read := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
		mux.Handle("GET /v1/mobile/widget/counts", middleware.Chain(widget.Counts, append(widgetChain, read)...))
	}
}

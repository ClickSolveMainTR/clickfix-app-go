// Package presence mounts agent presence and typing routes (CF-10).
package presence

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes; any member may read and set presence,
// typing needs tenant.conversations.read. A nil resolver mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	readConv := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	mux.Handle("GET /v1/tenant/presence", middleware.Chain(http.HandlerFunc(h.List), authn, org))
	mux.Handle("PUT /v1/tenant/presence/me", middleware.Chain(http.HandlerFunc(h.SetStatus), authn, org))
	mux.Handle("POST /v1/tenant/presence/idle", middleware.Chain(http.HandlerFunc(h.Idle), authn, org))
	mux.Handle("POST /v1/tenant/presence/heartbeat", middleware.Chain(http.HandlerFunc(h.Heartbeat), authn, org))
	mux.Handle("POST /v1/tenant/realtime/typing", middleware.Chain(http.HandlerFunc(h.Typing), authn, org, readConv))
}

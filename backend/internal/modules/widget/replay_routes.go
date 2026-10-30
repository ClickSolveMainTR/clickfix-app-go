package widget

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterAgentRoutes protects playback and co-browse with explicit permissions in addition to inbox visibility.
func RegisterAgentRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/public/widget-replays/{token}", h.ReplayDownload)
	if resolver == nil {
		return
	}
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		mux.Handle(pattern, middleware.Chain(
			fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(resolver),
			middleware.RequireTenantPermission(rbac.PermTenantConversationsRead),
			middleware.RequireTenantPermission(perm),
		))
	}
	route("GET /v1/tenant/conversations/{uuid}/sessions", h.AgentSessions, rbac.PermTenantReplaysRead)
	route("POST /v1/tenant/conversations/{uuid}/cobrowse", h.RequestCobrowse, rbac.PermTenantCobrowseUse)
	route("GET /v1/tenant/conversations/{uuid}/sessions/{session}/manifest", h.ReplayManifest, rbac.PermTenantReplaysRead)
	route("POST /v1/tenant/conversations/{uuid}/sessions/{session}/live-token", h.AgentLiveToken, rbac.PermTenantCobrowseUse)
	route("POST /v1/tenant/conversations/{uuid}/sessions/{session}/pointer", h.AgentPointer, rbac.PermTenantCobrowseUse)
	route("POST /v1/tenant/conversations/{uuid}/sessions/{session}/stop", h.AgentStop, rbac.PermTenantCobrowseUse)
}

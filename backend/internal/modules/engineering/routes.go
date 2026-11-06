// Package engineering mounts the native tracker and external provider bridge.
package engineering

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"net/http"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	auth := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantTicketsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantTicketsWrite)
	convRead := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	convAll := middleware.RequireTenantPermission(rbac.PermTenantConversationsReadAll)
	manage := middleware.RequireTenantPermission(rbac.PermTenantConversationsManage)
	settings := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	// The list is masked (no secrets), so settings readers may see status.
	settingsRead := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		chain := append([]func(http.Handler) http.Handler{auth, org}, guards...)
		chain = append(chain, h.WithViewer)
		mux.Handle(pattern, middleware.Chain(fn, chain...))
	}
	route("GET /v1/tenant/engineering/issues/meta", h.Meta, read)
	route("GET /v1/tenant/engineering/issues", h.List, read)
	route("POST /v1/tenant/engineering/issues", h.Create, write, convRead)
	route("GET /v1/tenant/engineering/issues/{issue}", h.Get, read)
	route("PATCH /v1/tenant/engineering/issues/{issue}", h.Patch, write)
	route("GET /v1/tenant/engineering/issues/{issue}/conversations", h.Links, read, convRead)
	route("POST /v1/tenant/engineering/issues/{issue}/conversations", h.Link, write, convRead)
	route("DELETE /v1/tenant/engineering/issues/{issue}/conversations/{conversation}", h.Unlink, write, convRead)
	route("GET /v1/tenant/engineering/issues/{issue}/comments", h.Comments, read)
	route("POST /v1/tenant/engineering/issues/{issue}/comments", h.Comment, write)
	route("GET /v1/tenant/engineering/issues/{issue}/impact", h.Impact, read, convAll)
	route("GET /v1/tenant/engineering/issues/{issue}/resolution-preview", h.Preview, read, convRead)
	route("GET /v1/tenant/engineering/issues/{issue}/deliveries", h.Deliveries, read, convAll)
	route("GET /v1/tenant/engineering/issues/{issue}/external-links", h.ExternalLinks, read)
	route("POST /v1/tenant/engineering/issues/{issue}/external-links", h.ConnectIssue, write, settings)
	route("GET /v1/tenant/engineering/projects", h.Projects, read)
	route("POST /v1/tenant/engineering/projects", h.CreateProject, write)
	route("GET /v1/tenant/engineering/releases", h.Releases, read)
	route("POST /v1/tenant/engineering/releases", h.CreateRelease, write)
	route("POST /v1/tenant/engineering/releases/{release}/publish", h.PublishRelease, write)
	route("GET /v1/tenant/engineering/conversations/{conversation}/suggestions", h.Suggestions, read, convRead)
	route("GET /v1/tenant/engineering/conversations/{conversation}/escalations", h.Escalations, read, convRead)
	route("POST /v1/tenant/engineering/conversations/{conversation}/escalations", h.Escalate, write, convRead, manage)
	route("GET /v1/tenant/engineering/conversations/{conversation}/errors", h.Errors, read, convRead)
	route("POST /v1/tenant/engineering/conversations/{conversation}/errors", h.AttachError, write, convRead)
	route("GET /v1/tenant/engineering/integrations", h.Integrations, settingsRead)
	route("PUT /v1/tenant/engineering/integrations/{provider}", h.SaveIntegration, settings)
	route("POST /v1/tenant/engineering/integrations/{provider}/oauth/start", h.StartOAuth, settings)
	route("POST /v1/tenant/engineering/integrations/{provider}/oauth/complete", h.CompleteOAuth, settings)
	mux.HandleFunc("POST /v1/webhooks/engineering/{integration}", h.Webhook)
}

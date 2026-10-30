// Package analytics mounts organization-wide metrics. Read-all is required to avoid leaking other teams.
package analytics

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"net/http"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	route := func(p string, fn http.HandlerFunc, perm string) {
		mux.Handle(p, middleware.Chain(fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(rbac.PermTenantConversationsRead), middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/analytics/dashboard", h.Dashboard, rbac.PermTenantConversationsReadAll)
	route("GET /v1/tenant/analytics/export/{format}", h.Dashboard, rbac.PermTenantConversationsReadAll)
	route("POST /v1/tenant/analytics/backfill", h.Backfill, rbac.PermTenantConversationsManage)
}
func EventConsumer(s *usecase.Service) eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "analytics_projection", Events: []string{"conversations.*", "conversation.*", "messages.*", "message.*", "sla.*", "surveys.*"}, MaxRetry: 8, Timeout: time.Minute, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil || ev.EntityType != "conversation" || ev.EntityID == nil {
			return nil
		}
		return s.Sync(ctx, *ev.TenantID, *ev.EntityID)
	}}
}

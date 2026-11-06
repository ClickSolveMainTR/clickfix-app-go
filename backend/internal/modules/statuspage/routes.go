// Package statuspage exposes public availability and tenant incident management.
package statuspage

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"net/http"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/public/status/{slug}", h.Public)
	mux.HandleFunc("GET /v1/public/status/{slug}/history", h.History)
	mux.HandleFunc("GET /v1/public/status/{slug}/rss", h.RSS)
	mux.HandleFunc("POST /v1/public/status/{slug}/subscriptions", h.Subscribe)
	mux.HandleFunc("GET /v1/public/status/{slug}/subscriptions/{token}", h.SubscriptionPreview)
	mux.HandleFunc("POST /v1/public/status/{slug}/subscriptions/{token}", h.SubscriptionAction)
	if resolver == nil {
		return
	}
	route := func(p string, fn http.HandlerFunc, write bool) {
		perm := rbac.PermTenantConversationsRead
		if write {
			perm = rbac.PermTenantConversationsManage
		}
		mux.Handle(p, middleware.Chain(fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(rbac.PermTenantConversationsReadAll), middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/status/page", h.Page, false)
	route("PUT /v1/tenant/status/page", h.Page, true)
	route("GET /v1/tenant/status/components", h.Components, false)
	route("POST /v1/tenant/status/components", h.Components, true)
	route("PATCH /v1/tenant/status/components/{uuid}", h.Components, true)
	// Agents read incidents without organization-wide conversation access
	// (CF-162): they answer differently during an outage. Managing stays above.
	mux.Handle("GET /v1/tenant/status/incidents", middleware.Chain(http.HandlerFunc(h.Incidents), middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)))
	route("POST /v1/tenant/status/incidents", h.Incidents, true)
	route("POST /v1/tenant/status/incidents/{uuid}/updates", h.Update, true)
	route("POST /v1/tenant/status/incidents/{uuid}/conversations", h.Link, true)
	route("GET /v1/tenant/status/incidents/{uuid}/notification-preview", h.Preview, true)
}
func EventConsumer(s *usecase.Service) eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "status_notifications", Events: []string{"status.*"}, MaxRetry: 8, Timeout: 10 * time.Minute, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil || ev.EntityUUID == nil {
			return nil
		}
		switch ev.Name {
		case "status.subscription_requested":
			return s.SendConfirmation(ctx, *ev.TenantID, *ev.EntityUUID)
		case "status.notify_contacts":
			return s.NotifyContacts(ctx, *ev.TenantID, *ev.EntityUUID)
		case "status.changed":
			public, _ := ev.Payload["public"].(bool)
			if !public {
				return nil
			}
			body, _ := ev.Payload["body"].(string)
			return s.NotifySubscribers(ctx, *ev.TenantID, *ev.EntityUUID, body)
		}
		return nil
	}}
}

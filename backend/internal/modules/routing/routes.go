// Package routing mounts automatic routing (CF-53) and agent capacity
// (CF-54), and provides the durable consumer that routes on conversation
// and presence events.
package routing

import (
	"context"
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// ConsumerName is the durable routing consumer.
const ConsumerName = "routing"

// RegisterRoutes mounts the endpoints. Routing belongs to team operations:
// read with tenant.teams.read, change with tenant.teams.write; the live
// capacity view is for supervisors (tenant.conversations.read_all).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantTeamsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantTeamsWrite)
	supervise := middleware.RequireTenantPermission(rbac.PermTenantConversationsReadAll)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/routing/settings", h.Settings, read)
	route("PUT /v1/tenant/routing/settings", h.SaveSettings, write)
	route("GET /v1/tenant/routing/rules", h.Rules, read)
	route("POST /v1/tenant/routing/rules", h.CreateRule, write)
	route("PATCH /v1/tenant/routing/rules/{uuid}", h.UpdateRule, write)
	route("DELETE /v1/tenant/routing/rules/{uuid}", h.DeleteRule, write)
	route("GET /v1/tenant/routing/events", h.Events, read)

	route("GET /v1/tenant/capacity", h.Capacity, supervise)
	route("GET /v1/tenant/capacity/limits", h.Limits, read)
	route("PUT /v1/tenant/capacity/limits", h.SetLimits, write)
}

// EventConsumer routes new and re-queued conversations and reacts to agents
// going offline (hand over) or online (drain the queue).
func EventConsumer(svc *usecase.Service) eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name: ConsumerName,
		Events: []string{
			events.ConversationsCreated, events.ConversationsReopened, events.ConversationsUpdated,
			events.ConversationsAssigned, events.MessagesReceived, events.AgentPresenceChanged,
		},
		MaxRetry: 5,
		Timeout:  time.Minute,
		Handle:   func(ctx context.Context, ev events.Event) error { return svc.HandleEvent(ctx, ev) },
	}
}

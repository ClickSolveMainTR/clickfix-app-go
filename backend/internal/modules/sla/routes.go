// Package sla mounts the SLA module (CF-46): business hours, holidays, SLA
// policies (/v1/tenant/sla/*), the SLA clocks of a conversation and the
// durable event consumer that keeps the clocks in step with the inbox.
package sla

import (
	"context"
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// ConsumerName is the durable consumer of conversation events.
const ConsumerName = "sla_clocks"

// RegisterRoutes mounts the SLA endpoints. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSlaRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantSlaWrite)
	convRead := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/sla/business-hours", h.ListBusinessHours, read)
	route("POST /v1/tenant/sla/business-hours", h.CreateBusinessHours, write)
	route("GET /v1/tenant/sla/business-hours/{uuid}", h.GetBusinessHours, read)
	route("PATCH /v1/tenant/sla/business-hours/{uuid}", h.UpdateBusinessHours, write)
	route("DELETE /v1/tenant/sla/business-hours/{uuid}", h.DeleteBusinessHours, write)

	route("GET /v1/tenant/sla/holidays", h.ListHolidays, read)
	route("POST /v1/tenant/sla/holidays", h.CreateHoliday, write)
	route("DELETE /v1/tenant/sla/holidays/{uuid}", h.DeleteHoliday, write)
	route("GET /v1/tenant/sla/public-holidays", h.PublicHolidays, read)

	route("GET /v1/tenant/sla/policies", h.ListPolicies, read)
	route("POST /v1/tenant/sla/policies", h.CreatePolicy, write)
	route("GET /v1/tenant/sla/policies/{uuid}", h.GetPolicy, read)
	route("PATCH /v1/tenant/sla/policies/{uuid}", h.UpdatePolicy, write)
	route("DELETE /v1/tenant/sla/policies/{uuid}", h.DeletePolicy, write)

	route("GET /v1/tenant/sla/companies", h.ContractedCompanies, read)
	route("PUT /v1/tenant/sla/companies/{uuid}", h.SetCompanyPolicy, write)

	route("GET /v1/tenant/conversations/{uuid}/sla", h.ConversationSLA, convRead)
}

// EventConsumer keeps the SLA clocks in step with the inbox: every
// conversation or message event re-syncs the conversation's clocks at the
// event time (Sync is idempotent, so replays and reordering are harmless).
func EventConsumer(svc *usecase.Service) eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name: ConsumerName,
		Events: []string{
			"conversations.*", events.MessagesReceived, events.MessagesSent,
			"conversation.*", events.MessageReceived, events.MessageSent, "brands.updated",
		},
		MaxRetry: 8,
		Timeout:  time.Minute,
		Handle: func(ctx context.Context, ev events.Event) error {
			if ev.TenantID != nil && ev.Name == "brands.updated" {
				return svc.ResyncOrganization(ctx, *ev.TenantID)
			}
			if ev.TenantID == nil || ev.EntityType != "conversation" || ev.EntityID == nil {
				return nil
			}
			return svc.Sync(ctx, *ev.TenantID, *ev.EntityID, ev.OccurredAt)
		},
	}
}

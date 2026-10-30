package surveys

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"net/http"
	"time"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/public/surveys/{token}", h.Public)
	mux.HandleFunc("POST /v1/public/surveys/{token}", h.Public)
	if resolver == nil {
		return
	}
	route := func(p string, fn http.HandlerFunc, perm string) {
		mux.Handle(p, middleware.Chain(fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(rbac.PermTenantConversationsReadAll), middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/surveys/templates", h.Templates, rbac.PermTenantConversationsRead)
	route("POST /v1/tenant/surveys/templates", h.SaveTemplate, rbac.PermTenantConversationsManage)
	route("PATCH /v1/tenant/surveys/templates/{uuid}", h.SaveTemplate, rbac.PermTenantConversationsManage)
	route("POST /v1/tenant/surveys/invitations", h.Invite, rbac.PermTenantConversationsManage)
	route("GET /v1/tenant/surveys/results", h.Results, rbac.PermTenantConversationsRead)
}
func EventConsumer(s *usecase.Service) eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "survey_delivery", Events: []string{events.ConversationsResolved, events.ConversationResolved, "surveys.invited"}, MaxRetry: 8, Timeout: 5 * time.Minute, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil || ev.EntityType != "conversation" || ev.EntityID == nil {
			return nil
		}
		if ev.Name == "surveys.invited" {
			v, _ := ev.Payload["invitation_uuid"].(string)
			id, err := uuid.Parse(v)
			if err != nil {
				return err
			}
			return s.Deliver(ctx, *ev.TenantID, id)
		}
		return s.Resolve(ctx, *ev.TenantID, *ev.EntityID)
	}}
}

package webhooks

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"net/http"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, t *jwt.Manager, l middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	auth := middleware.Authenticate(t, l)
	org := middleware.RequireOrganization(resolver)
	route := func(p string, fn http.HandlerFunc, perm string) {
		mux.Handle(p, middleware.Chain(fn, auth, org, middleware.RequireTenantPermission(perm)))
	}
	route("GET /v1/tenant/webhooks/meta", h.Meta, rbac.PermTenantWebhooksRead)
	route("GET /v1/tenant/webhooks/events", h.Events, rbac.PermTenantWebhooksRead)
	route("GET /v1/tenant/webhooks", h.CRUD, rbac.PermTenantWebhooksRead)
	route("POST /v1/tenant/webhooks", h.CRUD, rbac.PermTenantWebhooksWrite)
	route("PUT /v1/tenant/webhooks/{uuid}", h.CRUD, rbac.PermTenantWebhooksWrite)
	route("DELETE /v1/tenant/webhooks/{uuid}", h.CRUD, rbac.PermTenantWebhooksWrite)
	route("POST /v1/tenant/webhooks/{uuid}/test", h.Test, rbac.PermTenantWebhooksWrite)
	route("GET /v1/tenant/webhook-deliveries", h.Deliveries, rbac.PermTenantWebhooksRead)
	route("POST /v1/tenant/webhook-deliveries/{uuid}/replay", h.Deliveries, rbac.PermTenantWebhooksWrite)
}

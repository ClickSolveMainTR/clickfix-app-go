package automation

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/handler"
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
	if h.Service.Workflows != nil {
		route("GET /v1/tenant/workflows/meta", h.WorkflowMeta, rbac.PermTenantAutomationRead)
		route("GET /v1/tenant/workflows", h.Workflows, rbac.PermTenantAutomationRead)
		route("GET /v1/tenant/workflows/{uuid}", h.Workflows, rbac.PermTenantAutomationRead)
		route("POST /v1/tenant/workflows", h.Workflows, rbac.PermTenantAutomationWrite)
		route("PUT /v1/tenant/workflows/{uuid}", h.Workflows, rbac.PermTenantAutomationWrite)
		route("POST /v1/tenant/workflow-imports/routing", h.WorkflowImport, rbac.PermTenantAutomationWrite)
		route("GET /v1/tenant/workflow-runs/{run}", h.WorkflowRun, rbac.PermTenantAutomationRead)
		for _, command := range []string{"versions", "runs"} {
			cmd := command
			route("GET /v1/tenant/workflows/{uuid}/"+cmd, func(w http.ResponseWriter, r *http.Request) { r.SetPathValue("command", cmd); h.WorkflowCommand(w, r) }, rbac.PermTenantAutomationRead)
		}
		for _, command := range []string{"publish", "enable", "test", "webhook"} {
			cmd := command
			route("POST /v1/tenant/workflows/{uuid}/"+cmd, func(w http.ResponseWriter, r *http.Request) { r.SetPathValue("command", cmd); h.WorkflowCommand(w, r) }, rbac.PermTenantAutomationWrite)
		}
	}

	route("GET /v1/tenant/automations/meta", h.Meta, rbac.PermTenantAutomationRead)
	route("GET /v1/tenant/automations/triggers", h.Triggers, rbac.PermTenantAutomationRead)
	route("GET /v1/tenant/automations/executions", h.Executions, rbac.PermTenantAutomationRead)
	route("GET /v1/tenant/automations", h.CRUD, rbac.PermTenantAutomationRead)
	route("GET /v1/tenant/automations/{uuid}", h.CRUD, rbac.PermTenantAutomationRead)
	route("POST /v1/tenant/automations", h.CRUD, rbac.PermTenantAutomationWrite)
	route("PUT /v1/tenant/automations/{uuid}", h.CRUD, rbac.PermTenantAutomationWrite)
	route("DELETE /v1/tenant/automations/{uuid}", h.CRUD, rbac.PermTenantAutomationWrite)
	route("POST /v1/tenant/automations/{uuid}/test", h.DryRun, rbac.PermTenantAutomationWrite)
}

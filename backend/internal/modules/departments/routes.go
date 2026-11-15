// Package departments mounts organization departments (CF-215):
// /v1/tenant/departments, knowledge sharing between departments and the
// department of a conversation.
package departments

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the department routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantDepartmentsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantDepartmentsWrite)
	convRead := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/departments/meta", h.MetaH, read)
	route("GET /v1/tenant/departments", h.List, read)
	route("POST /v1/tenant/departments", h.Create, read, write)
	route("GET /v1/tenant/departments/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/departments/{uuid}", h.Patch, read, write)
	route("DELETE /v1/tenant/departments/{uuid}", h.Delete, read, write)
	// Leads share their department's knowledge; the use case checks the lead.
	route("GET /v1/tenant/departments/{uuid}/shares", h.Shares, read)
	route("POST /v1/tenant/departments/{uuid}/shares", h.Share, read)
	route("DELETE /v1/tenant/departments/{uuid}/shares/{shareUuid}", h.Unshare, read)
	route("GET /v1/tenant/conversations/{uuid}/department", h.ConversationDepartment, convRead, read)
	route("POST /v1/tenant/conversations/{uuid}/department-transfer", h.Transfer, convRead, read)
}

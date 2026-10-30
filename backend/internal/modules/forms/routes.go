// Package forms mounts dynamic request forms (CF-52):
// /v1/tenant/request-forms and /v1/tenant/conversations/{uuid}/form.
package forms

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the form endpoints. Forms are a view over custom
// fields, so they reuse the custom field permissions. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantCustomFieldsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantCustomFieldsManage)
	convRead := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	convManage := middleware.RequireTenantPermission(rbac.PermTenantConversationsManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/request-forms", h.List, read)
	route("POST /v1/tenant/request-forms", h.Create, manage)
	route("GET /v1/tenant/request-forms/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/request-forms/{uuid}", h.Update, manage)
	route("DELETE /v1/tenant/request-forms/{uuid}", h.Delete, manage)

	route("GET /v1/tenant/conversations/{uuid}/form", h.ConversationForm, convRead)
	route("PATCH /v1/tenant/conversations/{uuid}/form", h.UpdateConversationForm, convRead, convManage)
}

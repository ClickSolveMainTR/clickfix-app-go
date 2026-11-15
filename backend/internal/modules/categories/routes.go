// Package categories mounts the category taxonomy (CF-218):
// /v1/tenant/categories (tree, review queue, label export) and
// /v1/tenant/category-assignments (a record's category, confirm/correct).
package categories

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the category routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantCategoriesRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantCategoriesManage)
	label := middleware.RequireTenantPermission(rbac.PermTenantCategoriesLabel)
	export := middleware.RequireTenantPermission(rbac.PermTenantCategoriesExport)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/categories/meta", h.MetaH, read)
	route("GET /v1/tenant/categories/review", h.Review, read, label)
	route("GET /v1/tenant/categories/labels/export", h.ExportLabels, read, export)
	route("GET /v1/tenant/categories", h.List, read)
	route("POST /v1/tenant/categories", h.Create, read, manage)
	route("GET /v1/tenant/categories/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/categories/{uuid}", h.Patch, read, manage)
	route("DELETE /v1/tenant/categories/{uuid}", h.Delete, read, manage)

	route("GET /v1/tenant/category-assignments/{subjectType}/{subjectUuid}", h.GetAssignment, read)
	route("PUT /v1/tenant/category-assignments/{subjectType}/{subjectUuid}", h.PutAssignment, read, label)
	route("DELETE /v1/tenant/category-assignments/{subjectType}/{subjectUuid}", h.DeleteAssignment, read, label)
	route("POST /v1/tenant/category-assignments/{subjectType}/{subjectUuid}/classify", h.Classify, read, label)
}

// Package customfields mounts the custom field and custom object routes (CF-7).
package customfields

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/custom-fields/* and /v1/tenant/custom-objects/*.
// A nil resolver (tenancy off) mounts nothing.
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
	read := middleware.RequireTenantPermission(rbac.PermTenantCustomFieldsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantCustomFieldsManage)
	recRead := middleware.RequireTenantPermission(rbac.PermTenantCustomObjectsRead)
	recWrite := middleware.RequireTenantPermission(rbac.PermTenantCustomObjectsWrite)
	route := func(pattern string, fn http.HandlerFunc, perm func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, perm))
	}

	route("GET /v1/tenant/custom-fields/meta", h.Meta, read)
	contactsRead := middleware.RequireTenantPermission(rbac.PermTenantContactsRead)
	route("GET /v1/tenant/custom-fields/unknown-keys", h.ListUnknownKeys, read)
	route("POST /v1/tenant/custom-fields/unknown-keys/{uuid}/dismiss", h.DismissUnknownKey, manage)
	mux.Handle("GET /v1/tenant/custom-fields/values", middleware.Chain(http.HandlerFunc(h.Values), authn, org, read, contactsRead))
	route("GET /v1/tenant/custom-fields", h.ListDefinitions, read)
	route("POST /v1/tenant/custom-fields", h.CreateDefinition, manage)
	route("PUT /v1/tenant/custom-fields/order", h.Reorder, manage)
	route("GET /v1/tenant/custom-fields/{uuid}", h.GetDefinition, read)
	route("PATCH /v1/tenant/custom-fields/{uuid}", h.UpdateDefinition, manage)
	route("DELETE /v1/tenant/custom-fields/{uuid}", h.ArchiveDefinition, manage)
	route("POST /v1/tenant/custom-fields/{uuid}/restore", h.RestoreDefinition, manage)

	route("GET /v1/tenant/custom-objects", h.ListObjectTypes, read)
	route("POST /v1/tenant/custom-objects", h.CreateObjectType, manage)
	route("GET /v1/tenant/custom-objects/{uuid}", h.GetObjectType, read)
	route("PATCH /v1/tenant/custom-objects/{uuid}", h.UpdateObjectType, manage)
	route("GET /v1/tenant/custom-objects/{uuid}/records", h.ListRecords, recRead)
	route("POST /v1/tenant/custom-objects/{uuid}/records", h.CreateRecord, recWrite)
	route("GET /v1/tenant/custom-objects/{uuid}/records/{record_uuid}", h.GetRecord, recRead)
	route("PATCH /v1/tenant/custom-objects/{uuid}/records/{record_uuid}", h.UpdateRecord, recWrite)
	route("DELETE /v1/tenant/custom-objects/{uuid}/records/{record_uuid}", h.DeleteRecord, recWrite)
}

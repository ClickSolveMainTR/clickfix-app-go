// Package history mounts the history pipeline (CF-217): past support records
// become knowledge suggestions through a read-only archive.
package history

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Base is the route prefix.
const Base = "/v1/tenant/history"

// RegisterRoutes mounts /v1/tenant/history/*. A nil resolver (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, middleware.RequireTenantPermission(perm)))
	}
	read, manage, review := rbac.PermTenantHistoryRead, rbac.PermTenantHistoryManage, rbac.PermTenantHistoryReview
	route("GET "+Base+"/imports", h.Imports, read)
	route("POST "+Base+"/imports", h.Upload, manage)
	route("POST "+Base+"/imports/estimate", h.Estimate, manage)
	route("GET "+Base+"/imports/{uuid}", h.Import, read)
	route("POST "+Base+"/imports/{uuid}/cancel", h.Cancel, manage)
	route("POST "+Base+"/imports/{uuid}/rollback", h.Rollback, manage)
	route("GET "+Base+"/cases", h.Cases, read)
	route("GET "+Base+"/clusters", h.Clusters, read)
	route("GET "+Base+"/suggestions", h.Suggestions, read)
	route("POST "+Base+"/suggestions/{uuid}/decision", h.Decide, review)
	route("GET "+Base+"/decisions", h.Decisions, read)
	route("GET "+Base+"/settings", h.Settings, read)
	route("PUT "+Base+"/settings", h.Settings, manage)
	route("GET "+Base+"/sample", h.SampleFile, read)
}

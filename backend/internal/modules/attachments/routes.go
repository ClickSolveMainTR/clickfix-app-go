// Package attachments mounts the organization attachment routes (CF-11).
package attachments

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/attachments/* (membership + tenant
// permission) and the token-authenticated /v1/public/attachments/{token}. A
// nil resolver (tenancy off) mounts nothing: attachments are always owned by
// an organization.
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
	read := middleware.RequireTenantPermission(rbac.PermTenantAttachmentsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantAttachmentsWrite)

	mux.Handle("GET /v1/tenant/attachments", middleware.Chain(
		http.HandlerFunc(h.List), authn, org, read,
	))
	mux.Handle("GET /v1/tenant/attachments/usage", middleware.Chain(
		http.HandlerFunc(h.Usage), authn, org, read,
	))
	mux.Handle("POST /v1/tenant/attachments", middleware.Chain(
		http.HandlerFunc(h.Upload), authn, org, write,
	))
	mux.Handle("GET /v1/tenant/attachments/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, org, read,
	))
	mux.Handle("GET /v1/tenant/attachments/{uuid}/download", middleware.Chain(
		http.HandlerFunc(h.Download), authn, org, read,
	))
	mux.Handle("POST /v1/tenant/attachments/{uuid}/signed-url", middleware.Chain(
		http.HandlerFunc(h.SignedURL), authn, org, read,
	))
	mux.Handle("DELETE /v1/tenant/attachments/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Delete), authn, org, write,
	))
	mux.Handle("GET /v1/public/attachments/{token}", http.HandlerFunc(h.PublicDownload))
}

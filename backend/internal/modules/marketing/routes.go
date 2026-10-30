package marketing

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the platform settings (platform.settings.*), the
// sources report and organization attribution (platform.organizations.read)
// and the public read used by the marketing site.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	route := func(pattern string, fn http.HandlerFunc, slug string) {
		mux.Handle(pattern, middleware.Chain(fn, authn, middleware.RequirePermission(slug)))
	}
	route("GET /v1/platform/marketing", h.Get, rbac.PermPlatformSettingsRead)
	route("PUT /v1/platform/marketing", h.Put, rbac.PermPlatformSettingsWrite)
	route("PUT /v1/platform/marketing/og-image", h.UploadOGImage, rbac.PermPlatformSettingsWrite)
	route("DELETE /v1/platform/marketing/og-image", h.DeleteOGImage, rbac.PermPlatformSettingsWrite)
	route("GET /v1/platform/marketing/sources", h.Sources, rbac.PermPlatformOrganizationsRead)
	route("GET /v1/platform/organizations/{uuid}/attribution", h.OrganizationAttribution, rbac.PermPlatformOrganizationsRead)

	mux.HandleFunc("GET /v1/public/marketing", h.Public)
	mux.HandleFunc("GET /v1/public/marketing/og-image", h.StreamOGImage)
}

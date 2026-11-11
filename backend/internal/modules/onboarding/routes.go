package onboarding

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts self-serve organization sign-up (any signed-in
// account; answers SELF_SERVE_DISABLED while the platform keeps it off)
// and the set-up checklist of the organization in scope. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("POST /v1/onboarding/organizations", middleware.Chain(http.HandlerFunc(h.CreateOrganization), authn))

	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/onboarding", h.Get, read)
	route("POST /v1/tenant/onboarding/steps/{step}/skip", h.SkipStep, read, write)
	route("POST /v1/tenant/onboarding/finish", h.Finish, read, write)
}

// RegisterSetupRoutes mounts the AI-assisted set-up and template gallery
// (CF-140) for the organization in scope. Everything needs
// tenant.onboarding.manage: the runs create and remove workspace-wide
// configuration.
func RegisterSetupRoutes(mux *http.ServeMux, h *SetupHandler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	manage := middleware.RequireTenantPermission(rbac.PermTenantOnboardingManage)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, manage))
	}
	route("GET /v1/tenant/onboarding/setup/templates", h.Templates)
	route("POST /v1/tenant/onboarding/setup/templates/{key}", h.UseTemplate)
	route("POST /v1/tenant/onboarding/setup/discover", h.Discover)
	route("GET /v1/tenant/onboarding/setup/latest", h.Latest)
	route("GET /v1/tenant/onboarding/setup/runs/{uuid}", h.Get)
	route("POST /v1/tenant/onboarding/setup/runs/{uuid}/apply", h.Apply)
	route("POST /v1/tenant/onboarding/setup/runs/{uuid}/undo", h.Undo)
}

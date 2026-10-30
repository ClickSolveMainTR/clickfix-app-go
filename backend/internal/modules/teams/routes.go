// Package teams mounts organization teams and team membership (CF-5):
// /v1/tenant/teams, member skill tags and the skill catalog.
package teams

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the team routes. A nil resolver (tenancy off) mounts
// nothing.
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
	read := middleware.RequireTenantPermission(rbac.PermTenantTeamsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantTeamsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/teams/meta", h.MetaH, read)
	route("GET /v1/tenant/teams", h.List, read)
	route("POST /v1/tenant/teams", h.Create, read, write)
	route("GET /v1/tenant/teams/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/teams/{uuid}", h.Patch, read, write)
	route("DELETE /v1/tenant/teams/{uuid}", h.Delete, read, write)
	route("PUT /v1/tenant/teams/{uuid}/members/{userUuid}", h.PutMember, read, write)
	route("DELETE /v1/tenant/teams/{uuid}/members/{userUuid}", h.DeleteMember, read, write)
	route("PUT /v1/tenant/members/{userUuid}/skills", h.PutMemberSkills, read, write)
	route("GET /v1/tenant/skills", h.Skills, read)
}

package closingtexts

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// RegisterRoutes mounts the closing text routes. Agents who can read
// conversations list them; managing them needs tenant.macros.write (the
// same people who manage organization-wide saved replies), so no new slug.
func RegisterRoutes(r supportops.Router, s *Service) {
	route := func(pattern, perm string, fn supportops.Endpoint) {
		r.Tenant(pattern, perm, func(req *http.Request) (any, error) {
			if raw := req.PathValue("uuid"); raw != "" {
				if id, err := uuid.Parse(raw); err != nil || id == uuid.Nil {
					return nil, supportops.ErrInvalid
				}
			}
			return fn(req)
		})
	}
	org := func(req *http.Request) int64 { return orgctx.Must(req.Context()).ID }
	route("GET /v1/tenant/closing-texts", rbac.PermTenantConversationsRead, func(req *http.Request) (any, error) {
		return s.List(req.Context(), org(req))
	})
	route("GET /v1/tenant/closing-texts/{uuid}", rbac.PermTenantConversationsRead, func(req *http.Request) (any, error) {
		return s.Get(req.Context(), org(req), supportops.ID(req))
	})
	save := func(req *http.Request) (any, error) {
		in, err := supportops.Read[Input](req)
		if err != nil {
			return nil, err
		}
		return s.Save(req.Context(), org(req), supportops.Actor(req.Context()), supportops.ID(req), in)
	}
	route("POST /v1/tenant/closing-texts", rbac.PermTenantMacrosWrite, save)
	route("PUT /v1/tenant/closing-texts/{uuid}", rbac.PermTenantMacrosWrite, save)
	route("DELETE /v1/tenant/closing-texts/{uuid}", rbac.PermTenantMacrosWrite, func(req *http.Request) (any, error) {
		return map[string]any{"deleted": true}, s.Delete(req.Context(), org(req), supportops.ID(req))
	})
	route("PUT /v1/tenant/closing-text-settings", rbac.PermTenantMacrosWrite, func(req *http.Request) (any, error) {
		in, err := supportops.Read[Settings](req)
		if err != nil {
			return nil, err
		}
		return s.SaveSettings(req.Context(), org(req), in)
	})
}

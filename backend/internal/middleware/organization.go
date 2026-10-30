package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/getsentry/sentry-go"
)

// maxOrganizationRefLen bounds the X-Organization value: slugs are at most
// 64 characters and uuids 36. Longer values cannot match an organization, so
// they are answered like any other unknown reference without a lookup.
const maxOrganizationRefLen = 64

// RequireOrganization resolves the current organization from the
// X-Organization header (slug or uuid) and the caller's membership in it, and
// stores the orgctx.Scope on the request context. It must run after
// Authenticate. super_admin is not exempt: tenant routes need a membership.
//
// Unknown organization and missing membership get the same answer
// (403 NO_TENANT_MEMBERSHIP) so the header cannot be used to probe which
// organizations exist.
func RequireOrganization(resolver orgctx.Resolver) func(http.Handler) http.Handler {
	if resolver == nil {
		panic("middleware: RequireOrganization needs a resolver")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			ref := strings.TrimSpace(r.Header.Get(orgctx.HeaderName))
			if ref == "" {
				response.BadRequest(w, r, response.CodeOrganizationRequired, orgctx.HeaderName+" header is required")
				return
			}
			if len(ref) > maxOrganizationRefLen {
				writeNoMembership(w, r, errors.New("organization reference too long"))
				return
			}
			scope, err := resolver.Resolve(r.Context(), ref, p.UserInternal)
			// Platform support staff enter through an active grant (CF-14).
			if errors.Is(err, orgctx.ErrNotMember) && p.HasPermission(rbac.PermPlatformSupportAccess) && p.ImpersonatorUserID == nil {
				if sr, ok := resolver.(orgctx.SupportResolver); ok {
					scope, err = sr.ResolveSupport(r.Context(), ref, p.UserInternal)
				}
			}
			if err != nil {
				writeOrganizationError(w, r, err)
				return
			}
			if p.SSOOrganizationID > 0 && p.SSOOrganizationID != scope.ID {
				response.Forbidden(w, r, "SSO token belongs to another organization")
				return
			}
			if policy, ok := resolver.(OrganizationAccessPolicy); ok {
				if err := policy.CheckOrganizationAccess(r, scope, p); err != nil {
					writePolicyError(w, r, err)
					return
				}
			}
			tagOrganization(r, scope)
			next.ServeHTTP(w, r.WithContext(orgctx.WithScope(r.Context(), scope)))
		})
	}
}

// RequireTenantPermission requires a tenant permission slug in the current
// organization scope. Tenant permissions come from the membership only:
// there is no super_admin bypass. Must run after RequireOrganization.
func RequireTenantPermission(slug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				// Route wiring bug, not a client error.
				response.InternalErr(w, r, errors.New("RequireTenantPermission without RequireOrganization"), "")
				return
			}
			if !scope.Has(slug) {
				response.Forbidden(w, r, "Missing permission: "+slug)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeOrganizationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgctx.ErrOrganizationNotFound), errors.Is(err, orgctx.ErrNotMember):
		writeNoMembership(w, r, err)
	case errors.Is(err, orgctx.ErrSuspended):
		response.RecordFailure(w, err)
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationSuspended, "Organization is suspended")
	case errors.Is(err, orgctx.ErrAccessExpired):
		response.RecordFailure(w, err)
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationAccessExpired, "Organization access has expired")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func writeNoMembership(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	response.Error(w, r, http.StatusForbidden, response.CodeNoTenantMembership, "You are not a member of this organization")
}

// tagOrganization tags the request-scoped error tracker hub (set up by
// ServerErrors) with the organization uuid.
func tagOrganization(r *http.Request, scope orgctx.Scope) {
	if !observability.Enabled() {
		return
	}
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.Scope().SetTag("organization_uuid", scope.UUID.String())
	}
}

// writePolicyError names the IP allow-list and enforced SSO denials so the
// panel can offer the SSO sign-in; anything else stays a plain 403.
func writePolicyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgctx.ErrIPNotAllowed):
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationIPBlocked, "Organization IP allow-list excludes this address")
	case errors.Is(err, orgctx.ErrSSORequired):
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationSSORequired, "Organization requires SSO sign-in")
	default:
		response.Forbidden(w, r, "Organization access policy denied this request")
	}
}

// OrganizationAccessPolicy runs after membership resolution, including cache hits.
type OrganizationAccessPolicy interface {
	CheckOrganizationAccess(*http.Request, orgctx.Scope, authctx.Principal) error
}

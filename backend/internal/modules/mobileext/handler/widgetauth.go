package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// WidgetScheme is the Authorization scheme of widget tokens.
const WidgetScheme = "Widget"

type widgetOrgKey struct{}

// WidgetAuthenticator resolves widget tokens (usecase.Service).
type WidgetAuthenticator interface {
	AuthenticateWidget(ctx context.Context, raw string) (usecase.WidgetIdentity, error)
}

// WidgetOrganizationFrom returns the organization the widget token was issued for.
func WidgetOrganizationFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(widgetOrgKey{}).(int64)
	return id, ok
}

func widgetToken(header string) string {
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, WidgetScheme) {
		return ""
	}
	return strings.TrimSpace(value)
}

// AuthenticateWidget accepts only `Authorization: Widget <token>`. The
// principal is loaded like a session's but without platform roles, so only
// organization membership (resolved per request) grants anything.
func AuthenticateWidget(auth WidgetAuthenticator, loader middleware.IdentityLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := widgetToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Unauthorized(w, r, "Widget token is required")
				return
			}
			id, err := auth.AuthenticateWidget(r.Context(), raw)
			if err != nil {
				if errors.Is(err, usecase.ErrInvalidWidgetToken) {
					response.Unauthorized(w, r, "Invalid or expired widget token")
					return
				}
				response.InternalErr(w, r, err, "")
				return
			}
			claims := jwt.Claims{
				RegisteredClaims: jwtlib.RegisteredClaims{Subject: id.UserUUID.String()},
				Roles:            []string{},
			}
			if id.SessionID != [16]byte{} {
				claims.SessionID = id.SessionID.String()
			}
			p, err := loader.LoadPrincipal(r, claims)
			if err != nil {
				response.Unauthorized(w, r, "Session is no longer valid")
				return
			}
			// Belt and braces: a widget principal never carries platform power.
			p.IsSuperAdmin, p.Roles, p.Permissions, p.ImpersonatorUserID, p.SSOOrganizationID = false, []string{}, []string{}, nil, 0
			observability.SetUser(r.Context(), p.UserID.String())
			ctx := authctx.WithPrincipal(r.Context(), p)
			ctx = context.WithValue(ctx, widgetOrgKey{}, id.OrganizationID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireWidgetOrganization runs after middleware.RequireOrganization and
// rejects an X-Organization other than the token's organization.
func RequireWidgetOrganization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, ok := orgctx.From(r.Context())
		tokenOrg, ok2 := WidgetOrganizationFrom(r.Context())
		if !ok || !ok2 {
			response.InternalErr(w, r, errors.New("mobileext: widget organization check without scope"), "")
			return
		}
		if scope.ID != tokenOrg {
			response.Forbidden(w, r, "Widget token belongs to another organization")
			return
		}
		next.ServeHTTP(w, r)
	})
}

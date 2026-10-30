// Package mcpauth is the OAuth 2.1 authorization server of the MCP layers
// (docs/design/mcp.md). The OAuth endpoints sit outside /v1 at the paths MCP
// clients expect; the frontend proxies /oauth/*, /.well-known/* and /mcp/*
// to Go because only the frontend is public in production.
package mcpauth

import (
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
)

// RegisterPublicRoutes mounts the public OAuth endpoints (no session; per-IP
// limits in the usecase) and GET /oauth/authorize.
func RegisterPublicRoutes(mux *http.ServeMux, svc *usecase.Service, log *slog.Logger) {
	h := handler.New(svc, log)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.AuthorizationServer)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp/{layer}", h.ProtectedResource)
	mux.HandleFunc("POST /oauth/register", h.Register)
	mux.HandleFunc("POST /oauth/token", h.Token)
	mux.HandleFunc("POST /oauth/revoke", h.Revoke)
	for _, p := range []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource/mcp/{layer}",
		"/oauth/register", "/oauth/token", "/oauth/revoke",
	} {
		mux.HandleFunc("OPTIONS "+p, h.Preflight)
	}
	c := handler.NewConsent(svc, h)
	mux.HandleFunc("GET /oauth/authorize", c.Authorize)
}

// staffActor is the signed-in panel user (routes behind Authenticate).
func staffActor(r *http.Request) (usecase.Actor, bool) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok || p.UserInternal <= 0 {
		return usecase.Actor{}, false
	}
	return usecase.Actor{UserID: p.UserInternal, Request: r}, true
}

// RegisterSessionRoutes mounts the /v1 endpoints of the consent screens and
// the connected apps: staff sessions under /v1/mcp, customer panel sessions
// under /v1/customer/mcp, and the platform client list.
func RegisterSessionRoutes(
	mux *http.ServeMux,
	svc *usecase.Service,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	log *slog.Logger,
) {
	c := handler.NewConsent(svc, handler.New(svc, log))
	authn := middleware.Authenticate(tokens, loader)
	staff := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn) }
	mux.Handle("GET /v1/mcp/consent/{uuid}", staff(c.ConsentInfo(staffActor)))
	mux.Handle("POST /v1/mcp/consent/{uuid}/decide", staff(c.Decide(staffActor)))
	mux.Handle("GET /v1/mcp/grants", staff(c.ListGrants(staffActor)))
	mux.Handle("DELETE /v1/mcp/grants/{uuid}", staff(c.RevokeGrant(staffActor)))

}

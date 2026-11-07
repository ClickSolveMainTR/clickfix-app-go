package mcp

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// GuideTool is one tool of the organization layer as the connection guide
// lists it: what it does, whether it changes data and whether the member
// would get it right now.
type GuideTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Write: the tool changes data; it also needs the mcp:write scope, which
	// the member grants on the consent screen.
	Write bool `json:"write"`
	// Available: the member's permissions in this organization include the
	// tool (the same check tools/list runs on every request).
	Available bool `json:"available"`
}

// Guide is what the "Connect with MCP" pages show (CF-173): where a client
// connects, where it is authorized and which tools this member gets in the
// active organization. It carries no secret: MCP clients register themselves
// and receive their tokens through the OAuth flow.
type Guide struct {
	// ResourceURL is the MCP endpoint a client is pointed at.
	ResourceURL string `json:"resource_url"`
	// AuthorizationServer is the OAuth issuer the client discovers.
	AuthorizationServer string `json:"authorization_server"`
	// Scopes the consent screen can grant, in display order.
	Scopes []string `json:"scopes"`
	// CanWrite: the member may grant mcp:write here (tenant.mcp.write).
	CanWrite bool        `json:"can_write"`
	Tools    []GuideTool `json:"tools"`
}

// Addresses are the public URLs of the authorization server
// (*mcpauth/usecase.Service).
type Addresses interface {
	Issuer() string
	ResourceURL(path string) string
}

// Guide lists every tool of the organization layer with whether the scope's
// holder gets it. Write tools count as available when the member could grant
// mcp:write; without that permission they are listed as unavailable.
func (c *Catalog) Guide(scope orgctx.Scope, urls Addresses) Guide {
	p := Principal{Layer: model.LayerBusiness, Scope: &scope, CanWrite: scope.Has(rbac.PermTenantMCPWrite)}
	out := Guide{
		ResourceURL:         urls.ResourceURL(model.ResourceBusiness),
		AuthorizationServer: urls.Issuer(),
		Scopes:              model.Scopes,
		CanWrite:            p.CanWrite,
		Tools:               make([]GuideTool, 0, len(c.tools)),
	}
	for _, t := range c.tools {
		out.Tools = append(out.Tools, GuideTool{Name: t.Name, Description: t.Description, Write: t.Write, Available: c.Allowed(p, t)})
	}
	return out
}

// RegisterGuideRoute mounts GET /v1/tenant/mcp/guide for members who may
// connect (tenant.mcp.connect), in the organization of the request.
func RegisterGuideRoute(mux *http.ServeMux, c *Catalog, urls Addresses, t *jwt.Manager, l middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	h := func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, r, http.StatusOK, c.Guide(orgctx.Must(r.Context()), urls))
	}
	mux.Handle("GET /v1/tenant/mcp/guide", middleware.Chain(http.HandlerFunc(h),
		middleware.Authenticate(t, l), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(rbac.PermTenantMCPConnect)))
}

package mcp

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

type fakeAddresses struct{}

func (fakeAddresses) Issuer() string                 { return "https://app.example.test" }
func (fakeAddresses) ResourceURL(path string) string { return "https://app.example.test" + path }

func guideTools(g Guide) map[string]GuideTool {
	out := map[string]GuideTool{}
	for _, t := range g.Tools {
		out[t.Name] = t
	}
	return out
}

func TestGuideListsEveryToolWithTheMembersAccess(t *testing.T) {
	c := NewCatalog()
	scope := orgctx.Scope{Permissions: []string{rbac.PermTenantMCPConnect, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReply}}
	g := c.Guide(scope, fakeAddresses{})

	if g.ResourceURL != "https://app.example.test/mcp/business" || g.AuthorizationServer != "https://app.example.test" {
		t.Fatalf("unexpected addresses: %+v", g)
	}
	if len(g.Tools) != len(c.tools) {
		t.Fatalf("guide lists %d tools, catalog has %d", len(g.Tools), len(c.tools))
	}
	if g.CanWrite {
		t.Fatal("write offered without tenant.mcp.write")
	}
	tools := guideTools(g)
	if !tools["search_conversations"].Available || !tools["get_conversation"].Available {
		t.Fatal("read tools the member holds are not available")
	}
	if tools["reply"].Available {
		t.Fatal("write tool available without tenant.mcp.write")
	}
	if !tools["reply"].Write || tools["search_conversations"].Write {
		t.Fatal("write flag does not follow the catalog")
	}
	if tools["list_contacts"].Available {
		t.Fatal("tool available without its permission")
	}
}

func TestGuideOffersWriteToolsOnlyWithTheWritePermission(t *testing.T) {
	c := NewCatalog()
	scope := orgctx.Scope{Permissions: []string{rbac.PermTenantMCPConnect, rbac.PermTenantMCPWrite, rbac.PermTenantConversationsReply}}
	g := c.Guide(scope, fakeAddresses{})
	if !g.CanWrite {
		t.Fatal("tenant.mcp.write did not allow write")
	}
	tools := guideTools(g)
	if !tools["reply"].Available {
		t.Fatal("reply not available with its permission and write")
	}
	if tools["set_status"].Available {
		t.Fatal("write tool available without its own permission")
	}
}

// The guide must agree with what tools/list would answer for the same member
// once write was granted on consent.
func TestGuideAgreesWithToolsList(t *testing.T) {
	c := NewCatalog()
	scope := orgctx.Scope{Permissions: []string{rbac.PermTenantMCPConnect, rbac.PermTenantMCPWrite, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsManage, rbac.PermTenantContactsRead}}
	g := c.Guide(scope, fakeAddresses{})
	live := map[string]bool{}
	for _, tool := range c.Available(Principal{Layer: "business", Scope: &scope, CanWrite: true}) {
		live[tool.Name] = true
	}
	for _, tool := range g.Tools {
		if tool.Available != live[tool.Name] {
			t.Fatalf("%s: guide says %v, tools/list says %v", tool.Name, tool.Available, live[tool.Name])
		}
	}
}

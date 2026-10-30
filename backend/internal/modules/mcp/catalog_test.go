package mcp

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"testing"
)

func TestToolsFollowLivePermissionsAndScopes(t *testing.T) {
	c := NewCatalog()
	p := Principal{Layer: model.LayerBusiness, Scope: &orgctx.Scope{Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReply}}}
	if len(c.Available(p)) != 2 {
		t.Fatal("read-only grant leaked write tools")
	}
	p.CanWrite = true
	if len(c.Available(p)) != 3 {
		t.Fatal("reply permission absent")
	}
	p.Scope.Permissions = nil
	if len(c.Available(p)) != 0 {
		t.Fatal("revoked permission survived")
	}
}
func TestCatalogRejectsUnknownOrMalformedArguments(t *testing.T) {
	c := NewCatalog()
	for _, tool := range c.tools {
		if tool.Name == "get_conversation" {
			if tool.Validate(Args{"conversation_uuid": "../../auth/me"}) == nil {
				t.Fatal("path traversal accepted")
			}
			if tool.Validate(Args{"conversation_uuid": "74da01e6-edfd-4ac5-959c-e714ccac0ba2", "token": "secret"}) == nil {
				t.Fatal("unknown field accepted")
			}
		}
	}
	args := MaskArguments(json.RawMessage(`{"content":"private message","q":"person@example.test","conversation_uuid":"74da01e6-edfd-4ac5-959c-e714ccac0ba2"}`)).(map[string]any)
	if args["content"] != Masked || args["q"] != Masked || args["conversation_uuid"] == Masked {
		t.Fatal(args)
	}
}

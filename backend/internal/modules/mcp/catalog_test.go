package mcp

import (
	"context"
	"slices"

	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"testing"
)

func TestToolsFollowLivePermissionsAndScopes(t *testing.T) {
	c := testCatalog()
	p := Principal{Layer: model.LayerBusiness, Scope: &orgctx.Scope{Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReply}}}
	names := func() []string {
		out := []string{}
		for _, tool := range c.Available(p) {
			if tool.Write && !p.CanWrite {
				t.Fatalf("read-only grant leaked write tool %s", tool.Name)
			}
			out = append(out, tool.Name)
		}
		return out
	}
	if got := names(); !slices.Contains(got, "search_conversations") || slices.Contains(got, "reply") || slices.Contains(got, "list_contacts") {
		t.Fatalf("read-only grant: %v", got)
	}
	p.CanWrite = true
	if got := names(); !slices.Contains(got, "reply") || slices.Contains(got, "set_status") {
		t.Fatalf("reply permission: %v", got)
	}
	p.Scope.Permissions = nil
	if len(c.Available(p)) != 0 {
		t.Fatal("revoked permission survived")
	}
}
func TestCatalogRejectsUnknownOrMalformedArguments(t *testing.T) {
	c := testCatalog()
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

// testCatalog is the production catalog: the inbox capabilities.
func testCatalog() *Catalog {
	return NewCatalog(capability.NewRegistry().Register(conversations.Capabilities()...).Register(contacts.Capabilities()...))
}

// The catalog is the capability registry's MCP surface: same tool names
// as before (CF-173), and capabilities of other surfaces never leak.
func TestCatalogComesFromTheRegistry(t *testing.T) {
	names := []string{}
	for _, tool := range testCatalog().tools {
		names = append(names, tool.Name)
	}
	want := []string{"search_conversations", "get_conversation", "reply", "assign_conversation", "set_status", "list_contacts"}
	for _, w := range want {
		if !slices.Contains(names, w) {
			t.Fatalf("CF-173 tool %s missing from %v", w, names)
		}
	}
	reg := capability.NewRegistry().Register(conversations.Capabilities()...).Register(contacts.Capabilities()...).Register(&capability.Capability{
		Name: "assistant.only", Kind: capability.Read, Description: "web only",
		Run: func(context.Context, capability.Exec, capability.Params) (capability.Result, error) {
			return capability.Result{}, nil
		},
	})
	if len(NewCatalog(reg).tools) != len(names) {
		t.Fatal("a web capability reached MCP")
	}
}

type recordingAPI struct{ calls []string }

func (a *recordingAPI) Call(_ context.Context, method, path string, body any) (capability.Response, error) {
	raw, _ := json.Marshal(body)
	a.calls = append(a.calls, method+" "+path+" "+string(raw))
	return capability.Response{OK: true, Status: 200, Data: json.RawMessage(`{"items":[],"total":0}`)}, nil
}

// Tools build the same /v1 requests as the old request builders.
func TestToolsCallTheSameRoutes(t *testing.T) {
	c := testCatalog()
	api := &recordingAPI{}
	e := capability.Exec{API: api}
	id := "74da01e6-edfd-4ac5-959c-e714ccac0ba2"
	run := func(name string, args Args) {
		for _, tool := range c.tools {
			if tool.Name != name {
				continue
			}
			if err := tool.Validate(args); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			var err error
			if tool.Write {
				_, err = tool.Capability.Apply(context.Background(), e, capability.Params(args))
			} else {
				_, err = tool.Capability.Run(context.Background(), e, capability.Params(args))
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	run("search_conversations", Args{"q": "iade", "limit": float64(5)})
	run("reply", Args{"conversation_uuid": id, "content": "Merhaba", "client_id": id})
	run("set_status", Args{"conversation_uuid": id, "status": "resolved"})
	run("list_contacts", Args{"offset": float64(10)})
	want := []string{
		"GET /v1/tenant/conversations?limit=5&q=iade null",
		`POST /v1/tenant/conversations/` + id + `/messages {"client_id":"` + id + `","content":"Merhaba"}`,
		`PATCH /v1/tenant/conversations/` + id + ` {"status":"resolved"}`,
		"GET /v1/tenant/contacts?offset=10 null",
	}
	if !slices.Equal(api.calls, want) {
		t.Fatalf("calls\n%v\nwant\n%v", api.calls, want)
	}
	for _, bad := range []Args{{"limit": float64(500)}, {"limit": 1.5}, {"offset": float64(-1)}, {"status": 3}} {
		for _, tool := range c.tools {
			if tool.Name == "search_conversations" && tool.Validate(bad) == nil {
				t.Fatalf("accepted %v", bad)
			}
		}
	}
	for _, tool := range c.tools {
		if tool.Name == "set_status" && tool.Validate(Args{"conversation_uuid": id, "status": "deleted"}) == nil {
			t.Fatal("invalid status accepted")
		}
	}
}

// Module read capabilities are on MCP too; their writes are not.
func TestModuleReadsReachMCP(t *testing.T) {
	reg := capability.NewRegistry().Register(conversations.Capabilities()...).
		Register(channels.Capabilities()...).Register(automation.Capabilities()...).Register(automation.RuleCapabilities()...).
		Register(sla.Capabilities()...).Register(webhooks.Capabilities()...)
	got := map[string]*Tool{}
	for _, tool := range NewCatalog(reg).tools {
		got[tool.Name] = tool
	}
	for _, name := range []string{"channels_list", "channels_api_sample_request", "workflows_list", "workflows_reference", "workflows_dry_run",
		"automations_list", "automations_dry_run", "sla_check", "webhooks_list"} {
		if tool := got[name]; tool == nil || tool.Write {
			t.Fatalf("%s missing or writable: %v", name, tool)
		}
	}
	for _, name := range []string{"channels_api_create", "channels_test", "workflows_create", "webhooks_test", "channels_api_rotate_secret"} {
		if got[name] != nil {
			t.Fatalf("write %s reached MCP", name)
		}
	}
}

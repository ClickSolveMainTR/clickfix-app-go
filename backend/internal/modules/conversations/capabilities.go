package conversations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the inbox exposed to MCP clients (CF-192): the six tools
// of the MCP catalog (CF-173), now defined once in the capability registry.
// They run on the "mcp" surface only: answering customers is the job of the
// organization's own AI agent, not of the ClickFix assistant.

const conversationsPath = "/v1/tenant/conversations"

var uuidField = map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"}

func text(p capability.Params, k string) string { s, _ := p[k].(string); return s }

// listQuery copies the list arguments to the query string.
func listQuery(p capability.Params, keys ...string) string {
	q := url.Values{}
	for _, k := range keys {
		switch v := p[k].(type) {
		case float64:
			q.Set(k, strconv.Itoa(int(v)))
		case string:
			q.Set(k, v)
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// raw runs a /v1 call and returns its data as the result.
func raw(ctx context.Context, e capability.Exec, method, path string, body any) (capability.Result, error) {
	r, err := e.API.Call(ctx, method, path, body)
	if err != nil {
		return capability.Result{}, err
	}
	if err := r.Err(); err != nil {
		return capability.Result{}, err
	}
	return capability.Result{Summary: method + " " + path, Data: json.RawMessage(r.Data)}, nil
}

func checkUUIDs(p capability.Params, keys ...string) error {
	for _, k := range keys {
		if v := text(p, k); v != "" {
			if _, err := uuid.Parse(v); err != nil {
				return capability.Invalid("%s must be a UUID", k)
			}
		}
	}
	return nil
}

func object(required []string, props map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
}

var (
	limitField  = map[string]any{"type": "integer", "minimum": 1, "maximum": 100}
	offsetField = map[string]any{"type": "integer", "minimum": 0}
	textField   = map[string]any{"type": "string", "maxLength": 10000}
	mcpOnly     = []string{capability.SurfaceMCP}
)

// Capabilities are the inbox capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "conversations.search", MCPName: "search_conversations", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: mcpOnly,
			Description: "Search conversations visible to you in this organization.",
			Input:       object(nil, map[string]any{"q": textField, "limit": limitField, "offset": offsetField, "status": textField, "priority": textField}),
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodGet, conversationsPath+listQuery(p, "q", "limit", "offset", "status", "priority"), nil)
			},
		},
		{
			Name: "conversations.get", MCPName: "get_conversation", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: mcpOnly,
			Description: "Get one conversation visible to you.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": uuidField}),
			Validate:    func(p capability.Params) error { return checkUUIDs(p, "conversation_uuid") },
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodGet, conversationsPath+"/"+text(p, "conversation_uuid"), nil)
			},
		},
		{
			Name: "conversations.reply", MCPName: "reply", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsReply, Surfaces: mcpOnly,
			Description: "Send a reply. Confirm with the user. Reuse client_id when retrying the same reply.",
			Input: object([]string{"conversation_uuid", "content", "client_id"},
				map[string]any{"conversation_uuid": uuidField, "content": textField, "client_id": uuidField}),
			Validate: func(p capability.Params) error {
				if text(p, "content") == "" {
					return capability.Invalid("content is required")
				}
				return checkUUIDs(p, "conversation_uuid", "client_id")
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodPost, conversationsPath+"/"+text(p, "conversation_uuid")+"/messages",
					map[string]any{"content": text(p, "content"), "client_id": text(p, "client_id")})
			},
		},
		{
			Name: "conversations.assign", MCPName: "assign_conversation", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsAssign, Surfaces: mcpOnly,
			Description: "Assign a conversation to an organization member. Confirm with the user.",
			Input:       object([]string{"conversation_uuid", "user_uuid"}, map[string]any{"conversation_uuid": uuidField, "user_uuid": uuidField}),
			Validate:    func(p capability.Params) error { return checkUUIDs(p, "conversation_uuid", "user_uuid") },
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodPatch, conversationsPath+"/"+text(p, "conversation_uuid"), map[string]any{"assignee": text(p, "user_uuid")})
			},
		},
		{
			Name: "conversations.set_status", MCPName: "set_status", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: mcpOnly,
			Description: "Set open, pending, resolved or closed. Confirm with the user.",
			Input: object([]string{"conversation_uuid", "status"}, map[string]any{"conversation_uuid": uuidField,
				"status": map[string]any{"type": "string", "enum": []string{"open", "pending", "resolved", "closed"}}}),
			Validate: func(p capability.Params) error { return checkUUIDs(p, "conversation_uuid") },
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodPatch, conversationsPath+"/"+text(p, "conversation_uuid"), map[string]any{"status": text(p, "status")})
			},
		},
		{
			Name: "contacts.list", MCPName: "list_contacts", Module: "contacts", Kind: capability.Read,
			Permission: rbac.PermTenantContactsRead, Surfaces: mcpOnly,
			Description: "List organization contacts with pagination.",
			Input:       object(nil, map[string]any{"q": textField, "limit": limitField, "offset": offsetField}),
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return raw(ctx, e, http.MethodGet, "/v1/tenant/integration-contacts"+listQuery(p, "q", "limit", "offset"), nil)
			},
		},
	}
}

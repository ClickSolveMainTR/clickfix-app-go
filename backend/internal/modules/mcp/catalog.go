// Catalog adapts the staff /v1 request builders and permission-filtered tool
// registration from takwim-app-go modules/mcp/catalog.go to ClickFix.
package mcp

import (
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type Args map[string]any
type Call struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
}
type Tool struct {
	Name, Title, Description string
	Write, Destructive       bool
	AnyOf                    []string
	Input                    map[string]any
	Request                  func(Args) (Call, error)
	required                 []string
	fields                   []string
}
type ArgumentError struct{ Message string }

func (e *ArgumentError) Error() string { return e.Message }
func argErr(f string, a ...any) error  { return &ArgumentError{Message: fmt.Sprintf(f, a...)} }
func (t *Tool) Validate(args Args) error {
	for _, field := range t.required {
		v, ok := args[field]
		if !ok || v == nil || v == "" {
			return argErr("%s is required", field)
		}
	}
	for key, v := range args {
		if !slices.Contains(t.fields, key) {
			return argErr("unknown argument %s", key)
		}
		if key == "limit" || key == "offset" {
			n, ok := v.(float64)
			if !ok || n != float64(int(n)) || n < 0 || (key == "limit" && (n < 1 || n > 100)) {
				return argErr("invalid %s", key)
			}
			continue
		}
		str, ok := v.(string)
		if !ok || len(str) > 10000 {
			return argErr("invalid %s", key)
		}
		if strings.HasSuffix(key, "uuid") || key == "client_id" {
			if _, err := uuid.Parse(str); err != nil {
				return argErr("%s must be a UUID", key)
			}
		}
	}
	return nil
}

type Catalog struct{ tools []*Tool }

func (c *Catalog) Available(p Principal) []*Tool {
	out := []*Tool{}
	for _, t := range c.tools {
		if c.Allowed(p, t) {
			out = append(out, t)
		}
	}
	return out
}
func (c *Catalog) Allowed(p Principal, t *Tool) bool {
	return p.Layer == model.LayerBusiness && p.Scope != nil && (!t.Write || p.CanWrite) && (len(t.AnyOf) == 0 || slices.ContainsFunc(t.AnyOf, p.Scope.Has))
}
func tool(name, description string, write bool, permission string, fields, required []string, request func(Args) (Call, error)) *Tool {
	props := map[string]any{}
	for _, f := range fields {
		schema := map[string]any{"type": "string", "maxLength": 10000}
		if f == "limit" || f == "offset" {
			schema = map[string]any{"type": "integer", "minimum": 0}
			if f == "limit" {
				schema["minimum"] = 1
				schema["maximum"] = 100
			}
		}
		props[f] = schema
	}
	return &Tool{Name: name, Title: name, Description: description, Write: write, AnyOf: []string{permission}, Input: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}, Request: request, required: required, fields: fields}
}
func text(a Args, key string) string { s, _ := a[key].(string); return s }
func listCall(path string, a Args) (Call, error) {
	q := url.Values{}
	for _, key := range []string{"q", "limit", "offset", "status", "priority"} {
		if v, ok := a[key]; ok {
			if n, ok := v.(float64); ok {
				q.Set(key, strconv.Itoa(int(n)))
			} else {
				q.Set(key, fmt.Sprint(v))
			}
		}
	}
	return Call{Method: http.MethodGet, Path: path, Query: q}, nil
}
func NewCatalog() *Catalog {
	base := "/v1/tenant/conversations"
	c := &Catalog{}
	c.tools = []*Tool{
		tool("search_conversations", "Search conversations visible to you in this organization.", false, rbac.PermTenantConversationsRead, []string{"q", "limit", "offset", "status", "priority"}, nil, func(a Args) (Call, error) { return listCall(base, a) }),
		tool("get_conversation", "Get one conversation visible to you.", false, rbac.PermTenantConversationsRead, []string{"conversation_uuid"}, []string{"conversation_uuid"}, func(a Args) (Call, error) {
			return Call{Method: http.MethodGet, Path: base + "/" + text(a, "conversation_uuid")}, nil
		}),
		tool("reply", "Send a reply. Confirm with the user. Reuse client_id when retrying the same reply.", true, rbac.PermTenantConversationsReply, []string{"conversation_uuid", "content", "client_id"}, []string{"conversation_uuid", "content", "client_id"}, func(a Args) (Call, error) {
			return Call{Method: http.MethodPost, Path: base + "/" + text(a, "conversation_uuid") + "/messages", Body: map[string]any{"content": text(a, "content"), "client_id": text(a, "client_id")}}, nil
		}),
		tool("assign_conversation", "Assign a conversation to an organization member. Confirm with the user.", true, rbac.PermTenantConversationsAssign, []string{"conversation_uuid", "user_uuid"}, []string{"conversation_uuid", "user_uuid"}, func(a Args) (Call, error) {
			return Call{Method: http.MethodPatch, Path: base + "/" + text(a, "conversation_uuid"), Body: map[string]any{"assignee": text(a, "user_uuid")}}, nil
		}),
		tool("set_status", "Set open, pending, resolved or closed. Confirm with the user.", true, rbac.PermTenantConversationsManage, []string{"conversation_uuid", "status"}, []string{"conversation_uuid", "status"}, func(a Args) (Call, error) {
			status := text(a, "status")
			if !slices.Contains([]string{"open", "pending", "resolved", "closed"}, status) {
				return Call{}, argErr("invalid status")
			}
			return Call{Method: http.MethodPatch, Path: base + "/" + text(a, "conversation_uuid"), Body: map[string]any{"status": status}}, nil
		}),
		tool("list_contacts", "List organization contacts with pagination.", false, rbac.PermTenantContactsRead, []string{"q", "limit", "offset"}, nil, func(a Args) (Call, error) { return listCall("/v1/tenant/integration-contacts", a) }),
	}
	return c
}

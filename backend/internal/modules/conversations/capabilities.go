package conversations

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the inbox (CF-192, CF-211): the six MCP tools of CF-173
// plus the agent's daily inbox work. Since CF-211 they also run in the
// panel assistant (web, mobile): an agent asks the assistant to find,
// summarize, note, tag, assign or answer a conversation; every write is a
// confirmation card and runs through /v1 as the agent. Answering customers
// on its own stays the job of the organization's AI agent.

const conversationsPath = "/v1/tenant/conversations"

var uuidField = map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"}

func text(p capability.Params, k string) string { s, _ := p[k].(string); return s }

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
	textField   = map[string]any{"type": "string", "maxLength": 10000}
	surfaces    = capability.ReadSurfaces
	inboxRoutes = []string{"/inbox"}
)

func title(tr, en string) map[string]string { return map[string]string{"tr": tr, "en": en} }

// Capabilities are the inbox capabilities.
func Capabilities() []*capability.Capability {
	conv := capability.UUID("conversation uuid")
	return []*capability.Capability{
		{
			Name: "conversations.search", MCPName: "search_conversations", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Konuşmaları ara", "Search conversations"),
			Description: "Search conversations. assignee: me|unassigned|uuid; sort: -last_message_at|-waiting_since|priority.",
			Input: object(nil, capability.Paged(map[string]any{"q": textField, "status": capability.Text(80, "csv: open,pending,snoozed,resolved,closed"),
				"priority": capability.Text(60, "csv: low,normal,high,urgent"), "assignee": capability.Text(40, ""), "tag": capability.Text(400, "csv tag uuids"),
				"contact": capability.UUID(""), "company": capability.UUID(""), "team": capability.Text(40, "mine|uuid"),
				"unread": capability.Bool(""), "sla_breached": capability.Bool(""),
				"sort": capability.Enum("-last_message_at", "last_message_at", "-created_at", "created_at", "-waiting_since", "waiting_since", "-priority", "priority")})),
			Run: capability.Endpoint{Method: http.MethodGet, Path: conversationsPath,
				Query: []string{"q", "limit", "offset", "status", "priority", "assignee", "tag", "contact", "company", "team", "unread", "sla_breached", "sort"}}.Do(),
		},
		{
			Name: "conversations.counts", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Gelen kutusu özeti", "Inbox counts"),
			Description: "Inbox counts: open, mine, unassigned, snoozed, SLA breached, by status.",
			Run:         capability.Endpoint{Method: http.MethodGet, Path: conversationsPath + "/counts"}.Do(),
		},
		{
			Name: "conversations.get", MCPName: "get_conversation", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title: title("Konuşmayı getir", "Get a conversation"), FullText: true,
			Description: "One conversation: contact, status, assignee, tags, SLA times.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv}),
			Run:         capability.Endpoint{Method: http.MethodGet, Path: conversationsPath + "/{conversation_uuid}", Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.messages", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title: title("Konuşma mesajları", "Conversation messages"), FullText: true,
			Description: "Messages of a conversation, oldest first (notes are private). Page back with before=<before from the result>.",
			Input: object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv, "before": capability.UUID("message uuid cursor"),
				"limit": capability.Int(1, 50)}),
			Run: capability.Endpoint{Method: http.MethodGet, Path: conversationsPath + "/{conversation_uuid}/messages", Query: []string{"before", "limit"}}.Do(),
		},
		{
			Name: "conversations.sla", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Konuşmanın SLA durumu", "Conversation SLA"),
			Description: "SLA policy and timers (due, remaining, breached) of a conversation.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv}),
			Run:         capability.Endpoint{Method: http.MethodGet, Path: conversationsPath + "/{conversation_uuid}/sla"}.Do(),
		},
		{
			Name: "conversations.agents", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Atanabilir temsilciler", "Assignable agents"),
			Description: "Members a conversation can be assigned to (uuid, name).",
			Run:         capability.Endpoint{Method: http.MethodGet, Path: conversationsPath + "/agents"}.Do(),
		},
		{
			Name: "conversation_statuses.list", Module: "conversations", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Özel konuşma durumları", "Custom conversation statuses"),
			Description: "The organization's own statuses (uuid, name, color, category: open, pending or resolved), e.g. \"İşleniyor\"; pass the uuid to set_status as custom_status_uuid.",
			Run:         capability.Endpoint{Method: http.MethodGet, Path: "/v1/tenant/conversation-statuses"}.Do(),
		},
		{
			Name: "tags.list", Module: "tags", Kind: capability.Read,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Etiketler", "Tags"),
			Description: "Conversation tags (uuid, name, usage).",
			Run:         capability.Endpoint{Method: http.MethodGet, Path: "/v1/tenant/tags"}.Do(),
		},
		{
			Name: "conversations.reply", MCPName: "reply", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsReply, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Müşteriye yanıt gönder", "Send a reply"),
			Description: "Send a reply to the customer. Reuse client_id when retrying the same reply.",
			Input: object([]string{"conversation_uuid", "content"},
				map[string]any{"conversation_uuid": conv, "content": capability.Titled("Yanıt", textField), "client_id": uuidField}),
			Validate: func(p capability.Params) error {
				if text(p, "content") == "" {
					return capability.Invalid("content is required")
				}
				return checkUUIDs(p, "conversation_uuid", "client_id")
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				body := map[string]any{"content": text(p, "content")}
				if id := text(p, "client_id"); id != "" {
					body["client_id"] = id
				}
				res, err := raw(ctx, e, http.MethodPost, conversationsPath+"/"+text(p, "conversation_uuid")+"/messages", body)
				res.Route = "/conversations/" + text(p, "conversation_uuid")
				return res, err
			},
		},
		{
			Name: "conversations.add_note", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsNote, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("İç not ekle", "Add an internal note"),
			Description: "Add a private note (customers never see it). mentions: member uuids.",
			Input: object([]string{"conversation_uuid", "content"}, map[string]any{"conversation_uuid": conv,
				"content": capability.Titled("Not", textField), "mentions": map[string]any{"type": "array", "maxItems": 10, "items": uuidField}}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: conversationsPath + "/{conversation_uuid}/messages",
				Body: []string{"content", "mentions"}, Fixed: map[string]any{"private": true}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.assign", MCPName: "assign_conversation", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsAssign, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Konuşmayı ata", "Assign a conversation"),
			Description: "Assign to a member (user_uuid from conversations_agents) and/or a team.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv, "user_uuid": uuidField, "team_uuid": uuidField}),
			Validate: func(p capability.Params) error {
				if text(p, "user_uuid") == "" && text(p, "team_uuid") == "" {
					return capability.Invalid("user_uuid or team_uuid is required: ask the user who should take it")
				}
				return checkUUIDs(p, "conversation_uuid", "user_uuid", "team_uuid")
			},
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: conversationsPath + "/{conversation_uuid}",
				Body: []string{"user_uuid", "team_uuid"}, Rename: map[string]string{"user_uuid": "assignee", "team_uuid": "team"}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.set_status", MCPName: "set_status", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Konuşma durumunu değiştir", "Change conversation status"),
			Description: "Set open, pending, snoozed (needs snoozed_until, RFC3339), resolved or closed, or one of the organization's own statuses with custom_status_uuid (from conversation_statuses_list; its category becomes the status). A plain status drops the custom status.",
			Input: object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv,
				"status": capability.Enum("open", "pending", "snoozed", "resolved", "closed"), "snoozed_until": capability.Text(40, "RFC3339"),
				"custom_status_uuid": uuidField}),
			Validate: func(p capability.Params) error {
				if text(p, "status") == "" && text(p, "custom_status_uuid") == "" {
					return capability.Invalid("status or custom_status_uuid is required")
				}
				if text(p, "status") == "snoozed" && text(p, "snoozed_until") == "" {
					return capability.Invalid("snoozed_until is required to snooze: ask the user until when")
				}
				return checkUUIDs(p, "conversation_uuid", "custom_status_uuid")
			},
			// A plain status sends custom_status: null (drops the custom one).
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: conversationsPath + "/{conversation_uuid}",
				Body: []string{"status", "snoozed_until", "custom_status_uuid"}, Rename: map[string]string{"custom_status_uuid": "custom_status"},
				Fixed: map[string]any{"custom_status": nil}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.set_priority", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Önceliği değiştir", "Change priority"),
			Description: "Set the priority of a conversation.",
			Input: object([]string{"conversation_uuid", "priority"}, map[string]any{"conversation_uuid": conv,
				"priority": capability.Enum("low", "normal", "high", "urgent")}),
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: conversationsPath + "/{conversation_uuid}", Body: []string{"priority"}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.add_tag", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Etiket ekle", "Add a tag"),
			Description: "Tag a conversation by name (created if missing) or tag_uuid.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv, "name": capability.Text(64, ""), "tag_uuid": uuidField}),
			Validate: func(p capability.Params) error {
				if text(p, "name") == "" && text(p, "tag_uuid") == "" {
					return capability.Invalid("name or tag_uuid is required")
				}
				return checkUUIDs(p, "conversation_uuid", "tag_uuid")
			},
			Apply: capability.Endpoint{Method: http.MethodPost, Path: conversationsPath + "/{conversation_uuid}/tags",
				Body: []string{"name", "tag_uuid"}, Rename: map[string]string{"tag_uuid": "uuid"}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.remove_tag", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Etiketi kaldır", "Remove a tag"),
			Description: "Remove a tag (tag_uuid from tags_list) from a conversation.",
			Input:       object([]string{"conversation_uuid", "tag_uuid"}, map[string]any{"conversation_uuid": conv, "tag_uuid": uuidField}),
			Apply:       capability.Endpoint{Method: http.MethodDelete, Path: conversationsPath + "/{conversation_uuid}/tags/{tag_uuid}", Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.apply_macro", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantMacrosRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Makro uygula", "Apply a macro"),
			Description: "Run a macro (macro_uuid from macros_list) on a conversation; it may reply, note, tag, assign or change status.",
			Input:       object([]string{"conversation_uuid", "macro_uuid"}, map[string]any{"conversation_uuid": conv, "macro_uuid": uuidField}),
			Apply:       capability.Endpoint{Method: http.MethodPost, Path: conversationsPath + "/{conversation_uuid}/macros/{macro_uuid}", Fixed: map[string]any{}, Body: []string{}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.remind", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Hatırlatıcı kur", "Set a reminder"),
			Description: "Remind me about a conversation at due_at (RFC3339, future); condition no_reply = only if the customer has not replied.",
			Input: object([]string{"conversation_uuid", "due_at"}, map[string]any{"conversation_uuid": conv, "due_at": capability.Text(40, "RFC3339"),
				"condition": capability.Enum("always", "no_reply"), "note": capability.Text(500, "")}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: conversationsPath + "/{conversation_uuid}/reminders", Body: []string{"due_at", "condition", "note"}, Route: "/inbox"}.Do(),
		},
		{
			Name: "conversations.mark_unread", Module: "conversations", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsRead, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Okunmadı olarak işaretle", "Mark as unread"),
			Description: "Mark a conversation unread for me again (CF-247): the latest customer message counts as unread; other agents are not affected.",
			Input:       object([]string{"conversation_uuid"}, map[string]any{"conversation_uuid": conv}),
			Apply:       capability.Endpoint{Method: http.MethodPost, Path: conversationsPath + "/{conversation_uuid}/unread", Fixed: map[string]any{}, Body: []string{}, Route: "/inbox"}.Do(),
		},
		{
			Name: "tags.create", Module: "tags", Kind: capability.Write,
			Permission: rbac.PermTenantConversationsManage, Surfaces: surfaces, Routes: inboxRoutes,
			Title:       title("Etiket oluştur", "Create a tag"),
			Description: "Create a conversation tag (returns the existing one with the same name).",
			Input:       object([]string{"name"}, map[string]any{"name": capability.Titled("Ad", capability.Text(64, "")), "color": capability.Text(32, "")}),
			Apply:       capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/tags", Body: []string{"name", "color"}}.Do(),
		},
	}
}

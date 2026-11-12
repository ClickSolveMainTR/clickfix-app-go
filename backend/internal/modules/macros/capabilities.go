package macros

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of macros and saved replies (CF-211). Organization-wide
// items need tenant.macros.write; the route checks it again.

func macroProps() map[string]any {
	return map[string]any{
		"name":        capability.Titled("Ad", capability.Text(120, "")),
		"description": capability.Text(500, ""),
		"scope":       capability.Enum("personal", "organization"),
		"actions": map[string]any{"type": "array", "minItems": 1, "maxItems": 10, "title": "Eylemler",
			"description": "type: reply|note (value text), status (open|pending|resolved|closed), priority, assign (me|uuid|\"\"), tag|untag (name)",
			"items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type", "value"},
				"properties": map[string]any{"type": capability.Enum("reply", "note", "status", "priority", "assign", "tag", "untag"), "value": capability.Text(10000, "")}}},
	}
}

func cannedProps() map[string]any {
	return map[string]any{
		"title":    capability.Titled("Başlık", capability.Text(120, "")),
		"shortcut": capability.Titled("Kısayol", map[string]any{"type": "string", "pattern": "^/?[a-z0-9][a-z0-9_-]{0,39}$"}),
		"category": capability.Text(60, ""),
		"content":  capability.Titled("İçerik", capability.Text(10000, "{{contact.first_name}}, {{agent.name}}, {{conversation.number}} placeholders")),
		"scope":    capability.Enum("personal", "organization"),
	}
}

// Capabilities are the macro and saved-reply capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/inbox", "/settings/macros"}
	id := capability.UUID("")
	upd := macroProps()
	upd["macro_uuid"] = id
	delete(upd, "scope")
	cupd := cannedProps()
	cupd["canned_uuid"] = id
	delete(cupd, "scope")
	return []*capability.Capability{
		{
			Name: "macros.list", Module: "macros", Kind: capability.Read, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makrolar", "Macros"), Description: "Macros with their actions.",
			Run: capability.Get("/v1/tenant/macros"),
		},
		{
			Name: "macros.create", Module: "macros", Kind: capability.Write, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makro oluştur", "Create a macro"), Description: "Create a macro (default personal; organization needs macros.write).",
			Input: capability.Object([]string{"name", "actions"}, macroProps()),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/macros", Route: "/inbox"}.Do(),
		},
		{
			Name: "macros.update", Module: "macros", Kind: capability.Write, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makroyu güncelle", "Update a macro"), Description: "Replace a macro: send name and all actions again.",
			Input: capability.Object([]string{"macro_uuid", "name", "actions"}, upd),
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: "/v1/tenant/macros/{macro_uuid}", Route: "/inbox"}.Do(),
		},
		{
			Name: "macros.delete", Module: "macros", Kind: capability.Destructive, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makroyu sil", "Delete a macro"), Description: "Delete a macro.",
			Input: capability.Object([]string{"macro_uuid"}, map[string]any{"macro_uuid": id}),
			Apply: capability.Endpoint{Method: http.MethodDelete, Path: "/v1/tenant/macros/{macro_uuid}", Route: "/inbox"}.Do(),
		},
		{
			Name: "canned_responses.list", Module: "macros", Kind: capability.Read, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Hazır yanıtlar", "Saved replies"), Description: "Saved replies (q filters title, shortcut, content).",
			Input: capability.Object(nil, map[string]any{"q": capability.Text(200, "")}),
			Run:   capability.Get("/v1/tenant/canned-responses", "q"),
		},
		{
			Name: "canned_responses.create", Module: "macros", Kind: capability.Write, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Hazır yanıt oluştur", "Create a saved reply"), Description: "Create a saved reply with a /shortcut.",
			Input: capability.Object([]string{"title", "shortcut", "content"}, cannedProps()),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/canned-responses", Route: "/inbox"}.Do(),
		},
		{
			Name: "canned_responses.update", Module: "macros", Kind: capability.Write, Permission: rbac.PermTenantMacrosRead, Surfaces: s, Routes: routes,
			Title: capability.T("Hazır yanıtı güncelle", "Update a saved reply"), Description: "Replace a saved reply: send title, shortcut and content again.",
			Input: capability.Object([]string{"canned_uuid", "title", "shortcut", "content"}, cupd),
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: "/v1/tenant/canned-responses/{canned_uuid}", Route: "/inbox"}.Do(),
		},
	}
}

package closingtexts

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of closing texts (CF-250) for the assistant and MCP.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/inbox", "/settings/closing-texts"}
	id := capability.UUID("")
	props := func() map[string]any {
		return map[string]any{
			"name":       capability.Titled("Ad", capability.Text(MaxName, "")),
			"body_tr":    capability.Titled("Metin (TR)", capability.Text(MaxBody, "{{agent.name}}, {{agent.first_name}}, {{contact.first_name}}, {{brand.name}}, {{organization.name}}, {{conversation.number}} placeholders")),
			"body_en":    capability.Titled("Metin (EN)", capability.Text(MaxBody, "optional English text; empty uses body_tr")),
			"is_default": map[string]any{"type": "boolean", "title": "Varsayılan"},
			"position":   map[string]any{"type": "integer", "minimum": 0, "maximum": 10000},
			"brand_uuid": capability.UUID("brand; leave out for organization-wide"),
		}
	}
	upd := props()
	upd["closing_text_uuid"] = id
	return []*capability.Capability{
		{
			Name: "closing_texts.list", Module: "closing_texts", Kind: capability.Read, Permission: rbac.PermTenantConversationsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Bitiş metinleri", "Closing texts"), Description: "Closing texts appended to replies, the default per organization/brand and whether AI replies get the default.",
			Run: capability.Get("/v1/tenant/closing-texts"),
		},
		{
			Name: "closing_texts.create", Module: "closing_texts", Kind: capability.Write, Permission: rbac.PermTenantMacrosWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Bitiş metni oluştur", "Create a closing text"), Description: "Create a closing text (is_default replaces the previous default of the same organization/brand).",
			Input: capability.Object([]string{"name", "body_tr"}, props()),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/closing-texts", Route: "/settings/closing-texts"}.Do(),
		},
		{
			Name: "closing_texts.update", Module: "closing_texts", Kind: capability.Write, Permission: rbac.PermTenantMacrosWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Bitiş metnini güncelle", "Update a closing text"), Description: "Replace a closing text: send name, body_tr and the other fields again.",
			Input: capability.Object([]string{"closing_text_uuid", "name", "body_tr"}, upd),
			Apply: capability.Endpoint{Method: http.MethodPut, Path: "/v1/tenant/closing-texts/{closing_text_uuid}", Route: "/settings/closing-texts"}.Do(),
		},
		{
			Name: "closing_texts.delete", Module: "closing_texts", Kind: capability.Destructive, Permission: rbac.PermTenantMacrosWrite, Surfaces: capability.DefaultSurfaces, Routes: routes,
			Title: capability.T("Bitiş metnini sil", "Delete a closing text"), Description: "Delete a closing text.",
			Input: capability.Object([]string{"closing_text_uuid"}, map[string]any{"closing_text_uuid": id}),
			Apply: capability.Endpoint{Method: http.MethodDelete, Path: "/v1/tenant/closing-texts/{closing_text_uuid}", Route: "/settings/closing-texts"}.Do(),
		},
		{
			Name: "closing_texts.settings", Module: "closing_texts", Kind: capability.Write, Permission: rbac.PermTenantMacrosWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Bitiş metni ayarı", "Closing text setting"), Description: "apply_to_ai: append the default closing text to autonomous AI replies (off by default).",
			Input: capability.Object([]string{"apply_to_ai"}, map[string]any{"apply_to_ai": map[string]any{"type": "boolean", "title": "AI yanıtlarına ekle"}}),
			Apply: capability.Endpoint{Method: http.MethodPut, Path: "/v1/tenant/closing-text-settings", Route: "/settings/closing-texts"}.Do(),
		},
	}
}

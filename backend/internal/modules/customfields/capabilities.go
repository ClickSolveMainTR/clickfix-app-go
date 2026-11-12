package customfields

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of custom fields (CF-211): which fields exist, so the
// assistant can fill custom_fields and filters with real keys.

// Capabilities are the custom field capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{{
		Name: "custom_fields.list", Module: "custom_fields", Kind: capability.Read, Permission: rbac.PermTenantCustomFieldsRead,
		Surfaces: capability.ReadSurfaces, Routes: []string{"/settings/custom-fields"},
		Title:       capability.T("Özel alanlar", "Custom fields"),
		Description: "Custom field definitions (key, label, type, options) of an entity.",
		Input: capability.Object([]string{"entity_type"}, map[string]any{"entity_type": capability.Enum("contact", "company", "conversation", "ticket"),
			"include_archived": capability.Bool("")}),
		Run: capability.Get("/v1/tenant/custom-fields", "entity_type", "include_archived"),
	}}
}

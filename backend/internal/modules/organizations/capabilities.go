package organizations

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the organization profile, member roles and invitations
// (CF-211). Role changes follow the route's rules: only owners grant or
// revoke owner, nobody grants a permission they lack, the last owner stays.

// Capabilities are the organization capabilities.
func Capabilities() []*capability.Capability {
	s := capability.ReadSurfaces
	org, members := []string{"/settings/organization"}, []string{"/settings/members", "/settings/roles"}
	inv := capability.UUID("invitation uuid")
	return []*capability.Capability{
		{
			Name: "organization.get", Module: "organization", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: org,
			Title: capability.T("Organizasyon bilgileri", "Organization profile"), Description: "Name, time zone, locale, currency, company and branding.",
			Run: capability.Get("/v1/tenant/organization"),
		},
		{
			Name: "organization.update", Module: "organization", Kind: capability.Write, Permission: rbac.PermTenantSettingsWrite, Surfaces: s, Routes: org,
			Title: capability.T("Organizasyonu güncelle", "Update the organization"), Description: "Change name, time zone (IANA), locale, currency or country; omitted fields stay.",
			Input: capability.Object(nil, map[string]any{"name": capability.Titled("Ad", capability.Text(255, "")), "timezone": capability.Text(64, "IANA"),
				"locale": capability.Enum("tr", "en"), "currency": capability.Text(3, "ISO 4217"), "country_code": capability.Text(2, "ISO 3166")}),
			Validate: func(p capability.Params) error {
				if len(p) == 0 {
					return capability.Invalid("nothing to change: ask the user what to update")
				}
				return nil
			},
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: "/v1/tenant/organization", Route: "/settings/organization"}.Do(),
		},
		{
			Name: "roles.list", Module: "members", Kind: capability.Read, Permission: rbac.PermTenantRolesRead, Surfaces: s, Routes: members,
			Title: capability.T("Roller", "Roles"), Description: "Organization roles (uuid, key, name).",
			Run: capability.Get("/v1/tenant/roles"),
		},
		{
			Name: "members.set_roles", Module: "members", Kind: capability.Write, Permission: rbac.PermTenantMembersWrite, Surfaces: s, Routes: members,
			Title: capability.T("Üyenin rollerini değiştir", "Change a member's roles"), Description: "Replace a member's roles (role uuids from roles_list).",
			Input: capability.Object([]string{"user_uuid", "role_uuids"}, map[string]any{"user_uuid": capability.UUID("member uuid"),
				"role_uuids": map[string]any{"type": "array", "minItems": 1, "maxItems": 10, "items": capability.UUID("")}}),
			Apply: capability.Endpoint{Method: http.MethodPatch, Path: "/v1/tenant/members/{user_uuid}", Body: []string{"role_uuids"}, Route: "/settings/members"}.Do(),
		},
		{
			Name: "invitations.resend", Module: "members", Kind: capability.Write, Permission: rbac.PermTenantMembersWrite, Surfaces: s, Routes: members,
			Title: capability.T("Daveti yeniden gönder", "Resend an invitation"), Description: "Resend an open or expired invitation (the old link stops working).",
			Input: capability.Object([]string{"invitation_uuid"}, map[string]any{"invitation_uuid": inv}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/invitations/{invitation_uuid}/resend", Body: []string{}, Route: "/settings/members"}.Do(),
		},
		{
			Name: "invitations.revoke", Module: "members", Kind: capability.Destructive, Permission: rbac.PermTenantMembersWrite, Surfaces: s, Routes: members,
			Title: capability.T("Daveti iptal et", "Revoke an invitation"), Description: "Revoke an open invitation; its link stops working.",
			Input: capability.Object([]string{"invitation_uuid"}, map[string]any{"invitation_uuid": inv}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/invitations/{invitation_uuid}/revoke", Body: []string{}, Route: "/settings/members"}.Do(),
		},
	}
}

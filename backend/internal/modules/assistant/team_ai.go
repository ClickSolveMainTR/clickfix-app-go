package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Team and AI capabilities (CF-189): list members, invite one (the CF-141
// invite_member action, now through /v1/tenant/invitations), read the AI
// settings and turn AI off.

// TeamAndAICapabilities need the queries for the role lookup of an
// invitation (by slug, as the CF-141 action did).
func TeamAndAICapabilities(q *db.Queries) []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "members.list", Module: "members", Kind: capability.Read, Permission: rbac.PermTenantMembersRead,
			Routes: []string{"/settings/members"}, Surfaces: capability.ReadSurfaces,
			Title:       map[string]string{"tr": "Ekip üyelerini listele", "en": "List members"},
			Description: "List the organization's members (user uuid, name, roles) and pending invitations.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				out := map[string]json.RawMessage{}
				for key, path := range map[string]string{"members": "/v1/tenant/members?limit=50", "invitations": "/v1/tenant/invitations?limit=50"} {
					r, err := e.API.Call(ctx, http.MethodGet, path, nil)
					if err != nil {
						return capability.Result{}, err
					}
					var page struct {
						Items json.RawMessage `json:"items"`
					}
					if err := r.Decode(&page); err != nil {
						return capability.Result{}, err
					}
					out[key] = page.Items
				}
				return capability.Result{Summary: "members", Data: out}, nil
			},
		},
		{
			Name: "members.invite", Aliases: []string{"invite_member"}, Module: "members", Kind: capability.Write,
			Permission: rbac.PermTenantMembersWrite, Routes: []string{"/settings/members"},
			Title:       map[string]string{"tr": "Ekip arkadaşı davet et", "en": "Invite a teammate"},
			Description: "Invite a teammate by e-mail with a role (default agent). Use the address the user wrote; never guess one.",
			Undo:        "/settings/members",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"email"},
				"properties": map[string]any{
					"email": map[string]any{"type": "string", "title": "E-mail", "maxLength": 254},
					"role":  map[string]any{"type": "string", "title": "Role", "enum": tenantRoles},
				}},
			Validate: func(p capability.Params) error {
				if _, err := mail.ParseAddress(str(p, "email")); err != nil {
					return capability.Invalid("email must be an e-mail address: ask the user")
				}
				return nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				role := str(p, "role")
				if role == "" {
					role = orgctx.RoleAgent
				}
				roleUUID, err := q.AssistantTenantRoleUUID(ctx, db.AssistantTenantRoleUUIDParams{Slug: role, OrganizationID: i8(e.OrgID)})
				if err != nil {
					return capability.Result{}, capability.Invalid("role %s does not exist in this organization", role)
				}
				addr, _ := mail.ParseAddress(str(p, "email"))
				r, err := e.API.Call(ctx, http.MethodPost, "/v1/tenant/invitations", map[string]any{"email": addr.Address, "role_uuids": []uuid.UUID{roleUUID}})
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: addr.Address, Route: "/settings/members", Focus: &capability.Focus{Route: "/settings/members"}}, nil
			},
		},
		{
			Name: "ai.status", Module: "ai", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead,
			Routes: []string{"/settings/ai"}, Surfaces: capability.ReadSurfaces,
			Title:       map[string]string{"tr": "Yapay zekâ ayarlarını oku", "en": "Read AI settings"},
			Description: "Read whether AI is on for the organization, its daily limits and the AI agent's configuration.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				out := map[string]json.RawMessage{}
				for key, path := range map[string]string{"policy": "/v1/tenant/ai/policy", "agent": "/v1/tenant/ai/agent"} {
					r, err := e.API.Call(ctx, http.MethodGet, path, nil)
					if err != nil {
						return capability.Result{}, err
					}
					var v json.RawMessage
					if r.Decode(&v) == nil {
						out[key] = v
					}
				}
				return capability.Result{Summary: "AI settings", Data: out}, nil
			},
		},
		{
			Name: "ai.disable", Module: "ai", Kind: capability.Write, Permission: rbac.PermTenantSettingsWrite, Routes: []string{"/settings/ai"},
			Title:       map[string]string{"tr": "Yapay zekâyı kapat", "en": "Turn off AI"},
			Description: "Turn AI features off for the organization (reply suggestions, AI agent). Limits are kept; ai_enable turns it back on.",
			Undo:        "ai.enable",
			Apply: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/ai/policy", nil)
				if err != nil {
					return capability.Result{}, err
				}
				policy := map[string]any{}
				if err := r.Decode(&policy); err != nil {
					return capability.Result{}, err
				}
				body := map[string]any{"enabled": false}
				for _, k := range []string{"mask_pii", "allow_unmask", "daily_calls", "concurrency", "daily_budget_micros"} {
					if v, ok := policy[k]; ok {
						body[k] = v
					}
				}
				r, err = e.API.Call(ctx, http.MethodPut, "/v1/tenant/ai/policy", body)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: "/settings/ai", Route: "/settings/ai"}, r.Err()
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, _ capability.Result) capability.Check {
				p, err := q.AIPolicy(ctx, e.OrgID)
				if err == nil && !p.Enabled {
					return capability.Check{Status: capability.VerifyOK}
				}
				return capability.Check{Status: capability.VerifyFailed}
			},
		},
	}
}

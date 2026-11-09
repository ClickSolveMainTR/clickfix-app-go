package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Team capabilities of the assistant (CF-189): list teams, create one,
// put a member in a team. Through /v1/tenant/teams as the user.

const teamsPath = "/v1/tenant/teams"

var idPattern = map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"}

func say(e capability.Exec, tr, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(tr, a...)
}

func text(p capability.Params, k string) string { s, _ := p[k].(string); return strings.TrimSpace(s) }

func uuidParam(p capability.Params, k string) (string, error) {
	v := text(p, k)
	if _, err := uuid.Parse(v); err != nil {
		return "", capability.Invalid("%s must be a UUID", k)
	}
	return v, nil
}

// Capabilities are the team capabilities.
func Capabilities() []*capability.Capability {
	routes := []string{"/teams"}
	return []*capability.Capability{
		{
			Name: "teams.list", Module: "teams", Kind: capability.Read, Permission: rbac.PermTenantTeamsRead, Routes: routes, Surfaces: capability.ReadSurfaces,
			Title:       map[string]string{"tr": "Takımları listele", "en": "List teams"},
			Description: "List the organization's teams with their uuids and members.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodGet, teamsPath+"?limit=50", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var out json.RawMessage
				decErr := r.Decode(&out)
				return capability.Result{Summary: "teams", Data: out}, decErr
			},
		},
		{
			Name: "teams.create", Module: "teams", Kind: capability.Write, Permission: rbac.PermTenantTeamsWrite, Routes: routes,
			Title:       map[string]string{"tr": "Takım oluştur", "en": "Create a team"},
			Description: "Create a team (e.g. Billing, Technical support) to route conversations to.",
			Undo:        "/teams",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name"},
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 80},
					"description": map[string]any{"type": "string", "title": "Description", "maxLength": 500},
				}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				if err := teamDuplicate(ctx, e, text(p, "name")); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: say(e, "“%s” takımı oluşturulacak.", "Team “%s” will be created.", text(p, "name"))}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				if err := teamDuplicate(ctx, e, text(p, "name")); err != nil {
					return capability.Result{}, err
				}
				body := map[string]any{"name": text(p, "name")}
				if d := text(p, "description"); d != "" {
					body["description"] = d
				}
				r, err := e.API.Call(ctx, http.MethodPost, teamsPath, body)
				if err != nil {
					return capability.Result{}, err
				}
				var t struct {
					UUID uuid.UUID `json:"uuid"`
				}
				if err := r.Decode(&t); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: t.UUID.String(), Route: "/teams"}, nil
			},
		},
		{
			Name: "teams.add_member", Module: "teams", Kind: capability.Write, Permission: rbac.PermTenantTeamsWrite, Routes: routes,
			Title:       map[string]string{"tr": "Takıma üye ekle", "en": "Add a member to a team"},
			Description: "Put an organization member (user_uuid from members_list) into a team (team_uuid from teams_list).",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"team_uuid", "user_uuid"},
				"properties": map[string]any{"team_uuid": idPattern, "user_uuid": idPattern,
					"role": map[string]any{"type": "string", "enum": []string{"member", "lead"}}}},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				team, err := uuidParam(p, "team_uuid")
				if err != nil {
					return capability.Result{}, err
				}
				user, err := uuidParam(p, "user_uuid")
				if err != nil {
					return capability.Result{}, err
				}
				role := text(p, "role")
				if role == "" {
					role = "member"
				}
				r, err := e.API.Call(ctx, http.MethodPut, teamsPath+"/"+team+"/members/"+user, map[string]any{"role": role})
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: team, Route: "/teams"}, r.Err()
			},
		},
	}
}

// teamDuplicate refuses a second team with the same name (dry run and apply).
func teamDuplicate(ctx context.Context, e capability.Exec, name string) error {
	r, err := e.API.Call(ctx, http.MethodGet, teamsPath+"?limit=100", nil)
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := r.Decode(&page); err != nil {
		return err
	}
	for _, t := range page.Items {
		if strings.EqualFold(t.Name, name) {
			return capability.Invalid("%s", say(e, "“%s” takımı zaten var.", "Team “%s” already exists.", t.Name))
		}
	}
	return nil
}

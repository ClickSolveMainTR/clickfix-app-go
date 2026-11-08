package assistant

import (
	"context"
	"regexp"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// focusRxp is a data-guide target the page highlights (?focus=).
var focusRxp = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,59}$`)

// InternalRoute reports whether route is a path inside the app: no scheme,
// host, query or fragment, never protocol-relative.
func InternalRoute(route string) bool {
	return routeRxp.MatchString(route) && !strings.HasPrefix(route, "//")
}

// ConnectableChannels are the channel types whose connect page the
// assistant may open.
var ConnectableChannels = []string{"whatsapp", "instagram", "messenger", "telegram", "email", "sms", "voice", "slack"}

// CoreCapabilities are the assistant's own capabilities: the three
// actions of CF-141 redefined in the registry (their old kinds stay as
// aliases so cards stored in threads still confirm).
func CoreCapabilities(q *db.Queries) []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "navigation.open_route", Aliases: []string{"open_route"}, Module: "navigation", Kind: capability.Write,
			Title:       map[string]string{"tr": "Sayfaya git", "en": "Go to page"},
			Description: "Open a page of the app for the user and optionally highlight a field or button on it (target = a data-guide key from the page guide).",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"route"},
				"properties": map[string]any{
					"route":  map[string]any{"type": "string", "title": "Page", "description": "Internal path, e.g. /settings/sla", "maxLength": 151},
					"target": map[string]any{"type": "string", "title": "Highlight", "description": "data-guide key of the field to highlight", "maxLength": 60},
				}},
			Validate: func(p capability.Params) error {
				if !InternalRoute(str(p, "route")) {
					return capability.Invalid("route must be an internal path like /settings/sla")
				}
				if t := str(p, "target"); t != "" && !focusRxp.MatchString(t) {
					return capability.Invalid("target must be a data-guide key")
				}
				return nil
			},
			Apply: func(_ context.Context, _ capability.Exec, p capability.Params) (capability.Result, error) {
				f := &capability.Focus{Route: str(p, "route"), Target: str(p, "target")}
				return capability.Result{Summary: f.Route, Route: f.Route, Focus: f}, nil
			},
		},
		{
			Name: "channels.connect", Aliases: []string{"connect_channel"}, Module: "channels", Kind: capability.Write,
			Permission:  rbac.PermTenantChannelsManage,
			Routes:      []string{"/channels"},
			Title:       map[string]string{"tr": "Kanal bağla", "en": "Connect a channel"},
			Description: "Open the connect form of a channel type whose credentials or OAuth sign-in the user completes on the page.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type"},
				"properties": map[string]any{
					"type": map[string]any{"type": "string", "title": "Channel", "enum": ConnectableChannels},
				}},
			Apply: func(_ context.Context, _ capability.Exec, p capability.Params) (capability.Result, error) {
				route := "/channels/connect/" + str(p, "type")
				return capability.Result{Summary: route, Route: route, Focus: &capability.Focus{Route: route}}, nil
			},
		},
		{
			Name: "ai.enable", Aliases: []string{"enable_ai"}, Module: "ai", Kind: capability.Write,
			Permission:  rbac.PermTenantAiWrite,
			Routes:      []string{"/settings/ai"},
			Title:       map[string]string{"tr": "Yapay zekâyı aç", "en": "Turn on AI"},
			Description: "Turn on AI features (reply suggestions, AI agent) for the organization.",
			Undo:        "/settings/ai",
			Apply: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				if q == nil {
					return capability.Result{}, ErrActionInvalid
				}
				if err := q.AssistantEnableAI(ctx, e.OrgID); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: "/settings/ai", Route: "/settings/ai", Focus: &capability.Focus{Route: "/settings/ai"}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, _ capability.Result) capability.Check {
				if q == nil {
					return capability.Check{Status: capability.VerifySkipped}
				}
				if p, err := q.AIPolicy(ctx, e.OrgID); err == nil && p.Enabled {
					return capability.Check{Status: capability.VerifyOK}
				}
				return capability.Check{Status: capability.VerifyFailed}
			},
		},
	}
}

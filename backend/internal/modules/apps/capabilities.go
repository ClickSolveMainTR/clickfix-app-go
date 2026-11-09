package apps

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities: the integrations status the assistant can read (CF-190).
// Connecting an app needs its own OAuth/consent screen, so the assistant
// sends the user to the integrations page (navigation.open_route).
func Capabilities() []*capability.Capability {
	return []*capability.Capability{{
		Name: "integrations.list", Module: "integrations", Kind: capability.Read, Permission: rbac.PermTenantConversationsRead,
		Routes: []string{"/settings/integrations"}, Surfaces: capability.ReadSurfaces,
		Title:       map[string]string{"tr": "Entegrasyonları listele", "en": "List integrations"},
		Description: "List the apps connected to the organization (name, status). To connect a new one, propose navigation_open_route to /settings/integrations.",
		Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
			r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/apps?limit=50", nil)
			if err != nil {
				return capability.Result{}, err
			}
			var out json.RawMessage
			decErr := r.Decode(&out)
			return capability.Result{Summary: "integrations", Data: out}, decErr
		},
	}}
}

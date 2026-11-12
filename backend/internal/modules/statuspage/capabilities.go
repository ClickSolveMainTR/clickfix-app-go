package statuspage

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the status page (CF-211): agents check open incidents
// before answering "is something down?". Opening an incident notifies
// subscribers, so it stays on the status page for now.

// Capabilities are the status page capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{{
		Name: "status.incidents", Module: "status", Kind: capability.Read, Permission: rbac.PermTenantConversationsRead,
		Surfaces: capability.ReadSurfaces, Routes: []string{"/settings/status", "/team-status"},
		Title:       capability.T("Olaylar ve bakımlar", "Incidents and maintenance"),
		Description: "Status page incidents and maintenance with their updates.",
		Input:       capability.Object(nil, capability.Paged(map[string]any{})),
		Run:         capability.Get("/v1/tenant/status/incidents", "limit", "offset"),
	}}
}

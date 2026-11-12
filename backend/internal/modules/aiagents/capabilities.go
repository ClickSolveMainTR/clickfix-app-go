package aiagents

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the organization's AI agent reports (CF-211): how the AI
// performs and which topics the knowledge base misses.

// Capabilities are the AI agent report capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/settings/ai"}
	rfc := capability.Text(40, "RFC3339")
	return []*capability.Capability{
		{
			Name: "ai.performance", Module: "ai", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("AI performansı", "AI performance"), Description: "AI agent results (default last 30 days): resolution and handoff rate, cost, CSAT, saved hours, growing topics.",
			Input: capability.Object(nil, map[string]any{"since": rfc, "until": rfc}),
			Run:   capability.Get("/v1/tenant/ai/performance", "since", "until"),
		},
		{
			Name: "ai.knowledge_gaps", Module: "ai", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Bilgi eksikleri", "Knowledge gaps"), Description: "Topics customers asked that the knowledge base did not answer.",
			Input: capability.Object(nil, capability.Paged(map[string]any{"since": rfc})),
			Run:   capability.Get("/v1/tenant/ai/knowledge-gaps", "since", "limit", "offset"),
		},
	}
}

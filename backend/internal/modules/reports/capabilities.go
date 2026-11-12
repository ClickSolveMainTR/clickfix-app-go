package reports

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of reports and analytics (CF-211): the dashboard numbers,
// saved reports and survey (CSAT/NPS) results, read as the member. A saved
// report is evaluated with POST .../evaluate, which only runs a query; it
// is allowed through the read-only API as a simulation.

// Capabilities are the report capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/reports", "/dashboard"}
	rfc := capability.Text(40, "RFC3339")
	evaluate := "POST /v1/tenant/reports/{uuid}/evaluate"
	return []*capability.Capability{
		{
			Name: "analytics.dashboard", Module: "reports", Kind: capability.Read, Permission: rbac.PermTenantConversationsReadAll, Surfaces: s, Routes: routes,
			Title:       capability.T("Pano metrikleri", "Dashboard metrics"),
			Description: "Support metrics between from and to (RFC3339, to exclusive): volume, resolved, backlog, first response and resolution time, SLA hit %, CSAT, by channel/team/agent/tag, vs previous period.",
			Input: capability.Object([]string{"from", "to"}, map[string]any{"from": rfc, "to": rfc, "bucket": capability.Enum("hour", "day", "week", "month"),
				"channel": capability.Text(40, ""), "team": capability.UUID(""), "agent": capability.UUID(""), "tag": capability.UUID("")}),
			Run: capability.Get("/v1/tenant/analytics/dashboard", "from", "to", "bucket", "channel", "team", "agent", "tag"),
		},
		{
			Name: "reports.list", Module: "reports", Kind: capability.Read, Permission: rbac.PermTenantConversationsReadAll, Surfaces: s, Routes: routes,
			Title: capability.T("Kayıtlı raporlar", "Saved reports"), Description: "Saved custom reports and their definitions.",
			Input: capability.Object(nil, capability.Paged(map[string]any{})),
			Run:   capability.Get("/v1/tenant/reports", "limit", "offset"),
		},
		{
			Name: "reports.run", Module: "reports", Kind: capability.Read, Permission: rbac.PermTenantConversationsReadAll, Surfaces: s, Routes: routes,
			Title: capability.T("Raporu çalıştır", "Run a report"), Description: "Run a saved report between from and to (RFC3339); changes no data.",
			Simulations: []string{evaluate},
			Input:       capability.Object([]string{"report_uuid", "from", "to"}, map[string]any{"report_uuid": capability.UUID(""), "from": rfc, "to": rfc}),
			Run:         capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/reports/{report_uuid}/evaluate", Body: []string{"from", "to"}}.Do(),
		},
		{
			Name: "surveys.results", Module: "reports", Kind: capability.Read, Permission: rbac.PermTenantConversationsReadAll, Surfaces: s, Routes: routes,
			Title: capability.T("Anket sonuçları", "Survey results"), Description: "CSAT / NPS results between from and to (RFC3339).",
			Input: capability.Object([]string{"from", "to"}, map[string]any{"from": rfc, "to": rfc}),
			Run:   capability.Get("/v1/tenant/surveys/results", "from", "to"),
		},
	}
}

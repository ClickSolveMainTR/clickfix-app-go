package audit

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the audit log (CF-211): "who changed this?" as a read.

// Capabilities are the audit capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{{
		Name: "audit.list", Module: "audit", Kind: capability.Read, Permission: rbac.PermTenantAuditRead,
		Surfaces: capability.ReadSurfaces, Routes: []string{"/settings/audit"},
		Title:       capability.T("Denetim kaydı", "Audit log"),
		Description: "Audit log, newest first: q (action/resource), action and resource csv, actor (user uuid), from/to (YYYY-MM-DD or RFC3339).",
		Input: capability.Object(nil, capability.Paged(map[string]any{"q": capability.Text(200, ""), "action": capability.Text(400, ""),
			"resource": capability.Text(400, ""), "actor": capability.UUID(""), "from": capability.Text(40, ""), "to": capability.Text(40, "")})),
		Run: capability.Get("/v1/tenant/audit", "q", "action", "resource", "actor", "from", "to", "limit", "offset"),
	}}
}

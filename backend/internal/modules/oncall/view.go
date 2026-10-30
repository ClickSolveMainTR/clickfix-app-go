package oncall

import (
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

func view(v any) any {
	switch x := v.(type) {
	case db.OncallRota:
		return map[string]any{"uuid": x.Uuid, "name": x.Name, "timezone": x.Timezone, "layers": json.RawMessage(x.Layers), "overrides": json.RawMessage(x.Overrides), "created_at": x.CreatedAt, "updated_at": x.UpdatedAt}
	case db.EscalationPolicy:
		return map[string]any{"uuid": x.Uuid, "name": x.Name, "steps": json.RawMessage(x.Steps), "urgent_conversations": x.UrgentConversations, "incidents": x.Incidents, "enabled": x.Enabled, "created_at": x.CreatedAt, "updated_at": x.UpdatedAt}
	case db.UptimeMonitor:
		return map[string]any{"uuid": x.Uuid, "name": x.Name, "url": x.Url, "interval_seconds": x.IntervalSeconds, "expected_status": x.ExpectedStatus, "keyword": x.Keyword, "enabled": x.Enabled, "next_check_at": x.NextCheckAt, "last_checked_at": x.LastCheckedAt, "last_up": x.LastUp, "last_status": x.LastStatus}
	case []db.OncallRota:
		out := make([]any, 0, len(x))
		for _, r := range x {
			out = append(out, view(r))
		}
		return out
	case []db.EscalationPolicy:
		out := make([]any, 0, len(x))
		for _, r := range x {
			out = append(out, view(r))
		}
		return out
	case map[string]any:
		if items, ok := x["items"]; ok {
			x["items"] = view(items)
		}
		return x
	default:
		return v
	}
}

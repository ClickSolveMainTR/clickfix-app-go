package reports

import (
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

func reportView(r db.CustomReport) any {
	return map[string]any{"uuid": r.Uuid, "name": r.Name, "definition": json.RawMessage(r.Definition), "created_at": r.CreatedAt, "updated_at": r.UpdatedAt}
}

func view(v any) any {
	switch x := v.(type) {
	case db.CustomReport:
		return reportView(x)
	case []db.CustomReport:
		out := make([]any, 0, len(x))
		for _, r := range x {
			out = append(out, reportView(r))
		}
		return out
	case db.ReportSchedule:
		return map[string]any{"uuid": x.Uuid, "frequency": x.Frequency, "timezone": x.Timezone, "format": x.Format, "recipients": x.Recipients, "enabled": x.Enabled, "next_run_at": x.NextRunAt}
	case map[string]any:
		if items, ok := x["items"]; ok {
			x["items"] = view(items)
		}
		return x
	default:
		return v
	}
}

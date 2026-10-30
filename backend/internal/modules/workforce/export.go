package workforce

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"time"
)

const ResourceTimesheets = "tenant.workforce.timesheets"

// TimesheetAdapter adapts the same scoped dataset for API and worker registries.
type TimesheetAdapter struct{ Service *Service }

func (*TimesheetAdapter) Resource() string { return ResourceTimesheets }
func (*TimesheetAdapter) ExportColumns() []ioengine.Column {
	cols := []ioengine.Column{}
	for _, key := range []string{"agent_uuid", "conversation_uuid", "state", "seconds"} {
		cols = append(cols, ioengine.Column{Key: key, LabelKey: "workforce." + key, Type: ioengine.ColumnTypeString})
	}
	return cols
}
func (a *TimesheetAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	scope, ok := orgctx.From(ctx)
	if !ok {
		return ioengine.Dataset{}, supportops.ErrDenied
	}
	since, err := time.Parse(time.RFC3339, q["since"])
	if err != nil {
		return ioengine.Dataset{}, supportops.ErrInvalid
	}
	until, err := time.Parse(time.RFC3339, q["until"])
	if err != nil {
		return ioengine.Dataset{}, supportops.ErrInvalid
	}
	var agent *uuid.UUID
	if v := q["agent_uuid"]; v != "" {
		id, e := uuid.Parse(v)
		if e != nil || id == uuid.Nil {
			return ioengine.Dataset{}, supportops.ErrInvalid
		}
		agent = &id
	}
	rows, err := a.Service.Timesheets(ctx, scope.ID, since, until, agent)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds := ioengine.Dataset{Resource: a.Resource(), Columns: a.ExportColumns(), Rows: []map[string]any{}}
	for _, r := range rows {
		var c any
		if r.ConversationUuid.Valid {
			c = uuid.UUID(r.ConversationUuid.Bytes).String()
		}
		ds.Rows = append(ds.Rows, map[string]any{"agent_uuid": r.AgentUuid.String(), "conversation_uuid": c, "state": r.State, "seconds": r.Seconds})
	}
	return ds, nil
}
func (*TimesheetAdapter) ImportSchema() []ioengine.ImportField { return nil }
func (*TimesheetAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, supportops.ErrDenied
}
func (*TimesheetAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return supportops.ErrDenied
}

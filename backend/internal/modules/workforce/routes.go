package workforce

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/google/uuid"
	"net/http"
)

func AgentQuery(r *http.Request) (*uuid.UUID, error) {
	v := r.URL.Query().Get("agent_uuid")
	if v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil || id == uuid.Nil {
		return nil, supportops.ErrInvalid
	}
	return &id, nil
}
func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("POST /v1/tenant/workforce/forecast", "tenant.schedules.read", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ForecastInput](r)
		if err != nil {
			return nil, err
		}
		return s.Forecast(r.Context(), orgctx.Must(r.Context()).ID, in)
	})
	rt.Tenant("POST /v1/tenant/workforce/activity", "tenant.schedules.read", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ActivityInput](r)
		if err != nil {
			return nil, err
		}
		return s.Activity(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("GET /v1/tenant/workforce/timesheets/meta", "tenant.schedules.manage", func(*http.Request) (any, error) {
		return map[string]any{"resource": ResourceTimesheets, "default_sort": "", "default_fields": []string{"agent_uuid", "conversation_uuid", "state", "seconds"}, "capabilities": resourcemeta.Capabilities{Read: true, Export: true}, "searchable_fields": []string{}, "sortable_fields": []string{}, "filterable_fields": []string{"since", "until", "agent_uuid"}, "columns": []any{}, "filters": []any{}, "includes": []string{}, "bulk_actions": []any{}}, nil
	})
	rt.Tenant("GET /v1/tenant/workforce/timesheets", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		a, b, err := supportops.Window(r)
		if err != nil {
			return nil, err
		}
		agent, err := AgentQuery(r)
		if err != nil {
			return nil, err
		}
		return s.Timesheets(r.Context(), orgctx.Must(r.Context()).ID, a, b, agent)
	})
	if rt.Resolver == nil {
		return
	}
	rt.Mux.Handle("GET /v1/tenant/workforce/timesheets/export", middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := ioengine.ExportFormat(r.URL.Query().Get("format"))
		if format == "" {
			format = ioengine.ExportCSV
		}
		if format != ioengine.ExportCSV && format != ioengine.ExportXLSX && format != ioengine.ExportJSON {
			supportops.Fail(w, r, supportops.ErrInvalid)
			return
		}
		adapter := &TimesheetAdapter{Service: s}
		q := ioengine.ExportQuery{}
		for _, key := range []string{"since", "until", "agent_uuid"} {
			q[key] = r.URL.Query().Get(key)
		}
		ds, err := adapter.Export(r.Context(), q, i18n.Normalize("tr"))
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		data, err := ioengine.EncodeExport(format, ds, "tr", nil, "Timesheets")
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		supportops.Record(rt.Activity, r, "support_operations.updated", nil)
		ct := "text/csv; charset=utf-8"
		if format == ioengine.ExportXLSX {
			ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
		if format == ioengine.ExportJSON {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", `attachment; filename="timesheets.`+string(format)+`"`)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	}), middleware.Authenticate(rt.Tokens, rt.Loader), middleware.RequireOrganization(rt.Resolver), middleware.RequireTenantPermission("tenant.schedules.manage")))
}

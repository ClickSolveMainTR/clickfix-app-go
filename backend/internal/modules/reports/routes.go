package reports

import (
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

func RegisterRoutes(rt supportops.Router, s *usecase.Service) {
	rt.Mux.HandleFunc("GET /v1/public/storage/reports/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		org, _ := strconv.ParseInt(q.Get("org"), 10, 64)
		expires, _ := strconv.ParseInt(q.Get("expires"), 10, 64)
		recipient, _ := uuid.Parse(q.Get("recipient"))
		delivery, _ := uuid.Parse(r.PathValue("uuid"))
		out, err := s.Download(r.Context(), org, delivery, recipient, expires, q.Get("signature"))
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		mime := "text/csv; charset=utf-8"
		if out.Format == "xlsx" {
			mime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", `attachment; filename="report.`+out.Format+`"`)
		_, _ = w.Write(out.Content)
	})

	route := func(p string, write bool, fn supportops.Endpoint) {
		rt.Tenant(p, rbac.PermTenantConversationsReadAll, func(r *http.Request) (any, error) {
			scope := orgctx.Must(r.Context())
			if !scope.Has(rbac.PermTenantConversationsRead) || (write && !scope.Has(rbac.PermTenantConversationsManage)) {
				return nil, supportops.ErrDenied
			}
			out, err := fn(r)
			return view(out), err
		})
	}
	route("GET /v1/tenant/reports/meta", false, func(*http.Request) (any, error) {
		return map[string]any{"dimensions": []string{"agent", "team", "channel", "day"}, "max_dimensions": 1, "metrics": model.Metrics, "chart_types": []string{"table", "bar", "line", "pie"}}, nil
	})
	route("GET /v1/tenant/agent-scorecards", false, func(r *http.Request) (any, error) {
		from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, supportops.ErrInvalid
		}
		to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, supportops.ErrInvalid
		}
		d := model.Definition{Dimensions: []string{"agent"}, Metrics: model.Metrics, ChartType: "table", Timezone: "UTC", Filters: model.Filters{Channel: r.URL.Query().Get("channel")}}
		if team := r.URL.Query().Get("team"); team != "" {
			id, e := uuid.Parse(team)
			if e != nil {
				return nil, supportops.ErrInvalid
			}
			d.Filters.Team = &id
		}
		return usecase.Evaluate(r.Context(), s.Repo.Q, orgctx.Must(r.Context()).ID, d, model.Evaluation{From: from, To: to})
	})
	route("GET /v1/tenant/reports", false, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.ReportsList(r.Context(), db.ReportsListParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.ReportsCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	for _, p := range []string{"POST /v1/tenant/reports", "PUT /v1/tenant/reports/{uuid}"} {
		route(p, true, func(r *http.Request) (any, error) {
			in, err := supportops.Read[model.Input](r)
			if err != nil {
				return nil, err
			}
			if r.Method == http.MethodPut && supportops.ID(r) == uuid.Nil {
				return nil, supportops.ErrInvalid
			}
			return s.Save(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in)
		})
	}
	route("GET /v1/tenant/reports/{uuid}", false, func(r *http.Request) (any, error) {
		return s.Repo.Q.ReportsGet(r.Context(), db.ReportsGetParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
	})
	route("DELETE /v1/tenant/reports/{uuid}", true, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.ReportsDelete(r.Context(), db.ReportsDeleteParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		return map[string]bool{"deleted": n > 0}, err
	})
	route("POST /v1/tenant/reports/{uuid}/evaluate", false, func(r *http.Request) (any, error) {
		in, err := supportops.Read[model.Evaluation](r)
		if err != nil {
			return nil, err
		}
		return s.Evaluate(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
	})
	route("POST /v1/tenant/report-schedules", true, func(r *http.Request) (any, error) {
		in, err := supportops.Read[model.ScheduleInput](r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		report, err := s.Repo.Q.ReportsGet(r.Context(), db.ReportsGetParams{OrganizationID: org, Uuid: in.ReportUUID})
		if err != nil {
			return nil, err
		}
		row, err := s.Schedule(r.Context(), org, supportops.Actor(r.Context()), in)
		if err != nil {
			return nil, err
		}
		out := view(row).(map[string]any)
		out["report_uuid"] = report.Uuid
		out["name"] = report.Name
		return out, nil
	})
	route("GET /v1/tenant/report-schedules", false, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.ReportsSchedules(r.Context(), db.ReportsSchedulesParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.ReportsScheduleCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	route("DELETE /v1/tenant/report-schedules/{uuid}", true, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.ReportsDeleteSchedule(r.Context(), db.ReportsDeleteScheduleParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		return map[string]bool{"deleted": n > 0}, err
	})
}

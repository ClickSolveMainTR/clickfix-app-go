package oncall

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	read := rbac.PermTenantSchedulesRead
	write := rbac.PermTenantSchedulesManage
	// On-call users need organization-wide conversation access because alerts can refer to any team.
	route := func(p string, perm string, fn supportops.Endpoint) {
		rt.Tenant(p, perm, func(r *http.Request) (any, error) {
			if !orgctx.Must(r.Context()).Has(rbac.PermTenantConversationsReadAll) {
				return nil, supportops.ErrDenied
			}
			out, err := fn(r)
			return view(out), err
		})
	}
	route("GET /v1/tenant/oncall/rotas", read, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.OncallRotas(r.Context(), db.OncallRotasParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.OncallRotaCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	for _, p := range []string{"POST /v1/tenant/oncall/rotas", "PUT /v1/tenant/oncall/rotas/{uuid}"} {
		route(p, write, func(r *http.Request) (any, error) {
			in, err := supportops.Read[RotaInput](r)
			if err != nil {
				return nil, err
			}
			if r.Method == http.MethodPut && supportops.ID(r) == uuid.Nil {
				return nil, supportops.ErrInvalid
			}
			return s.Rota(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
		})
	}
	route("DELETE /v1/tenant/oncall/rotas/{uuid}", write, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.OncallDeleteRota(r.Context(), db.OncallDeleteRotaParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		return map[string]bool{"deleted": n > 0}, err
	})
	route("GET /v1/tenant/oncall/rotas/{uuid}/current", read, func(r *http.Request) (any, error) {
		at := time.Now()
		if raw := r.URL.Query().Get("at"); raw != "" {
			var err error
			at, err = time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, supportops.ErrInvalid
			}
		}
		ids, err := s.current(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), at)
		return map[string]any{"agents": ids, "at": at}, err
	})
	route("GET /v1/tenant/oncall/policies", read, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.OncallPolicies(r.Context(), db.OncallPoliciesParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.OncallPolicyCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	for _, p := range []string{"POST /v1/tenant/oncall/policies", "PUT /v1/tenant/oncall/policies/{uuid}"} {
		route(p, write, func(r *http.Request) (any, error) {
			in, err := supportops.Read[PolicyInput](r)
			if err != nil {
				return nil, err
			}
			if r.Method == http.MethodPut && supportops.ID(r) == uuid.Nil {
				return nil, supportops.ErrInvalid
			}
			return s.Policy(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
		})
	}
	route("DELETE /v1/tenant/oncall/policies/{uuid}", write, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.OncallDeletePolicy(r.Context(), db.OncallDeletePolicyParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		return map[string]bool{"deleted": n > 0}, err
	})
	route("GET /v1/tenant/oncall/alerts", read, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.OncallAlerts(r.Context(), db.OncallAlertsParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.OncallAlertCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	route("POST /v1/tenant/oncall/alerts/{uuid}/acknowledge", read, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.OncallAcknowledge(r.Context(), db.OncallAcknowledgeParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r), AcknowledgedBy: pgtype.Int8{Int64: supportops.Actor(r.Context()), Valid: true}})
		return map[string]bool{"acknowledged": n > 0}, err
	})
	route("GET /v1/tenant/uptime-monitors", read, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.OncallMonitors(r.Context(), db.OncallMonitorsParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.OncallMonitorCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	for _, p := range []string{"POST /v1/tenant/uptime-monitors", "PUT /v1/tenant/uptime-monitors/{uuid}"} {
		route(p, write, func(r *http.Request) (any, error) {
			in, err := supportops.Read[MonitorInput](r)
			if err != nil {
				return nil, err
			}
			if r.Method == http.MethodPut && supportops.ID(r) == uuid.Nil {
				return nil, supportops.ErrInvalid
			}
			row, err := s.Monitor(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
			if err != nil {
				return nil, err
			}
			out := view(row).(map[string]any)
			out["component_uuid"] = in.Component
			out["policy_uuid"] = in.Policy
			return out, nil
		})
	}
	route("DELETE /v1/tenant/uptime-monitors/{uuid}", write, func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.OncallDeleteMonitor(r.Context(), db.OncallDeleteMonitorParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		return map[string]bool{"deleted": n > 0}, err
	})
	route("GET /v1/tenant/uptime-monitors/{uuid}/results", read, func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.OncallResults(r.Context(), db.OncallResultsParams{OrganizationID: org, Uuid: supportops.ID(r), RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.OncallResultCount(r.Context(), db.OncallResultCountParams{OrganizationID: org, Uuid: supportops.ID(r)})
		return supportops.Page(rows, n, q), err
	})
}

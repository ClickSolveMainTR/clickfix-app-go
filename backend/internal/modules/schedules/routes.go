package schedules

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("GET /v1/tenant/schedules/templates/meta", "tenant.schedules.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.schedules.templates"), nil })

	rt.Tenant("GET /v1/tenant/schedules/settings", "tenant.schedules.read", func(r *http.Request) (any, error) {
		v, err := s.Repo.Q.ScheduleSettings(r.Context(), orgctx.Must(r.Context()).ID)
		return map[string]bool{"enforce_shifts": v}, err
	})
	rt.Tenant("PUT /v1/tenant/schedules/settings", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Enforce bool `json:"enforce_shifts"`
		}](r)
		if err != nil {
			return nil, err
		}
		return in, s.Repo.Q.ScheduleSaveSettings(r.Context(), db.ScheduleSaveSettingsParams{OrganizationID: orgctx.Must(r.Context()).ID, EnforceShifts: in.Enforce})
	})
	rt.Tenant("GET /v1/tenant/schedules/templates", "tenant.schedules.read", func(r *http.Request) (any, error) {
		return s.Repo.Q.ScheduleTemplates(r.Context(), orgctx.Must(r.Context()).ID)
	})
	rt.Tenant("POST /v1/tenant/schedules/templates", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[TemplateInput](r)
		if err != nil {
			return nil, err
		}
		return s.Template(r.Context(), orgctx.Must(r.Context()).ID, in)
	})
	rt.Tenant("GET /v1/tenant/schedules/calendar", "tenant.schedules.read", func(r *http.Request) (any, error) {
		a, b, err := supportops.Window(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		shifts, err := s.Repo.Q.ScheduleShifts(r.Context(), db.ScheduleShiftsParams{OrganizationID: org, EndsAt: repository.TS(a), StartsAt: repository.TS(b)})
		if err != nil {
			return nil, err
		}
		off, err := s.Repo.Q.ScheduleTimeOff(r.Context(), db.ScheduleTimeOffParams{OrganizationID: org, EndsAt: repository.TS(a), StartsAt: repository.TS(b)})
		return map[string]any{"shifts": shifts, "time_off": off}, err
	})
	rt.Tenant("POST /v1/tenant/schedules/shifts", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ShiftInput](r)
		if err != nil {
			return nil, err
		}
		return s.Shift(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("DELETE /v1/tenant/schedules/shifts/{uuid}", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.ScheduleDeleteShift(r.Context(), db.ScheduleDeleteShiftParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r)})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"deleted": true}, err
	})
	rt.Tenant("POST /v1/tenant/schedules/time-off", "tenant.schedules.read", func(r *http.Request) (any, error) {
		in, err := supportops.Read[LeaveInput](r)
		if err != nil {
			return nil, err
		}
		return s.Leave(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("PATCH /v1/tenant/schedules/time-off/{uuid}", "tenant.schedules.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Status string `json:"status"`
		}](r)
		if err != nil {
			return nil, err
		}
		return s.Review(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Status)
	})
	rt.Tenant("DELETE /v1/tenant/schedules/time-off/{uuid}", "tenant.schedules.read", func(r *http.Request) (any, error) {
		n, err := s.Repo.Q.ScheduleCancelLeave(r.Context(), db.ScheduleCancelLeaveParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r), UserID: supportops.Actor(r.Context())})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"canceled": true}, err
	})
}

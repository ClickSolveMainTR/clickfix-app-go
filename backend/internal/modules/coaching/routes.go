package coaching

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("GET /v1/tenant/quality/calibrations/meta", "tenant.quality.manage", func(*http.Request) (any, error) { return supportops.Meta("tenant.quality.calibrations"), nil })
	rt.Tenant("GET /v1/tenant/quality/calibrations", "tenant.quality.manage", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.CalibrationList(r.Context(), db.CalibrationListParams{OrganizationID: org, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		// Team-restricted reviewers may not discover conversations outside their inbox scope.
		visible := make([]db.CalibrationListRow, 0, len(rows))
		for _, row := range rows {
			if _, e := supportops.Conversation(r.Context(), s.Repo.Q, org, supportops.Actor(r.Context()), row.ConversationUuid); e == nil {
				visible = append(visible, row)
			} else {
				return nil, e
			}
		}
		n, err := s.Repo.Q.CalibrationCount(r.Context(), org)
		return supportops.Page(visible, n, q), err
	})
	rt.Tenant("POST /v1/tenant/quality/calibrations", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[SessionInput](r)
		if err != nil {
			return nil, err
		}
		return s.Session(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("GET /v1/tenant/quality/calibrations/{uuid}", "tenant.quality.manage", func(r *http.Request) (any, error) {
		return s.Report(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r))
	})
	rt.Tenant("POST /v1/tenant/quality/calibrations/{uuid}/scores", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Scores []quality.Score `json:"scores"`
		}](r)
		if err != nil {
			return nil, err
		}
		return s.Score(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Scores)
	})
	rt.Tenant("POST /v1/tenant/quality/calibrations/{uuid}/close", "tenant.quality.manage", func(r *http.Request) (any, error) {
		err := s.Close(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r))
		return map[string]bool{"closed": err == nil}, err
	})
	rt.Tenant("GET /v1/tenant/coaching/meta", "tenant.quality.manage", func(*http.Request) (any, error) { return supportops.Meta("tenant.coaching"), nil })
	rt.Tenant("POST /v1/tenant/coaching", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ItemInput](r)
		if err != nil {
			return nil, err
		}
		return s.Item(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("GET /v1/tenant/coaching", "tenant.quality.manage", func(r *http.Request) (any, error) {
		agent, err := uuid.Parse(r.URL.Query().Get("agent_uuid"))
		if err != nil || agent == uuid.Nil {
			return nil, supportops.ErrInvalid
		}
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := s.Repo.Q.CoachingList(r.Context(), db.CoachingListParams{OrganizationID: org, Uuid: agent, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.CoachingCount(r.Context(), db.CoachingCountParams{OrganizationID: org, Uuid: agent})
		return supportops.Page(rows, n, q), err
	})
	rt.Tenant("PATCH /v1/tenant/coaching/{uuid}", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Completed *bool `json:"completed"`
		}](r)
		if err != nil || in.Completed == nil {
			return nil, supportops.ErrInvalid
		}
		n, err := s.Repo.Q.CoachingComplete(r.Context(), db.CoachingCompleteParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r), Completed: *in.Completed})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"completed": *in.Completed}, err
	})
	rt.Tenant("GET /v1/tenant/gamification/rules", "tenant.schedules.read", func(r *http.Request) (any, error) {
		return s.Repo.Q.GameRules(r.Context(), orgctx.Must(r.Context()).ID)
	})
	rt.Tenant("PUT /v1/tenant/gamification/rules", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[RuleInput](r)
		if err != nil {
			return nil, err
		}
		return s.Rule(r.Context(), orgctx.Must(r.Context()).ID, in)
	})
	rt.Tenant("GET /v1/tenant/gamification/preferences", "tenant.schedules.read", func(r *http.Request) (any, error) {
		v, err := s.Repo.Q.GamePreference(r.Context(), db.GamePreferenceParams{OrganizationID: orgctx.Must(r.Context()).ID, UserID: supportops.Actor(r.Context())})
		return map[string]bool{"opted_out": v}, err
	})
	rt.Tenant("PUT /v1/tenant/gamification/preferences", "tenant.schedules.read", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			OptedOut *bool `json:"opted_out"`
		}](r)
		if err != nil || in.OptedOut == nil {
			return nil, supportops.ErrInvalid
		}
		err = s.Repo.Q.GameOptOut(r.Context(), db.GameOptOutParams{OrganizationID: orgctx.Must(r.Context()).ID, UserID: supportops.Actor(r.Context()), OptedOut: *in.OptedOut})
		return map[string]bool{"opted_out": *in.OptedOut}, err
	})
	rt.Tenant("GET /v1/tenant/gamification/leaderboard", "tenant.schedules.read", func(r *http.Request) (any, error) {
		a, b, err := supportops.Window(r)
		if err != nil {
			return nil, err
		}
		var team *uuid.UUID
		if v := r.URL.Query().Get("team_uuid"); v != "" {
			id, e := uuid.Parse(v)
			if e != nil || id == uuid.Nil {
				return nil, supportops.ErrInvalid
			}
			team = &id
		}
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		rows, err := s.Leaderboard(r.Context(), orgctx.Must(r.Context()).ID, a, b, team)
		if err != nil {
			return nil, err
		}
		n := len(rows)
		start := min(int(q.Offset), n)
		end := min(start+int(q.Limit), n)
		return supportops.Page(rows[start:end], int64(n), q), nil
	})
}

package workforce

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

type Service struct{ Repo *repository.Repository }

func OptionalUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
func (s *Service) Forecast(ctx context.Context, org int64, in ForecastInput) (any, error) {
	now := time.Now().UTC()
	if err := in.validate(now); err != nil {
		return nil, err
	}
	if in.Team != nil {
		if _, err := s.Repo.Q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: org, Uuid: *in.Team}); err != nil {
			return nil, err
		}
	}
	tz, err := s.Repo.Q.AnalyticsTimezone(ctx, org)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	end := now.Truncate(time.Hour)
	start := end.Add(-8 * 7 * 24 * time.Hour)
	rows, err := s.Repo.Q.WorkforceHistory(ctx, db.WorkforceHistoryParams{OrganizationID: org, Since: repository.TS(start), Until: repository.TS(end), Channel: in.Channel, Team: OptionalUUID(in.Team)})
	if err != nil {
		return nil, err
	}
	counts := map[time.Time]float64{}
	for _, r := range rows {
		counts[r.Bucket.Time.UTC()] = float64(r.Volume)
	}
	model := Seasonal(start, end, loc, counts, in.Alpha)
	coverage, err := s.Repo.Q.WorkforceCoverage(ctx, db.WorkforceCoverageParams{OrganizationID: org, Since: repository.TS(in.Since), Until: repository.TS(in.Until), Team: OptionalUUID(in.Team)})
	if err != nil {
		return nil, err
	}
	hours := make([]Bucket, 0, len(coverage))
	days := []Daily{}
	for _, c := range coverage {
		t := c.Bucket.Time
		local := t.In(loc)
		volume := model[int(local.Weekday())*24+local.Hour()]
		required, limited := RequiredAgents(volume, in.HandleSeconds, in.SLASeconds, in.TargetSLA)
		gap := int64(required) - c.Agents
		if gap < 0 {
			gap = 0
		}
		hours = append(hours, Bucket{Start: t, Volume: volume, Required: required, Scheduled: c.Agents, Gap: gap, CapacityLimited: limited})
		date := local.Format("2006-01-02")
		if len(days) == 0 || days[len(days)-1].Date != date {
			days = append(days, Daily{Date: date})
		}
		d := &days[len(days)-1]
		d.Volume += volume
		d.Required += required
		d.Scheduled += c.Agents
		d.Gap += gap
		d.CapacityLimited = d.CapacityLimited || limited
	}
	var buckets any = hours
	if in.Granularity == "day" {
		buckets = days
	}
	return map[string]any{"timezone": tz, "model": "weekday_hour_ewma", "history_since": start, "history_until": end, "parameters": in, "history_conversations": sumCounts(rows), "buckets": buckets}, nil
}
func sumCounts(rows []db.WorkforceHistoryRow) int64 {
	var n int64
	for _, r := range rows {
		n += r.Volume
	}
	return n
}

type ActivityInput struct {
	ClientUUID   uuid.UUID  `json:"client_uuid"`
	State        string     `json:"state"`
	Conversation *uuid.UUID `json:"conversation_uuid"`
}

func (s *Service) Activity(ctx context.Context, org, actor int64, in ActivityInput) (db.WorkforceInsertEventRow, error) {
	var out db.WorkforceInsertEventRow
	if in.ClientUUID == uuid.Nil || (in.State != "active" && in.State != "away" && in.State != "handling" && in.State != "offline") || ((in.State == "handling") != (in.Conversation != nil)) {
		return out, supportops.ErrInvalid
	}
	var conv pgtype.Int8
	if in.Conversation != nil {
		c, err := supportops.Conversation(ctx, s.Repo.Q, org, actor, *in.Conversation)
		if err != nil {
			return out, err
		}
		conv = pgtype.Int8{Int64: c.ID, Valid: true}
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.ScheduleLockMember(ctx, db.ScheduleLockMemberParams{OrganizationID: org, UserID: actor}); err != nil {
			return err
		}
		var err error
		out, err = r.Q.WorkforceInsertEvent(ctx, db.WorkforceInsertEventParams{OrganizationID: org, UserID: actor, ClientUuid: in.ClientUUID, State: in.State, ConversationID: conv})
		if err != nil {
			return err
		}
		ev := events.New("workforce.activity").WithOrganization(org).WithActor(actor).WithEntity("support_operations", nil, &out.Uuid).WithIdempotencyKey(out.Uuid.String())
		ev.OccurredAt = out.OccurredAt.Time
		return r.Emit(ctx, ev)
	})
	return out, err
}
func (s *Service) Timesheets(ctx context.Context, org int64, since, until time.Time, agent *uuid.UUID) ([]db.WorkforceTimesheetsRow, error) {
	if !until.After(since) || until.Sub(since) > 31*24*time.Hour {
		return nil, supportops.ErrInvalid
	}
	rows, err := s.Repo.Q.WorkforceTimesheets(ctx, db.WorkforceTimesheetsParams{OrganizationID: org, Since: repository.TS(since), Until: repository.TS(until), Agent: OptionalUUID(agent)})
	if len(rows) > 10000 {
		return nil, supportops.ErrConflict
	}
	return rows, err
}

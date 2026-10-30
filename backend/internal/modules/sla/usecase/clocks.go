package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Sweep limits.
const (
	sweepBatch   = 200
	sweepBatches = 25
	resyncBatch  = 200
)

// Resyncer schedules the recomputation of an organization's open
// conversations after a policy, schedule or holiday change.
type Resyncer interface {
	EnqueueSLAResync(ctx context.Context, orgID int64) error
}

// WithResyncer sets the resync queue. Without one the resync runs in the
// background of the request process.
func (s *Service) WithResyncer(r Resyncer) *Service { s.resyncer = r; return s }

func (s *Service) resyncLater(orgID int64) {
	ctx := context.Background()
	if s.resyncer != nil {
		err := s.resyncer.EnqueueSLAResync(ctx, orgID)
		if err == nil {
			return
		}
		s.log.Warn("sla_resync_enqueue_failed", "org", orgID, "error", err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := s.ResyncOrganization(ctx, orgID); err != nil {
			s.log.Warn("sla_resync_failed", "org", orgID, "error", err)
		}
	}()
}

// ResyncOrganization recomputes the clocks of every open conversation.
func (s *Service) ResyncOrganization(ctx context.Context, orgID int64) error {
	after := int64(0)
	for {
		ids, err := s.q.ListOpenConversationIDsForSLA(ctx, db.ListOpenConversationIDsForSLAParams{
			OrganizationID: orgID, AfterID: after, MaxRows: resyncBatch,
		})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.Sync(ctx, orgID, id, s.now()); err != nil {
				return err
			}
		}
		if len(ids) < resyncBatch {
			return nil
		}
		after = ids[len(ids)-1]
	}
}

// Sync reconciles the SLA clocks of one conversation with its current state.
// at is when the change that triggered it happened (the event time).
func (s *Service) Sync(ctx context.Context, orgID, conversationID int64, at time.Time) error {
	return s.inTx(ctx, func(ctx context.Context, tx pgx.Tx, q *db.Queries) error {
		conv, err := q.LockConversation(ctx, db.LockConversationParams{OrganizationID: orgID, ID: conversationID})
		if err != nil {
			if isNoRows(err) {
				return nil // deleted meanwhile
			}
			return err
		}
		rows, err := q.ListConversationSLAMetrics(ctx, db.ListConversationSLAMetricsParams{OrganizationID: orgID, ConversationID: conv.ID})
		if err != nil {
			return err
		}
		stored := map[string]engine.Clock{}
		for _, r := range rows {
			stored[r.Metric] = toClock(r)
		}
		pol, err := s.match(ctx, q, conv)
		if err != nil {
			return err
		}
		res := engine.Plan(toEngineConversation(conv), stored, pol, at.UTC().Truncate(time.Microsecond))
		res, err = engineeringHandoff(ctx, q, conv, stored, res, at)
		if err != nil {
			return err
		}
		for _, m := range res.Delete {
			if err := q.DeleteConversationSLAMetric(ctx, db.DeleteConversationSLAMetricParams{OrganizationID: orgID, ConversationID: conv.ID, Metric: m}); err != nil {
				return err
			}
		}
		for _, c := range res.Upsert {
			if err := q.UpsertConversationSLAMetric(ctx, upsertParams(orgID, conv.ID, c)); err != nil {
				return err
			}
		}
		if err := s.mirror(ctx, q, conv, pol, res.Clocks); err != nil {
			return err
		}
		for _, tr := range res.Transitions {
			name := map[string]string{engine.Applied: events.SLAApplied, engine.Met: events.SLAMet, engine.Breached: events.SLABreached}[tr.Kind]
			if tr.Kind == engine.Breached {
				if err := q.MarkConversationSLABreached(ctx, db.MarkConversationSLABreachedParams{At: ts(tr.At), OrganizationID: orgID, ID: conv.ID}); err != nil {
					return err
				}
			}
			if err := s.emit(ctx, tx, name, conv, tr.Clock); err != nil {
				return err
			}
		}
		return nil
	})
}

// mirror writes the policy and the open deadlines onto the conversation (the
// inbox list sorts, filters and badges on these columns).
func (s *Service) mirror(ctx context.Context, q *db.Queries, conv db.Conversation, pol *engine.Policy, clocks map[string]engine.Clock) error {
	due := func(m string) pgtype.Timestamptz {
		c, ok := clocks[m]
		if !ok || c.CompletedAt != nil || c.DueAt == nil {
			return pgtype.Timestamptz{}
		}
		return ts(*c.DueAt)
	}
	p := db.SetConversationSLAParams{
		OrganizationID: conv.OrganizationID, ID: conv.ID, SlaBreachedAt: conv.SlaBreachedAt,
		FirstResponseDueAt: due(engine.FirstResponse), NextResponseDueAt: due(engine.NextResponse),
		ResolutionDueAt: due(engine.Resolution),
	}
	if pol != nil {
		p.SlaPolicyID = pgtype.Int8{Int64: pol.ID, Valid: true}
	}
	for _, c := range clocks {
		if c.BreachedAt != nil && (!p.SlaBreachedAt.Valid || c.BreachedAt.Before(p.SlaBreachedAt.Time)) {
			p.SlaBreachedAt = ts(*c.BreachedAt)
		}
	}
	if p.SlaPolicyID == conv.SlaPolicyID && sameTS(p.FirstResponseDueAt, conv.FirstResponseDueAt) &&
		sameTS(p.NextResponseDueAt, conv.NextResponseDueAt) && sameTS(p.ResolutionDueAt, conv.ResolutionDueAt) &&
		sameTS(p.SlaBreachedAt, conv.SlaBreachedAt) {
		return nil
	}
	return q.SetConversationSLA(ctx, p)
}

// match picks the policy of a conversation: the company's contracted policy
// when it is active and has targets for the priority, else the first active
// policy (by position) whose conditions fit.
func (s *Service) match(ctx context.Context, q *db.Queries, conv db.Conversation) (*engine.Policy, error) {
	policies, err := q.ListActiveSLAPolicies(ctx, conv.OrganizationID)
	if err != nil || len(policies) == 0 {
		return nil, err
	}
	if conv.BrandID.Valid {
		b, err := q.GetBrand(ctx, db.GetBrandParams{OrganizationID: conv.OrganizationID, Uuid: uuid.UUID(conv.BrandID.Bytes)})
		if err != nil {
			return nil, err
		}
		if b.SlaPolicyID.Valid {
			for _, p := range policies {
				if p.Uuid == uuid.UUID(b.SlaPolicyID.Bytes) {
					if out, err := s.policyFor(ctx, q, conv, p); err != nil || out != nil {
						return out, err
					}
				}
			}
		}
	}
	var company *db.GetCompanySLARow
	if conv.CompanyID.Valid {
		row, err := q.GetCompanySLA(ctx, db.GetCompanySLAParams{OrganizationID: conv.OrganizationID, ID: conv.CompanyID.Int64})
		if err != nil && !isNoRows(err) {
			return nil, err
		}
		if err == nil {
			company = &row
		}
	}
	if company != nil && company.SlaPolicyID.Valid {
		i := slices.IndexFunc(policies, func(p db.SlaPolicy) bool { return p.ID == company.SlaPolicyID.Int64 })
		if i >= 0 {
			if out, err := s.policyFor(ctx, q, conv, policies[i]); err != nil || out != nil {
				return out, err
			}
		}
	}
	var tags []uuid.UUID
	tagsLoaded := false
	for _, p := range policies {
		var c model.Conditions
		if err := json.Unmarshal(p.Conditions, &c); err != nil {
			s.log.Warn("sla_policy_conditions_invalid", "policy", p.ID, "error", err)
			continue
		}
		if len(c.Priorities) > 0 && !slices.Contains(c.Priorities, conv.Priority) {
			continue
		}
		if len(c.ChannelTypes) > 0 && !slices.Contains(c.ChannelTypes, conv.ChannelType) {
			continue
		}
		if len(c.TagUUIDs) > 0 {
			if !tagsLoaded {
				if tags, err = q.ListConversationTagUUIDs(ctx, db.ListConversationTagUUIDsParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID}); err != nil {
					return nil, err
				}
				tagsLoaded = true
			}
			if !slices.ContainsFunc(c.TagUUIDs, func(u uuid.UUID) bool { return slices.Contains(tags, u) }) {
				continue
			}
		}
		if len(c.CompanyUUIDs) > 0 && (company == nil || !slices.Contains(c.CompanyUUIDs, company.Uuid)) {
			continue
		}
		if len(c.CompanyTiers) > 0 && (company == nil || !slices.ContainsFunc(c.CompanyTiers, func(t string) bool {
			return strings.EqualFold(t, company.Tier)
		})) {
			continue
		}
		if len(c.CompanyPlans) > 0 && (company == nil || !slices.ContainsFunc(c.CompanyPlans, func(p string) bool {
			return strings.EqualFold(p, company.RevenuePlan)
		})) {
			continue
		}
		if c.MinCompanyMRRMinor != nil && (company == nil || company.MrrMinor < *c.MinCompanyMRRMinor) {
			continue
		}
		out, err := s.policyFor(ctx, q, conv, p)
		if err != nil || out != nil {
			return out, err
		}
	}
	return nil, nil
}

// policyFor builds the engine policy of p for the conversation's priority;
// nil when p has no targets for it.
func (s *Service) policyFor(ctx context.Context, q *db.Queries, conv db.Conversation, p db.SlaPolicy) (*engine.Policy, error) {
	targets, err := q.ListSLAPolicyTargetsFor(ctx, db.ListSLAPolicyTargetsForParams{OrganizationID: conv.OrganizationID, PolicyID: p.ID})
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(targets, func(t db.SlaPolicyTarget) bool { return t.Priority == conv.Priority })
	if i < 0 {
		return nil, nil
	}
	t := targets[i]
	out := &engine.Policy{ID: p.ID, WarningPercent: int(p.WarningPercent), Targets: engine.Targets{
		FirstResponse: minutes(t.FirstResponseMinutes), NextResponse: minutes(t.NextResponseMinutes),
		Resolution: minutes(t.ResolutionMinutes),
	}}
	if out.Calendar, err = s.calendarFor(ctx, q, conv, p.BusinessHoursID); err != nil {
		return nil, err
	}
	return out, nil
}

// calendarFor resolves the schedule: the policy's, else the conversation
// team's, else the organization default, else none (24/7).
func (s *Service) calendarFor(ctx context.Context, q *db.Queries, conv db.Conversation, policyHours pgtype.Int8) (*businesshours.Calendar, error) {
	var (
		bh  db.BusinessHour
		err error
	)
	switch {
	case policyHours.Valid:
		bh, err = q.GetBusinessHoursByID(ctx, db.GetBusinessHoursByIDParams{OrganizationID: conv.OrganizationID, ID: policyHours.Int64})
	case conv.AssigneeTeamID.Valid:
		bh, err = q.GetTeamBusinessHours(ctx, db.GetTeamBusinessHoursParams{OrganizationID: conv.OrganizationID, TeamID: conv.AssigneeTeamID})
		if isNoRows(err) {
			bh, err = q.GetDefaultBusinessHours(ctx, conv.OrganizationID)
			if isNoRows(err) {
				bh, err = db.BusinessHour{OrganizationID: conv.OrganizationID, Timezone: "Europe/Istanbul", UsePublicHolidays: true, HolidayCountry: "TR"}, nil
			}
			if err == nil {
				var own bool
				if bh, own, err = s.teamHours(ctx, q, conv, bh); err == nil && !own && bh.ID == 0 {
					return nil, nil // no schedule anywhere: 24/7
				}
			}
		}
	default:
		bh, err = q.GetDefaultBusinessHours(ctx, conv.OrganizationID)
	}
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.compile(ctx, q, bh)
}

// teamHours applies a team's own working hours (CF-5 teams.working_hours,
// {"days": [{"day": "mon", "open": true, "start": "09:00", "end": "18:00"}]})
// onto base, the organization's schedule, whose holidays still apply. own
// reports whether the team had hours.
func (s *Service) teamHours(ctx context.Context, q *db.Queries, conv db.Conversation, base db.BusinessHour) (db.BusinessHour, bool, error) {
	row, err := q.GetTeamWorkingHours(ctx, db.GetTeamWorkingHoursParams{OrganizationID: conv.OrganizationID, ID: conv.AssigneeTeamID.Int64})
	if err != nil {
		if isNoRows(err) {
			return base, false, nil
		}
		return base, false, err
	}
	if len(row.WorkingHours) == 0 {
		return base, false, nil
	}
	var team struct {
		Days []struct {
			Day   string `json:"day"`
			Open  bool   `json:"open"`
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"days"`
	}
	if err := json.Unmarshal(row.WorkingHours, &team); err != nil || len(team.Days) == 0 {
		return base, false, nil
	}
	week := businesshours.Week{}
	for _, d := range team.Days {
		week[d.Day] = businesshours.Day{Closed: !d.Open, Open: d.Start, Close: d.End}
	}
	raw, err := json.Marshal(week)
	if err != nil {
		return base, false, err
	}
	base.WorkingHours = raw
	if row.Timezone.Valid && row.Timezone.String != "" {
		base.Timezone = row.Timezone.String
	}
	return base, true, nil
}

// calendarSpan is how far around now holidays are loaded.
const (
	calendarPast   = 90 * 24 * time.Hour
	calendarFuture = 400 * 24 * time.Hour
)

func (s *Service) compile(ctx context.Context, q *db.Queries, bh db.BusinessHour) (*businesshours.Calendar, error) {
	week := businesshours.Week{}
	if err := json.Unmarshal(bh.WorkingHours, &week); err != nil {
		return nil, fmt.Errorf("sla: business hours %d: %w", bh.ID, err)
	}
	now := s.now()
	from, to := pgDate(now.Add(-calendarPast)), pgDate(now.Add(calendarFuture))
	var ex []businesshours.Exception
	if bh.UsePublicHolidays {
		rows, err := q.ListPublicHolidays(ctx, db.ListPublicHolidaysParams{CountryCode: bh.HolidayCountry, FromDate: from, ToDate: to})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			e := businesshours.Exception{Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, Closed: !r.HalfDay}
			if r.HalfDay {
				e.Open, e.Close = "00:00", businesshours.HalfDayClose
			}
			ex = append(ex, e)
		}
	}
	rows, err := q.ListBusinessHolidays(ctx, db.ListBusinessHolidaysParams{
		OrganizationID: bh.OrganizationID, BusinessHoursID: pgtype.Int8{Int64: bh.ID, Valid: true}, FromDate: from, ToDate: to,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		e := businesshours.Exception{Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, Closed: r.Closed}
		if !r.Closed {
			e.Open, e.Close = clockString(r.OpenTime), clockString(r.CloseTime)
		}
		ex = append(ex, e)
	}
	cal, err := businesshours.New(bh.Timezone, week, ex)
	if err != nil {
		// A schedule that no longer validates must not stop the inbox: 24/7.
		s.log.Warn("sla_business_hours_invalid", "business_hours", bh.ID, "error", err)
		return nil, nil
	}
	return cal, nil
}

// Sweep raises warnings and breaches of running clocks whose time has come.
// It returns how many clocks it moved.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	total := 0
	for range sweepBatches {
		n := 0
		err := s.inTx(ctx, func(ctx context.Context, tx pgx.Tx, q *db.Queries) error {
			now := s.now()
			rows, err := q.ClaimDueSLAMetrics(ctx, db.ClaimDueSLAMetricsParams{Now: ts(now), MaxRows: sweepBatch})
			if err != nil {
				return err
			}
			n = len(rows)
			convs := map[int64]db.Conversation{}
			for _, r := range rows {
				conv, ok := convs[r.ConversationID]
				if !ok {
					if conv, err = q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: r.OrganizationID, ID: r.ConversationID}); err != nil {
						if isNoRows(err) {
							continue
						}
						return err
					}
					convs[r.ConversationID] = conv
				}
				clock := toClock(r)
				if r.DueAt.Valid && !r.DueAt.Time.After(now) {
					at := r.DueAt.Time.UTC()
					clock.BreachedAt = &at
					if err := q.MarkSLAMetricBreached(ctx, db.MarkSLAMetricBreachedParams{OrganizationID: r.OrganizationID, ConversationID: r.ConversationID, Metric: r.Metric, BreachedAt: ts(at)}); err != nil {
						return err
					}
					if err := q.MarkConversationSLABreached(ctx, db.MarkConversationSLABreachedParams{At: ts(at), OrganizationID: r.OrganizationID, ID: r.ConversationID}); err != nil {
						return err
					}
					if err := s.emit(ctx, tx, events.SLABreached, conv, clock); err != nil {
						return err
					}
					continue
				}
				clock.WarnedAt = &now
				if err := q.MarkSLAMetricWarned(ctx, db.MarkSLAMetricWarnedParams{OrganizationID: r.OrganizationID, ConversationID: r.ConversationID, Metric: r.Metric, WarnedAt: ts(now)}); err != nil {
					return err
				}
				if err := s.emit(ctx, tx, events.SLAWarning, conv, clock); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < sweepBatch {
			break
		}
	}
	return total, nil
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, conv db.Conversation, c engine.Clock) error {
	if s.outbox == nil {
		return nil
	}
	p := events.SLAPayload{TargetType: "conversation", TargetID: conv.ID, Metric: c.Metric, BreachedAt: c.BreachedAt}
	if c.PolicyID != nil {
		p.PolicyID = *c.PolicyID
	}
	switch {
	case c.DueAt != nil:
		p.DueAt = *c.DueAt
	case c.CompletedAt != nil:
		p.DueAt = *c.CompletedAt
	default:
		p.DueAt = c.StartedAt
	}
	if conv.AssigneeUserID.Valid {
		id := conv.AssigneeUserID.Int64
		p.AssigneeUserID = &id
	}
	ev, err := events.NewDomain(name, conv.OrganizationID, p)
	if err != nil {
		return err
	}
	ev = ev.WithEntity("conversation", &conv.ID, &conv.Uuid)
	if err := s.outbox.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("sla: outbox %s: %w", name, err)
	}
	return nil
}

// ConversationSLA returns the clocks of one conversation.
func (s *Service) ConversationSLA(ctx context.Context, orgID int64, id uuid.UUID) (model.ConversationSLA, error) {
	conv, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.ConversationSLA{}, notFound(err)
	}
	out := model.ConversationSLA{ConversationUUID: conv.Uuid, Metrics: []model.Metric{}}
	var cal *businesshours.Calendar
	if conv.SlaPolicyID.Valid {
		p, err := s.q.GetSLAPolicyByID(ctx, db.GetSLAPolicyByIDParams{OrganizationID: orgID, ID: conv.SlaPolicyID.Int64})
		if err == nil {
			out.Policy = &model.PolicyRef{UUID: p.Uuid, Name: p.Name}
			if cal, err = s.calendarFor(ctx, s.q, conv, p.BusinessHoursID); err != nil {
				return out, err
			}
		} else if !isNoRows(err) {
			return out, err
		}
	}
	rows, err := s.q.ListConversationSLAMetrics(ctx, db.ListConversationSLAMetricsParams{OrganizationID: orgID, ConversationID: conv.ID})
	if err != nil {
		return out, err
	}
	now := s.now()
	for _, m := range engine.Metrics {
		i := slices.IndexFunc(rows, func(r db.ConversationSlaMetric) bool { return r.Metric == m })
		if i < 0 {
			continue
		}
		c := toClock(rows[i])
		out.Metrics = append(out.Metrics, model.Metric{
			Metric: m, Status: c.Status(), Cycle: safeconv.ToInt32(c.Cycle), TargetSeconds: int64(c.Target / time.Second),
			StartedAt: c.StartedAt, DueAt: c.DueAt, WarnAt: c.WarnAt, PausedAt: c.PausedAt,
			RemainingSeconds: int64(c.Remaining(cal, now) / time.Second), BreachedAt: c.BreachedAt, CompletedAt: c.CompletedAt,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------

func toEngineConversation(c db.Conversation) engine.Conversation {
	return engine.Conversation{
		Status: c.Status, Source: c.Source, CreatedAt: c.CreatedAt.Time.UTC(),
		FirstResponseAt: tptr(c.FirstResponseAt), WaitingSince: tptr(c.WaitingSince),
		LastOutboundAt: tptr(c.LastOutboundAt), ResolvedAt: tptr(c.ResolvedAt), ClosedAt: tptr(c.ClosedAt),
	}
}

func toClock(r db.ConversationSlaMetric) engine.Clock {
	return engine.Clock{
		Metric: r.Metric, PolicyID: int8ptr(r.PolicyID), Target: time.Duration(r.TargetSeconds) * time.Second,
		Cycle: int(r.Cycle), StartedAt: r.StartedAt.Time.UTC(), ResumedAt: r.ResumedAt.Time.UTC(),
		Consumed: time.Duration(r.ConsumedSeconds) * time.Second, PausedAt: tptr(r.PausedAt),
		WarnAt: tptr(r.WarnAt), DueAt: tptr(r.DueAt), WarnedAt: tptr(r.WarnedAt), BreachedAt: tptr(r.BreachedAt),
		CompletedAt: tptr(r.CompletedAt),
	}
}

func upsertParams(orgID, convID int64, c engine.Clock) db.UpsertConversationSLAMetricParams {
	return db.UpsertConversationSLAMetricParams{
		OrganizationID: orgID, ConversationID: convID, Metric: c.Metric, PolicyID: int8p(c.PolicyID),
		TargetSeconds: int64(c.Target / time.Second), Cycle: safeconv.ToInt32(c.Cycle), StartedAt: ts(c.StartedAt),
		ResumedAt: ts(c.ResumedAt), ConsumedSeconds: int64(c.Consumed / time.Second), PausedAt: tsp(c.PausedAt),
		WarnAt: tsp(c.WarnAt), DueAt: tsp(c.DueAt), WarnedAt: tsp(c.WarnedAt), BreachedAt: tsp(c.BreachedAt),
		CompletedAt: tsp(c.CompletedAt),
	}
}

func minutes(v pgtype.Int4) time.Duration {
	if !v.Valid {
		return 0
	}
	return time.Duration(v.Int32) * time.Minute
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func tptr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func sameTS(a, b pgtype.Timestamptz) bool {
	return a.Valid == b.Valid && (!a.Valid || a.Time.Equal(b.Time))
}

// SetCompanyPolicy sets a company's contracted SLA and recomputes its open
// conversations.
func (s *Service) SetCompanyPolicy(ctx context.Context, orgID int64, companyUUID uuid.UUID, policyUUID *uuid.UUID) (model.CompanyPolicy, error) {
	var policyID pgtype.Int8
	if policyUUID != nil {
		p, err := s.q.GetSLAPolicyByUUID(ctx, db.GetSLAPolicyByUUIDParams{OrganizationID: orgID, Uuid: *policyUUID})
		if err != nil {
			if isNoRows(err) {
				return model.CompanyPolicy{}, invalidf("unknown SLA policy")
			}
			return model.CompanyPolicy{}, err
		}
		policyID = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	companyID, err := s.q.SetCompanySLAPolicy(ctx, db.SetCompanySLAPolicyParams{OrganizationID: orgID, Uuid: companyUUID, SlaPolicyID: policyID})
	if err != nil {
		return model.CompanyPolicy{}, notFound(err)
	}
	ids, err := s.q.ListOpenConversationIDsForCompany(ctx, db.ListOpenConversationIDsForCompanyParams{
		OrganizationID: orgID, CompanyID: pgtype.Int8{Int64: companyID, Valid: true},
	})
	if err != nil {
		return model.CompanyPolicy{}, err
	}
	for _, id := range ids {
		if err := s.Sync(ctx, orgID, id, s.now()); err != nil {
			return model.CompanyPolicy{}, err
		}
	}
	return model.CompanyPolicy{CompanyUUID: companyUUID, PolicyUUID: policyUUID}, nil
}

// ContractedCompanies lists companies with a contracted SLA.
func (s *Service) ContractedCompanies(ctx context.Context, orgID int64) ([]model.ContractedCompany, error) {
	rows, err := s.q.ListContractedCompanies(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ContractedCompany, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.ContractedCompany{
			CompanyUUID: r.CompanyUuid, Name: r.Name, Tier: r.Tier,
			Policy: model.PolicyRef{UUID: r.PolicyUuid, Name: r.PolicyName},
		})
	}
	return out, nil
}

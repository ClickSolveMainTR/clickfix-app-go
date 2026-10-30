// Package coaching extends CF-108 with blind calibration, development tasks and KPI badges.
package coaching

import (
	"context"
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/workforce"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"math"
	"sort"
	"strings"
	"time"
)

type Service struct{ Repo *repository.Repository }
type SessionInput struct {
	Name         string    `json:"name"`
	Conversation uuid.UUID `json:"conversation_uuid"`
	Scorecard    uuid.UUID `json:"scorecard_uuid"`
}

func (s *Service) Session(ctx context.Context, org, actor int64, in SessionInput) (any, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
		return nil, supportops.ErrInvalid
	}
	c, err := supportops.Conversation(ctx, s.Repo.Q, org, actor, in.Conversation)
	if err != nil {
		return nil, err
	}
	if c.Status != "resolved" {
		return nil, supportops.ErrConflict
	}
	card, err := s.Repo.Q.QAGetScorecard(ctx, db.QAGetScorecardParams{OrganizationID: org, Uuid: in.Scorecard})
	if err != nil {
		return nil, err
	}
	row, err := s.Repo.Q.CalibrationCreate(ctx, db.CalibrationCreateParams{OrganizationID: org, ConversationID: c.ID, ScorecardID: card.ID, Rubric: card.Criteria, Name: in.Name, CreatedBy: actor})
	return map[string]any{"uuid": row.Uuid, "name": row.Name, "conversation_uuid": in.Conversation, "rubric": json.RawMessage(row.Rubric), "created_at": row.CreatedAt}, err
}
func (s *Service) visibleSession(ctx context.Context, org, actor int64, id uuid.UUID) (db.CalibrationGetRow, error) {
	row, err := s.Repo.Q.CalibrationGet(ctx, db.CalibrationGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return row, err
	}
	_, err = supportops.Conversation(ctx, s.Repo.Q, org, actor, row.ConversationUuid)
	return row, err
}
func (s *Service) Score(ctx context.Context, org, actor int64, id uuid.UUID, scores []quality.Score) (any, error) {
	if _, err := s.visibleSession(ctx, org, actor, id); err != nil {
		return nil, err
	}
	var out db.CalibrationScoreRow
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		session, err := r.Q.CalibrationLock(ctx, db.CalibrationLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if session.ClosedAt.Valid {
			return supportops.ErrConflict
		}
		var criteria []quality.Criterion
		if err = json.Unmarshal(session.Rubric, &criteria); err != nil {
			return err
		}
		score, critical, err := quality.Calculate(criteria, scores)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(scores)
		if err != nil {
			return err
		}
		out, err = r.Q.CalibrationScore(ctx, db.CalibrationScoreParams{OrganizationID: org, CalibrationID: session.ID, ReviewerID: actor, Scores: raw, Score: score, CriticalFailure: critical})
		return err
	})
	return out, err
}
func (s *Service) Close(ctx context.Context, org, actor int64, id uuid.UUID) error {
	if _, err := s.visibleSession(ctx, org, actor, id); err != nil {
		return err
	}
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		session, err := r.Q.CalibrationLock(ctx, db.CalibrationLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if session.ClosedAt.Valid {
			return nil
		}
		scores, err := r.Q.CalibrationScores(ctx, db.CalibrationScoresParams{OrganizationID: org, CalibrationID: session.ID})
		if err != nil {
			return err
		}
		if len(scores) < 2 {
			return supportops.ErrConflict
		}
		return r.Q.CalibrationClose(ctx, db.CalibrationCloseParams{OrganizationID: org, ID: session.ID})
	})
}

type Variance struct {
	Count    int     `json:"count"`
	Mean     float64 `json:"mean"`
	Variance float64 `json:"variance"`
	Range    float64 `json:"range"`
}

func Summarize(values []float64) Variance {
	out := Variance{Count: len(values)}
	if len(values) == 0 {
		return out
	}
	low, high := values[0], values[0]
	m2 := 0.0
	for i, v := range values {
		delta := v - out.Mean
		out.Mean += delta / float64(i+1)
		m2 += delta * (v - out.Mean)
		low = math.Min(low, v)
		high = math.Max(high, v)
	}
	out.Variance = m2 / float64(len(values))
	out.Range = high - low
	return out
}
func (s *Service) Report(ctx context.Context, org, actor int64, id uuid.UUID) (any, error) {
	row, err := s.visibleSession(ctx, org, actor, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"uuid": row.Uuid, "name": row.Name, "conversation_uuid": row.ConversationUuid, "rubric": json.RawMessage(row.Rubric), "closed_at": row.ClosedAt, "created_at": row.CreatedAt}
	if !row.ClosedAt.Valid {
		return out, nil
	}
	scores, err := s.Repo.Q.CalibrationScores(ctx, db.CalibrationScoresParams{OrganizationID: org, CalibrationID: row.ID})
	if err != nil {
		return nil, err
	}
	totals := []float64{}
	byKey := map[string][]float64{}
	reviews := []map[string]any{}
	for _, r := range scores {
		totals = append(totals, r.Score)
		var values []quality.Score
		if err = json.Unmarshal(r.Scores, &values); err != nil {
			return nil, err
		}
		for _, v := range values {
			byKey[v.Key] = append(byKey[v.Key], float64(v.Value))
		}
		reviews = append(reviews, map[string]any{"reviewer_uuid": r.ReviewerUuid, "scores": values, "score": r.Score, "critical_failure": r.CriticalFailure})
	}
	criteria := map[string]Variance{}
	for k, vs := range byKey {
		criteria[k] = Summarize(vs)
	}
	out["summary"] = Summarize(totals)
	out["criteria"] = criteria
	out["reviews"] = reviews
	return out, nil
}

type ItemInput struct {
	Agent  uuid.UUID  `json:"agent_uuid"`
	Kind   string     `json:"kind"`
	Body   string     `json:"body"`
	DueAt  *time.Time `json:"due_at"`
	Parent *uuid.UUID `json:"parent_uuid"`
}

func (s *Service) Item(ctx context.Context, org, actor int64, in ItemInput) (any, error) {
	if strings.TrimSpace(in.Body) == "" || len(in.Body) > 5000 || (in.Kind != "note" && in.Kind != "goal" && in.Kind != "task") || (in.Kind == "note" && (in.DueAt != nil || in.Parent != nil)) || (in.Kind == "task" && (in.DueAt == nil || in.Parent == nil)) {
		return nil, supportops.ErrInvalid
	}
	var out db.CoachingCreateRow
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		agent, err := r.Q.ScheduleMember(ctx, db.ScheduleMemberParams{OrganizationID: org, Uuid: in.Agent})
		if err != nil {
			return err
		}
		var parent pgtype.Int8
		if in.Parent != nil {
			id, err := r.Q.CoachingParent(ctx, db.CoachingParentParams{OrganizationID: org, Uuid: *in.Parent, AgentID: agent})
			if err != nil {
				return err
			}
			parent = pgtype.Int8{Int64: id, Valid: true}
		}
		var due pgtype.Timestamptz
		if in.DueAt != nil {
			due = repository.TS(*in.DueAt)
		}
		out, err = r.Q.CoachingCreate(ctx, db.CoachingCreateParams{OrganizationID: org, AgentID: agent, CoachID: actor, Kind: in.Kind, Body: in.Body, DueAt: due, ParentID: parent})
		return err
	})
	return out, err
}

type RuleInput struct {
	Name       string  `json:"name"`
	Metric     string  `json:"metric"`
	Threshold  float64 `json:"threshold"`
	Points     int32   `json:"points"`
	MinSamples int32   `json:"min_samples"`
	Enabled    bool    `json:"enabled"`
}

func (s *Service) Rule(ctx context.Context, org int64, in RuleInput) (any, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || (in.Metric != "csat" && in.Metric != "frt" && in.Metric != "resolutions" && in.Metric != "qa") || math.IsNaN(in.Threshold) || math.IsInf(in.Threshold, 0) || in.Threshold < 0 || in.Threshold > 1000000 || in.Points < 0 || in.Points > 10000 || in.MinSamples < 1 || in.MinSamples > 100000 {
		return nil, supportops.ErrInvalid
	}
	if (in.Metric == "qa" && in.Threshold > 100) || (in.Metric == "csat" && in.Threshold > 5) {
		return nil, supportops.ErrInvalid
	}
	var out db.GameRuleSaveRow
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		// Serialize rule creation so the bounded ruleset cannot be exceeded concurrently.
		if err := r.Q.GameLock(ctx, org); err != nil {
			return err
		}
		rules, err := r.Q.GameRules(ctx, org)
		if err != nil {
			return err
		}
		found := false
		for _, v := range rules {
			found = found || v.Name == in.Name
		}
		if !found && len(rules) >= 100 {
			return supportops.ErrConflict
		}
		out, err = r.Q.GameRuleSave(ctx, db.GameRuleSaveParams{OrganizationID: org, Name: in.Name, Metric: in.Metric, Threshold: in.Threshold, Points: in.Points, MinSamples: in.MinSamples, Enabled: in.Enabled})
		return err
	})
	return out, err
}

type Badge struct {
	UUID   uuid.UUID `json:"uuid"`
	Name   string    `json:"name"`
	Points int32     `json:"points"`
}
type Standing struct {
	Agent  uuid.UUID `json:"agent_uuid"`
	Points int64     `json:"points"`
	Badges []Badge   `json:"badges"`
	Rank   int       `json:"rank"`
}

func Award(m db.GameMetricsRow, rules []db.GameRulesRow) Standing {
	out := Standing{Agent: m.AgentUuid, Badges: []Badge{}}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		var value float64
		var samples int64
		switch r.Metric {
		case "csat":
			value = m.Csat
			samples = m.CsatSamples
		case "frt":
			value = m.Frt
			samples = m.FrtSamples
		case "resolutions":
			value = float64(m.Resolutions)
			samples = m.Resolutions
		case "qa":
			value = m.Qa
			samples = m.QaSamples
		default:
			continue
		}
		hit := value >= r.Threshold
		if r.Metric == "frt" {
			hit = value <= r.Threshold
		}
		if hit && samples >= int64(r.MinSamples) {
			out.Points += int64(r.Points)
			out.Badges = append(out.Badges, Badge{UUID: r.Uuid, Name: r.Name, Points: r.Points})
		}
	}
	return out
}
func (s *Service) Leaderboard(ctx context.Context, org int64, since, until time.Time, team *uuid.UUID) ([]Standing, error) {
	if !until.After(since) || until.Sub(since) > 366*24*time.Hour {
		return nil, supportops.ErrInvalid
	}
	if team != nil {
		if _, err := s.Repo.Q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: org, Uuid: *team}); err != nil {
			return nil, err
		}
	}
	metrics, err := s.Repo.Q.GameMetrics(ctx, db.GameMetricsParams{OrganizationID: org, Since: repository.TS(since), Until: repository.TS(until), Team: workforce.OptionalUUID(team)})
	if err != nil {
		return nil, err
	}
	if len(metrics) > 10000 {
		return nil, supportops.ErrConflict
	}
	rules, err := s.Repo.Q.GameRules(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]Standing, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, Award(m, rules))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Points == out[j].Points {
			return out[i].Agent.String() < out[j].Agent.String()
		}
		return out[i].Points > out[j].Points
	})
	for i := range out {
		out[i].Rank = i + 1
		if i > 0 && out[i].Points == out[i-1].Points {
			out[i].Rank = out[i-1].Rank
		}
	}
	return out, nil
}

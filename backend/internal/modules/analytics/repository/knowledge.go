package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// KnowledgeStore is the built-in, table-backed source of the AI knowledge
// panel (CF-220). Until departments land (CF-215) conversations, AI runs,
// triage and suggestions carry no department, so those sources answer
// for the whole organization only; evaluation runs already carry one.
type KnowledgeStore struct{ q *db.Queries }

// NewKnowledge returns the built-in panel store.
func NewKnowledge(q *db.Queries) *KnowledgeStore { return &KnowledgeStore{q} }

func window(s model.KnowledgeScope) (pgtype.Timestamptz, pgtype.Timestamptz) {
	return pgtype.Timestamptz{Time: s.From, Valid: true}, pgtype.Timestamptz{Time: s.Until, Valid: true}
}

func (k *KnowledgeStore) Usage(ctx context.Context, s model.KnowledgeScope) ([]model.TaskUsage, error) {
	if s.Department != nil {
		return nil, model.ErrDepartmentScope
	}
	since, until := window(s)
	rows, err := k.q.AIInsightUsage(ctx, db.AIInsightUsageParams{OrganizationID: s.Organization, Since: since, Until: until})
	out := make([]model.TaskUsage, len(rows))
	for i, r := range rows {
		out[i] = model.TaskUsage{Task: r.Task, Calls: r.Calls, Failed: r.Failed, CostMicros: r.CostMicros, AvgLatencyMs: r.AvgLatencyMs}
	}
	return out, err
}

func (k *KnowledgeStore) Gaps(ctx context.Context, s model.KnowledgeScope) ([]model.GapTopic, error) {
	if s.Department != nil {
		return nil, model.ErrDepartmentScope
	}
	since, until := window(s)
	rows, err := k.q.AIInsightGaps(ctx, db.AIInsightGapsParams{OrganizationID: s.Organization, Since: since, Until: until})
	out := make([]model.GapTopic, len(rows))
	for i, r := range rows {
		out[i] = model.GapTopic{Topic: r.Topic, Conversations: r.Conversations}
	}
	return out, err
}

func (k *KnowledgeStore) Suggestions(ctx context.Context, s model.KnowledgeScope) (map[string]int64, error) {
	if s.Department != nil {
		return nil, model.ErrDepartmentScope
	}
	since, until := window(s)
	rows, err := k.q.AIInsightSuggestions(ctx, db.AIInsightSuggestionsParams{OrganizationID: s.Organization, Since: since, Until: until})
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Status] = r.Total
	}
	return out, err
}

func (k *KnowledgeStore) Classifications(ctx context.Context, s model.KnowledgeScope) ([]model.Classification, error) {
	if s.Department != nil {
		return nil, model.ErrDepartmentScope
	}
	since, until := window(s)
	rows, err := k.q.AIInsightClassifications(ctx, db.AIInsightClassificationsParams{OrganizationID: s.Organization, Since: since, Until: until})
	out := make([]model.Classification, len(rows))
	for i, r := range rows {
		out[i] = model.Classification{Category: r.Category}
		if r.Confidence >= 0 {
			c := r.Confidence
			out[i].Confidence = &c
		}
	}
	return out, err
}

func (k *KnowledgeStore) ReplyOutcomes(ctx context.Context, s model.KnowledgeScope) (int64, int64, error) {
	if s.Department != nil {
		return 0, 0, model.ErrDepartmentScope
	}
	since, until := window(s)
	r, err := k.q.AIInsightReplyOutcomes(ctx, db.AIInsightReplyOutcomesParams{OrganizationID: s.Organization, Since: since, Until: until})
	return r.Conversations, r.Handoffs, err
}

func float(v pgtype.Float8) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

// Evaluations are the scope's last two completed runs, whatever the window:
// the latest measurement stays relevant until the next run.
func (k *KnowledgeStore) Evaluations(ctx context.Context, s model.KnowledgeScope) ([]model.EvaluationPoint, error) {
	dept := pgtype.Int8{}
	if s.Department != nil {
		dept = pgtype.Int8{Int64: s.Department.ID, Valid: true}
	}
	rows, err := k.q.AIInsightEvaluations(ctx, db.AIInsightEvaluationsParams{OrganizationID: s.Organization, DepartmentID: dept})
	out := make([]model.EvaluationPoint, len(rows))
	for i, r := range rows {
		out[i] = model.EvaluationPoint{UUID: r.Uuid, Trigger: r.TriggerKind, Model: r.Model, CreatedAt: r.CreatedAt.Time, AnswerAccuracy: float(r.AnswerAccuracy), SourceHitRate: float(r.SourceHitRate), CacheHitRate: float(r.CacheHitRate), AvgLatencyMs: float(r.AvgLatencyMs), CostPerCaseMicros: float(r.CostPerCaseMicros)}
	}
	return out, err
}

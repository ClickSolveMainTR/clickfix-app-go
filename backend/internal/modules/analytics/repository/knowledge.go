package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// KnowledgeStore is the built-in, table-backed source of the AI knowledge
// panel (CF-220). With a department (CF-225) AI runs, reply and triage
// jobs follow their conversation's current department, suggestions and
// evaluation runs carry their own; rows without one count only for the
// organization.
type KnowledgeStore struct{ q *db.Queries }

// NewKnowledge returns the built-in panel store.
func NewKnowledge(q *db.Queries) *KnowledgeStore { return &KnowledgeStore{q} }

func window(s model.KnowledgeScope) (pgtype.Timestamptz, pgtype.Timestamptz) {
	return pgtype.Timestamptz{Time: s.From, Valid: true}, pgtype.Timestamptz{Time: s.Until, Valid: true}
}

// department is the scope's department filter (NULL = organization).
func department(s model.KnowledgeScope) pgtype.Int8 {
	if s.Department == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: s.Department.ID, Valid: true}
}

func (k *KnowledgeStore) Usage(ctx context.Context, s model.KnowledgeScope) ([]model.TaskUsage, error) {
	since, until := window(s)
	rows, err := k.q.AIInsightUsage(ctx, db.AIInsightUsageParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
	out := make([]model.TaskUsage, len(rows))
	for i, r := range rows {
		out[i] = model.TaskUsage{Task: r.Task, Calls: r.Calls, Failed: r.Failed, CostMicros: r.CostMicros, AvgLatencyMs: r.AvgLatencyMs}
	}
	return out, err
}

func (k *KnowledgeStore) Gaps(ctx context.Context, s model.KnowledgeScope) ([]model.GapTopic, error) {
	since, until := window(s)
	rows, err := k.q.AIInsightGaps(ctx, db.AIInsightGapsParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
	out := make([]model.GapTopic, len(rows))
	for i, r := range rows {
		out[i] = model.GapTopic{Topic: r.Topic, Conversations: r.Conversations}
	}
	return out, err
}

func (k *KnowledgeStore) Suggestions(ctx context.Context, s model.KnowledgeScope) (map[string]int64, error) {
	since, until := window(s)
	rows, err := k.q.AIInsightSuggestions(ctx, db.AIInsightSuggestionsParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Status] = r.Total
	}
	return out, err
}

func (k *KnowledgeStore) Classifications(ctx context.Context, s model.KnowledgeScope) ([]model.Classification, error) {
	since, until := window(s)
	rows, err := k.q.AIInsightClassifications(ctx, db.AIInsightClassificationsParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
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
	since, until := window(s)
	r, err := k.q.AIInsightReplyOutcomes(ctx, db.AIInsightReplyOutcomesParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
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
	rows, err := k.q.AIInsightEvaluations(ctx, db.AIInsightEvaluationsParams{OrganizationID: s.Organization, DepartmentID: department(s)})
	out := make([]model.EvaluationPoint, len(rows))
	for i, r := range rows {
		out[i] = model.EvaluationPoint{UUID: r.Uuid, Trigger: r.TriggerKind, Model: r.Model, CreatedAt: r.CreatedAt.Time, AnswerAccuracy: float(r.AnswerAccuracy), SourceHitRate: float(r.SourceHitRate), CacheHitRate: float(r.CacheHitRate), AvgLatencyMs: float(r.AvgLatencyMs), CostPerCaseMicros: float(r.CostPerCaseMicros)}
	}
	return out, err
}

// DepartmentStore lists the organization's departments (CF-215) for the
// panel's department filter.
type DepartmentStore struct{ q *db.Queries }

// NewDepartments returns the panel's department source.
func NewDepartments(q *db.Queries) *DepartmentStore { return &DepartmentStore{q} }

// maxPanelDepartments bounds the filter list; an organization has a handful.
const maxPanelDepartments = 500

func (d *DepartmentStore) Departments(ctx context.Context, org int64) ([]model.Department, error) {
	rows, err := d.q.ListDepartments(ctx, db.ListDepartmentsParams{OrganizationID: org, Ids: []int64{}, LimitCount: maxPanelDepartments})
	if err != nil {
		return nil, err
	}
	out := make([]model.Department, len(rows))
	for i, r := range rows {
		out[i] = model.Department{ID: r.ID, UUID: r.Uuid, Name: r.Name}
	}
	return out, nil
}

// CacheStore reads the answer cache counters (CF-216, ai_answer_cache_stats).
type CacheStore struct{ q *db.Queries }

// NewCache returns the panel's answer cache source.
func NewCache(q *db.Queries) *CacheStore { return &CacheStore{q} }

func (c *CacheStore) CacheStats(ctx context.Context, s model.KnowledgeScope) (int64, int64, error) {
	r, err := c.q.AIInsightCacheStats(ctx, db.AIInsightCacheStatsParams{
		OrganizationID: s.Organization, Since: pgtype.Date{Time: s.From, Valid: true}, Until: pgtype.Date{Time: s.Until, Valid: true}, DepartmentID: department(s),
	})
	return r.Lookups, r.Hits, err
}

// CategoryStore reads conversation categories from the categorization
// layer (CF-218, category_assignments).
type CategoryStore struct{ q *db.Queries }

// NewCategories returns the panel's classification source.
func NewCategories(q *db.Queries) *CategoryStore { return &CategoryStore{q} }

func (c *CategoryStore) Classifications(ctx context.Context, s model.KnowledgeScope) ([]model.Classification, error) {
	since, until := window(s)
	rows, err := c.q.AIInsightCategoryAssignments(ctx, db.AIInsightCategoryAssignmentsParams{OrganizationID: s.Organization, Since: since, Until: until, DepartmentID: department(s)})
	out := make([]model.Classification, len(rows))
	for i, r := range rows {
		v := r.Confidence
		out[i] = model.Classification{Category: r.Category, Confidence: &v}
	}
	return out, err
}

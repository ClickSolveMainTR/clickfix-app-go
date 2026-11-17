package usecase

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/google/uuid"
)

// CF-220: the AI knowledge panel composes independent sources. Built-in
// sources read existing tables; departments (CF-215), the answer cache
// (CF-216) and categorization (CF-218) plug in through the interfaces
// below (wired in CF-225). A nil or organization-only source shows its
// section as "no data" with a reason.

// KnowledgeStore holds the built-in, table-backed sources. A source that
// cannot filter by department returns model.ErrDepartmentScope.
type KnowledgeStore interface {
	Usage(context.Context, model.KnowledgeScope) ([]model.TaskUsage, error)
	Gaps(context.Context, model.KnowledgeScope) ([]model.GapTopic, error)
	Suggestions(context.Context, model.KnowledgeScope) (map[string]int64, error)
	Classifications(context.Context, model.KnowledgeScope) ([]model.Classification, error)
	ReplyOutcomes(ctx context.Context, s model.KnowledgeScope) (conversations, handoffs int64, err error)
	Evaluations(context.Context, model.KnowledgeScope) ([]model.EvaluationPoint, error)
}

// DepartmentSource lists the organization's departments (CF-215).
type DepartmentSource interface {
	Departments(ctx context.Context, org int64) ([]model.Department, error)
}

// CacheSource reports answer cache lookups and hits (CF-216).
type CacheSource interface {
	CacheStats(context.Context, model.KnowledgeScope) (lookups, hits int64, err error)
}

// ClassificationSource replaces triage-based categories with the
// categorization layer (CF-218).
type ClassificationSource interface {
	Classifications(context.Context, model.KnowledgeScope) ([]model.Classification, error)
}

// LowConfidence is the confidence under which a classification counts as low.
const LowConfidence = 0.6

// KnowledgeService builds the AI knowledge panel.
type KnowledgeService struct {
	Store       KnowledgeStore
	Departments DepartmentSource
	Cache       CacheSource
	Categories  ClassificationSource
	Now         func() time.Time
}

// NewKnowledge returns a panel service over the built-in store.
func NewKnowledge(store KnowledgeStore) *KnowledgeService {
	return &KnowledgeService{Store: store, Now: time.Now}
}

// availability turns a source error into a section state; other errors
// are returned.
func availability(err error) (model.Availability, error) {
	if errors.Is(err, model.ErrDepartmentScope) {
		return model.Availability{Reason: model.ReasonDepartmentScopeUnavailable}, nil
	}
	if err != nil {
		return model.Availability{}, err
	}
	return model.Availability{Available: true}, nil
}

func rate(n, d int64) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

// Panel returns the panel of an organization, or of one department, over
// [from, until). Zero times default to the last 30 days.
func (s *KnowledgeService) Panel(ctx context.Context, org int64, department *uuid.UUID, from, until time.Time) (out model.KnowledgePanel, err error) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if until.IsZero() {
		until = now()
	}
	if from.IsZero() {
		from = until.AddDate(0, 0, -30)
	}
	if org <= 0 || !until.After(from) || until.Sub(from) > 366*24*time.Hour {
		return out, ErrInvalid
	}
	scope := model.KnowledgeScope{Organization: org, From: from, Until: until}
	out = model.KnowledgePanel{From: from, Until: until, Departments: []model.Department{}}
	if s.Departments != nil {
		if out.Departments, err = s.Departments.Departments(ctx, org); err != nil {
			return out, err
		}
	}
	if department != nil {
		for i := range out.Departments {
			if out.Departments[i].UUID == *department {
				d := out.Departments[i]
				scope.Department, out.Department = &d, &d
			}
		}
		if scope.Department == nil {
			if s.Departments == nil {
				return out, model.ErrDepartmentsUnavailable
			}
			return out, ErrInvalid
		}
	}
	if out.Cache, err = s.cache(ctx, scope); err != nil {
		return out, err
	}
	if out.Usage, err = s.usage(ctx, scope); err != nil {
		return out, err
	}
	out.KnowledgeGaps.Topics = []model.GapTopic{}
	topics, e := s.Store.Gaps(ctx, scope)
	if out.KnowledgeGaps.Availability, err = availability(e); err != nil {
		return out, err
	}
	if topics != nil && out.KnowledgeGaps.Available {
		out.KnowledgeGaps.Topics = topics
	}
	counts, e := s.Store.Suggestions(ctx, scope)
	if out.Suggestions.Availability, err = availability(e); err != nil {
		return out, err
	}
	if out.Suggestions.Available {
		out.Suggestions.Pending, out.Suggestions.Approved, out.Suggestions.Rejected = counts["pending"], counts["approved"], counts["rejected"]
	}
	if out.Categories, err = s.categories(ctx, scope); err != nil {
		return out, err
	}
	conversations, handoffs, e := s.Store.ReplyOutcomes(ctx, scope)
	if out.Handoff.Availability, err = availability(e); err != nil {
		return out, err
	}
	if out.Handoff.Available {
		out.Handoff.Conversations, out.Handoff.Handoffs, out.Handoff.Rate = conversations, handoffs, rate(handoffs, conversations)
	}
	points, e := s.Store.Evaluations(ctx, scope)
	if out.Evaluation.Availability, err = availability(e); err != nil {
		return out, err
	}
	if !out.Evaluation.Available {
		points = nil
	}
	if len(points) > 0 {
		out.Evaluation.Latest = &points[0]
	}
	if len(points) > 1 {
		out.Evaluation.Previous = &points[1]
	}
	return out, nil
}

func (s *KnowledgeService) cache(ctx context.Context, scope model.KnowledgeScope) (model.CacheStats, error) {
	if s.Cache == nil {
		return model.CacheStats{Availability: model.Availability{Reason: model.ReasonSourceNotConnected}}, nil
	}
	lookups, hits, e := s.Cache.CacheStats(ctx, scope)
	a, err := availability(e)
	if err != nil || !a.Available {
		return model.CacheStats{Availability: a}, err
	}
	return model.CacheStats{Availability: a, Lookups: lookups, Hits: hits, HitRate: rate(hits, lookups)}, nil
}

func (s *KnowledgeService) usage(ctx context.Context, scope model.KnowledgeScope) (model.UsageStats, error) {
	rows, e := s.Store.Usage(ctx, scope)
	a, err := availability(e)
	out := model.UsageStats{Availability: a, ByTask: []model.TaskUsage{}}
	if err != nil || !a.Available {
		return out, err
	}
	for _, r := range rows {
		out.Calls += r.Calls
		out.Failed += r.Failed
		out.CostMicros += r.CostMicros
		out.ByTask = append(out.ByTask, r)
	}
	return out, nil
}

func (s *KnowledgeService) categories(ctx context.Context, scope model.KnowledgeScope) (model.CategoryStats, error) {
	var rows []model.Classification
	var e error
	if s.Categories != nil {
		rows, e = s.Categories.Classifications(ctx, scope)
	}
	// Without the categorization layer, or before it classified anything in
	// the window (no category tree yet), the latest triage results stand in.
	if s.Categories == nil || (e == nil && len(rows) == 0) {
		rows, e = s.Store.Classifications(ctx, scope)
	}
	a, err := availability(e)
	out := model.CategoryStats{Availability: a, Categories: []model.CategoryShare{}, Buckets: []model.ConfidenceBucket{}}
	if err != nil || !a.Available {
		return out, err
	}
	return summarizeCategories(rows, a), nil
}

// summarizeCategories builds the distribution, mean confidence and five
// equal-width confidence buckets (the last one includes 1.0).
func summarizeCategories(rows []model.Classification, a model.Availability) model.CategoryStats {
	out := model.CategoryStats{Availability: a, Classified: int64(len(rows)), Categories: []model.CategoryShare{}, Buckets: make([]model.ConfidenceBucket, 5)}
	for i := range out.Buckets {
		out.Buckets[i] = model.ConfidenceBucket{From: float64(i) / 5, To: float64(i+1) / 5}
	}
	type agg struct {
		n, scored int64
		sum       float64
	}
	byCategory := map[string]*agg{}
	var scored int64
	var sum float64
	for _, r := range rows {
		g := byCategory[r.Category]
		if g == nil {
			g = &agg{}
			byCategory[r.Category] = g
		}
		g.n++
		if r.Confidence == nil {
			continue
		}
		c := min(max(*r.Confidence, 0), 1)
		g.scored++
		g.sum += c
		scored++
		sum += c
		if c < LowConfidence {
			out.LowConfidence++
		}
		out.Buckets[min(int(c*5), 4)].Count++
	}
	if scored > 0 {
		v := sum / float64(scored)
		out.AvgConfidence = &v
	}
	for name, g := range byCategory {
		share := model.CategoryShare{Category: name, Conversations: g.n}
		if g.scored > 0 {
			v := g.sum / float64(g.scored)
			share.AvgConfidence = &v
		}
		out.Categories = append(out.Categories, share)
	}
	sort.Slice(out.Categories, func(i, j int) bool {
		if out.Categories[i].Conversations == out.Categories[j].Conversations {
			return out.Categories[i].Category < out.Categories[j].Category
		}
		return out.Categories[i].Conversations > out.Categories[j].Conversations
	})
	return out
}

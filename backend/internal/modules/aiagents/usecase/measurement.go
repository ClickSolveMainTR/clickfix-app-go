package usecase

// CF-220: knowledge layer measurement. A department keeps a question set
// (ai_evaluation_cases with question, expected answer or sources and a
// category); every run records answer accuracy, source hit rate, cache hit
// rate, mean latency and cost per question, compares itself with the
// previous run of the same scope, and an opt-in watch reruns the set when
// the agent config, the reply model or the published knowledge changes.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DepartmentDirectory resolves the organization's departments (CF-215).
// nil: departments do not exist yet and every case and run is
// organization wide (department_id NULL).
type DepartmentDirectory interface {
	// DepartmentID returns the internal id of an organization's
	// department; pgx.ErrNoRows when it is not the organization's.
	DepartmentID(ctx context.Context, org int64, id uuid.UUID) (int64, error)
	// DepartmentUUIDs maps internal ids to public UUIDs.
	DepartmentUUIDs(ctx context.Context, org int64, ids []int64) (map[int64]uuid.UUID, error)
}

// AnswerCache is the answer cache (CF-216). Lookup reports whether the
// cache would answer the question without a model call. nil: no cache is
// connected and cache hit rate is not measured.
type AnswerCache interface {
	Lookup(ctx context.Context, org int64, department *int64, question, locale string) (bool, error)
}

// ErrDepartmentsUnavailable is returned when a department is requested
// before departments exist.
var ErrDepartmentsUnavailable = errors.New("aiagents: departments are not available yet")

// AnswerMatchThreshold is the share of the expected answer's content words
// the reply must contain to count as a correct answer.
const AnswerMatchThreshold = 0.6

// EvaluationMetrics are the before/after numbers of one run. A nil metric
// could not be measured (no expected sources, no cache, no completed case).
type EvaluationMetrics struct {
	Cases             int      `json:"cases"`
	Completed         int      `json:"completed"`
	AnswerAccuracy    *float64 `json:"answer_accuracy"`
	SourceHitRate     *float64 `json:"source_hit_rate"`
	CacheHitRate      *float64 `json:"cache_hit_rate"`
	AvgLatencyMs      *float64 `json:"avg_latency_ms"`
	CostPerCaseMicros *float64 `json:"cost_per_case_micros"`
}

// DepartmentMetrics is one department's share of a run. A nil
// DepartmentUUID is the organization-wide part of the set.
type DepartmentMetrics struct {
	DepartmentUUID *uuid.UUID `json:"department_uuid"`
	EvaluationMetrics
}

// metricAccumulator sums per-case outcomes into EvaluationMetrics.
type metricAccumulator struct {
	cases, completed, correct         int
	sourceCases, sourceHits           int
	cacheCases, cacheHits             int
	latencyMs, costMicros, costedRuns int64
}

func (a *metricAccumulator) add(o caseOutcome) {
	a.cases++
	a.costMicros += o.CostMicros
	a.costedRuns++
	if o.Failed {
		return
	}
	a.completed++
	a.latencyMs += o.LatencyMs
	if o.AnswerCorrect {
		a.correct++
	}
	if o.SourceHit != nil {
		a.sourceCases++
		if *o.SourceHit {
			a.sourceHits++
		}
	}
	if o.CacheHit != nil {
		a.cacheCases++
		if *o.CacheHit {
			a.cacheHits++
		}
	}
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

func (a *metricAccumulator) metrics() EvaluationMetrics {
	out := EvaluationMetrics{Cases: a.cases, Completed: a.completed, AnswerAccuracy: ratio(a.correct, a.completed), SourceHitRate: ratio(a.sourceHits, a.sourceCases), CacheHitRate: ratio(a.cacheHits, a.cacheCases)}
	if a.completed > 0 {
		v := float64(a.latencyMs) / float64(a.completed)
		out.AvgLatencyMs = &v
	}
	if a.costedRuns > 0 {
		v := float64(a.costMicros) / float64(a.costedRuns)
		out.CostPerCaseMicros = &v
	}
	return out
}

// caseOutcome is the measured result of one question.
type caseOutcome struct {
	Score         float64
	Failed        bool
	AnswerCorrect bool
	AnswerMatch   *float64
	SourceHit     *bool
	CacheHit      *bool
	LatencyMs     int64
	CostMicros    int64
}

// measureCase scores one simulated answer against the case. Everything is
// deterministic (no LLM judge): substring checks, handoff, confidence, the
// content-word recall of the expected answer and whether the reply cited
// one of the expected knowledge articles.
func measureCase(expected Expectations, expectedAnswer string, expectedArticles []uuid.UUID, out SimulationResult) caseOutcome {
	o := caseOutcome{LatencyMs: out.LatencyMs, CostMicros: out.CostMicros}
	parts, sum := 0, 0.0
	checks := expected.Handoff != nil || len(expected.Contains)+len(expected.Excludes) > 0 || expected.MinimumConfidence > 0
	correct := true
	if checks {
		s := scoreEvaluation(expected, out)
		parts++
		sum += s
		correct = correct && s == 1
	}
	if strings.TrimSpace(expectedAnswer) != "" {
		m := answerMatch(expectedAnswer, out.Reply.Text)
		o.AnswerMatch = &m
		parts++
		sum += m
		correct = correct && m >= AnswerMatchThreshold && !out.Reply.Handoff
	}
	if len(expectedArticles) > 0 {
		hit := false
		for _, cited := range out.CitedArticles {
			for _, want := range expectedArticles {
				hit = hit || cited == want
			}
		}
		o.SourceHit = &hit
		parts++
		if hit {
			sum++
		}
	}
	if !checks && strings.TrimSpace(expectedAnswer) == "" {
		// Source-only case: a correct answer is a grounded, non-handoff reply.
		correct = !out.Reply.Handoff && strings.TrimSpace(out.Reply.Text) != "" && o.SourceHit != nil && *o.SourceHit
	}
	if parts > 0 {
		o.Score = sum / float64(parts)
	}
	o.AnswerCorrect = correct
	return o
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "you": true, "your": true, "are": true, "with": true, "can": true, "this": true, "that": true, "from": true, "will": true, "our": true, "has": true, "have": true,
	"bir": true, "ve": true, "ile": true, "için": true, "bu": true, "şu": true, "da": true, "de": true, "mi": true, "mı": true, "çok": true, "daha": true, "olarak": true, "olan": true, "gibi": true, "ama": true, "veya": true, "siz": true, "biz": true,
}

func contentWords(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 2 && !stopWords[w] {
			out[w] = true
		}
	}
	return out
}

// answerMatch is the share of the expected answer's content words found in
// the reply (0..1). Digits count, so "14 gün" matches "14 gün içinde".
func answerMatch(expected, reply string) float64 {
	want := contentWords(expected)
	if len(want) == 0 {
		return 0
	}
	got := contentWords(reply)
	n := 0
	for w := range want {
		if got[w] {
			n++
		}
	}
	return float64(n) / float64(len(want))
}

func optionalFloat(v *float64) pgtype.Float8 {
	if v == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *v, Valid: true}
}

func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func optionalDepartment(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

// ResolveDepartment maps a public department UUID to its internal id. nil
// in, nil out; a department before CF-215 is ErrDepartmentsUnavailable.
func (s *Service) ResolveDepartment(ctx context.Context, org int64, id *uuid.UUID) (*int64, error) {
	if id == nil {
		return nil, nil
	}
	if s.Departments == nil {
		return nil, ErrDepartmentsUnavailable
	}
	v, err := s.Departments.DepartmentID(ctx, org, *id)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// DepartmentUUIDs maps internal department ids to public UUIDs (empty
// without a directory).
func (s *Service) DepartmentUUIDs(ctx context.Context, org int64, ids []int64) (map[int64]uuid.UUID, error) {
	if s.Departments == nil || len(ids) == 0 {
		return map[int64]uuid.UUID{}, nil
	}
	return s.Departments.DepartmentUUIDs(ctx, org, ids)
}

// Fingerprint is what a run was measured against. A watch reruns the set
// when it changes.
type Fingerprint struct {
	ConfigRevision     int32     `json:"config_revision"`
	KnowledgeUpdatedAt time.Time `json:"knowledge_updated_at"`
	KnowledgePublished int64     `json:"knowledge_published"`
	ReplyModel         string    `json:"reply_model"`
}

// CurrentFingerprint reads the organization's current fingerprint.
func (s *Service) CurrentFingerprint(ctx context.Context, org int64) (Fingerprint, error) {
	p, err := s.Repo.Q.AIEvaluationFingerprintParts(ctx, org)
	if err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{ConfigRevision: p.ConfigRevision, KnowledgeUpdatedAt: p.KnowledgeUpdatedAt.Time.UTC(), KnowledgePublished: p.KnowledgePublished, ReplyModel: p.ReplyModel}, nil
}

// ChangeKind names why a watch reruns the set; "" when nothing changed.
// Config changes win over model changes, model over knowledge.
func ChangeKind(prev *Fingerprint, cur Fingerprint) string {
	switch {
	case prev == nil:
		return "initial"
	case prev.ConfigRevision != cur.ConfigRevision:
		return "config_change"
	case prev.ReplyModel != cur.ReplyModel && prev.ReplyModel != "":
		return "model_change"
	case !prev.KnowledgeUpdatedAt.Equal(cur.KnowledgeUpdatedAt) || prev.KnowledgePublished != cur.KnowledgePublished:
		return "knowledge_change"
	}
	return ""
}

func parseFingerprint(raw []byte) *Fingerprint {
	if len(raw) == 0 || string(raw) == "{}" {
		return nil
	}
	var f Fingerprint
	if json.Unmarshal(raw, &f) != nil {
		return nil
	}
	return &f
}

// EvaluationWatchView is the organization's automatic run setting.
type EvaluationWatchView struct {
	Enabled         bool         `json:"enabled"`
	LastCheckedAt   *time.Time   `json:"last_checked_at"`
	LastFingerprint *Fingerprint `json:"last_fingerprint"`
}

func viewWatch(w db.AiEvaluationWatch) EvaluationWatchView {
	out := EvaluationWatchView{Enabled: w.Enabled, LastFingerprint: parseFingerprint(w.LastFingerprint)}
	if w.LastCheckedAt.Valid {
		t := w.LastCheckedAt.Time
		out.LastCheckedAt = &t
	}
	return out
}

// EvaluationWatch returns the automatic run setting (disabled when never set).
func (s *Service) EvaluationWatch(ctx context.Context, org int64) (EvaluationWatchView, error) {
	w, err := s.Repo.Q.AIEvaluationWatchGet(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationWatchView{}, nil
	}
	if err != nil {
		return EvaluationWatchView{}, err
	}
	return viewWatch(w), nil
}

// SetEvaluationWatch turns automatic runs on or off. Automatic runs spend
// AI credits and are recorded under the member who turned them on.
func (s *Service) SetEvaluationWatch(ctx context.Context, org, actor int64, enabled bool) (EvaluationWatchView, error) {
	w, err := s.Repo.Q.AIEvaluationWatchSave(ctx, db.AIEvaluationWatchSaveParams{OrganizationID: org, Enabled: enabled, ActorID: actor})
	if err != nil {
		return EvaluationWatchView{}, err
	}
	return viewWatch(w), nil
}

// SweepEvaluations checks up to three due watches and reruns the set of
// each organization whose fingerprint changed since the last check.
func (s *Service) SweepEvaluations(ctx context.Context) error {
	watches, err := s.Repo.Q.AIEvaluationWatchClaim(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, w := range watches {
		if err = s.checkWatch(ctx, w); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) checkWatch(ctx context.Context, w db.AiEvaluationWatch) (err error) {
	cur, err := s.CurrentFingerprint(ctx, w.OrganizationID)
	if err != nil {
		return err
	}
	// store is what the next check compares with. A failed run keeps the
	// previous fingerprint so the change is retried on the next check.
	store := cur
	defer func() {
		raw, e := json.Marshal(store)
		if err != nil {
			raw, e = w.LastFingerprint, nil
			if len(raw) == 0 {
				raw = []byte("{}")
			}
		}
		if e == nil {
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			e = s.Repo.Q.AIEvaluationWatchFinish(finish, db.AIEvaluationWatchFinishParams{OrganizationID: w.OrganizationID, LastFingerprint: raw})
		}
		err = errors.Join(err, e)
	}()
	kind := ChangeKind(parseFingerprint(w.LastFingerprint), cur)
	if kind == "" {
		return nil
	}
	cases, err := s.Repo.Q.AIEvaluationRunCases(ctx, db.AIEvaluationRunCasesParams{OrganizationID: w.OrganizationID})
	if err != nil || len(cases) == 0 {
		return err
	}
	if _, err = s.evaluate(ctx, w.OrganizationID, w.ActorID, EvaluationInput{}, kind, cur); err != nil {
		if errors.Is(err, model.ErrInvalid) {
			return nil
		}
		return err
	}
	// The run itself writes reply runs; keep the fingerprint read after it
	// so its model does not look like a change on the next check.
	if after, e := s.CurrentFingerprint(ctx, w.OrganizationID); e == nil {
		store = after
	}
	return nil
}

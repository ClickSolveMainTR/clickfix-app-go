package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAnswerMatchCountsContentWords(t *testing.T) {
	t.Parallel()
	if got := answerMatch("İade 14 gün içinde yapılır", "İadeniz 14 gün içinde yapılır, kargo ücretsiz"); got < 0.79 || got > 0.81 {
		t.Fatalf("match %v", got) // iade does not match iadeniz: 4 of 5
	}
	if got := answerMatch("the and for", "anything"); got != 0 {
		t.Fatalf("stop words only %v", got)
	}
	if got := answerMatch("Refund within 14 days", "REFUND within 14 DAYS."); got != 1 {
		t.Fatalf("case and punctuation %v", got)
	}
}

func TestMeasureCaseAnswerSourceAndHandoff(t *testing.T) {
	t.Parallel()
	article, other := uuid.New(), uuid.New()
	reply := SimulationResult{Reply: model.Reply{Text: "Refund within 14 days", Confidence: 0.9}, CitedArticles: []uuid.UUID{article}, LatencyMs: 120, CostMicros: 30}
	o := measureCase(Expectations{}, "refund 14 days", []uuid.UUID{article}, reply)
	if !o.AnswerCorrect || o.SourceHit == nil || !*o.SourceHit || o.Score != 1 || o.LatencyMs != 120 || o.CostMicros != 30 {
		t.Fatalf("grounded correct %+v", o)
	}
	o = measureCase(Expectations{}, "refund 14 days", []uuid.UUID{other}, reply)
	if !o.AnswerCorrect || *o.SourceHit || o.Score != 0.5 {
		t.Fatalf("wrong source %+v", o)
	}
	handoff := reply
	handoff.Reply.Handoff = true
	if o = measureCase(Expectations{}, "refund 14 days", nil, handoff); o.AnswerCorrect || o.SourceHit != nil {
		t.Fatalf("handoff counted as answer %+v", o)
	}
	if o = measureCase(Expectations{}, "", []uuid.UUID{article}, reply); !o.AnswerCorrect {
		t.Fatalf("source-only grounded reply %+v", o)
	}
	if o = measureCase(Expectations{Contains: []string{"30 days"}}, "", nil, reply); o.AnswerCorrect || o.Score != 0 {
		t.Fatalf("failed substring %+v", o)
	}
}

func TestMetricAccumulatorSkipsUnmeasured(t *testing.T) {
	t.Parallel()
	hit, miss := true, false
	var a metricAccumulator
	a.add(caseOutcome{AnswerCorrect: true, SourceHit: &hit, LatencyMs: 100, CostMicros: 10})
	a.add(caseOutcome{AnswerCorrect: false, SourceHit: &miss, LatencyMs: 300, CostMicros: 30})
	a.add(caseOutcome{Failed: true, CostMicros: 5})
	m := a.metrics()
	if m.Cases != 3 || m.Completed != 2 || *m.AnswerAccuracy != 0.5 || *m.SourceHitRate != 0.5 || *m.AvgLatencyMs != 200 || *m.CostPerCaseMicros != 15 {
		t.Fatalf("metrics %+v", m)
	}
	if m.CacheHitRate != nil {
		t.Fatal("cache hit rate without a cache")
	}
	if empty := (&metricAccumulator{}).metrics(); empty.AnswerAccuracy != nil || empty.AvgLatencyMs != nil || empty.CostPerCaseMicros != nil {
		t.Fatalf("empty %+v", empty)
	}
}

func TestChangeKind(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	base := Fingerprint{ConfigRevision: 2, KnowledgeUpdatedAt: at, KnowledgePublished: 3, ReplyModel: "m1"}
	for _, tc := range []struct {
		name string
		prev *Fingerprint
		cur  Fingerprint
		want string
	}{
		{"first check", nil, base, "initial"},
		{"unchanged", &base, base, ""},
		{"config", &base, Fingerprint{ConfigRevision: 3, KnowledgeUpdatedAt: at.Add(time.Hour), KnowledgePublished: 3, ReplyModel: "m2"}, "config_change"},
		{"model", &base, Fingerprint{ConfigRevision: 2, KnowledgeUpdatedAt: at, KnowledgePublished: 3, ReplyModel: "m2"}, "model_change"},
		{"first model seen", &Fingerprint{ConfigRevision: 2, KnowledgeUpdatedAt: at, KnowledgePublished: 3}, base, ""},
		{"knowledge", &base, Fingerprint{ConfigRevision: 2, KnowledgeUpdatedAt: at, KnowledgePublished: 4, ReplyModel: "m1"}, "knowledge_change"},
	} {
		if got := ChangeKind(tc.prev, tc.cur); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

type fakeCache struct{ hit map[string]bool }

func (f fakeCache) Lookup(_ context.Context, _ int64, _ *int64, question, _ string) (bool, error) {
	return f.hit[question], nil
}

type fakeDepartments struct {
	org int64
	ids map[uuid.UUID]int64
}

func (f fakeDepartments) DepartmentID(_ context.Context, org int64, id uuid.UUID) (int64, error) {
	if v, ok := f.ids[id]; ok && org == f.org {
		return v, nil
	}
	return 0, pgx.ErrNoRows
}
func (f fakeDepartments) DepartmentUUIDs(_ context.Context, org int64, ids []int64) (map[int64]uuid.UUID, error) {
	out := map[int64]uuid.UUID{}
	for u, id := range f.ids {
		if org == f.org {
			out[id] = u
		}
	}
	return out, nil
}

// testDepartment creates a department of org and returns its id.
func testDepartment(t *testing.T, org int64, name string) int64 {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var id int64
	if err := pool.QueryRow(ctx, "INSERT INTO departments(organization_id,name) VALUES($1,$2) RETURNING id", org, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// One run measures accuracy, sources, cache, latency and cost per
// department; the next run of the same scope carries it as "before".
func TestEvaluationMetricsBeforeAfterAndDepartments(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	ctx := context.Background()
	article := uuid.New()
	source := kbmodel.Citation{ChunkUUID: uuid.New(), ArticleUUID: article, Content: "Refund within 14 days", Locale: "en"}
	engine.RAG = fakeSources{citation: source}
	cfg := model.DefaultConfig()
	cfg.Mode = "draft"
	cfg.Channels = []string{"web"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	billingDept := uuid.New()
	if _, err := svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Refund", Question: "When can I return?", Locale: "en", Department: &billingDept, ExpectedAnswer: "refund 14 days"}); !errors.Is(err, ErrDepartmentsUnavailable) {
		t.Fatalf("department before CF-215: %v", err)
	}
	// A real department of the organization: department_id is a foreign key (CF-224).
	billingDeptID := testDepartment(t, org, "Billing")
	svc.DepartmentSets = fakeDepartments{org: org, ids: map[uuid.UUID]int64{billingDept: billingDeptID}}
	svc.Cache = fakeCache{hit: map[string]bool{"Customer: When can I return?": true}}
	refund, err := svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Refund", Question: "When can I return?", Category: "billing", Locale: "en", Department: &billingDept, ExpectedAnswer: "refund 14 days", ExpectedArticles: []uuid.UUID{article}})
	if err != nil {
		t.Fatal(err)
	}
	if refund.Transcript != "Customer: When can I return?" || !refund.DepartmentID.Valid || refund.DepartmentID.Int64 != billingDeptID || len(refund.ExpectedArticleUuids) != 1 {
		t.Fatalf("case %+v", refund)
	}
	if _, err = svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Shipping", Question: "Where is my parcel?", Locale: "en", ExpectedArticles: []uuid.UUID{uuid.New()}}); err != nil {
		t.Fatal(err)
	}
	inactive := false
	if _, err = svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Old", Question: "Old question", Locale: "en", ExpectedAnswer: "old", Active: &inactive}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Empty", Question: "No expectation", Locale: "en"}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("case without expectation %v", err)
	}
	fake.response = `{"reply":"Refund within 14 days","summary":"Refund","confidence":0.95,"handoff":false,"citation_ids":["` + source.ChunkUUID.String() + `"]}`
	first, err := svc.Evaluate(ctx, org, actor, EvaluationInput{})
	if err != nil {
		t.Fatal(err)
	}
	var report EvaluationReport
	if err = json.Unmarshal(first.Report, &report); err != nil {
		t.Fatal(err)
	}
	m := report.Metrics
	if report.Cases != 2 || m.Completed != 2 || *m.AnswerAccuracy != 0.5 || *m.SourceHitRate != 0.5 || *m.CacheHitRate != 0.5 || m.AvgLatencyMs == nil || m.CostPerCaseMicros == nil || report.Before != nil {
		b, _ := json.Marshal(m)
		t.Fatalf("first run %+v metrics %s", report, b)
	}
	if first.TriggerKind != "manual" || first.Model == "" || !first.AnswerAccuracy.Valid || first.AnswerAccuracy.Float64 != 0.5 || !first.CacheHitRate.Valid {
		t.Fatalf("stored metrics %+v", first)
	}
	if len(report.Departments) != 2 || report.Departments[0].DepartmentUUID != nil || report.Departments[1].DepartmentUUID == nil || *report.Departments[1].DepartmentUUID != billingDept || *report.Departments[1].AnswerAccuracy != 1 {
		t.Fatalf("departments %+v", report.Departments)
	}
	results, err := svc.Repo.Q.AIEvaluationResults(ctx, db.AIEvaluationResultsParams{OrganizationID: org, EvaluationUuid: first.Uuid, Limit: 10})
	if err != nil || len(results) != 2 || !results[0].AnswerCorrect || !results[0].SourceHit.Bool || !results[0].CacheHit.Bool || results[1].SourceHit.Bool {
		t.Fatalf("results %+v %v", results, err)
	}

	// Department run: only that department's cases, its own before/after chain.
	dept, err := svc.Evaluate(ctx, org, actor, EvaluationInput{Department: &billingDept})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(dept.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Cases != 1 || report.Before != nil || dept.BaselineUuid.Valid {
		t.Fatalf("department run %+v", report)
	}

	fake.response = `{"reply":"Please ask support","summary":"Refund","confidence":0.95,"handoff":false,"citation_ids":["` + source.ChunkUUID.String() + `"]}`
	second, err := svc.Evaluate(ctx, org, actor, EvaluationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(second.Report, &report); err != nil {
		t.Fatal(err)
	}
	if !second.BaselineUuid.Valid || second.BaselineUuid.Bytes != first.Uuid || report.Before == nil || *report.Before.AnswerAccuracy != 0.5 || *report.Metrics.AnswerAccuracy != 0 || len(report.Regressions) != 1 || report.Regressions[0] != refund.Uuid {
		t.Fatalf("before/after %+v", report)
	}

	// Another organization sees none of it.
	if _, err = svc.Evaluate(ctx, org+999999, actor, EvaluationInput{}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("foreign organization ran this set: %v", err)
	}
	if _, err = svc.Evaluate(ctx, org+999999, actor, EvaluationInput{Department: &billingDept}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign department resolved: %v", err)
	}
	if _, err = svc.UpdateEvaluationCase(ctx, org+999999, refund.Uuid, EvaluationCaseInput{Name: "x", Question: "x", Locale: "en", ExpectedAnswer: "x"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign update %v", err)
	}
	updated, err := svc.UpdateEvaluationCase(ctx, org, refund.Uuid, EvaluationCaseInput{Name: "Refund", Question: "When can I return?", Locale: "en", ExpectedAnswer: "refund 30 days", Department: &billingDept})
	if err != nil || updated.Revision != 2 || updated.ExpectedAnswer != "refund 30 days" {
		t.Fatalf("update %+v %v", updated, err)
	}
}

// The watch runs the set once at first, stays quiet while nothing
// changes, and reruns on a config change.
func TestEvaluationWatchRerunsOnChange(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	ctx := context.Background()
	source := kbmodel.Citation{ChunkUUID: uuid.New(), ArticleUUID: uuid.New(), Content: "Refund within 14 days", Locale: "en"}
	engine.RAG = fakeSources{citation: source}
	fake.response = `{"reply":"Refund within 14 days","summary":"Refund","confidence":0.95,"handoff":false,"citation_ids":["` + source.ChunkUUID.String() + `"]}`
	cfg := model.DefaultConfig()
	cfg.Mode = "draft"
	cfg.Channels = []string{"web"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Refund", Question: "When can I return?", Locale: "en", ExpectedAnswer: "refund 14 days"}); err != nil {
		t.Fatal(err)
	}
	if w, err := svc.EvaluationWatch(ctx, org); err != nil || w.Enabled {
		t.Fatalf("default watch %+v %v", w, err)
	}
	if _, err := svc.SetEvaluationWatch(ctx, org, actor, true); err != nil {
		t.Fatal(err)
	}
	runs := func() []db.AiEvaluation {
		rows, err := svc.Repo.Q.AIEvaluationList(ctx, db.AIEvaluationListParams{OrganizationID: org, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	check := func() {
		w, err := svc.Repo.Q.AIEvaluationWatchGet(ctx, org)
		if err != nil {
			t.Fatal(err)
		}
		if err = svc.checkWatch(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	check()
	if r := runs(); len(r) != 1 || r[0].TriggerKind != "initial" || r[0].AuthorID != actor {
		t.Fatalf("initial run %+v", r)
	}
	check()
	if r := runs(); len(r) != 1 {
		t.Fatalf("reran without a change: %d runs", len(r))
	}
	cfg.Confidence = 0.9
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	check()
	if r := runs(); len(r) != 2 || r[0].TriggerKind != "config_change" {
		t.Fatalf("config change run %+v", r)
	}
	// A run that fails keeps the old fingerprint, so the change is retried.
	before, err := svc.Repo.Q.AIEvaluationWatchGet(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Confidence = 0.95
	if _, err = svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = svc.checkWatch(cancelled, before); err == nil {
		t.Fatal("cancelled check succeeded")
	}
	after, err := svc.Repo.Q.AIEvaluationWatchGet(ctx, org)
	if err != nil || string(after.LastFingerprint) != string(before.LastFingerprint) {
		t.Fatalf("failed run advanced the fingerprint: %s -> %s (%v)", before.LastFingerprint, after.LastFingerprint, err)
	}
	check()
	if r := runs(); r[0].TriggerKind != "config_change" || r[0].Status != "completed" {
		t.Fatalf("retried run %+v", r[0])
	}
	w, err := svc.EvaluationWatch(ctx, org)
	if err != nil || !w.Enabled || w.LastCheckedAt == nil || w.LastFingerprint == nil || w.LastFingerprint.ConfigRevision == 0 {
		t.Fatalf("watch %+v %v", w, err)
	}
	if _, err = svc.SetEvaluationWatch(ctx, org, actor, false); err != nil {
		t.Fatal(err)
	}
	claimed, err := svc.Repo.Q.AIEvaluationWatchClaim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range claimed {
		if c.OrganizationID == org {
			t.Fatal("disabled watch claimed")
		}
	}
}

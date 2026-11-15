package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	memmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	deptusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fakeProbe answers like the knowledge layer's Probe and records queries.
type fakeProbe struct {
	ready   bool
	answers map[string]bool
	queries []memmodel.Query
}

func (f *fakeProbe) Ready() bool { return f.ready }
func (f *fakeProbe) Probe(_ context.Context, q memmodel.Query) (bool, error) {
	f.queries = append(f.queries, q)
	return f.answers[q.Question], nil
}

func TestLastCustomerTurn(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Customer: When can I return?":                        "When can I return?",
		"Customer: hi\nAgent: hello\nCustomer:  And refunds?": "And refunds?",
		"plain question":                                      "plain question",
	} {
		if got := lastCustomerTurn(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestWireMeasurementNeedsReadyMemory(t *testing.T) {
	t.Parallel()
	s := &Service{}
	WireMeasurement(s, fakeDepartments{}, &fakeProbe{})
	if s.DepartmentSets == nil || s.Cache != nil {
		t.Fatalf("not ready memory wired a cache: %+v", s.Cache)
	}
	WireMeasurement(s, fakeDepartments{}, &fakeProbe{ready: true})
	if s.Cache == nil {
		t.Fatal("ready memory not wired")
	}
}

// CF-225: a department case is saved and run against the real department
// directory, and its cache hit is measured through the knowledge layer
// adapter with the case's department as the lookup scope.
func TestEvaluationWithDepartmentDirectoryAndCache(t *testing.T) {
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
	dept, err := svc.Repo.Q.CreateDepartment(ctx, db.CreateDepartmentParams{OrganizationID: org, Name: "Billing " + uuid.NewString()[:8], AiEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	probe := &fakeProbe{ready: true, answers: map[string]bool{"When can I return?": true}}
	WireMeasurement(svc, deptusecase.NewDirectory(svc.Repo.Q), probe)

	if _, err = svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Foreign", Question: "x", Locale: "en", ExpectedAnswer: "x", Department: ptr(uuid.New())}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown department accepted: %v", err)
	}
	c, err := svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Refund", Question: "When can I return?", Locale: "en", Department: &dept.Uuid, ExpectedAnswer: "refund 14 days"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.DepartmentID.Valid || c.DepartmentID.Int64 != dept.ID {
		t.Fatalf("case department %+v", c.DepartmentID)
	}
	if _, err = svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Parcel", Question: "Where is my parcel?", Locale: "en", ExpectedAnswer: "parcel"}); err != nil {
		t.Fatal(err)
	}
	run, err := svc.Evaluate(ctx, org, actor, EvaluationInput{Department: &dept.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	var report EvaluationReport
	if err = json.Unmarshal(run.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Cases != 1 || report.Metrics.CacheHitRate == nil || *report.Metrics.CacheHitRate != 1 || !run.CacheHitRate.Valid {
		b, _ := json.Marshal(report.Metrics)
		t.Fatalf("department run %+v metrics %s", report, b)
	}
	if len(report.Departments) != 1 || report.Departments[0].DepartmentUUID == nil || *report.Departments[0].DepartmentUUID != dept.Uuid {
		t.Fatalf("department breakdown %+v", report.Departments)
	}
	if len(probe.queries) != 1 || probe.queries[0].Scope.DepartmentID == nil || *probe.queries[0].Scope.DepartmentID != dept.ID || probe.queries[0].Organization != org || probe.queries[0].Audience != memmodel.AudienceCustomer {
		t.Fatalf("probe scope %+v", probe.queries)
	}

	// Organization run: the organization-wide case misses the cache.
	all, err := svc.Evaluate(ctx, org, actor, EvaluationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !all.CacheHitRate.Valid || all.CacheHitRate.Float64 != 0.5 {
		t.Fatalf("organization cache hit rate %+v", all.CacheHitRate)
	}

	// Another organization cannot resolve this department.
	if _, err = svc.Evaluate(ctx, org+999999, actor, EvaluationInput{Department: &dept.Uuid}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign organization resolved the department: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

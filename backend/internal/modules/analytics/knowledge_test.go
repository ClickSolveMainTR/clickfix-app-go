package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	am "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	ar "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/repository"
	au "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	"github.com/google/uuid"
)

// fakeStore records the scope it was asked for and answers like the
// built-in store: organization only.
type fakeStore struct{ scopes []am.KnowledgeScope }

func (f *fakeStore) scope(s am.KnowledgeScope) error {
	f.scopes = append(f.scopes, s)
	if s.Department != nil {
		return am.ErrDepartmentScope
	}
	return nil
}
func (f *fakeStore) Usage(_ context.Context, s am.KnowledgeScope) ([]am.TaskUsage, error) {
	return []am.TaskUsage{{Task: "reply", Calls: 3, Failed: 1, CostMicros: 90}, {Task: "triage", Calls: 2, CostMicros: 10}}, f.scope(s)
}
func (f *fakeStore) Gaps(_ context.Context, s am.KnowledgeScope) ([]am.GapTopic, error) {
	return []am.GapTopic{{Topic: "refund", Conversations: 2}}, f.scope(s)
}
func (f *fakeStore) Suggestions(_ context.Context, s am.KnowledgeScope) (map[string]int64, error) {
	return map[string]int64{"pending": 2, "approved": 1}, f.scope(s)
}
func (f *fakeStore) Classifications(_ context.Context, s am.KnowledgeScope) ([]am.Classification, error) {
	low, high := 0.3, 0.9
	return []am.Classification{{Category: "billing", Confidence: &high}, {Category: "billing", Confidence: &low}, {Category: "shipping"}}, f.scope(s)
}
func (f *fakeStore) ReplyOutcomes(_ context.Context, s am.KnowledgeScope) (int64, int64, error) {
	return 4, 1, f.scope(s)
}
func (f *fakeStore) Evaluations(_ context.Context, s am.KnowledgeScope) ([]am.EvaluationPoint, error) {
	f.scopes = append(f.scopes, s)
	return []am.EvaluationPoint{{UUID: uuid.New()}, {UUID: uuid.New()}}, nil
}

type fakeDepartments []am.Department

func (f fakeDepartments) Departments(context.Context, int64) ([]am.Department, error) { return f, nil }

type fakeCache struct{ lookups, hits int64 }

func (f fakeCache) CacheStats(context.Context, am.KnowledgeScope) (int64, int64, error) {
	return f.lookups, f.hits, nil
}

func TestKnowledgePanelOrganizationScope(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	until := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	svc := au.NewKnowledge(store)
	svc.Now = func() time.Time { return until }
	out, err := svc.Panel(context.Background(), 5, nil, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.From.Equal(until.AddDate(0, 0, -30)) || !out.Until.Equal(until) {
		t.Fatalf("default window %v %v", out.From, out.Until)
	}
	for _, s := range store.scopes {
		if s.Organization != 5 || s.Department != nil {
			t.Fatalf("scope %+v", s)
		}
	}
	if out.Cache.Available || out.Cache.Reason != am.ReasonSourceNotConnected || out.Cache.HitRate != nil {
		t.Fatalf("cache without a source %+v", out.Cache)
	}
	if !out.Usage.Available || out.Usage.Calls != 5 || out.Usage.Failed != 1 || out.Usage.CostMicros != 100 || len(out.Usage.ByTask) != 2 {
		t.Fatalf("usage %+v", out.Usage)
	}
	if out.Suggestions.Pending != 2 || out.Suggestions.Approved != 1 || out.Suggestions.Rejected != 0 {
		t.Fatalf("suggestions %+v", out.Suggestions)
	}
	c := out.Categories
	if c.Classified != 3 || c.LowConfidence != 1 || *c.AvgConfidence != 0.6 || len(c.Categories) != 2 || c.Categories[0].Category != "billing" || c.Categories[1].AvgConfidence != nil {
		t.Fatalf("categories %+v", c)
	}
	if len(c.Buckets) != 5 || c.Buckets[1].Count != 1 || c.Buckets[4].Count != 1 {
		t.Fatalf("buckets %+v", c.Buckets)
	}
	if *out.Handoff.Rate != 0.25 || out.Evaluation.Latest == nil || out.Evaluation.Previous == nil {
		t.Fatalf("handoff/evaluation %+v %+v", out.Handoff, out.Evaluation)
	}
	if _, err = svc.Panel(context.Background(), 5, nil, until, until.Add(-time.Hour)); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("reversed window %v", err)
	}
}

func TestKnowledgePanelDepartmentScope(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	svc := au.NewKnowledge(store)
	dept := uuid.New()
	if _, err := svc.Panel(context.Background(), 5, &dept, time.Time{}, time.Time{}); !errors.Is(err, am.ErrDepartmentsUnavailable) {
		t.Fatalf("department without a department source: %v", err)
	}
	svc.Departments = fakeDepartments{{ID: 9, UUID: dept, Name: "Muhasebe"}}
	svc.Cache = fakeCache{lookups: 10, hits: 4}
	foreign := uuid.New()
	if _, err := svc.Panel(context.Background(), 5, &foreign, time.Time{}, time.Time{}); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("unknown department %v", err)
	}
	out, err := svc.Panel(context.Background(), 5, &dept, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Department == nil || out.Department.UUID != dept || len(out.Departments) != 1 {
		t.Fatalf("department %+v", out.Department)
	}
	// Organization-only sources (here the fake store) say so instead of
	// showing the whole organization.
	for name, a := range map[string]am.Availability{"usage": out.Usage.Availability, "gaps": out.KnowledgeGaps.Availability, "suggestions": out.Suggestions.Availability, "categories": out.Categories.Availability, "handoff": out.Handoff.Availability} {
		if a.Available || a.Reason != am.ReasonDepartmentScopeUnavailable {
			t.Errorf("%s: %+v", name, a)
		}
	}
	if out.Usage.Calls != 0 || len(out.KnowledgeGaps.Topics) != 0 || out.Handoff.Rate != nil || out.Categories.Classified != 0 {
		t.Fatalf("organization data leaked into department scope %+v", out)
	}
	if !out.Cache.Available || *out.Cache.HitRate != 0.4 || !out.Evaluation.Available {
		t.Fatalf("department-aware sources %+v %+v", out.Cache, out.Evaluation)
	}
}

// The built-in store reads only the requested organization and window.
func TestKnowledgePanelTenantIsolation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	var msg, otherConv, otherMsg, otherContact int64
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','q') RETURNING id", f.org, f.conv).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Other') RETURNING id", f.other).Scan(&otherContact); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status) VALUES($1,1,'web',$2,'open') RETURNING id", f.other, otherContact).Scan(&otherConv); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','q') RETURNING id", f.other, otherConv).Scan(&otherMsg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, sql := range []string{"DELETE FROM knowledge_suggestions WHERE organization_id=ANY($1::bigint[])", "DELETE FROM ai_agent_jobs WHERE organization_id=ANY($1::bigint[])", "DELETE FROM ai_runs WHERE organization_id=ANY($1::bigint[])"} {
			_, _ = f.pool.Exec(context.Background(), sql, []int64{f.org, f.other})
		}
	})
	exec("INSERT INTO ai_runs(organization_id,task,provider,model,status,input,cost_micros,latency_ms) VALUES($1,'reply','fake','m','completed','x',40,200),($1,'triage','fake','m','failed','x',5,0),($2,'reply','fake','m','completed','x',9000,1)", f.org, f.other)
	exec(`INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task,status,result) VALUES
	($1,$2,$3,'triage','completed','{"category":"billing","confidence":0.4}'),($1,$2,$3,'reply','handoff','{}'),
	($4,$5,$6,'triage','completed','{"category":"secret","confidence":0.99}'),($4,$5,$6,'reply','completed','{}')`, f.org, f.conv, msg, f.other, otherConv, otherMsg)
	exec("INSERT INTO knowledge_suggestions(organization_id,topic,conversation_id,title,body,status) VALUES($1,'billing',$2,'t','b','rejected'),($3,'secret',$4,'t','b','pending')", f.org, f.conv, f.other, otherConv)

	svc := au.NewKnowledge(ar.NewKnowledge(f.q))
	out, err := svc.Panel(ctx, f.org, nil, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if out.Usage.Calls != 2 || out.Usage.CostMicros != 45 || out.Usage.Failed != 1 {
		t.Fatalf("usage crossed organizations %+v", out.Usage)
	}
	if len(out.KnowledgeGaps.Topics) != 1 || out.KnowledgeGaps.Topics[0].Topic != "billing" {
		t.Fatalf("gaps %+v", out.KnowledgeGaps)
	}
	if out.Suggestions.Rejected != 1 || out.Suggestions.Pending != 0 {
		t.Fatalf("suggestions %+v", out.Suggestions)
	}
	if out.Categories.Classified != 1 || out.Categories.Categories[0].Category != "billing" || out.Categories.LowConfidence != 1 {
		t.Fatalf("categories %+v", out.Categories)
	}
	if out.Handoff.Conversations != 1 || out.Handoff.Handoffs != 1 {
		t.Fatalf("handoff %+v", out.Handoff)
	}
	if out.Evaluation.Latest != nil {
		t.Fatalf("evaluation %+v", out.Evaluation)
	}
	// Outside the window nothing is counted.
	later, err := svc.Panel(ctx, f.org, nil, now.Add(time.Hour), now.Add(2*time.Hour))
	if err != nil || later.Usage.Calls != 0 || later.Handoff.Conversations != 0 || later.Categories.Classified != 0 {
		t.Fatalf("window %+v %v", later, err)
	}
}

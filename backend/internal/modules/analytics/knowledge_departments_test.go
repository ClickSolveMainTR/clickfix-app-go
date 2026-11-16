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

// CF-225: with the department, cache and category sources wired, a
// department filter narrows every department-bearing section to that
// department's conversations, and nothing crosses organizations.
func TestKnowledgePanelDepartmentSources(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scan := func(sql string, args []any, dst ...any) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(dst...); err != nil {
			t.Fatal(err)
		}
	}
	var dept, foreignDept, deptConv, deptMsg, otherContact, otherConv, otherMsg, category, otherCategory int64
	var deptUUID, foreignUUID uuid.UUID
	scan("INSERT INTO departments(organization_id,name) VALUES($1,'Muhasebe') RETURNING id,uuid", []any{f.org}, &dept, &deptUUID)
	scan("INSERT INTO departments(organization_id,name) VALUES($1,'Gizli') RETURNING id,uuid", []any{f.other}, &foreignDept, &foreignUUID)
	scan("INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,department_id,department_source) VALUES($1,2,'web',$2,'open',$3,'ai') RETURNING id", []any{f.org, f.contact, dept}, &deptConv)
	scan("INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','q') RETURNING id", []any{f.org, deptConv}, &deptMsg)
	scan("INSERT INTO contacts(organization_id,name) VALUES($1,'Other') RETURNING id", []any{f.other}, &otherContact)
	scan("INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,department_id,department_source) VALUES($1,1,'web',$2,'open',$3,'ai') RETURNING id", []any{f.other, otherContact, foreignDept}, &otherConv)
	scan("INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','q') RETURNING id", []any{f.other, otherConv}, &otherMsg)
	scan("INSERT INTO categories(organization_id,name,slug) VALUES($1,'İade','iade') RETURNING id", []any{f.org}, &category)
	scan("INSERT INTO categories(organization_id,name,slug) VALUES($1,'Gizli','gizli') RETURNING id", []any{f.other}, &otherCategory)
	t.Cleanup(func() {
		for _, sql := range []string{"DELETE FROM category_assignments WHERE organization_id=ANY($1::bigint[])", "DELETE FROM categories WHERE organization_id=ANY($1::bigint[])", "DELETE FROM knowledge_suggestions WHERE organization_id=ANY($1::bigint[])", "DELETE FROM ai_agent_jobs WHERE organization_id=ANY($1::bigint[])", "DELETE FROM ai_runs WHERE organization_id=ANY($1::bigint[])"} {
			_, _ = f.pool.Exec(context.Background(), sql, []int64{f.org, f.other})
		}
	})
	exec(`INSERT INTO ai_runs(organization_id,task,provider,model,status,input,cost_micros,latency_ms,conversation_id) VALUES
	($1,'reply','fake','m','completed','x',7,100,$2),($1,'embedding','fake','m','completed','x',40,10,NULL),($3,'reply','fake','m','completed','x',9000,1,$4)`, f.org, deptConv, f.other, otherConv)
	exec(`INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task,status,result) VALUES
	($1,$2,$3,'triage','completed','{"category":"refund","confidence":0.8}'),($1,$2,$3,'reply','handoff','{}'),
	($4,$5,$6,'triage','completed','{"category":"secret","confidence":0.99}'),($4,$5,$6,'reply','handoff','{}')`, f.org, deptConv, deptMsg, f.other, otherConv, otherMsg)
	// The department suggestion inherits its conversation's department.
	exec("INSERT INTO knowledge_suggestions(organization_id,topic,conversation_id,title,body,status) VALUES($1,'refund',$2,'t','b','pending'),($1,'general',$3,'t','b','approved'),($4,'secret',$5,'t','b','pending')", f.org, deptConv, f.conv, f.other, otherConv)
	// The fixture session runs in America/Los_Angeles while the panel window
	// below is in UTC: use the UTC day, not current_date (which is the LA day
	// and lags the window between 00:00 and 08:00 UTC).
	exec(`INSERT INTO ai_answer_cache_stats(organization_id,day,audience,department_id,exact_hits,semantic_hits,misses) VALUES
	($1,(now() AT TIME ZONE 'UTC')::date,'customer',NULL,2,3,5),($1,(now() AT TIME ZONE 'UTC')::date,'customer',$2,1,2,1),($3,(now() AT TIME ZONE 'UTC')::date,'customer',$4,50,50,0)`, f.org, dept, f.other, foreignDept)
	exec(`INSERT INTO category_assignments(organization_id,subject_type,subject_id,subject_uuid,category_id,confidence,source) VALUES
	($1,'conversation',$2,$3,$4,0.7,'embedding'),($5,'conversation',$6,$7,$8,0.99,'ai')`, f.org, deptConv, uuid.New(), category, f.other, otherConv, uuid.New(), otherCategory)

	svc := au.NewKnowledge(ar.NewKnowledge(f.q))
	svc.Departments, svc.Cache, svc.Categories = ar.NewDepartments(f.q), ar.NewCache(f.q), ar.NewCategories(f.q)
	from, until := now.Add(-time.Hour), now.Add(time.Hour)

	org, err := svc.Panel(ctx, f.org, nil, from, until)
	if err != nil {
		t.Fatal(err)
	}
	if len(org.Departments) != 1 || org.Departments[0].UUID != deptUUID || org.Departments[0].Name != "Muhasebe" {
		t.Fatalf("departments %+v", org.Departments)
	}
	if !org.Cache.Available || org.Cache.Lookups != 14 || org.Cache.Hits != 8 {
		t.Fatalf("organization cache %+v", org.Cache)
	}
	if org.Usage.Calls != 2 || org.Usage.CostMicros != 47 {
		t.Fatalf("organization usage %+v", org.Usage)
	}
	if org.Suggestions.Pending != 1 || org.Suggestions.Approved != 1 {
		t.Fatalf("organization suggestions %+v", org.Suggestions)
	}
	if org.Categories.Classified != 1 || org.Categories.Categories[0].Category != "İade" {
		t.Fatalf("organization categories %+v", org.Categories)
	}

	out, err := svc.Panel(ctx, f.org, &deptUUID, from, until)
	if err != nil {
		t.Fatal(err)
	}
	if out.Department == nil || out.Department.UUID != deptUUID {
		t.Fatalf("department %+v", out.Department)
	}
	for name, a := range map[string]am.Availability{"cache": out.Cache.Availability, "usage": out.Usage.Availability, "gaps": out.KnowledgeGaps.Availability, "suggestions": out.Suggestions.Availability, "categories": out.Categories.Availability, "handoff": out.Handoff.Availability, "evaluation": out.Evaluation.Availability} {
		if !a.Available {
			t.Errorf("%s not available in department scope: %+v", name, a)
		}
	}
	if out.Cache.Lookups != 4 || out.Cache.Hits != 3 || *out.Cache.HitRate != 0.75 {
		t.Fatalf("department cache %+v", out.Cache)
	}
	if out.Usage.Calls != 1 || out.Usage.CostMicros != 7 {
		t.Fatalf("department usage %+v", out.Usage)
	}
	if len(out.KnowledgeGaps.Topics) != 1 || out.KnowledgeGaps.Topics[0].Topic != "refund" {
		t.Fatalf("department gaps %+v", out.KnowledgeGaps)
	}
	if out.Suggestions.Pending != 1 || out.Suggestions.Approved != 0 {
		t.Fatalf("department suggestions %+v", out.Suggestions)
	}
	if out.Categories.Classified != 1 || out.Categories.Categories[0].Category != "İade" || *out.Categories.AvgConfidence < 0.69 {
		t.Fatalf("department categories %+v", out.Categories)
	}
	if out.Handoff.Conversations != 1 || out.Handoff.Handoffs != 1 {
		t.Fatalf("department handoff %+v", out.Handoff)
	}

	// Another organization's department is not a filter here, and the
	// other organization cannot use this one.
	if _, err := svc.Panel(ctx, f.org, &foreignUUID, from, until); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("foreign department %v", err)
	}
	if _, err := svc.Panel(ctx, f.other, &deptUUID, from, until); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("department used by another organization %v", err)
	}

	// Before the category layer classified anything, triage results stand in.
	exec("DELETE FROM category_assignments WHERE organization_id=$1", f.org)
	fallback, err := svc.Panel(ctx, f.org, &deptUUID, from, until)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Categories.Classified != 1 || fallback.Categories.Categories[0].Category != "refund" {
		t.Fatalf("triage fallback %+v", fallback.Categories)
	}
}

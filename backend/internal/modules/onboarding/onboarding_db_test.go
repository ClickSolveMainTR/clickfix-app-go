package onboarding_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/onboarding"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
)

// TestOnboardingAgainstPostgres runs the flow on the real schema: self-serve
// sign-up makes the caller owner, the counts query sees channels, members
// and invitations, manual steps are idempotent and finish is final.
func TestOnboardingAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for onboarding DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	var userID, peerID int64
	email := uuid.NewString() + "@example.test"
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Yıldız', 'active') RETURNING id", email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Mert', 'Demir', 'active') RETURNING id", uuid.NewString()+"@example.test").Scan(&peerID); err != nil {
		t.Fatal(err)
	}

	orgs := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	svc := onboarding.New(q, orgs, onboarding.Config{SelfServe: true})
	org, err := svc.CreateOrganization(ctx, onboarding.CreateInput{UserID: userID, Name: "Onboarding Test " + uuid.NewString()[:6]})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", []int64{userID, peerID})
	})

	owned, err := q.CountOwnedOrganizations(ctx, userID)
	if err != nil || owned != 1 {
		t.Fatalf("owned = %d, %v", owned, err)
	}

	st, err := svc.Get(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Completed != 0 || len(st.Steps) != 5 || st.Steps[0].Key != onboarding.StepCustomer {
		t.Fatalf("fresh: %+v", st)
	}

	// The facts come from the tables they live in: a company profile, a
	// contact with an external id, a company with a tier, a published
	// procedure and an article tick their conditions; the identity key
	// comes from the channels module (here: a stub).
	if _, err := pool.Exec(ctx, `UPDATE organizations SET settings = settings || '{"company":{"legal_name":"Onboarding A.Ş."}}'::jsonb WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO contacts (organization_id, name, external_id) VALUES ($1, 'Ada', 'crm-1')", org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO companies (organization_id, name, tier) VALUES ($1, 'Acme', 'gold')", org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO channel_instances (organization_id, channel_type, name, external_id) VALUES ($1, 'widget', 'Web sitesi', $2)", org.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO knowledge_articles (organization_id, slug, locale, title, body, visibility, author_id) VALUES ($1, 'iade', 'tr', 'İade', 'İade adımları', 'internal', $2)", org.ID, userID); err != nil {
		t.Fatal(err)
	}
	svc.WithIdentityKeys(func(context.Context, int64) (bool, error) { return true, nil })
	st, err = svc.Get(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	// customer (all four conditions) + knowledge.
	if st.Completed != 2 || !st.Steps[0].Done || !st.Steps[3].Done || onboarding.Next(st.Steps) != onboarding.StepSystem {
		t.Fatalf("after facts: %+v", st)
	}

	// The remaining conditions, one row each, against the real tables the
	// OrganizationSetupCounts query reads.
	stepOf := func(key string) onboarding.Step {
		st, err := svc.Get(ctx, org.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range st.Steps {
			if s.Key == key {
				return s
			}
		}
		t.Fatalf("no step %s", key)
		return onboarding.Step{}
	}
	condition := func(step onboarding.Step, key string) bool {
		for _, c := range step.Conditions {
			if c.Key == key {
				return c.Done
			}
		}
		t.Fatalf("no condition %s on %s", key, step.Key)
		return false
	}
	// package: a company's revenue plan counts as well as its tier.
	if _, err := pool.Exec(ctx, "DELETE FROM companies WHERE organization_id = $1", org.ID); err != nil {
		t.Fatal(err)
	}
	if condition(stepOf(onboarding.StepCustomer), onboarding.CondPackage) {
		t.Fatal("package: no company, must be unmet")
	}
	var planned int64
	if err := pool.QueryRow(ctx, "INSERT INTO companies (organization_id, name) VALUES ($1, 'Planlı') RETURNING id", org.ID).Scan(&planned); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO company_revenue (organization_id, company_id, plan) VALUES ($1, $2, 'pro')", org.ID, planned); err != nil {
		t.Fatal(err)
	}
	if !condition(stepOf(onboarding.StepCustomer), onboarding.CondPackage) {
		t.Fatal("package: a revenue plan must count")
	}
	// system: an integration connection.
	if stepOf(onboarding.StepSystem).Done {
		t.Fatal("system: must be unmet without a connection")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO integration_connections (organization_id, name, base_url) VALUES ($1, 'ERP', 'https://erp.example.com')", org.ID); err != nil {
		t.Fatal(err)
	}
	if !stepOf(onboarding.StepSystem).Done {
		t.Fatal("system: a connection must finish the step")
	}
	// ai: the switch and a published procedure, a draft is not enough.
	if _, err := pool.Exec(ctx, "INSERT INTO ai_policies (organization_id, enabled) VALUES ($1, TRUE) ON CONFLICT (organization_id) DO UPDATE SET enabled = TRUE", org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO ai_procedures (organization_id, intent, locale, title, draft) VALUES ($1, 'refund', 'tr', 'İade', '{}')", org.ID); err != nil {
		t.Fatal(err)
	}
	ai := stepOf(onboarding.StepAI)
	if !condition(ai, onboarding.CondAIEnabled) || condition(ai, onboarding.CondProcedure) || ai.Done {
		t.Fatalf("ai: switch on, draft procedure must not count: %+v", ai)
	}
	if _, err := pool.Exec(ctx, "UPDATE ai_procedures SET published = draft, published_revision = revision WHERE organization_id = $1", org.ID); err != nil {
		t.Fatal(err)
	}
	if !stepOf(onboarding.StepAI).Done {
		t.Fatal("ai: a published procedure must finish the step")
	}
	// automation: an SLA policy and a rule or a workflow.
	if _, err := pool.Exec(ctx, "INSERT INTO sla_policies (organization_id, name) VALUES ($1, 'Standart')", org.ID); err != nil {
		t.Fatal(err)
	}
	auto := stepOf(onboarding.StepAutomation)
	if !condition(auto, onboarding.CondSLAPolicy) || condition(auto, onboarding.CondAutomation) || auto.Done {
		t.Fatalf("automation: policy alone is not enough: %+v", auto)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO workflows (organization_id, name, draft, created_by) VALUES ($1, 'Karşılama', '{}', $2)", org.ID, userID); err != nil {
		t.Fatal(err)
	}
	if !stepOf(onboarding.StepAutomation).Done {
		t.Fatal("automation: a workflow must finish the step")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM workflows WHERE organization_id = $1", org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO automation_rules (organization_id, created_by, name, trigger_event) VALUES ($1, $2, 'Etiketle', 'conversations.created')", org.ID, userID); err != nil {
		t.Fatal(err)
	}
	st, _ = svc.Get(ctx, org.ID)
	if st.Completed != 5 || onboarding.Next(st.Steps) != "" {
		t.Fatalf("all conditions met: %+v", st)
	}
	// Undo the system connection so the skip below has something to skip.
	if _, err := pool.Exec(ctx, "DELETE FROM integration_connections WHERE organization_id = $1", org.ID); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := svc.SkipStep(ctx, org.ID, onboarding.StepSystem); err != nil {
			t.Fatal(err)
		}
	}
	row, err := q.GetOrganizationOnboarding(ctx, org.ID)
	if err != nil || len(row.Steps) != 1 {
		t.Fatalf("skips must be idempotent: %+v %v", row.Steps, err)
	}
	st, _ = svc.Get(ctx, org.ID)
	// Every other step is done, so nothing is next; the skipped step stays
	// listed as a warning.
	if !st.Steps[1].Skipped || st.Skipped != 1 || st.Completed != 4 || onboarding.Next(st.Steps) != "" || len(onboarding.MissingKeys(st.Steps)) != 1 {
		t.Fatalf("skipped system: %+v", st)
	}

	st, err = svc.Finish(ctx, org.ID)
	if err != nil || st.Active || st.FinishedAt == nil {
		t.Fatalf("finish: %+v %v", st, err)
	}
}

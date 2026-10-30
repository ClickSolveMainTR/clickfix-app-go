package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/repository"
	convmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	convh "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workflowEntitlements struct{ denied bool }

func (e *workflowEntitlements) Require(context.Context, int64, string) error {
	if e.denied {
		return &entitlement.Error{Code: entitlement.CodeFeatureRequired}
	}
	return nil
}
func (e *workflowEntitlements) RequireWritable(context.Context, int64) error { return nil }
func (e *workflowEntitlements) CheckLimit(_ context.Context, _ int64, key string, used, delta int64) (entitlement.Decision, error) {
	return entitlement.Check(entitlement.Value{Feature: key, Kind: "limit", Limit: 1000}, used, delta)
}
func TestWorkflowVersionsRecoveryDryRunAndTenantIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	var org, user, contact int64
	var orgUUID, userUUID, conversation uuid.UUID
	suffix := uuid.NewString()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Workflow test',$1) RETURNING id,uuid", suffix).Scan(&org, &orgUUID))
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", user)
	}()
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','Workflow','Owner','active') RETURNING id,uuid", suffix+"@example.test").Scan(&user, &userUUID))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Sample') RETURNING id", org).Scan(&contact))
	must(pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status) VALUES($1,1,'email',$2,'pending') RETURNING uuid", org, contact).Scan(&conversation))
	scope := orgctx.Scope{ID: org, UUID: orgUUID, Status: "active", Permissions: []string{rbac.PermTenantAutomationWrite, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll, rbac.PermTenantConversationsManage}}
	resolver := fixedResolver{scope: scope}
	inbox := convuc.New(convrepo.New(pool, q), nil, nil).WithOutbox(outbox.NewStore(pool, q))
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	must(err)
	mux := http.NewServeMux()
	convmodule.RegisterRoutes(mux, convh.New(inbox, nil), tokens, fixedLoader{principal: authctx.Principal{UserID: userUUID, UserInternal: user}}, resolver)
	auto := &Service{Store: repository.New(q), Conversations: inbox, Bridge: &apibridge.Bridge{Handler: mux, Tokens: tokens}, Resolver: resolver}
	plan := &workflowEntitlements{}
	workflows := &Workflows{Store: &repository.WorkflowStore{Pool: pool, Q: q}, Automation: auto, Entitlements: plan}
	auto.Workflows = workflows
	g := testGraph()
	condition := model.Condition{Field: "priority", Operator: "eq", Value: "urgent"}
	g.Nodes = append(g.Nodes, model.Node{ID: "check", Type: "condition", Condition: &condition})
	g.Edges[1].To = "check"
	g.Edges = append(g.Edges, model.Edge{From: "check", Port: "true", To: "e", Input: "in"}, model.Edge{From: "check", Port: "false", To: "e", Input: "in"})

	row, err := workflows.Save(ctx, org, user, uuid.Nil, model.WorkflowInput{Name: "VIP workflow", Graph: g})
	must(err)
	row, err = workflows.Publish(ctx, org, user, row.Uuid, row.Revision)
	must(err)
	trace, err := workflows.DryRun(ctx, row, user, conversation)
	must(err)
	if len(trace) != 4 || trace[2].Port != "true" {
		t.Fatal(trace)
	}
	count, err := q.WorkflowRunCount(ctx, db.WorkflowRunCountParams{OrganizationID: org, WorkflowID: row.ID})
	must(err)
	if count != 0 {
		t.Fatal("dry run wrote runs")
	}
	event := events.New(events.ConversationCreated).WithOrganization(org).WithEntity("conversation", nil, &conversation)
	must(auto.Consume(ctx, event))
	must(auto.Consume(ctx, event))
	count, err = q.WorkflowRunCount(ctx, db.WorkflowRunCountParams{OrganizationID: org, WorkflowID: row.ID})
	must(err)
	if count != 1 {
		t.Fatal("event idempotency", count)
	}
	runs, err := q.WorkflowRunList(ctx, db.WorkflowRunListParams{OrganizationID: org, WorkflowID: row.ID, Limit: 20})
	must(err)
	oldVersion := runs[0].VersionID
	g.Nodes[1].Action.Value = "low"
	edited, err := workflows.Save(ctx, org, user, row.Uuid, model.WorkflowInput{Name: row.Name, Graph: g, Revision: row.Revision})
	must(err)
	_, err = workflows.Save(ctx, org, user, row.Uuid, model.WorkflowInput{Name: row.Name, Graph: g, Revision: row.Revision})
	if !errors.Is(err, ErrWorkflowConflict) {
		t.Fatal("stale revision", err)
	}
	_, err = workflows.Publish(ctx, org, user, row.Uuid, edited.Revision)
	must(err)
	must(workflows.Sweep(ctx))
	run, err := q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org, Uuid: runs[0].Uuid})
	must(err)
	if run.Status != "succeeded" || run.VersionID != oldVersion {
		t.Fatal("version pinning", run)
	}
	c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	must(err)
	if c.Priority != "urgent" {
		t.Fatal("ran edited draft instead of immutable version", c.Priority)
	}
	steps, err := q.WorkflowSteps(ctx, db.WorkflowStepsParams{OrganizationID: org, RunID: run.ID})
	must(err)
	if len(steps) != 4 {
		t.Fatal(steps)
	}
	must(auto.Consume(ctx, event))
	must(workflows.Sweep(ctx))
	_, err = q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org + 1000000, Uuid: run.Uuid})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-org run", err)
	}
	plan.denied = true
	_, err = workflows.Save(ctx, org, user, uuid.Nil, model.WorkflowInput{Name: "denied", Graph: g})
	if !errors.Is(err, entitlement.ErrDenied) {
		t.Fatal("missing plan gate", err)
	}
	plan.denied = false
	// Crash after starting an action: no automatic replay of an unknown outcome.
	event.EventID = uuid.New()
	must(auto.Consume(ctx, event))
	due, err := q.WorkflowRunDue(ctx)
	must(err)
	if len(due) != 1 {
		t.Fatal(due)
	}
	r := due[0]
	must(workflows.execute(ctx, r))
	r, err = q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org, Uuid: r.Uuid})
	must(err)
	must(q.WorkflowStepStart(ctx, db.WorkflowStepStartParams{OrganizationID: org, RunID: r.ID, StepIndex: r.StepIndex, NodeID: r.NodeID}))
	must(workflows.execute(ctx, r))
	r, err = q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org, Uuid: r.Uuid})
	must(err)
	if r.Status != "failed" {
		t.Fatal("uncertain outcome retried", r)
	}
	// Waits persist their cursor, release the lease and resume on a later Asynq scan.
	waitGraph := model.Graph{SchemaVersion: 1, Nodes: []model.Node{{ID: "t", Type: "trigger", Trigger: "webhook"}, {ID: "w", Type: "wait", DelaySeconds: 60}, {ID: "e", Type: "end"}}, Edges: []model.Edge{{From: "t", Port: "next", To: "w", Input: "in"}, {From: "w", Port: "next", To: "e", Input: "in"}}}
	waiting, err := workflows.Save(ctx, org, user, uuid.Nil, model.WorkflowInput{Name: "Wait", Graph: waitGraph})
	must(err)
	waiting, err = workflows.Publish(ctx, org, user, waiting.Uuid, waiting.Revision)
	must(err)
	versions, err := q.WorkflowMatching(ctx, db.WorkflowMatchingParams{OrganizationID: org, TriggerEvent: "webhook"})
	must(err)
	wr, err := workflows.StartWebhook(ctx, versions[0], uuid.New(), conversation)
	must(err)
	must(workflows.Sweep(ctx))
	wr, err = q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org, Uuid: wr.Uuid})
	must(err)
	if wr.Status != "waiting" || wr.NodeID != "e" || !wr.DueAt.Time.After(time.Now()) {
		t.Fatal("wait not persisted", wr)
	}
	_, err = pool.Exec(ctx, "UPDATE workflow_runs SET due_at=now()-interval '1 second' WHERE organization_id=$1 AND id=$2", org, wr.ID)
	must(err)
	must(workflows.Sweep(ctx))
	wr, err = q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: org, Uuid: wr.Uuid})
	must(err)
	if wr.Status != "succeeded" {
		t.Fatal("wait not resumed", wr)
	}
	routingRule, err := q.InsertRoutingRule(ctx, db.InsertRoutingRuleParams{OrganizationID: org, Name: "Import me", IsActive: true, Conditions: []byte(`{"channel_types":["email"],"priorities":["urgent"],"tag_uuids":[]}`), Skills: []string{"billing"}})
	must(err)
	imported, err := workflows.ImportRouting(ctx, org, user, routingRule.Uuid)
	must(err)
	if imported.Enabled || !imported.PublishedVersion.Valid {
		t.Fatal("import should be published but disabled", imported)
	}
	original, err := q.GetRoutingRuleByUUID(ctx, db.GetRoutingRuleByUUIDParams{OrganizationID: org, Uuid: routingRule.Uuid})
	must(err)
	if !original.IsActive {
		t.Fatal("import changed original rule")
	}

	// Persisted JSON remains an editor graph, not a base64 blob.
	var stored model.Graph
	must(json.Unmarshal(row.Draft, &stored))
	if stored.SchemaVersion != 1 {
		t.Fatal(stored)
	}
}

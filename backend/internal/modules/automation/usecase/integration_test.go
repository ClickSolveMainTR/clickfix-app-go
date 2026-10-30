package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/repository"
	convmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	convh "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type fixedResolver struct{ scope orgctx.Scope }

func (f fixedResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return f.scope, nil
}

type fixedLoader struct{ principal authctx.Principal }

func (f fixedLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return f.principal, nil
}
func TestEngineDryRunOrderingIdempotencyAndLoopTrace(t *testing.T) {
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
	var orgUUID, userUUID, convUUID uuid.UUID
	suffix := uuid.NewString()
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Automation test',$1) RETURNING id,uuid", suffix).Scan(&org, &orgUUID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org)
		if user > 0 {
			_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", user)
		}
	}()
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','Rule','Owner','active') RETURNING id,uuid", suffix+"@example.test").Scan(&user, &userUUID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Test') RETURNING id", org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,custom_fields) VALUES($1,1,'email',$2,'pending','{\"tier\":\"vip\"}') RETURNING uuid", org, contact).Scan(&convUUID); err != nil {
		t.Fatal(err)
	}
	scope := orgctx.Scope{ID: org, UUID: orgUUID, Status: "active", Permissions: []string{rbac.PermTenantAutomationsWrite, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll, rbac.PermTenantConversationsManage, rbac.PermTenantConversationsNote}}
	resolver := fixedResolver{scope: scope}
	conversations := convuc.New(convrepo.New(pool, q), nil, nil).WithOutbox(outbox.NewStore(pool, q))
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	convmodule.RegisterRoutes(mux, convh.New(conversations, nil), tokens, fixedLoader{principal: authctx.Principal{UserID: userUUID, UserInternal: user}}, resolver)
	svc := &Service{Store: repository.New(q), Conversations: conversations, Bridge: &apibridge.Bridge{Handler: mux, Tokens: tokens}, Resolver: resolver}
	rule, err := svc.Save(ctx, org, user, uuid.Nil, model.RuleInput{Name: "VIP pending", Trigger: events.ConversationCreated, Enabled: true, StopOnMatch: true, Conditions: condition(t, `{"all":[{"field":"status","operator":"eq","value":"pending"},{"field":"custom_fields.tier","operator":"eq","value":"vip"}]}`), Actions: []model.Action{{Type: "set_status", Value: "resolved"}, {Type: "add_note", Content: "Rule matched"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Save(ctx, org, user, uuid.Nil, model.RuleInput{Name: "must stop", Trigger: events.ConversationCreated, Position: 1, Enabled: true, Conditions: condition(t, `{"all":[]}`), Actions: []model.Action{{Type: "set_status", Value: "closed"}}})
	if err != nil {
		t.Fatal(err)
	}
	matched, err := svc.DryRun(ctx, rule, convUUID)
	if err != nil || !matched {
		t.Fatal(matched, err)
	}
	n, err := q.CountAutomationExecutions(ctx, org)
	if err != nil || n != 0 {
		t.Fatal("dry run wrote execution", n, err)
	}
	event := events.New(events.ConversationCreated).WithOrganization(org).WithEntity("conversation", nil, &convUUID)
	for range 2 {
		if err = svc.Consume(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	detail, err := conversations.Get(ctx, org, user, convUUID)
	if err != nil || detail.Status != "resolved" {
		t.Fatal("ordering/stop failed", detail.Status, err)
	}
	executions, err := q.ListAutomationExecutions(ctx, db.ListAutomationExecutionsParams{OrganizationID: org, Limit: 100})
	if err != nil || len(executions) != 1 || executions[0].ActionIndex != 2 || executions[0].Status != "succeeded" {
		t.Fatal(executions, err)
	}
	// Even after a new event id, a rule already in the ancestry cannot run again.
	loop := events.New(events.ConversationCreated).WithOrganization(org).WithEntity("conversation", nil, &convUUID).WithPayload(map[string]any{"_automation_trace": []string{rule.Uuid.String(), second.Uuid.String()}})
	if err = svc.Consume(ctx, loop); err != nil {
		t.Fatal(err)
	}
	n, err = q.CountAutomationExecutions(ctx, org)
	if err != nil || n != 1 {
		t.Fatal("loop reexecuted", n, err)
	}
	// Trace persists in the outbox rather than relying on a process-local context.
	var traces int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE payload::text LIKE '%'||$1||'%'", rule.Uuid.String()).Scan(&traces); err != nil || traces < 1 {
		t.Fatal("trace not persisted", traces, err)
	}

	// A successful scheduled scan advances the due timestamp only after all
	// conversations complete, and a second scan does not repeat the note.
	if _, err = pool.Exec(ctx, "UPDATE conversations SET status='pending' WHERE organization_id=$1 AND uuid=$2", org, convUUID); err != nil {
		t.Fatal(err)
	}
	scheduled, err := svc.Save(ctx, org, user, uuid.Nil, model.RuleInput{Name: "scheduled note", Trigger: "scheduled", ScheduleMinutes: 60, Enabled: true, Conditions: condition(t, `{"all":[]}`), Actions: []model.Action{{Type: "add_note", Content: "Scheduled"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	n, err = q.CountAutomationExecutions(ctx, org)
	if err != nil || n != 2 {
		t.Fatal("scheduled scan repeated", n, err)
	}
	next, err := q.GetAutomationRule(ctx, db.GetAutomationRuleParams{OrganizationID: org, Uuid: scheduled.Uuid})
	if err != nil || !next.NextRunAt.Time.After(time.Now()) {
		t.Fatal("scan was not advanced", err)
	}

	// If an earlier action resolves a conversation, a later failure must
	// still be resumed on retry even though active scans exclude it.
	failed, err := svc.Save(ctx, org, user, uuid.Nil, model.RuleInput{Name: "retry resolved", Trigger: "scheduled", ScheduleMinutes: 60, Enabled: true, Conditions: condition(t, `{"all":[]}`), Actions: []model.Action{{Type: "set_status", Value: "resolved"}, {Type: "send_canned_reply", Content: "No channel attached"}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = svc.Sweep(ctx); err == nil {
			t.Fatal("expected missing-channel action failure")
		}
	}
	attempts, err := q.ListAutomationExecutions(ctx, db.ListAutomationExecutionsParams{OrganizationID: org, Limit: 100})
	if err != nil || len(attempts) != 3 || attempts[0].Status != "failed" || attempts[0].ActionIndex != 1 {
		t.Fatal("failed scan did not resume checkpoint", attempts, err)
	}
	due, err := q.GetAutomationRule(ctx, db.GetAutomationRuleParams{OrganizationID: org, Uuid: failed.Uuid})
	if err != nil || !due.NextRunAt.Time.Equal(failed.NextRunAt.Time) {
		t.Fatal("failed scan advanced the schedule", err)
	}
	_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE payload::text LIKE '%'||$1||'%'", failed.Uuid.String())
	_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE payload::text LIKE '%'||$1||'%'", scheduled.Uuid.String())
	_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE payload::text LIKE '%'||$1||'%'", rule.Uuid.String())
}

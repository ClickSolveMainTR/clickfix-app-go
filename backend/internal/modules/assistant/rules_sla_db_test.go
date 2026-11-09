package assistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	automationmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation"
	automationhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/handler"
	automationrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/repository"
	automationuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	convmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	convh "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	slamodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla"
	slahandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/handler"
	slausecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// CF-188: rules and SLA set up from the chat through the real routes.
func TestRulesAndSLAFromTheChat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	var contact int64
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Can') RETURNING id", f.scope.ID).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,priority) VALUES($1,1,'email',$2,'open','urgent')", f.scope.ID, contact); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, stmt := range []string{"DELETE FROM automation_rules WHERE organization_id = $1", "DELETE FROM sla_policies WHERE organization_id = $1",
			"DELETE FROM business_hours WHERE organization_id = $1", "DELETE FROM conversations WHERE organization_id = $1", "DELETE FROM contacts WHERE organization_id = $1"} {
			_, _ = f.pool.Exec(c, stmt, f.scope.ID)
		}
	})
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantAutomationRead, rbac.PermTenantAutomationWrite,
		rbac.PermTenantSlaRead, rbac.PermTenantSlaWrite, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}
	resolver := wfResolver{scope}
	mux := http.NewServeMux()
	inbox := convuc.New(convrepo.New(f.pool, f.q), nil, nil).WithOutbox(outbox.NewStore(f.pool, f.q))
	convmodule.RegisterRoutes(mux, convh.New(inbox, nil), tokens, loader, resolver)
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	auto := &automationuc.Service{Store: automationrepo.New(f.q), Conversations: inbox, Bridge: bridge, Resolver: resolver}
	automationmodule.RegisterRoutes(mux, &automationhandler.Handler{Service: auto}, tokens, loader, resolver)
	slamodule.RegisterRoutes(mux, slahandler.New(slausecase.New(f.pool, f.q, outbox.NewStore(f.pool, f.q), nil), nil), tokens, loader, resolver)
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).
		Register(automationmodule.RuleCapabilities()...).Register(automationmodule.RuleWriteCapabilities()...).
		Register(slamodule.Capabilities()...).Register(slamodule.WriteCapabilities()...).
		RegisterPages(slamodule.Pages()...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/rules"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	propose := func(prompt string, calls ...ai.ToolCall) assistant.ProposedAction {
		t.Helper()
		f.prov.steps = []ai.Result{{Calls: calls}, {Text: `{"reply":"Onayınıza sundum.","suggestions":[]}`}}
		res, err := f.svc.Send(ctx, c, th.UUID, prompt, pc)
		if err != nil || len(res.Reply.Actions) != 1 {
			t.Fatalf("%s: %+v %v", prompt, res.Reply, err)
		}
		return res.Reply.Actions[0]
	}
	confirm := func(a assistant.ProposedAction) assistant.ProposedAction {
		t.Helper()
		out, err := f.svc.Confirm(ctx, c, th.UUID, a.ID, nil)
		if err != nil || out.Actions[0].Status != "done" {
			t.Fatalf("confirm %s: %+v %v", a.Kind, out.Actions, err)
		}
		return out.Actions[0]
	}

	// 1. "Note every new urgent conversation": a rule with a condition.
	card := propose("Acil yeni konuşmalara not düş", ai.ToolCall{ID: "r1", Name: "automations_create", Input: json.RawMessage(
		`{"summary":"Acil konuşmalara not","params":{"name":"Acil not","trigger":"conversation.created","enabled":true,` +
			`"conditions":{"all":[{"field":"priority","operator":"eq","value":"urgent"}]},"actions":[{"type":"add_note","content":"Acil: hemen ilgilenin."}]}}`)})
	if pv, err := f.svc.DryRun(ctx, c, th.UUID, card.ID, nil); err != nil || !pv.Preview.OK {
		t.Fatalf("dry run %+v %v", pv.Preview, err)
	}
	done := confirm(card)
	if done.Verification == nil || !strings.Contains(done.Verification.Message, "sağlanıyor") {
		t.Fatalf("simulation %+v", done.Verification)
	}
	ruleID := done.Result
	var enabled bool
	var conditions string
	if err := f.pool.QueryRow(ctx, "SELECT enabled, conditions::text FROM automation_rules WHERE organization_id=$1 AND uuid=$2", f.scope.ID, ruleID).Scan(&enabled, &conditions); err != nil ||
		!enabled || !strings.Contains(conditions, "urgent") {
		t.Fatalf("rule %v %s %v", enabled, conditions, err)
	}
	// A rule without a condition is sent as {"all":[]} and accepted.
	confirm(propose("Her yeni mesajda öncelik normal olsun", ai.ToolCall{ID: "r2", Name: "automations_create", Input: json.RawMessage(
		`{"params":{"name":"Varsayılan öncelik","trigger":"message.received","actions":[{"type":"set_priority","value":"normal"}]}}`)}))
	// Pause the first one (update), then delete it (destructive).
	confirm(propose("Acil not kuralını kapat", ai.ToolCall{ID: "u1", Name: "automations_update", Input: json.RawMessage(
		`{"params":{"rule_uuid":"` + ruleID + `","enabled":false}}`)}))
	_ = f.pool.QueryRow(ctx, "SELECT enabled, conditions::text FROM automation_rules WHERE organization_id=$1 AND uuid=$2", f.scope.ID, ruleID).Scan(&enabled, &conditions)
	if enabled || !strings.Contains(conditions, "urgent") {
		t.Fatalf("update lost fields: enabled=%v %s", enabled, conditions)
	}
	del := propose("Acil not kuralını sil", ai.ToolCall{ID: "d1", Name: "automations_delete", Input: json.RawMessage(`{"params":{"rule_uuid":"` + ruleID + `"}}`)})
	if del.Risk != "destructive" {
		t.Fatalf("delete risk %q", del.Risk)
	}
	if d := confirm(del); d.Verification == nil || d.Verification.Status != "ok" {
		t.Fatalf("delete verify %+v", d.Verification)
	}

	// 2. SLA: business hours and a policy; the page state then has no gap.
	pc = assistant.Context{Surface: "web", Route: "/settings/sla"}
	if st, ok, _ := f.svc.PageState(ctx, c, pc); !ok || len(st.Missing) != 2 || st.Missing[0].Capability != "sla.business_hours.create" {
		t.Fatalf("sla page %+v", st)
	}
	confirm(propose("Hafta içi 9-18 çalışıyoruz", ai.ToolCall{ID: "h1", Name: "sla_business_hours_create", Input: json.RawMessage(
		`{"params":{"name":"Mesai","timezone":"Europe/Istanbul","is_default":true,"days":{"mon":{"open":"09:00","close":"18:00"},"tue":{"open":"09:00","close":"18:00"},"sat":{"closed":true}}}}`)}))
	pol := propose("Acil talepler 30 dakikada yanıtlansın", ai.ToolCall{ID: "p1", Name: "sla_policies_create", Input: json.RawMessage(
		`{"params":{"name":"Standart","description":"Ana politika","priorities":["urgent","normal"],"targets":[{"priority":"urgent","first_response_minutes":30,"resolution_minutes":240},{"priority":"normal","first_response_minutes":240}]}}`)})
	if pv, err := f.svc.DryRun(ctx, c, th.UUID, pol.ID, nil); err != nil || len(pv.Preview.Details) != 2 || !strings.Contains(pv.Preview.Details[0].Value, "30 dk") {
		t.Fatalf("policy preview %+v %v", pv.Preview, err)
	}
	if d := confirm(pol); d.Verification == nil || d.Verification.Status != "ok" {
		t.Fatalf("policy verify %+v", d.Verification)
	}
	if st, _, _ := f.svc.PageState(ctx, c, pc); len(st.Missing) != 0 || len(st.Tests) != 1 {
		t.Fatalf("sla page after %+v", st)
	}
	// Updates change only what they give (fetch-then-merge).
	var hoursID, policyID string
	_ = f.pool.QueryRow(ctx, "SELECT uuid::text FROM business_hours WHERE organization_id = $1", f.scope.ID).Scan(&hoursID)
	_ = f.pool.QueryRow(ctx, "SELECT uuid::text FROM sla_policies WHERE organization_id = $1", f.scope.ID).Scan(&policyID)
	hu := propose("Cumartesi 10-14 açık olsun", ai.ToolCall{ID: "hu", Name: "sla_business_hours_update", Input: json.RawMessage(
		`{"params":{"business_hours_uuid":"` + hoursID + `","days":{"sat":{"open":"10:00","close":"14:00"}}}}`)})
	if pv, err := f.svc.DryRun(ctx, c, th.UUID, hu.ID, nil); err != nil || !pv.Preview.OK || !strings.Contains(pv.Preview.Summary, "Mesai") {
		t.Fatalf("hours update preview %+v %v", pv.Preview, err)
	}
	confirm(hu)
	var hName, tz, week string
	var def bool
	if err := f.pool.QueryRow(ctx, "SELECT name, timezone, is_default, working_hours::text FROM business_hours WHERE organization_id = $1", f.scope.ID).
		Scan(&hName, &tz, &def, &week); err != nil || hName != "Mesai" || tz != "Europe/Istanbul" || !def ||
		!strings.Contains(week, "10:00") || !strings.Contains(week, "09:00") {
		t.Fatalf("hours lost fields: %s %s %v %s %v", hName, tz, def, week, err)
	}
	confirm(propose("Acil çözüm süresi 2 saat olsun", ai.ToolCall{ID: "pu", Name: "sla_policies_update", Input: json.RawMessage(
		`{"params":{"policy_uuid":"` + policyID + `","targets":[{"priority":"urgent","resolution_minutes":120}]}}`)}))
	var desc, cond, targets string
	if err := f.pool.QueryRow(ctx, "SELECT description, conditions::text FROM sla_policies WHERE organization_id = $1", f.scope.ID).Scan(&desc, &cond); err != nil ||
		desc != "Ana politika" || !strings.Contains(cond, "urgent") {
		t.Fatalf("policy lost fields: %q %s %v", desc, cond, err)
	}
	_ = f.pool.QueryRow(ctx, `SELECT string_agg(priority||':'||coalesce(first_response_minutes,0)||'/'||coalesce(resolution_minutes,0), ',' ORDER BY priority)
		FROM sla_policy_targets t JOIN sla_policies p ON p.id = t.policy_id WHERE p.organization_id = $1`, f.scope.ID).Scan(&targets)
	if targets != "normal:240/0,urgent:30/120" {
		t.Fatalf("targets %q", targets)
	}
	// A duplicate name is caught by the dry run.
	dup := propose("Bir Standart daha", ai.ToolCall{ID: "p2", Name: "sla_policies_create", Input: json.RawMessage(
		`{"params":{"name":"standart","targets":[{"priority":"low","resolution_minutes":2880}]}}`)})
	if pv, _ := f.svc.DryRun(ctx, c, th.UUID, dup.ID, nil); pv.Preview == nil || pv.Preview.OK {
		t.Fatalf("duplicate preview %+v", pv.Preview)
	}
	// Bad hours never reach the API.
	f.prov.steps = []ai.Result{{Calls: []ai.ToolCall{{ID: "h2", Name: "sla_business_hours_create", Input: json.RawMessage(`{"params":{"days":{"mon":{"open":"18:00","close":"09:00"}}}}`)}}},
		{Text: `{"reply":"Saatler hatalı.","suggestions":[]}`}}
	if res, _ := f.svc.Send(ctx, c, th.UUID, "Pazartesi 18-9", pc); len(res.Reply.Actions) != 0 {
		t.Fatal("invalid hours proposed")
	}
}

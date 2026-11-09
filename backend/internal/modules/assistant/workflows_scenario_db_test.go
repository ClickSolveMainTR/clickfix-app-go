package assistant_test

import (
	"context"
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
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

type wfResolver struct{ scope orgctx.Scope }

func (r wfResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return r.scope, nil
}

type wfLoader struct{ p authctx.Principal }

func (l wfLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return l.p, nil
}

type openPlan struct{}

func (openPlan) Require(context.Context, int64, string) error { return nil }
func (openPlan) RequireWritable(context.Context, int64) error { return nil }
func (openPlan) CheckLimit(_ context.Context, _ int64, key string, used, delta int64) (entitlement.Decision, error) {
	return entitlement.Check(entitlement.Value{Feature: key, Kind: "limit", Limit: 1000}, used, delta)
}

func TestScenarioWorkflowFromSentence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	var contact int64
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Ayşe') RETURNING id", f.scope.ID).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,subject) VALUES($1,1,'email',$2,'open','iade talebi')", f.scope.ID, contact); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, stmt := range []string{"DELETE FROM workflows WHERE organization_id = $1", "DELETE FROM conversations WHERE organization_id = $1", "DELETE FROM contacts WHERE organization_id = $1"} {
			_, _ = f.pool.Exec(c, stmt, f.scope.ID)
		}
	})

	// The real conversation and workflow routes behind the /v1 bridge.
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantAutomationRead, rbac.PermTenantAutomationWrite,
		rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll, rbac.PermTenantConversationsManage}
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
	auto.Workflows = &automationuc.Workflows{Store: &automationrepo.WorkflowStore{Pool: f.pool, Q: f.q}, Automation: auto, Entitlements: openPlan{}}
	automationmodule.RegisterRoutes(mux, &automationhandler.Handler{Service: auto}, tokens, loader, resolver)
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(automationmodule.Capabilities()...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/workflows"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)

	// "On every new message, if the subject mentions a refund, raise the
	//  priority and leave a note": the assistant reads the reference and
	//  drafts the workflow as a card.
	draft := `{"summary":"İade içeren yeni mesajlarda öncelik yükselir ve not eklenir","params":{"name":"İade talepleri","trigger":"message.received","enable":true,` +
		`"condition":{"all":[{"field":"subject","operator":"contains","value":"iade"}]},` +
		`"actions":[{"type":"set_priority","value":"high"},{"type":"add_note","content":"İade talebi: muhasebeye bilgi verin."}]}}`
	f.prov.steps = []ai.Result{
		call("r1", "workflows_reference", `{}`),
		call("d1", "workflows_create", draft),
		{Text: `{"reply":"İş akışını taslak olarak hazırladım.","suggestions":["Onayla"]}`},
	}
	res, err := f.svc.Send(ctx, c, th.UUID, "Her yeni mesajda konu 'iade' içeriyorsa önceliği yüksek yap ve iç not ekle", pc)
	if err != nil || len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != "workflows.create" {
		t.Fatalf("draft %+v %v", res.Reply, err)
	}
	if tool := toolTexts(f.prov.reqs[1]); !strings.Contains(tool, "message.received") || !strings.Contains(tool, "set_priority") {
		t.Fatalf("reference: %q", tool)
	}
	card := res.Reply.Actions[0]
	var n int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM workflows WHERE organization_id = $1", f.scope.ID).Scan(&n)
	if n != 0 {
		t.Fatal("drafting saved a workflow")
	}

	// Dry run before saving: the graph is validated locally, nothing saved.
	pv, err := f.svc.DryRun(ctx, c, th.UUID, card.ID, nil)
	if err != nil || !pv.Preview.OK || len(pv.Preview.Details) != 4 {
		t.Fatalf("dry run %+v %v", pv.Preview, err)
	}
	// An edit that breaks the workflow is refused at confirm.
	if _, err := f.svc.Confirm(ctx, c, th.UUID, card.ID, capability.Params{"actions": []any{map[string]any{"type": "set_status", "value": "snoozed"}}}); err == nil {
		t.Fatal("invalid edited draft accepted")
	}
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM workflows WHERE organization_id = $1", f.scope.ID).Scan(&n)
	if n != 0 {
		t.Fatal("invalid draft saved")
	}

	// Confirm: saved, published, enabled; verified by a simulation on the
	// latest conversation (the condition holds, both actions would run).
	out, err := f.svc.Confirm(ctx, c, th.UUID, card.ID, capability.Params{"name": "İade talepleri (asistan)"})
	if err != nil {
		t.Fatal(err)
	}
	done := out.Actions[0]
	if done.Status != "done" || done.Verification == nil || done.Verification.Status != "ok" || !strings.Contains(done.Verification.Message, "2 eylem") {
		t.Fatalf("confirm %+v %+v", done, done.Verification)
	}
	var name string
	var enabled bool
	var published *int32
	if err := f.pool.QueryRow(ctx, "SELECT name, enabled, published_version FROM workflows WHERE organization_id = $1 AND uuid = $2", f.scope.ID, done.Result).Scan(&name, &enabled, &published); err != nil ||
		name != "İade talepleri (asistan)" || !enabled || published == nil {
		t.Fatalf("workflow %s enabled=%v published=%v %v", name, enabled, published, err)
	}
	var runs int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM workflow_runs WHERE organization_id = $1", f.scope.ID).Scan(&runs)
	var priority string
	_ = f.pool.QueryRow(ctx, "SELECT priority FROM conversations WHERE organization_id = $1", f.scope.ID).Scan(&priority)
	if runs != 0 || priority == "high" {
		t.Fatalf("the simulation ran the workflow: runs=%d priority=%s", runs, priority)
	}
	if done.Focus == nil || done.Focus.Route != "/workflows/"+done.Result {
		t.Fatalf("focus %+v", done.Focus)
	}

	// Later: "dry-run it" is a read tool (simulation only) and "turn it
	// off" is a confirmed card.
	f.prov.steps = []ai.Result{
		call("s1", "workflows_dry_run", `{"workflow_uuid":"`+done.Result+`"}`),
		call("o1", "workflows_enable", `{"summary":"Kapatılır","params":{"workflow_uuid":"`+done.Result+`","enabled":false}}`),
		{Text: `{"reply":"Simülasyon tamam; kapatmayı onayınıza sundum.","suggestions":[]}`},
	}
	res, err = f.svc.Send(ctx, c, th.UUID, "Bir daha dene, sonra kapat", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("%+v %v", res.Reply, err)
	}
	if tool := toolTexts(f.prov.reqs[len(f.prov.reqs)-2]); !strings.Contains(tool, "would_execute") {
		t.Fatalf("dry run tool: %q", tool)
	}
	if _, err := f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	_ = f.pool.QueryRow(ctx, "SELECT enabled FROM workflows WHERE organization_id = $1", f.scope.ID).Scan(&enabled)
	if enabled {
		t.Fatal("not turned off")
	}
}

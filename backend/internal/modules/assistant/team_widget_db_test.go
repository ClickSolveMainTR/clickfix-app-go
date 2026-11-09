package assistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	teamsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams"
	teamshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/handler"
	teamsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/usecase"
	widgetmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget"
	widgethandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	widgetusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	widgetchannel "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// CF-189: teams and the web widget set up from the chat (real routes).
func TestTeamsAndWidgetFromTheChat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, stmt := range []string{"DELETE FROM teams WHERE organization_id = $1", "DELETE FROM channel_instances WHERE organization_id = $1"} {
			_, _ = f.pool.Exec(c, stmt, f.scope.ID)
		}
	})
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantTeamsRead, rbac.PermTenantTeamsWrite,
		rbac.PermTenantChannelsRead, rbac.PermTenantChannelsManage, rbac.PermTenantWidgetsPublish}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}
	resolver := wfResolver{scope}
	mux := http.NewServeMux()
	teamsmodule.RegisterRoutes(mux, teamshandler.New(teamsusecase.New(f.pool, f.q), nil), tokens, loader, resolver)
	box, _ := crypto.NewSecretBox(strings.Repeat("k", 32))
	vault, _ := channels.NewVault(box)
	chSvc := channelsusecase.New(f.q, channels.NewRegistry(widgetchannel.New(nil, nil)), vault)
	channelsmodule.RegisterRoutes(mux, channelshandler.New(chSvc, nil, nil, nil), tokens, loader, resolver)
	widgetmodule.RegisterConfigRoutes(mux, widgethandler.NewConfig(widgetusecase.NewConfigs(f.q, convrepo.New(f.pool, f.q), chSvc), nil), tokens, loader, resolver, openPlan{})
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).
		Register(teamsmodule.Capabilities()...).Register(widgetmodule.Capabilities("https://app.clickfix.example")...).
		RegisterPages(assistant.CorePages(f.q)...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/widget"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	run := func(prompt string, calls ...ai.ToolCall) []assistant.ProposedAction {
		t.Helper()
		f.prov.steps = []ai.Result{{Calls: calls}, {Text: `{"reply":"Tamam.","suggestions":[]}`}}
		res, err := f.svc.Send(ctx, c, th.UUID, prompt, pc)
		if err != nil {
			t.Fatal(err)
		}
		return res.Reply.Actions
	}
	confirm := func(a assistant.ProposedAction) assistant.ProposedAction {
		t.Helper()
		out, err := f.svc.Confirm(ctx, c, th.UUID, a.ID, nil)
		if err != nil || out.Actions[0].Status != "done" {
			t.Fatalf("confirm %s: %+v %v", a.Kind, out.Actions, err)
		}
		return out.Actions[0]
	}

	// The widget page says what is missing and links the capability.
	if st, ok, _ := f.svc.PageState(ctx, c, pc); !ok || len(st.Missing) != 1 || st.Missing[0].Capability != "widget.create" {
		t.Fatalf("widget page %+v", st)
	}
	// Create the widget: the card gives the install code.
	acts := run("Siteme sohbet widget'ı ekle", ai.ToolCall{ID: "w1", Name: "widget_create", Input: json.RawMessage(`{"params":{"name":"Web sitesi"}}`)})
	done := confirm(acts[0])
	widgetID := done.Result
	if len(done.Details) == 0 || !strings.Contains(done.Details[0].Code, `data-widget="`+widgetID+`"`) || !strings.Contains(done.Details[0].Code, "https://app.clickfix.example/widget.js") {
		t.Fatalf("install code %+v", done.Details)
	}
	// Change the colour and greeting as a draft, then publish.
	acts = run("Rengi lacivert yap", ai.ToolCall{ID: "w2", Name: "widget_update", Input: json.RawMessage(
		`{"params":{"widget_uuid":"` + widgetID + `","color":"#1F2A44","greeting_title":"Merhaba!","allowed_domains":["shop.example.com"]}}`)})
	if pv, err := f.svc.DryRun(ctx, c, th.UUID, acts[0].ID, nil); err != nil || !pv.Preview.OK || !strings.Contains(pv.Preview.Summary, "taslak") {
		t.Fatalf("widget preview %+v %v", pv.Preview, err)
	}
	confirm(acts[0])
	var draft string
	if err := f.pool.QueryRow(ctx, "SELECT settings::text FROM widget_drafts WHERE organization_id = $1", f.scope.ID).Scan(&draft); err != nil || !strings.Contains(strings.ToLower(draft), "#1f2a44") {
		t.Fatalf("draft %s %v", draft, err)
	}
	acts = run("Yayınla", ai.ToolCall{ID: "w3", Name: "widget_update", Input: json.RawMessage(`{"params":{"widget_uuid":"` + widgetID + `","greeting_text":"Size nasıl yardımcı olabiliriz?","publish":true}}`)})
	confirm(acts[0])
	var versions int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM widget_versions WHERE organization_id = $1", f.scope.ID).Scan(&versions)
	if versions == 0 {
		t.Fatal("not published")
	}
	// A domain with a path never reaches the API.
	if acts := run("Alan adı ekle", ai.ToolCall{ID: "w4", Name: "widget_update", Input: json.RawMessage(`{"params":{"widget_uuid":"` + widgetID + `","allowed_domains":["https://evil.example/x"]}}`)}); len(acts) != 0 {
		t.Fatal("bad domain proposed")
	}

	// Teams: create one and put the user in it.
	acts = run("Fatura ekibi kur", ai.ToolCall{ID: "t1", Name: "teams_create", Input: json.RawMessage(`{"params":{"name":"Fatura"}}`)})
	team := confirm(acts[0]).Result
	acts = run("Beni ekibe ekle", ai.ToolCall{ID: "t2", Name: "teams_add_member", Input: json.RawMessage(`{"params":{"team_uuid":"` + team + `","user_uuid":"` + userUUID.String() + `","role":"lead"}}`)})
	confirm(acts[0])
	var role string
	if err := f.pool.QueryRow(ctx, "SELECT tm.role FROM team_members tm JOIN teams t ON t.id = tm.team_id WHERE t.uuid = $1", team).Scan(&role); err != nil || role != "lead" {
		t.Fatalf("membership %q %v", role, err)
	}
	// The duplicate name is caught by the dry run.
	acts = run("Bir Fatura ekibi daha", ai.ToolCall{ID: "t3", Name: "teams_create", Input: json.RawMessage(`{"params":{"name":"fatura"}}`)})
	if pv, _ := f.svc.DryRun(ctx, c, th.UUID, acts[0].ID, nil); pv.Preview == nil || pv.Preview.OK {
		t.Fatalf("duplicate team %+v", pv.Preview)
	}
	// ...and by the confirm itself, without a dry run.
	if out, err := f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil); err != nil || out.Actions[0].Status != "failed" {
		t.Fatalf("duplicate team confirm %+v %v", out.Actions, err)
	}
	// An empty domain list (= every site) is never proposed.
	if acts := run("Alan adı kısıtını kaldır", ai.ToolCall{ID: "w5", Name: "widget_update", Input: json.RawMessage(`{"params":{"widget_uuid":"` + widgetID + `","allowed_domains":[]}}`)}); len(acts) != 0 {
		t.Fatal("empty domain list proposed")
	}
}

// policyAPI answers the AI policy and invitation routes and records writes.
type policyAPI struct {
	mu    sync.Mutex
	calls []string
}

func (a *policyAPI) Call(_ context.Context, method, path string, body any) (capability.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw, _ := json.Marshal(body)
	a.calls = append(a.calls, method+" "+path+" "+string(raw))
	if method == http.MethodGet && path == "/v1/tenant/ai/policy" {
		return capability.Response{OK: true, Data: json.RawMessage(`{"enabled":true,"mask_pii":true,"allow_unmask":false,"daily_calls":500,"concurrency":4,"daily_budget_micros":9000000}`)}, nil
	}
	return capability.Response{OK: true, Status: 200, Data: json.RawMessage(`{}`)}, nil
}

// echoProvider answers like a real model: it repeats the e-mail
// placeholder it saw in the user's message ({{EMAIL}} in a scripted call).
type echoProvider struct{ *scripted }

var placeholderRxp = regexp.MustCompile(`\[PII_EMAIL_[a-z]+_\d+\]`)

func (p echoProvider) Generate(ctx context.Context, req ai.Request) (ai.Result, error) {
	raw, _ := json.Marshal(req.Turns)
	token := placeholderRxp.FindString(string(raw))
	res, err := p.scripted.Generate(ctx, req)
	for i := range res.Calls {
		res.Calls[i].Input = json.RawMessage(strings.ReplaceAll(string(res.Calls[i].Input), "{{EMAIL}}", token))
	}
	return res, err
}

func (p echoProvider) Stream(ctx context.Context, req ai.Request, _ ai.Delta) (ai.Result, error) {
	return p.Generate(ctx, req)
}

// Invitations and turning AI off keep the rest of the settings.
func TestInviteAndAIDisable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	api := &policyAPI{}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(assistant.TeamAndAICapabilities(f.q)...)
	f.svc.API = func(uuid.UUID, uuid.UUID) capability.API { return api }
	route := f.svc.AI.Routes["reply"]
	route.Provider = echoProvider{f.prov}
	f.svc.AI.Routes["reply"] = route
	c := f.caller(rbac.PermTenantMembersWrite, rbac.PermTenantSettingsWrite)
	pc := assistant.Context{Surface: "web", Route: "/settings/members"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{{Calls: []ai.ToolCall{
		{ID: "i1", Name: "members_invite", Input: json.RawMessage(`{"params":{"email":"{{EMAIL}}","role":"supervisor"}}`)},
		{ID: "a1", Name: "ai_disable", Input: json.RawMessage(`{"params":{}}`)},
	}}, {Text: `{"reply":"İkisini de onayınıza sundum.","suggestions":[]}`}}
	res, err := f.svc.Send(ctx, c, th.UUID, "zeynep@acme.example adresini süpervizör olarak davet et ve AI'ı kapat", pc)
	if err != nil || len(res.Reply.Actions) != 2 {
		t.Fatalf("%+v %v", res.Reply, err)
	}
	// The e-mail placeholder of the user's message is restored on the card.
	if res.Reply.Actions[0].Params["email"] != "zeynep@acme.example" {
		t.Fatalf("email %+v", res.Reply.Actions[0].Params)
	}
	for _, a := range res.Reply.Actions {
		if _, err := f.svc.Confirm(ctx, c, th.UUID, a.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	all := strings.Join(api.calls, "\n")
	if !strings.Contains(all, `POST /v1/tenant/invitations {"email":"zeynep@acme.example","role_uuids":["`) {
		t.Fatalf("invitation call:\n%s", all)
	}
	if !strings.Contains(all, `PUT /v1/tenant/ai/policy {"allow_unmask":false,"concurrency":4,"daily_budget_micros":9000000,"daily_calls":500,"enabled":false,"mask_pii":true}`) {
		t.Fatalf("policy call:\n%s", all)
	}
}

package assistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	widgetmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget"
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

func toolNames(req ai.Request) []string {
	out := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		out = append(out, t.Name)
	}
	return out
}

// CF-193: the support widget surface offers only its allowlist and
// identifies the member with a server-side signature.
func TestWidgetSurface(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM channel_instances WHERE organization_id = $1", f.scope.ID)
	})
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantChannelsRead, rbac.PermTenantChannelsManage,
		rbac.PermTenantWidgetsPublish, rbac.PermTenantAiWrite}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	box, _ := crypto.NewSecretBox(strings.Repeat("k", 32))
	vault, _ := channels.NewVault(box)
	chSvc := channelsusecase.New(f.q, channels.NewRegistry(widgetchannel.New(nil, nil)), vault)
	channelsmodule.RegisterRoutes(mux, channelshandler.New(chSvc, nil, nil, nil), tokens, wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}, wfResolver{scope})
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).
		Register(channelsmodule.Capabilities()...).Register(widgetmodule.Capabilities("https://app.clickfix.example")...).
		AllowOn(capability.SurfaceWidget, assistant.WidgetAllowed)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	f.svc.Instances = chSvc

	// 1. The allowlist: reads, navigation and small confirmed set-ups;
	// never secret-holding or broad writes, never the legacy actions.
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "widget", Route: "/channels"}
	th, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	if err != nil {
		t.Fatal(err)
	}
	f.prov.reqs = nil
	f.prov.steps = []ai.Result{{Calls: []ai.ToolCall{{ID: "a1", Name: "channels_api_create", Input: json.RawMessage(`{"params":{"name":"X","webhook_url":"https://shop.example.com/x"}}`)}}},
		{Text: `{"reply":"Bunu panelden yapabilirsiniz.","suggestions":[]}`}}
	res, err := f.svc.Send(ctx, c, th.UUID, "REST API kanalı kur", pc)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Reply.Actions) != 0 {
		t.Fatalf("a capability off the allowlist was proposed: %+v", res.Reply.Actions)
	}
	names := strings.Join(toolNames(f.prov.reqs[0]), ",")
	for _, want := range []string{"channels_list", "channels_test", "navigation_open_route", "channels_connect", "ai_enable", assistant.ToolSearchDocs} {
		if !strings.Contains(names, want) {
			t.Fatalf("widget tools miss %s: %s", want, names)
		}
	}
	for _, banned := range []string{"channels_api_create", "channels_api_rotate_secret", "widget_create", assistant.ToolProposeAction, assistant.ToolEscalate} {
		if strings.Contains(","+names+",", ","+banned+",") {
			t.Fatalf("widget tools hold %s: %s", banned, names)
		}
	}
	if sys := f.prov.reqs[0].Instructions; !strings.Contains(sys, "support widget") || strings.Contains(sys, "propose_action") {
		t.Fatal("widget system prompt")
	}
	// The same thread on the web surface still gets everything.
	f.prov.reqs, f.prov.steps = nil, nil
	if _, err := f.svc.Send(ctx, c, th.UUID, "Merhaba", assistant.Context{Surface: "web", Route: "/channels"}); err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(toolNames(f.prov.reqs[0]), ","); !strings.Contains(names, "channels_api_create") {
		t.Fatalf("web tools %s", names)
	}
	// Navigation is proposed and confirmed on the widget surface.
	f.prov.steps = []ai.Result{{Calls: []ai.ToolCall{{ID: "n1", Name: "navigation_open_route", Input: json.RawMessage(`{"params":{"route":"/settings/sla"}}`)}}},
		{Text: `{"reply":"Sizi götürebilirim.","suggestions":[]}`}}
	res, err = f.svc.Send(ctx, c, th.UUID, "SLA nerede?", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("navigation %+v %v", res.Reply, err)
	}
	if out, err := f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil); err != nil || out.Actions[0].Status != "done" {
		t.Fatalf("navigation confirm %+v %v", out, err)
	}
	// An unknown surface is refused.
	if _, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Surface: "kiosk"}); err == nil {
		t.Fatal("unknown surface accepted")
	}

	// 2. Support widget identity: unavailable until the support
	// organization has a widget with identity verification.
	if sw, err := f.svc.SupportWidgetFor(ctx, scope, userUUID); err != nil || sw.Available {
		t.Fatalf("no support org %+v %v", sw, err)
	}
	if _, err := f.q.AssistantSettingsSave(ctx, db.AssistantSettingsSaveParams{Enabled: true, ModelRoute: "reply", DailyLimitPerOrg: 100,
		SupportOrgUuid: pgtype.UUID{Bytes: f.scope.UUID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if sw, _ := f.svc.SupportWidgetFor(ctx, scope, userUUID); sw.Available {
		t.Fatal("available without a widget")
	}
	secret := strings.Repeat("s", 32)
	view, err := chSvc.Create(ctx, channelsusecase.CreateInput{OrganizationID: f.scope.ID, ActorID: f.user, Type: channels.TypeWidget, Name: "Destek",
		Credentials: channels.Credentials{widgetchannel.IdentitySecretField: secret}})
	if err != nil {
		t.Fatal(err)
	}
	sw, err := f.svc.SupportWidgetFor(ctx, scope, userUUID)
	if err != nil || !sw.Available || sw.WidgetID != view.UUID.String() {
		t.Fatalf("support widget %+v %v", sw, err)
	}
	if !strings.Contains(sw.UserID, f.scope.UUID.String()) || !strings.Contains(sw.UserID, userUUID.String()) ||
		!widgetchannel.VerifyUserHash(secret, sw.UserID, sw.UserHash) {
		t.Fatalf("identity %+v", sw)
	}
	raw, _ := json.Marshal(sw)
	if strings.Contains(string(raw), secret) {
		t.Fatal("identity secret leaked")
	}
}

// The allowlist predicate itself.
func TestWidgetAllowed(t *testing.T) {
	run := func(context.Context, capability.Exec, capability.Params) (capability.Result, error) {
		return capability.Result{}, nil
	}
	apply := run
	cases := []struct {
		c    *capability.Capability
		want bool
	}{
		{&capability.Capability{Name: "x.list", Kind: capability.Read, Run: run}, true},
		{&capability.Capability{Name: "x.mcp", Kind: capability.Read, Run: run, Surfaces: []string{capability.SurfaceMCP}}, false},
		{&capability.Capability{Name: "channels.test", Kind: capability.Write, Apply: apply}, true},
		{&capability.Capability{Name: "x.create", Kind: capability.Write, Apply: apply}, false},
		{&capability.Capability{Name: "ai.enable", Kind: capability.Destructive, Apply: apply}, false},
	}
	for _, tc := range cases {
		if got := assistant.WidgetAllowed(tc.c); got != tc.want {
			t.Errorf("%s: %v", tc.c.Name, got)
		}
	}
}

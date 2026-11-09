package assistant_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	webhooksmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks"
	webhookshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/handler"
	webhooksmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/model"
	webhooksuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// endpoint answers every webhook delivery with 204 and records it.
type endpoint struct{ bodies []string }

func (e *endpoint) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	e.bodies = append(e.bodies, string(b))
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
}

// Scenario 3: on the integrations page the assistant says what needs
// attention and runs a webhook test delivery from the chat.
func TestScenarioPageStateAndTests(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM outbound_webhooks WHERE organization_id = $1", f.scope.ID)
	})
	box, err := crypto.NewSecretBox(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	hooks := webhooksuc.New(f.q, box)
	receiver := &endpoint{}
	hooks.Client = &http.Client{Transport: receiver}
	event := events.WebhookEvents()[0]
	saved, err := hooks.Save(ctx, f.scope.ID, uuid.Nil, webhooksmodel.Input{Name: "ERP", URL: "https://erp.example.com/hooks", Events: []string{event}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	hookID := saved["uuid"].(uuid.UUID)

	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantWebhooksRead, rbac.PermTenantWebhooksWrite}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	webhooksmodule.RegisterRoutes(mux, &webhookshandler.Handler{Service: hooks}, tokens,
		wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}, wfResolver{scope})
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).
		Register(webhooksmodule.Capabilities()...).
		RegisterPages(assistant.CorePages(f.q)...).
		RegisterPages(webhooksmodule.Pages()...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/settings/integrations"}

	// The page state: the webhook can be tested.
	st, ok, err := f.svc.PageState(ctx, c, pc)
	if err != nil || !ok || st.Module != "webhooks" || len(st.Tests) != 1 || st.Tests[0].Capability != "webhooks.test" || !strings.Contains(st.Tests[0].Title, "ERP") {
		t.Fatalf("page state %+v %v %v", st, ok, err)
	}
	// A viewer sees the page but no test they could not run.
	viewer := c
	viewer.Scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantWebhooksRead}
	if st, _, _ := f.svc.PageState(ctx, viewer, pc); len(st.Tests) != 0 {
		t.Fatalf("viewer tests %+v", st.Tests)
	}
	// Pages of the assistant's own settings come from the set-up snapshot.
	if st, ok, _ := f.svc.PageState(ctx, c, assistant.Context{Surface: "web", Route: "/settings/ai"}); !ok || len(st.Missing) == 0 || st.Missing[0].Key != "enable_ai" || st.Missing[0].Capability != "" {
		t.Fatalf("ai page %+v", st)
	}
	if _, ok, _ := f.svc.PageState(ctx, c, assistant.Context{Surface: "web", Route: "/inbox"}); ok {
		t.Fatal("a page without a provider has a state")
	}

	// HTTP: the drawer reads it.
	h := assistant.NewHandler(f.svc, nil, nil)
	r := httptest.NewRequest("GET", "/v1/tenant/assistant/page-state?surface=web&route=/settings/integrations", nil)
	r = r.WithContext(authctx.WithPrincipal(orgctx.WithScope(r.Context(), scope), authctx.Principal{UserInternal: f.user}))
	w := httptest.NewRecorder()
	h.PageState(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"webhooks.test"`) {
		t.Fatalf("page-state %d %s", w.Code, w.Body)
	}

	// "What can I test here?": the state is in the context, the model
	// proposes the test delivery, the user confirms, the event is queued.
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{
		call("l1", "webhooks_list", `{}`),
		call("t1", "webhooks_test", `{"summary":"ERP'ye test olayı","params":{"webhook_uuid":"`+hookID.String()+`"}}`),
		{Text: `{"reply":"ERP webhook'una test olayı göndermeyi onayınıza sundum.","suggestions":[]}`},
	}
	res, err := f.svc.Send(ctx, c, th.UUID, "Bu sayfada neyi test edebilirim?", pc)
	if err != nil || len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != "webhooks.test" {
		t.Fatalf("%+v %v", res.Reply, err)
	}
	raw, _ := json.Marshal(f.prov.reqs[0].Turns)
	if !strings.Contains(string(raw), "Current page state") || !strings.Contains(string(raw), "webhooks.test") {
		t.Fatal("page state missing from the context")
	}
	var queued int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbound_webhook_deliveries WHERE organization_id = $1 AND event_name = 'webhook.test'", f.scope.ID).Scan(&queued)
	if queued != 0 {
		t.Fatal("test queued before confirmation")
	}
	out, err := f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil)
	if err != nil || out.Actions[0].Status != "done" || out.Actions[0].Verification == nil {
		t.Fatalf("confirm %+v %v", out, err)
	}
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbound_webhook_deliveries WHERE organization_id = $1 AND event_name = 'webhook.test'", f.scope.ID).Scan(&queued)
	if queued != 1 {
		t.Fatalf("test deliveries queued: %d", queued)
	}
	// The worker delivers it signed like any event.
	if err := hooks.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(receiver.bodies) != 1 || !strings.Contains(receiver.bodies[0], `"webhook.test"`) {
		t.Fatalf("delivered %v", receiver.bodies)
	}
}

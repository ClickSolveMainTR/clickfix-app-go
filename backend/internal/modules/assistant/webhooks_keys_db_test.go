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
	publicapimodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi"
	publicapihandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/handler"
	publicapirepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/repository"
	publicapiuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/usecase"
	webhooksmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks"
	webhookshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/handler"
	webhooksuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// CF-190: webhooks and API keys from the chat; their secrets are shown
// once and stored nowhere else.
func TestWebhooksAndAPIKeysFromTheChat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, stmt := range []string{"DELETE FROM outbound_webhooks WHERE organization_id = $1", "DELETE FROM public_api_keys WHERE organization_id = $1"} {
			_, _ = f.pool.Exec(c, stmt, f.scope.ID)
		}
	})
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantWebhooksRead, rbac.PermTenantWebhooksWrite, rbac.PermTenantAPIKeysManage}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := wfLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}
	resolver := wfResolver{scope}
	box, _ := crypto.NewSecretBox(strings.Repeat("k", 32))
	mux := http.NewServeMux()
	webhooksmodule.RegisterRoutes(mux, &webhookshandler.Handler{Service: webhooksuc.New(f.q, box)}, tokens, loader, resolver)
	publicapimodule.RegisterRoutes(mux, &publicapihandler.Handler{Service: &publicapiuc.Service{Store: publicapirepo.New(f.q), Box: box}}, tokens, loader, resolver)
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).
		Register(webhooksmodule.Capabilities()...).Register(webhooksmodule.WriteCapabilities()...).
		Register(publicapimodule.Capabilities("https://api.clickfix.example")...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/settings/integrations"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	propose := func(prompt, tool, input string) []assistant.ProposedAction {
		t.Helper()
		f.prov.steps = []ai.Result{call("x", tool, input), {Text: `{"reply":"Onayınıza sundum.","suggestions":[]}`}}
		res, err := f.svc.Send(ctx, c, th.UUID, prompt, pc)
		if err != nil {
			t.Fatal(err)
		}
		return res.Reply.Actions
	}
	secretsStored := func(secret string) int {
		var n int
		_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM assistant_messages WHERE organization_id = $1 AND (content LIKE '%'||$2||'%' OR actions::text LIKE '%'||$2||'%')", f.scope.ID, secret).Scan(&n)
		return n
	}
	event := events.WebhookEvents()[0]

	// A private target never becomes a proposal.
	if acts := propose("Webhook kur", "webhooks_create", `{"params":{"name":"İç","url":"https://10.0.0.5/hook","events":["`+event+`"]}}`); len(acts) != 0 {
		t.Fatal("private webhook proposed")
	}
	acts := propose("ERP'ye olay gönder", "webhooks_create", `{"params":{"name":"ERP","url":"https://erp.example.com/hooks","events":["`+event+`"]}}`)
	out, err := f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil)
	if err != nil || out.Actions[0].Status != "done" || out.Actions[0].Verification == nil || out.Actions[0].Verification.Status != "ok" {
		t.Fatalf("webhook %+v %v", out.Actions, err)
	}
	hookSecret := out.SecretOnce["signing_secret"]
	if len(hookSecret) < 32 || secretsStored(hookSecret) != 0 {
		t.Fatalf("webhook secret %d stored=%d", len(hookSecret), secretsStored(hookSecret))
	}
	hookID := out.Actions[0].Result
	// The same URL again is caught by the dry run.
	dup := propose("Bir daha", "webhooks_create", `{"params":{"name":"ERP 2","url":"https://erp.example.com/hooks","events":["`+event+`"]}}`)
	if pv, _ := f.svc.DryRun(ctx, c, th.UUID, dup[0].ID, nil); pv.Preview == nil || pv.Preview.OK {
		t.Fatalf("duplicate %+v", pv.Preview)
	}
	// Update keeps the other fields; delete is destructive.
	acts = propose("ERP'yi durdur", "webhooks_update", `{"params":{"webhook_uuid":"`+hookID+`","enabled":false}}`)
	if _, err := f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var url string
	_ = f.pool.QueryRow(ctx, "SELECT enabled, url FROM outbound_webhooks WHERE uuid = $1", hookID).Scan(&enabled, &url)
	if enabled || url != "https://erp.example.com/hooks" {
		t.Fatalf("update %v %s", enabled, url)
	}
	acts = propose("ERP'yi sil", "webhooks_delete", `{"params":{"webhook_uuid":"`+hookID+`"}}`)
	if acts[0].Risk != "destructive" {
		t.Fatal("delete not destructive")
	}
	if _, err := f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil); err != nil {
		t.Fatal(err)
	}

	// API key: shown once, the example uses a placeholder.
	acts = propose("Okuma anahtarı oluştur", "api_keys_create", `{"params":{"name":"Raporlama","scopes":["conversations:read"],"expires_in_days":30}}`)
	out, err = f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := out.SecretOnce["api_key"]
	if !strings.HasPrefix(key, "cf_sk_") || secretsStored(key) != 0 || !strings.Contains(out.Actions[0].Details[0].Code, "$CLICKFIX_API_KEY") {
		t.Fatalf("key %q stored=%d %+v", key, secretsStored(key), out.Actions[0].Details)
	}
	// Listing never returns the key itself to the model.
	f.prov.reqs = nil
	f.prov.steps = []ai.Result{call("l", "api_keys_list", `{}`), {Text: `{"reply":"Bir anahtar var.","suggestions":[]}`}}
	if _, err := f.svc.Send(ctx, c, th.UUID, "Anahtarlarım neler?", pc); err != nil {
		t.Fatal(err)
	}
	for _, req := range f.prov.reqs {
		raw, _ := json.Marshal(req)
		if strings.Contains(string(raw), key) || strings.Contains(string(raw), hookSecret) {
			t.Fatal("the model saw a secret")
		}
	}
	acts = propose("Raporlama anahtarını iptal et", "api_keys_revoke", `{"params":{"key_uuid":"`+out.Actions[0].Result+`"}}`)
	if _, err := f.svc.Confirm(ctx, c, th.UUID, acts[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	var revoked bool
	_ = f.pool.QueryRow(ctx, "SELECT revoked_at IS NOT NULL FROM public_api_keys WHERE uuid = $1", out.Actions[0].Result).Scan(&revoked)
	if !revoked {
		t.Fatal("not revoked")
	}
}

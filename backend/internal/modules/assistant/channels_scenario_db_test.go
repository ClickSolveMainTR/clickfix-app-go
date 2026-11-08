package assistant_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/apichannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

type scopeResolver struct{ scope orgctx.Scope }

func (r scopeResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return r.scope, nil
}

type principalLoader struct{ p authctx.Principal }

func (l principalLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return l.p, nil
}

// countingTransport records every outbound HTTP request of the process
// made through http.DefaultTransport (dry runs must make none).
type countingTransport struct {
	next http.RoundTripper
	hits atomic.Int64
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.hits.Add(1)
	return c.next.RoundTrip(r)
}

// customerSystem stands in for the organization's own HTTPS endpoint
// (https://example.com/clickfix, routed to a local TLS server).
type customerSystem struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies [][]byte
	heads  []http.Header
}

func newCustomerSystem(t *testing.T) *customerSystem {
	cs := &customerSystem{}
	cs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cs.mu.Lock()
		cs.bodies = append(cs.bodies, b)
		cs.heads = append(cs.heads, r.Header.Clone())
		cs.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(cs.srv.Close)
	return cs
}

// client dials the local server whatever host the URL names (the
// httptest certificate is valid for example.com).
func (cs *customerSystem) client() *http.Client {
	c := cs.srv.Client()
	tr := c.Transport.(*http.Transport).Clone()
	addr := cs.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	c.Transport = tr
	return c
}

func TestScenarioRESTAPIChannel(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var userUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", f.user).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM channel_instances WHERE organization_id = $1", f.scope.ID)
	})

	// The real channel routes behind the /v1 bridge, as in the server.
	cs := newCustomerSystem(t)
	box, err := crypto.NewSecretBox(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := channels.NewVault(box)
	if err != nil {
		t.Fatal(err)
	}
	svc := channelsusecase.New(f.q, channels.NewRegistry(apichannel.New(cs.client())), vault)
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	scope := f.scope
	scope.Status = "active"
	scope.Permissions = []string{rbac.PermTenantAssistantUse, rbac.PermTenantChannelsRead, rbac.PermTenantChannelsManage}
	mux := http.NewServeMux()
	channelsmodule.RegisterRoutes(mux, channelshandler.New(svc, nil, nil, nil), tokens,
		principalLoader{authctx.Principal{UserID: userUUID, UserInternal: f.user}}, scopeResolver{scope})
	bridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	f.svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(channelsmodule.Capabilities()...)
	f.svc.API = func(user, org uuid.UUID) capability.API { return capability.Bridge{API: bridge, User: user, Org: org} }
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	pc := assistant.Context{Surface: "web", Route: "/channels"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)

	// 1. "Connect our REST API": the assistant looks at the channels
	//    first (none) and asks for the webhook address.
	f.prov.steps = []ai.Result{
		call("l1", "channels_list", `{"type":"api"}`),
		{Text: `{"reply":"Henüz REST API kanalınız yok. Yanıtları göndereceğimiz HTTPS adresiniz nedir?","suggestions":[]}`},
	}
	if _, err := f.svc.Send(ctx, c, th.UUID, "REST API kanalını sistemimize bağlamak istiyorum", pc); err != nil {
		t.Fatal(err)
	}
	if tool := toolTexts(f.prov.reqs[len(f.prov.reqs)-1]); !strings.Contains(tool, `"total":0`) {
		t.Fatalf("list: %q", tool)
	}

	// 2. The user gives the address; the assistant proposes the channel.
	f.prov.reqs = nil
	f.prov.steps = []ai.Result{
		call("p1", "channels_api_create", `{"summary":"Siparis sistemi için REST API kanalı","params":{"name":"Sipariş sistemi","webhook_url":"https://example.com/clickfix"}}`),
		{Text: `{"reply":"Kanalı onayınıza sundum.","suggestions":["Onayla"]}`},
	}
	res, err := f.svc.Send(ctx, c, th.UUID, "Adresimiz https://example.com/clickfix", pc)
	if err != nil || len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != "channels.api.create" {
		t.Fatalf("proposal %+v %v", res.Reply, err)
	}
	card := res.Reply.Actions[0]

	// SSRF: a private target in the card is refused before anything runs.
	for _, bad := range []string{"https://127.0.0.1/x", "http://example.com/x", "https://user:pw@example.com/x", "https://intranet.local/x"} {
		if _, err := f.svc.DryRun(ctx, c, th.UUID, card.ID, capability.Params{"webhook_url": bad}); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}

	// 3. Dry run: no channel, no outbound request.
	counter := &countingTransport{next: http.DefaultTransport}
	http.DefaultTransport = counter
	pv, err := f.svc.DryRun(ctx, c, th.UUID, card.ID, nil)
	http.DefaultTransport = counter.next
	if err != nil || !pv.Preview.OK || !strings.Contains(pv.Preview.Summary, "https://example.com/clickfix") {
		t.Fatalf("dry run %+v %v", pv.Preview, err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM channel_instances WHERE organization_id = $1", f.scope.ID).Scan(&n)
	if n != 0 || counter.hits.Load() != 0 || len(cs.bodies) != 0 {
		t.Fatalf("dry run changed data or left the process: channels=%d http=%d endpoint=%d", n, counter.hits.Load(), len(cs.bodies))
	}

	// 4. Confirm: the channel exists, the secret is shown once only.
	out, err := f.svc.Confirm(ctx, c, th.UUID, card.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	secret := out.SecretOnce["signing_secret"]
	done := out.Actions[0]
	if len(secret) != 64 || done.Status != "done" || done.Verification == nil || done.Verification.Status != "ok" {
		t.Fatalf("confirm %+v secret=%d", done, len(secret))
	}
	channelID := done.Result
	var typ, name string
	var credentials []byte
	if err := f.pool.QueryRow(ctx, "SELECT channel_type, name, credentials_enc FROM channel_instances WHERE organization_id = $1 AND uuid = $2", f.scope.ID, channelID).Scan(&typ, &name, &credentials); err != nil || typ != "api" || name != "Sipariş sistemi" {
		t.Fatalf("channel %s %s %v", typ, name, err)
	}
	if strings.Contains(string(credentials), secret) {
		t.Fatal("secret stored in clear")
	}
	var leaks int
	if err := f.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM assistant_messages WHERE organization_id = $1 AND (content LIKE '%'||$2||'%' OR actions::text LIKE '%'||$2||'%')) +
		(SELECT COUNT(*) FROM activity_events WHERE payload::text LIKE '%'||$2||'%')`, f.scope.ID, secret).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("secret leaked into %d rows (%v)", leaks, err)
	}
	codes := ""
	for _, d := range done.Details {
		codes += d.Code
	}
	if !strings.Contains(codes, "$CLICKFIX_SIGNING_SECRET") || strings.Contains(codes, secret) || !strings.Contains(codes, "/v1/webhooks/channels/api/") {
		t.Fatalf("sample request: %s", codes)
	}

	// 5. Example requests on demand: placeholders, never the secret.
	f.prov.reqs = nil
	f.prov.steps = []ai.Result{
		call("s1", "channels_api_sample_request", `{"channel_uuid":"`+channelID+`"}`),
		{Text: `{"reply":"Örnekler yukarıda.","suggestions":[]}`},
	}
	if _, err := f.svc.Send(ctx, c, th.UUID, "Bana örnek istek ver", pc); err != nil {
		t.Fatal(err)
	}
	for _, req := range f.prov.reqs {
		raw, _ := json.Marshal(req)
		if strings.Contains(string(raw), secret) {
			t.Fatal("the model saw the secret")
		}
	}
	if tool := toolTexts(f.prov.reqs[len(f.prov.reqs)-1]); !strings.Contains(tool, "inbound_example") {
		t.Fatalf("sample: %q", tool)
	}

	// 6. Test the set-up: a signed test event reaches the endpoint.
	f.prov.steps = []ai.Result{
		call("t1", "channels_test", `{"summary":"Test","params":{"channel_uuid":"`+channelID+`"}}`),
		{Text: `{"reply":"Testi onayınıza sundum.","suggestions":[]}`},
	}
	res, err = f.svc.Send(ctx, c, th.UUID, "Kurulumu test et", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("test proposal %+v %v", res.Reply, err)
	}
	if len(cs.bodies) != 0 {
		t.Fatal("test ran before confirmation")
	}
	out, err = f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil)
	if err != nil || out.Actions[0].Verification == nil || out.Actions[0].Verification.Status != "ok" || len(cs.bodies) != 1 {
		t.Fatalf("test %+v %v deliveries=%d", out.Actions[0], err, len(cs.bodies))
	}
	h := cs.heads[0]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h.Get("X-ClickFix-Timestamp") + "."))
	mac.Write(cs.bodies[0])
	if h.Get("X-ClickFix-Event") != "test" || h.Get("X-ClickFix-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("test delivery not signed with the shown secret: %v", h)
	}

	// 7. The channel exists: the assistant proposes an update, not a copy.
	f.prov.steps = []ai.Result{
		call("u0", "channels_list", `{"type":"api"}`),
		call("u1", "channels_api_update", `{"summary":"Adres değişir","params":{"channel_uuid":"`+channelID+`","webhook_url":"https://example.com/v2/clickfix"}}`),
		{Text: `{"reply":"Mevcut kanalı güncellemeyi önerdim.","suggestions":[]}`},
	}
	res, err = f.svc.Send(ctx, c, th.UUID, "REST API kanalını kur, adres https://example.com/v2/clickfix", pc)
	if err != nil || len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != "channels.api.update" {
		t.Fatalf("update proposal %+v %v", res.Reply, err)
	}
	if tool := toolTexts(f.prov.reqs[len(f.prov.reqs)-2]); !strings.Contains(tool, "Sipariş sistemi") || strings.Contains(tool, "signing_secret") {
		t.Fatalf("list shows the channel, never its secret: %q", tool)
	}
	if _, err := f.svc.Confirm(ctx, c, th.UUID, res.Reply.Actions[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	var hint string
	ch, _ := svc.Get(ctx, f.scope.ID, uuid.MustParse(channelID))
	hint = ch.CredentialHints["webhook_url"]
	if hint != "https://example.com/v2/clickfix" {
		t.Fatalf("update not applied: %q", hint)
	}
}

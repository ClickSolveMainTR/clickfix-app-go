package assistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// fakeAPI records the /v1 calls a capability makes.
type fakeAPI struct {
	mu    sync.Mutex
	calls []string
}

func (a *fakeAPI) Call(_ context.Context, method, path string, _ any) (capability.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, method+" "+path)
	return capability.Response{OK: true, Status: 200, Data: json.RawMessage(`{"items":[{"name":"Mevcut"}]}`)}, nil
}

func (a *fakeAPI) methods() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for _, c := range a.calls {
		out = append(out, strings.Fields(c)[0])
	}
	return out
}

// thingCap is a write capability with a generated secret, a dry run that
// tries to write (and must be refused) and a verify step.
type thingCap struct {
	applied  []capability.Params
	secrets  []map[string]string
	dryWrite error
}

func (tc *thingCap) caps() []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "things.create", Module: "things", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage,
			Routes: []string{"/things"}, Title: map[string]string{"tr": "Şey oluştur", "en": "Create thing"},
			Description: "Create a thing.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "url"},
				"properties": map[string]any{
					"name":   map[string]any{"type": "string", "title": "Name", "maxLength": 40},
					"url":    map[string]any{"type": "string", "title": "URL", "format": "uri"},
					"secret": map[string]any{"type": "string", "title": "Secret"},
				}},
			Secrets: []capability.Secret{{Key: "secret", Generate: capability.RandomSecret}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				if _, err := e.API.Call(ctx, "GET", "/v1/tenant/things", nil); err != nil {
					return capability.Result{}, err
				}
				_, tc.dryWrite = e.API.Call(ctx, "POST", "/v1/tenant/things", p)
				return capability.Result{Summary: "would create " + p["name"].(string)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				tc.applied = append(tc.applied, p)
				tc.secrets = append(tc.secrets, e.Secrets)
				_, _ = e.API.Call(ctx, "POST", "/v1/tenant/things", p)
				return capability.Result{Summary: "created", Details: []capability.Detail{{Label: "Örnek", Code: "curl -H 'X-Secret: $SECRET'"}},
					Secrets: map[string]string{"secret": e.Secrets["secret"]}}, nil
			},
			Verify: func(context.Context, capability.Exec, capability.Params, capability.Result) capability.Check {
				return capability.Check{Status: capability.VerifyOK}
			},
		},
		{
			Name: "things.list", Module: "things", Kind: capability.Read, Permission: rbac.PermTenantChannelsRead,
			Description: "List things.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, "GET", "/v1/tenant/things", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var data any
				_ = r.Decode(&data)
				return capability.Result{Summary: "1 thing", Data: data}, nil
			},
		},
	}
}

func withThings(f *fixture) (*thingCap, *fakeAPI) {
	tc := &thingCap{}
	api := &fakeAPI{}
	f.svc.Caps = capability.NewRegistry().Register(append(assistant.CoreCapabilities(f.q), tc.caps()...)...)
	f.svc.API = func(uuid.UUID, uuid.UUID) capability.API { return api }
	return tc, api
}

func call(id, name, input string) ai.Result {
	return ai.Result{Calls: []ai.ToolCall{{ID: id, Name: name, Input: json.RawMessage(input)}}}
}

// toolTexts are the tool results the model saw in the last round.
func toolTexts(req ai.Request) string {
	var b strings.Builder
	for i := len(req.Turns) - 1; i >= 0 && req.Turns[i].Role == "tool"; i-- {
		b.WriteString(req.Turns[i].Text + "\n")
	}
	return b.String()
}

func TestCapabilityProposalNeverAppliesWithoutConfirm(t *testing.T) {
	f := setup(t)
	tc, api := withThings(f)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/things"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	manager := f.caller(rbac.PermTenantChannelsManage, rbac.PermTenantChannelsRead)
	f.prov.steps = []ai.Result{
		// A read, and a proposal with a missing field (the model is told
		// to ask; nothing is proposed).
		{Calls: []ai.ToolCall{
			{ID: "c1", Name: "things_list", Input: json.RawMessage(`{}`)},
			{ID: "c2", Name: "things_create", Input: json.RawMessage(`{"summary":"x","params":{"name":"Uygulama"}}`)},
		}},
		{Calls: []ai.ToolCall{
			{ID: "c3", Name: "things_create", Input: json.RawMessage(`{"summary":"Uygulama oluşturulur","params":{"name":"Uygulama","url":"https://example.com/hook"}}`)},
		}},
		{Text: `{"reply":"Onayınıza sundum.","suggestions":["Onayla"]}`},
	}
	res, err := f.svc.Send(ctx, manager, th.UUID, "Bir şey oluştur", pc)
	if err != nil {
		t.Fatal(err)
	}
	if tools := toolTexts(f.prov.reqs[1]); !strings.Contains(tools, "Mevcut") || !strings.Contains(tools, "ask the user") {
		t.Fatalf("tool results: %q", tools)
	}
	// The page's capabilities are named in the context and secrets never
	// appear in the tool schema the model gets.
	tools, _ := json.Marshal(f.prov.reqs[0].Tools)
	if strings.Contains(string(tools), `"secret"`) || !strings.Contains(string(tools), "PROPOSES") {
		t.Fatalf("tools %s", tools)
	}
	if len(res.Reply.Actions) != 1 {
		t.Fatalf("actions %+v", res.Reply.Actions)
	}
	a := res.Reply.Actions[0]
	if a.Kind != "things.create" || a.Capability != "things.create" || a.Risk != "write" || !a.DryRun || a.Title != "Şey oluştur" || len(a.Form) != 3 {
		t.Fatalf("action %+v", a)
	}
	if len(tc.applied) != 0 || slices.Contains(api.methods(), "POST") {
		t.Fatal("a proposal changed data")
	}

	// A secret from the model fails the tool schema: the turn ends with
	// the fallback answer and nothing is proposed.
	th2, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{call("s1", "things_create", `{"params":{"name":"a","url":"https://example.com/hook","secret":"from-the-model"}}`)}
	if res2, err := f.svc.Send(ctx, manager, th2.UUID, "anahtar şu", pc); err != nil || len(res2.Reply.Actions) != 0 {
		t.Fatalf("secret from the model: %+v %v", res2.Reply.Actions, err)
	}

	// Dry run: GET-only API, the attempted write is refused, data unchanged.
	pv, err := f.svc.DryRun(ctx, manager, th.UUID, a.ID, capability.Params{"name": "Yeni ad"})
	if err != nil || pv.Preview == nil || !pv.Preview.OK || pv.Preview.Summary != "would create Yeni ad" {
		t.Fatalf("dry run %+v %v", pv.Preview, err)
	}
	if !errors.Is(tc.dryWrite, capability.ErrReadOnly) || slices.Contains(api.methods(), "POST") || len(tc.applied) != 0 {
		t.Fatalf("dry run wrote: %v %v", tc.dryWrite, api.methods())
	}
	if _, err := f.svc.DryRun(ctx, manager, th.UUID, a.ID, capability.Params{"name": 7}); !errors.Is(err, capability.ErrInvalid) {
		t.Fatalf("bad edited params: %v", err)
	}

	// Without the permission (revoked): forbidden, not applied.
	if _, err := f.svc.Confirm(ctx, f.caller(), th.UUID, a.ID, nil); !errors.Is(err, assistant.ErrForbidden) {
		t.Fatalf("confirm without permission: %v", err)
	}
	// Edited params are checked again.
	if _, err := f.svc.Confirm(ctx, manager, th.UUID, a.ID, capability.Params{"url": "not a url at all", "extra": true}); !errors.Is(err, capability.ErrInvalid) {
		t.Fatalf("bad confirm params: %v", err)
	}
	if len(tc.applied) != 0 {
		t.Fatal("applied on an invalid confirm")
	}
	out, err := f.svc.Confirm(ctx, manager, th.UUID, a.ID, capability.Params{"name": "Düzenlendi"})
	if err != nil {
		t.Fatal(err)
	}
	secret := out.SecretOnce["secret"]
	if len(tc.applied) != 1 || tc.applied[0]["name"] != "Düzenlendi" || len(secret) != 64 || tc.secrets[0]["secret"] != secret {
		t.Fatalf("applied %+v secret %q", tc.applied, secret)
	}
	done := out.Actions[0]
	if done.Status != "done" || done.Verification == nil || done.Verification.Status != "ok" || len(done.Details) != 1 {
		t.Fatalf("done %+v", done)
	}
	// The secret is never stored: not in messages, actions or params.
	var stored int
	if err := f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM assistant_messages WHERE organization_id = $1 AND (content LIKE '%'||$2||'%' OR actions::text LIKE '%'||$2||'%')", f.scope.ID, secret).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("secret stored: %d %v", stored, err)
	}
	if _, err := f.svc.Confirm(ctx, manager, th.UUID, a.ID, nil); !errors.Is(err, assistant.ErrActionState) {
		t.Fatalf("double confirm: %v", err)
	}
}

func TestCapabilityTypeErrorsAndHiddenTools(t *testing.T) {
	f := setup(t)
	tc, _ := withThings(f)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/things"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	// A wrong type fails the schema of the tool loop: the call is not run,
	// the model reads an error result (CF-251) and nothing is proposed.
	f.prov.steps = []ai.Result{call("c1", "things_create", `{"params":{"name":1,"url":"https://example.com"}}`)}
	res, err := f.svc.Send(ctx, f.caller(rbac.PermTenantChannelsManage), th.UUID, "oluştur", pc)
	if err != nil || len(res.Reply.Actions) != 0 || len(tc.applied) != 0 {
		t.Fatalf("type error: %+v %v", res.Reply, err)
	}
	// Without the permission the tool is not offered and a forged call is
	// refused by the loop.
	unprivileged := len(f.prov.reqs)
	f.prov.steps = []ai.Result{call("c1", "things_create", `{"params":{"name":"a","url":"https://example.com"}}`)}
	res, err = f.svc.Send(ctx, f.caller(), th.UUID, "oluştur", pc)
	if err != nil || len(res.Reply.Actions) != 0 || len(tc.applied) != 0 {
		t.Fatalf("forged call: %+v %v", res.Reply, err)
	}
	for _, req := range f.prov.reqs[unprivileged:] {
		raw, _ := json.Marshal(req.Turns)
		if !strings.Contains(string(raw), "oluştur") || len(req.Tools) == 0 {
			continue // the title request
		}
		for _, tool := range req.Tools {
			if strings.HasPrefix(tool.Name, "things_") {
				t.Fatalf("tool %s offered without permission", tool.Name)
			}
		}
	}
}

func TestOpenRouteFocusAndSecretRedaction(t *testing.T) {
	f := setup(t)
	withThings(f)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/inbox"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{
		call("c1", "navigation_open_route", `{"params":{"route":"https://evil.example/phish"}}`),
		call("c2", "navigation_open_route", `{"params":{"route":"//evil.example"}}`),
		call("c3", "navigation_open_route", `{"summary":"SLA sayfası","params":{"route":"/settings/sla","target":"sla.new-policy"}}`),
		{Text: `{"reply":"Sayfayı açabilirim.","suggestions":[]}`},
	}
	key := "sk-live-ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"
	res, err := f.svc.Send(ctx, f.caller(), th.UUID, "Anahtarım "+key+" SLA nerede?", pc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Message.Content, key) {
		t.Fatal("secret stored")
	}
	for _, req := range f.prov.reqs {
		raw, _ := json.Marshal(req.Turns)
		if strings.Contains(string(raw), key) {
			t.Fatal("provider saw the secret")
		}
	}
	if len(res.Reply.Actions) != 1 {
		t.Fatalf("external routes accepted: %+v", res.Reply.Actions)
	}
	out, err := f.svc.Confirm(ctx, f.caller(), th.UUID, res.Reply.Actions[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := out.Actions[0]
	if a.Route != "/settings/sla" || a.Focus == nil || a.Focus.Target != "sla.new-policy" {
		t.Fatalf("focus %+v", a)
	}
}

// streamScripted streams the scripted text answers as deltas.
type streamScripted struct{ *scripted }

func (p streamScripted) Stream(ctx context.Context, req ai.Request, emit ai.Delta) (ai.Result, error) {
	r, err := p.Generate(ctx, req)
	if err == nil && len(r.Calls) == 0 && r.Text != "" {
		err = emit(r.Text)
	}
	return r, err
}

func TestStreamEmitsCapabilityCard(t *testing.T) {
	f := setup(t)
	withThings(f)
	route := f.svc.AI.Routes["reply"]
	route.Provider = streamScripted{f.prov}
	f.svc.AI.Routes["reply"] = route
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/things"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{
		call("c1", "things_create", `{"summary":"oluşturulur","params":{"name":"A","url":"https://example.com/h"}}`),
		{Text: "Hazırladım."},
	}
	var events []string
	var card assistant.ProposedAction
	_, err := f.svc.SendStream(ctx, f.caller(rbac.PermTenantChannelsManage), th.UUID, "oluştur", pc, func(name string, data any) error {
		events = append(events, name)
		if name == "action" {
			card = data.(assistant.ProposedAction)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, "action") || card.Capability != "things.create" || !card.DryRun || len(card.Form) == 0 {
		t.Fatalf("events %v card %+v", events, card)
	}
}

func TestActionEndpoints(t *testing.T) {
	f := setup(t)
	withThings(f)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/things"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	manager := f.caller(rbac.PermTenantChannelsManage)
	f.prov.steps = []ai.Result{
		call("c1", "things_create", `{"summary":"oluşturulur","params":{"name":"A","url":"https://example.com/h"}}`),
		{Text: `{"reply":"Hazır.","suggestions":[]}`},
	}
	res, err := f.svc.Send(ctx, manager, th.UUID, "oluştur", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("%+v %v", res.Reply, err)
	}
	id := res.Reply.Actions[0].ID
	h := assistant.NewHandler(f.svc, nil, nil)
	do := func(fn http.HandlerFunc, scope orgctx.Scope, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.SetPathValue("uuid", th.UUID.String())
		r.SetPathValue("action_id", id)
		c := orgctx.WithScope(r.Context(), scope)
		c = authctx.WithPrincipal(c, authctx.Principal{UserInternal: f.user})
		w := httptest.NewRecorder()
		fn(w, r.WithContext(c))
		return w
	}
	if w := do(h.DryRun, manager.Scope, `{"params":{"name":"B"}}`); w.Code != 200 || !strings.Contains(w.Body.String(), "would create B") {
		t.Fatalf("dry run %d %s", w.Code, w.Body)
	}
	if w := do(h.DryRun, manager.Scope, `{"params":{"name":"B","admin":true}}`); w.Code != 400 || !strings.Contains(w.Body.String(), "additional properties") {
		t.Fatalf("bad dry run %d %s", w.Code, w.Body)
	}
	if w := do(h.Confirm, f.caller().Scope, ``); w.Code != 403 {
		t.Fatalf("forbidden %d %s", w.Code, w.Body)
	}
	w := do(h.Confirm, manager.Scope, `{"params":{"name":"C"}}`)
	var out struct {
		Data struct {
			SecretOnce map[string]string          `json:"secret_once"`
			Actions    []assistant.ProposedAction `json:"actions"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || len(out.Data.SecretOnce["secret"]) != 64 || out.Data.Actions[0].Params["name"] != "C" {
		t.Fatalf("confirm %d %s", w.Code, w.Body)
	}
}

func TestConfirmIsMarkedBeforeApply(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	var actionID string
	var reentry error
	applied := 0
	f.svc.Caps = capability.NewRegistry().Register(&capability.Capability{
		Name: "once.do", Kind: capability.Write, Description: "Do it once.",
		Apply: func(ctx context.Context, _ capability.Exec, _ capability.Params) (capability.Result, error) {
			applied++
			// A second confirm while this one runs (double click, retry
			// after a lost response) must not run the action again.
			_, reentry = f.svc.Confirm(ctx, f.caller(), th.UUID, actionID, nil)
			return capability.Result{Summary: "ok"}, nil
		},
	})
	f.svc.API = func(uuid.UUID, uuid.UUID) capability.API { return &fakeAPI{} }
	f.prov.steps = []ai.Result{call("c1", "once_do", `{"params":{}}`), {Text: `{"reply":"Hazır.","suggestions":[]}`}}
	res, err := f.svc.Send(ctx, f.caller(), th.UUID, "yap", pc)
	if err != nil || len(res.Reply.Actions) != 1 {
		t.Fatalf("%+v %v", res.Reply, err)
	}
	actionID = res.Reply.Actions[0].ID
	out, err := f.svc.Confirm(ctx, f.caller(), th.UUID, actionID, nil)
	if err != nil || out.Actions[0].Status != "done" {
		t.Fatalf("%+v %v", out, err)
	}
	if applied != 1 || !errors.Is(reentry, assistant.ErrActionState) {
		t.Fatalf("applied %d times, re-entry: %v", applied, reentry)
	}
}

// Review fix: without the panel API the action is not confirmed (it stays
// pending and can be retried), and a panic in Apply stores a failed card.
func TestConfirmWithoutAPIStaysPendingAndPanicFails(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pc := assistant.Context{Surface: "web", Route: "/"}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	boom := false
	f.svc.Caps = capability.NewRegistry().Register(&capability.Capability{
		Name: "api.do", Kind: capability.Write, Description: "Calls the API.",
		Apply: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
			if boom {
				panic("boom")
			}
			if _, err := e.API.Call(ctx, "GET", "/v1/tenant/x", nil); err != nil {
				return capability.Result{}, err
			}
			return capability.Result{Summary: "ok"}, nil
		},
	})
	f.svc.API = nil
	propose := func() string {
		t.Helper()
		f.prov.steps = []ai.Result{call("c1", "api_do", `{"params":{}}`), {Text: `{"reply":"Hazır.","suggestions":[]}`}}
		res, err := f.svc.Send(ctx, f.caller(), th.UUID, "yap", pc)
		if err != nil || len(res.Reply.Actions) != 1 {
			t.Fatalf("%+v %v", res.Reply, err)
		}
		return res.Reply.Actions[0].ID
	}
	id := propose()
	if _, err := f.svc.Confirm(ctx, f.caller(), th.UUID, id, nil); !errors.Is(err, assistant.ErrAPIUnavailable) {
		t.Fatalf("no API: %v", err)
	}
	// Still pending: once the API is back the same card confirms.
	f.svc.API = func(uuid.UUID, uuid.UUID) capability.API { return &fakeAPI{} }
	if out, err := f.svc.Confirm(ctx, f.caller(), th.UUID, id, nil); err != nil || out.Actions[0].Status != "done" {
		t.Fatalf("retry %+v %v", out, err)
	}
	boom = true
	id = propose()
	out, err := f.svc.Confirm(ctx, f.caller(), th.UUID, id, nil)
	if err != nil || out.Actions[0].Status != "failed" {
		t.Fatalf("panic %+v %v", out, err)
	}
}

package assistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/guides"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// scripted is a fake provider: each Generate returns the next result.
type scripted struct {
	mu    sync.Mutex
	steps []ai.Result
	reqs  []ai.Request
}

func (p *scripted) Generate(_ context.Context, req ai.Request) (ai.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	if len(p.steps) == 0 {
		return ai.Result{FinalUsage: true, InputTokens: 10, OutputTokens: 5, Text: `{"reply":"Tamam.","suggestions":["Devam"]}`, Calls: []ai.ToolCall{}}, nil
	}
	r := p.steps[0]
	p.steps = p.steps[1:]
	r.FinalUsage, r.InputTokens, r.OutputTokens = true, 100, 50
	if r.Calls == nil {
		r.Calls = []ai.ToolCall{}
	}
	return r, nil
}

func (p *scripted) Stream(ctx context.Context, req ai.Request, _ ai.Delta) (ai.Result, error) {
	return p.Generate(ctx, req)
}

type fakeEscalator struct {
	calls []assistant.Escalation
}

func (f *fakeEscalator) Escalate(_ context.Context, _ uuid.UUID, e assistant.Escalation) (string, error) {
	f.calls = append(f.calls, e)
	return "#42", nil
}

type fixture struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	svc   *assistant.Service
	prov  *scripted
	esc   *fakeEscalator
	scope orgctx.Scope
	user  int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for assistant DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var userID int64
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ayşe', 'Demir', 'active') RETURNING id",
		uuid.NewString()+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	org, err := orgusecase.New(orgrepository.NewPostgres(pool, q), nil).
		CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Asistan " + uuid.NewString()[:6], OwnerUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	prev, _ := q.AssistantSettingsGet(ctx)
	t.Cleanup(func() {
		c := context.Background()
		_, _ = q.AssistantSettingsSave(c, db.AssistantSettingsSaveParams{Enabled: prev.Enabled, ModelRoute: prev.ModelRoute,
			DailyLimitPerOrg: prev.DailyLimitPerOrg, SystemPromptExtra: prev.SystemPromptExtra, SupportOrgUuid: prev.SupportOrgUuid})
		for _, stmt := range []string{
			"DELETE FROM assistant_usage WHERE organization_id = $1",
			"DELETE FROM assistant_threads WHERE organization_id = $1",
			"DELETE FROM guide_progress WHERE organization_id = $1",
			"DELETE FROM ai_policies WHERE organization_id = $1",
			"DELETE FROM domain_event_log WHERE organization_id = $1",
			"DELETE FROM outbox_events WHERE organization_id = $1",
			"DELETE FROM organizations WHERE id = $1",
		} {
			_, _ = pool.Exec(c, stmt, org.ID)
		}
		_, _ = pool.Exec(c, "DELETE FROM notification_preferences WHERE user_id = $1", userID)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", userID)
	})
	if _, err := q.AssistantSettingsSave(ctx, db.AssistantSettingsSaveParams{Enabled: true, ModelRoute: "reply", DailyLimitPerOrg: 100}); err != nil {
		t.Fatal(err)
	}
	gs := guides.New(q)
	if err := gs.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	prov := &scripted{}
	esc := &fakeEscalator{}
	svc := &assistant.Service{
		Pool: pool, Q: q, Guides: gs, Escalator: esc,
		AI: &assistant.PlatformAI{Q: q, Routes: map[string]ai.Route{"reply": {Provider: prov, ProviderName: "anthropic", Model: "test-model", InputPrice: 3, OutputPrice: 15}}},
	}
	svc.Actions = assistant.DefaultActions(assistant.ActionDeps{Q: q, Escalate: svc.EscalateFromAction})
	svc.Caps = capability.NewRegistry().Register(assistant.CoreCapabilities(q)...)
	scope := orgctx.Scope{ID: org.ID, UUID: org.UUID, Slug: org.Slug, Name: org.Name, MemberRole: "agent",
		Permissions: []string{rbac.PermTenantAssistantUse}}
	return &fixture{pool: pool, q: q, svc: svc, prov: prov, esc: esc, scope: scope, user: userID}
}

func (f *fixture) caller(perms ...string) assistant.Caller {
	sc := f.scope
	sc.Permissions = append([]string{rbac.PermTenantAssistantUse}, perms...)
	return assistant.Caller{Scope: sc, UserID: f.user, Locale: "tr"}
}

func TestProgressAPI(t *testing.T) {
	f := setup(t)
	h := guides.NewHandler(f.svc.Guides, nil)
	do := func(method, target, body string, fn http.HandlerFunc, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if key != "" {
			r.SetPathValue("key", key)
		}
		ctx := orgctx.WithScope(r.Context(), f.scope)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: f.user})
		w := httptest.NewRecorder()
		fn(w, r.WithContext(ctx))
		return w
	}
	type listOut struct {
		Data struct {
			Items []guides.Guide `json:"items"`
		} `json:"data"`
	}
	list := func() guides.Guide {
		w := do("GET", "/v1/tenant/guides?surface=web&route=/inbox&locale=tr", "", h.List, "")
		if w.Code != 200 {
			t.Fatalf("list %d %s", w.Code, w.Body)
		}
		var out listOut
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, g := range out.Data.Items {
			if g.Key == "web.inbox" {
				return g
			}
		}
		t.Fatalf("web.inbox missing: %s", w.Body)
		return guides.Guide{}
	}
	if g := list(); g.Progress.Status != "new" || len(g.Steps) < 2 || g.Title != "Gelen kutusu" {
		t.Fatalf("initial guide %+v", g)
	}
	if w := do("PUT", "/", `{"status":"in_progress","step_index":2}`, h.Progress, "web.inbox"); w.Code != 204 {
		t.Fatalf("progress %d %s", w.Code, w.Body)
	}
	if g := list(); g.Progress.Status != "in_progress" || g.Progress.StepIndex != 2 {
		t.Fatalf("progress not saved %+v", g.Progress)
	}
	if w := do("PUT", "/", `{"status":"done","step_index":0}`, h.Progress, "web.inbox"); w.Code != 422 && w.Code != 400 {
		t.Fatalf("invalid status accepted: %d", w.Code)
	}
	if w := do("PUT", "/", `{"status":"completed","step_index":0}`, h.Progress, "web.nope"); w.Code != 404 {
		t.Fatalf("unknown key: %d", w.Code)
	}
	if w := do("PUT", "/", `{"status":"skipped","step_index":0}`, h.Progress, "setup.channels"); w.Code != 204 {
		t.Fatalf("setup progress %d", w.Code)
	}
	w := do("GET", "/v1/tenant/guides/setup?surface=web&locale=en", "", h.Setup, "")
	var setupOut struct {
		Data guides.SetupTour `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &setupOut)
	if w.Code != 200 || setupOut.Data.Total != 8 || setupOut.Data.Completed != 1 || setupOut.Data.Items[0].Key != "setup.channels" || setupOut.Data.Items[0].Title != "Connect your channels" {
		t.Fatalf("setup %d %+v", w.Code, setupOut.Data)
	}
	if w := do("POST", "/", "", h.Reset, ""); w.Code != 204 {
		t.Fatalf("reset %d", w.Code)
	}
	if g := list(); g.Progress.Status != "new" {
		t.Fatalf("reset kept progress %+v", g.Progress)
	}
}

func proposeEnableAI() []ai.Result {
	return []ai.Result{
		{Calls: []ai.ToolCall{{ID: "c1", Name: "get_setup_state", Input: json.RawMessage(`{}`)}}},
		{Calls: []ai.ToolCall{{ID: "c2", Name: "ai_enable", Input: json.RawMessage(`{"summary":"AI önerileri açılır","params":{}}`)}}},
		{Text: `{"reply":"Yapay zekâyı açmayı onayınıza sundum.","suggestions":["Onayla","Başka ne yapmalıyım?"]}`},
	}
}

func TestConfirmEnforcesRBAC(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := f.caller()
	th, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Surface: "web", Route: "/settings/ai"})
	if err != nil {
		t.Fatal(err)
	}
	f.prov.steps = proposeEnableAI()
	// The tool is offered only to a caller holding tenant.ai.write; the
	// permission is checked again at confirm (here: it was revoked).
	res, err := f.svc.Send(ctx, f.caller(rbac.PermTenantAiWrite), th.UUID, "AI'ı nasıl açarım? Bana ali@example.com yazdı.", assistant.Context{Surface: "web", Route: "/settings/ai"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Message.Content, "ali@example.com") {
		t.Fatal("user message stored with raw PII")
	}
	for _, req := range f.prov.reqs {
		raw, _ := json.Marshal(req.Turns)
		if strings.Contains(string(raw), "ali@example.com") {
			t.Fatal("provider saw raw PII")
		}
		if !strings.Contains(string(raw), "Yapay zekâ ayarları") {
			t.Fatal("page guide missing from context data")
		}
	}
	if len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != "ai.enable" || res.Reply.Actions[0].Status != "proposed" || len(res.Reply.Suggestions) != 2 {
		t.Fatalf("reply %+v", res.Reply)
	}
	id := res.Reply.Actions[0].ID

	// An agent without tenant.ai.write cannot run it, whatever the model proposed.
	if _, err := f.svc.Confirm(ctx, c, th.UUID, id, nil); !errors.Is(err, assistant.ErrForbidden) {
		t.Fatalf("confirm without permission: %v", err)
	}
	if p, _ := f.q.AIPolicy(ctx, f.scope.ID); p.Enabled {
		t.Fatal("action ran without permission")
	}
	// Another user's caller cannot see the thread.
	other := c
	other.UserID = f.user + 1_000_000
	if _, err := f.svc.Confirm(ctx, other, th.UUID, id, nil); !errors.Is(err, assistant.ErrNotFound) {
		t.Fatalf("foreign confirm: %v", err)
	}
	msg, err := f.svc.Confirm(ctx, f.caller(rbac.PermTenantAiWrite), th.UUID, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != "tool" || len(msg.Actions) != 1 || msg.Actions[0].Status != "done" || msg.Actions[0].Route != "/settings/ai" {
		t.Fatalf("confirm message %+v", msg)
	}
	if p, err := f.q.AIPolicy(ctx, f.scope.ID); err != nil || !p.Enabled {
		t.Fatalf("ai not enabled: %v %+v", err, p)
	}
	if _, err := f.svc.Confirm(ctx, f.caller(rbac.PermTenantAiWrite), th.UUID, id, nil); !errors.Is(err, assistant.ErrActionState) {
		t.Fatalf("double confirm: %v", err)
	}
	msgs, _ := f.svc.Messages(ctx, f.scope.ID, f.user, th.UUID)
	if len(msgs) != 3 || msgs[1].Actions[0].Status != "done" {
		t.Fatalf("history %+v", msgs)
	}
	// Usage is recorded for the platform, nothing is reserved from the wallet.
	var calls, tokens int64
	if err := f.pool.QueryRow(ctx, "SELECT COUNT(*), COALESCE(SUM(input_tokens),0) FROM assistant_usage WHERE organization_id = $1", f.scope.ID).Scan(&calls, &tokens); err != nil || calls != 1 || tokens != 300 {
		t.Fatalf("usage calls=%d tokens=%d err=%v", calls, tokens, err)
	}
	var runs int64
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM ai_runs WHERE organization_id = $1", f.scope.ID).Scan(&runs)
	if runs != 0 {
		t.Fatal("assistant used the organization's AI runs")
	}
}

func TestRejectAction(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Route: "/"})
	f.prov.steps = proposeEnableAI()
	res, err := f.svc.Send(ctx, f.caller(rbac.PermTenantAiWrite), th.UUID, "AI açalım", assistant.Context{Route: "/"})
	if err != nil {
		t.Fatal(err)
	}
	id := res.Reply.Actions[0].ID
	if err := f.svc.Reject(ctx, f.caller(), th.UUID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Confirm(ctx, f.caller(rbac.PermTenantAiWrite), th.UUID, id, nil); !errors.Is(err, assistant.ErrActionState) {
		t.Fatalf("confirm after reject: %v", err)
	}
}

func TestDailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.SaveSettings(ctx, 0, assistant.Settings{Enabled: true, ModelRoute: "reply", DailyLimitPerOrg: 1}); err != nil {
		t.Fatal(err)
	}
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Route: "/inbox"})
	if _, err := f.svc.Send(ctx, f.caller(), th.UUID, "Merhaba", assistant.Context{Route: "/inbox"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Send(ctx, f.caller(), th.UUID, "Tekrar merhaba", assistant.Context{Route: "/inbox"}); !errors.Is(err, assistant.ErrDailyLimit) {
		t.Fatalf("second message: %v", err)
	}
	st, _, err := f.svc.Status(ctx, f.scope.ID)
	if err != nil || st.Enabled || st.Reason != assistant.ReasonDailyLimit || st.UsedToday != 1 || st.DailyLimit != 1 {
		t.Fatalf("status %+v %v", st, err)
	}
	if _, err := f.svc.SaveSettings(ctx, 0, assistant.Settings{Enabled: false, ModelRoute: "reply", DailyLimitPerOrg: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Send(ctx, f.caller(), th.UUID, "Merhaba", assistant.Context{Route: "/inbox"}); !errors.Is(err, assistant.ErrDisabled) {
		t.Fatalf("disabled: %v", err)
	}
}

func TestEscalate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Route: "/channels"})
	if _, err := f.svc.Send(ctx, f.caller(), th.UUID, "WhatsApp mesajları gelmiyor", assistant.Context{Route: "/channels"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Escalate(ctx, f.caller(), th.UUID, ""); !errors.Is(err, assistant.ErrSupportUnavailable) {
		t.Fatalf("no support org: %v", err)
	}
	support := uuid.New()
	if _, err := f.svc.SaveSettings(ctx, 0, assistant.Settings{Enabled: true, ModelRoute: "reply", DailyLimitPerOrg: 100, SupportOrgUUID: &support}); err != nil {
		t.Fatal(err)
	}
	ref, err := f.svc.Escalate(ctx, f.caller(), th.UUID, "Acil, müşteri 0532 123 45 67")
	if err != nil || ref != "#42" {
		t.Fatalf("escalate %q %v", ref, err)
	}
	if len(f.esc.calls) != 1 || !strings.Contains(f.esc.calls[0].Transcript, "WhatsApp") || f.esc.calls[0].Route != "/channels" || f.esc.calls[0].UserEmail == "" {
		t.Fatalf("escalation %+v", f.esc.calls)
	}
	if strings.Contains(f.esc.calls[0].Note, "123 45 67") {
		t.Fatal("note sent with raw PII")
	}
	// Idempotent: the same thread keeps its ticket.
	if ref2, err := f.svc.Escalate(ctx, f.caller(), th.UUID, ""); err != nil || ref2 != "#42" || len(f.esc.calls) != 1 {
		t.Fatalf("second escalate %q %v %d", ref2, err, len(f.esc.calls))
	}
	// The escalate card from a failed AI reply runs through confirm.
	th2, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{Route: "/"})
	f.prov.steps = []ai.Result{{Text: "not json"}}
	res, err := f.svc.Send(ctx, f.caller(), th2.UUID, "Yardım", assistant.Context{Route: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Kind != assistant.KindEscalate {
		t.Fatalf("fallback reply %+v", res.Reply)
	}
	msg, err := f.svc.Confirm(ctx, f.caller(), th2.UUID, res.Reply.Actions[0].ID, nil)
	if err != nil || msg.Actions[0].Status != "done" || msg.Actions[0].Result != "#42" {
		t.Fatalf("confirm escalate %+v %v", msg, err)
	}
}

// streamingFake exposes text as several provider chunks and can wait for disconnect.
type streamingFake struct {
	wait    bool
	failed  bool
	partial chan struct{}
}

func (p *streamingFake) Generate(context.Context, ai.Request) (ai.Result, error) {
	return ai.Result{}, ai.ErrUnavailable
}
func (p *streamingFake) Stream(ctx context.Context, req ai.Request, emit ai.Delta) (ai.Result, error) {
	out := ai.Result{Text: "Merhaba dünya.", InputTokens: 10, OutputTokens: 5, FinalUsage: true}
	for _, chunk := range []string{"Merhaba ", "dünya."} {
		if err := emit(chunk); err != nil {
			return out, err
		}
	}
	if p.partial != nil {
		close(p.partial)
	}
	if p.wait {
		<-ctx.Done()
		return out, ctx.Err()
	}
	if p.failed {
		return out, ai.ErrUnavailable
	}
	if req.Schema != nil || req.Effort != "low" || !req.CachePrefix {
		return out, errors.New("stream request regressed")
	}
	return out, nil
}

func TestStreamHandler(t *testing.T) {
	f := setup(t)
	p := &streamingFake{}
	route := f.svc.AI.Routes["reply"]
	route.Provider = p
	f.svc.AI.Routes["reply"] = route
	th, err := f.svc.CreateThread(context.Background(), f.scope.ID, f.user, assistant.Context{Route: "/inbox"})
	if err != nil {
		t.Fatal(err)
	}
	h := assistant.NewHandler(f.svc, nil, nil)
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"content":"Merhaba","context":{"route":"/inbox"}}`))
	r.SetPathValue("uuid", th.UUID.String())
	ctx := orgctx.WithScope(r.Context(), f.scope)
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: f.user})
	w := httptest.NewRecorder()
	h.SendStream(w, r.WithContext(ctx))
	body := w.Body.String()
	if strings.Contains(body, "\"tool_calls\"") {
		t.Fatal("raw provider fields exposed in SSE usage")
	}
	last := -1
	for _, event := range []string{"message.created", "status", "delta", "suggestions", "done"} {
		i := strings.Index(body, "event: "+event+"\n")
		if i <= last {
			t.Fatalf("event %s out of order: %s", event, body)
		}
		last = i
	}
	messages, err := f.svc.Messages(context.Background(), f.scope.ID, f.user, th.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "Merhaba dünya." || messages[1].Interrupted {
		t.Fatalf("%+v", messages)
	}
}

func TestStreamDisconnectPersistsInterrupted(t *testing.T) {
	f := setup(t)
	p := &streamingFake{wait: true, partial: make(chan struct{})}
	route := f.svc.AI.Routes["reply"]
	route.Provider = p
	f.svc.AI.Routes["reply"] = route
	th, err := f.svc.CreateThread(context.Background(), f.scope.ID, f.user, assistant.Context{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := f.svc.SendStream(ctx, f.caller(), th.UUID, "Merhaba", assistant.Context{}, func(string, any) error { return nil })
		done <- e
	}()
	<-p.partial
	cancel()
	if err := <-done; err == nil {
		t.Fatal("disconnect should cancel")
	}
	messages, err := f.svc.Messages(context.Background(), f.scope.ID, f.user, th.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !messages[1].Interrupted || !strings.Contains(messages[1].Content, "Merhaba") {
		t.Fatalf("%+v", messages)
	}
	used, err := f.q.AssistantUsageToday(context.Background(), pgtype.Int8{Int64: f.scope.ID, Valid: true})
	if err != nil || used != 1 {
		t.Fatalf("cancelled usage: %d %v", used, err)
	}
}

func TestGuideClaimSharedAcrossSurfaces(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	for _, key := range []string{"web.inbox", "mobile.inbox"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			started, e := f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, key)
			if e != nil {
				t.Error(e)
			}
			wins <- started
		}(key)
	}
	wg.Wait()
	close(wins)
	count := 0
	for win := range wins {
		if win {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d automatic starts", count)
	}
	if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "web.inbox", guides.Progress{Status: "skipped", StepIndex: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Guides.ForRoute(ctx, f.scope.ID, f.user, "mobile", "/inbox", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Progress.Status != "skipped" {
		t.Fatalf("not shared: %+v", got)
	}
	if err := f.q.GuidePreferencesSet(ctx, db.GuidePreferencesSetParams{UserID: f.user, AutoStart: false}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Guides.Reset(ctx, f.scope.ID, f.user); err != nil {
		t.Fatal(err)
	}
	started, err := f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, "web.inbox")
	if err != nil || started {
		t.Fatalf("preference ignored: %v %v", started, err)
	}
	if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "web.inbox", guides.Progress{Status: "in_progress"}); err != nil {
		t.Fatal("manual start denied", err)
	}
}

type toolStreamFake struct{ calls int }

func (p *toolStreamFake) Generate(context.Context, ai.Request) (ai.Result, error) {
	return ai.Result{}, ai.ErrUnavailable
}
func (p *toolStreamFake) Stream(_ context.Context, _ ai.Request, emit ai.Delta) (ai.Result, error) {
	p.calls++
	out := ai.Result{FinalUsage: true, InputTokens: 10, OutputTokens: 5}
	if p.calls == 1 {
		out.Calls = []ai.ToolCall{{ID: "setup", Name: assistant.ToolSetupState, Input: json.RawMessage(`{}`)}}
		return out, nil
	}
	if err := emit("Kurulumunuz hazır. "); err != nil {
		return out, err
	}
	out.Text = "Kurulumunuz hazır. "
	out.Calls = []ai.ToolCall{{ID: "suggest", Name: "suggest_replies", Input: json.RawMessage(`{"replies":["Sıradaki adım"]}`)}}
	return out, nil
}

func TestToolStatusAndTrailingSuggestions(t *testing.T) {
	f := setup(t)
	p := &toolStreamFake{}
	route := f.svc.AI.Routes["reply"]
	route.Provider = p
	f.svc.AI.Routes["reply"] = route
	th, err := f.svc.CreateThread(context.Background(), f.scope.ID, f.user, assistant.Context{})
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	out, err := f.svc.SendStream(context.Background(), f.caller(), th.UUID, "Kurulum", assistant.Context{}, func(name string, data any) error {
		if name == "status" {
			statuses = append(statuses, data.(map[string]string)["text"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Fatalf("extra model round: %d", p.calls)
	}
	if out.Usage.InputTokens != 20 || out.Reply.Content != "Kurulumunuz hazır. " || len(out.Reply.Suggestions) != 1 {
		t.Fatalf("lost output or usage: %+v", out)
	}
	if !strings.Contains(strings.Join(statuses, "|"), "Kurulum durumu kontrol ediliyor") {
		t.Fatalf("tool status missing: %v", statuses)
	}
}

func TestHandlerDisconnectPersistsPartial(t *testing.T) {
	f := setup(t)
	p := &streamingFake{wait: true, partial: make(chan struct{})}
	route := f.svc.AI.Routes["reply"]
	route.Provider = p
	f.svc.AI.Routes["reply"] = route
	th, err := f.svc.CreateThread(context.Background(), f.scope.ID, f.user, assistant.Context{})
	if err != nil {
		t.Fatal(err)
	}
	h := assistant.NewHandler(f.svc, nil, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("uuid", th.UUID.String())
		ctx := orgctx.WithScope(r.Context(), f.scope)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: f.user})
		h.SendStream(w, r.WithContext(ctx))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(`{"content":"Merhaba"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.partial:
	case <-ctx.Done():
		_ = resp.Body.Close()
		t.Fatal("provider did not stream")
	}
	cancel()
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		messages, e := f.svc.Messages(context.Background(), f.scope.ID, f.user, th.UUID)
		if e != nil {
			t.Fatal(e)
		}
		if len(messages) == 2 && messages[1].Interrupted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("handler disconnect did not save interrupted reply")
}

func TestSetupDismissalSkipsBothSequences(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "setup.channels", guides.Progress{Status: "skipped", StepIndex: 2, DismissSetup: true}); err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"web", "mobile"} {
		tour, err := f.svc.Guides.Setup(ctx, f.scope.ID, f.user, surface, "tr")
		if err != nil {
			t.Fatal(err)
		}
		if tour.Total == 0 || tour.Completed != tour.Total {
			t.Fatalf("%s setup not dismissed: %+v", surface, tour)
		}
		for _, g := range tour.Items {
			started, err := f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, g.Key)
			if err != nil || started {
				t.Fatalf("setup restarted: %s %v", g.Key, err)
			}
			if g.Key == "setup.channels" && g.Progress.StepIndex != 2 {
				t.Fatal("lost dismissed step")
			}
		}
	}
	if err := f.svc.Guides.Reset(ctx, f.scope.ID, f.user); err != nil {
		t.Fatal(err)
	}
	started, err := f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, "setup.channels")
	if err != nil || !started {
		t.Fatalf("reset not honored: %v %v", started, err)
	}
}

func TestSetupAutoStartClaimIsShared(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	started, err := f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, "setup.channels")
	if err != nil || !started {
		t.Fatalf("first setup: %v %v", started, err)
	}
	started, err = f.svc.Guides.ClaimAutoStart(ctx, f.scope.ID, f.user, "mobile.notifications")
	if err != nil || started {
		t.Fatalf("second surface repeated setup: %v %v", started, err)
	}
	tour, err := f.svc.Guides.Setup(ctx, f.scope.ID, f.user, "mobile", "tr")
	if err != nil || tour.AutoStart {
		t.Fatalf("setup still auto-starts: %v %v", tour.AutoStart, err)
	}
	if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "mobile.notifications", guides.Progress{Status: "in_progress"}); err != nil {
		t.Fatal("manual replay failed", err)
	}
}

func TestStreamProviderErrorEvent(t *testing.T) {
	f := setup(t)
	route := f.svc.AI.Routes["reply"]
	route.Provider = &streamingFake{failed: true}
	f.svc.AI.Routes["reply"] = route
	th, err := f.svc.CreateThread(context.Background(), f.scope.ID, f.user, assistant.Context{})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"content":"Merhaba"}`))
	r.SetPathValue("uuid", th.UUID.String())
	ctx := orgctx.WithScope(r.Context(), f.scope)
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: f.user})
	w := httptest.NewRecorder()
	assistant.NewHandler(f.svc, nil, nil).SendStream(w, r.WithContext(ctx))
	if !strings.Contains(w.Body.String(), "event: error") || !strings.Contains(w.Body.String(), assistant.CodeAIUnavailable) || strings.Contains(w.Body.String(), "event: done") {
		t.Fatal(w.Body.String())
	}
}

func TestDismissedGuideIgnoresDelayedStepWrite(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, progress := range []guides.Progress{{Status: "in_progress", StepIndex: 0}, {Status: "skipped", StepIndex: 2}, {Status: "in_progress", StepIndex: 1}} {
		if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "web.inbox", progress); err != nil {
			t.Fatal(err)
		}
	}
	items, err := f.svc.Guides.ForRoute(ctx, f.scope.ID, f.user, "mobile", "/inbox", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].Progress.Status != "skipped" || items[0].Progress.StepIndex != 2 {
		t.Fatalf("dismissal overwritten: %+v", items)
	}
	if err := f.svc.Guides.SaveProgress(ctx, f.scope.ID, f.user, "mobile.inbox", guides.Progress{Status: "in_progress", StepIndex: 0, Restart: true}); err != nil {
		t.Fatal(err)
	}
	items, err = f.svc.Guides.ForRoute(ctx, f.scope.ID, f.user, "web", "/inbox", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Progress.Status != "in_progress" {
		t.Fatal("manual replay blocked")
	}
}

func TestFirstExchangeGeneratesCheapTurkishTitle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	route := f.svc.AI.Routes["reply"]
	route.Provider = &streamingFake{}
	f.svc.AI.Routes["reply"] = route
	titles := &scripted{steps: []ai.Result{{Text: "Ekip kurulumu"}}}
	f.svc.AI.Routes["triage"] = ai.Route{Provider: titles, ProviderName: "fake", Model: "haiku-fake", InputPrice: 1, OutputPrice: 1}
	th, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, assistant.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SendStream(ctx, f.caller(), th.UUID, "Ekibimi kurmak istiyorum", assistant.Context{}, func(string, any) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		threads, e := f.svc.Threads(ctx, f.scope.ID, f.user)
		if e != nil {
			t.Fatal(e)
		}
		if len(threads) > 0 && threads[0].Title == "Ekip kurulumu" {
			titles.mu.Lock()
			defer titles.mu.Unlock()
			if len(titles.reqs) != 1 || titles.reqs[0].Effort != "low" || titles.reqs[0].MaxTokens != 64 {
				t.Fatal("title used an expensive route")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("asynchronous title was not saved")
}

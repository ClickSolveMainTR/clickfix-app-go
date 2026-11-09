package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	sysmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/systemstest"
	systems "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
)

// sequenceAI answers each call with the next response and records inputs.
type sequenceAI struct {
	mu        sync.Mutex
	responses []string
	inputs    []string
}

func (f *sequenceAI) Generate(_ context.Context, req ai.Request) (ai.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, req.Turns[0].Text)
	text := `{}`
	if len(f.responses) > 0 {
		text, f.responses = f.responses[0], f.responses[1:]
	}
	return ai.Result{Text: text, Provider: ai.ProviderAnthropic, Model: req.Model, FinalUsage: true, InputTokens: 12, OutputTokens: 5, Calls: []ai.ToolCall{}}, nil
}

func (f *sequenceAI) Stream(ctx context.Context, req ai.Request, _ ai.Delta) (ai.Result, error) {
	return f.Generate(ctx, req)
}

type noSources struct{}

func (noSources) Search(context.Context, int64, string, string, bool) ([]kbmodel.Citation, error) {
	return nil, nil
}

// CF-197 accounting scenario end to end: the organization defines the
// invoice concept, its endpoint, tries it, picks fields, defines an
// action and a plain-language playbook; then a customer known by
// external id writes from the widget's invoice screen and the AI
// classifies the free sentence onto the playbook, pulls the right
// concept for that customer and drafts a grounded reply.
func TestConnectedSystemAccountingEndToEnd(t *testing.T) {
	engine, _, org, actor := agentCoreFixture(t)
	ctx := context.Background()
	pool := engine.Repo.Billing.Pool
	engine.RAG = noSources{}
	fake := &sequenceAI{}
	for task, route := range engine.Routes {
		route.Provider = fake
		engine.Routes[task] = route
	}
	f := systemstest.SeedInto(t, ctx, pool, org, actor)
	acc := systemstest.NewAccounting(t)
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	sys := systems.New(db.New(pool), box, nil)
	sys.Caller = acc.Caller()
	sys.ValidateBaseURL = func(string) error { return nil }

	// 1. Connection, 2. concept endpoint, 3. "Dene", 4. pick fields.
	if _, err := sys.SaveConnection(ctx, org, actor, sysmodel.ConnectionInput{BaseURL: acc.URL + "/api", AuthType: sysmodel.AuthBearer, Secret: systemstest.Token}); err != nil {
		t.Fatal(err)
	}
	path, items := "/faturalar?musteri={external_id}", "data.faturalar"
	tried, err := sys.TestSource(ctx, org, actor, f.ObjectTypeUUID, sysmodel.TestInput{ExternalID: "CUST-42", PathTemplate: &path, ItemsPath: &items})
	if err != nil || !tried.OK {
		t.Fatalf("Dene: %v %+v", err, tried)
	}
	fields := []sysmodel.Field{}
	for _, opt := range tried.Paths {
		switch opt.Path {
		case "no":
			fields = append(fields, sysmodel.Field{Path: opt.Path, Label: "Fatura no"})
		case "durum":
			fields = append(fields, sysmodel.Field{Path: opt.Path, Label: "Durum"})
		case "hata.metin":
			fields = append(fields, sysmodel.Field{Path: opt.Path, Label: "Hata metni"})
		}
	}
	if len(fields) != 3 {
		t.Fatalf("fields not offered by the answer: %+v", tried.Paths)
	}
	if _, err := sys.SaveSource(ctx, org, f.ObjectTypeUUID, sysmodel.SourceInput{PathTemplate: path, ItemsPath: items, Fields: fields, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// 5. An action, 6. the playbook in plain words.
	if _, err := sys.SaveAction(ctx, org, "fatura_tekrar_gonder", sysmodel.ActionInput{Title: "Faturayı tekrar gönder", ConceptKey: "fatura", Method: "POST",
		PathTemplate: "/faturalar/{fatura_no}/tekrar-gonder", Params: []sysmodel.Param{{Key: "fatura_no", Label: "Fatura no", Type: "string", Required: true}},
		AllowedFor: []string{sysmodel.ChannelAI, sysmodel.ChannelAgent}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := sys.SavePlaybook(ctx, org, "fatura_gelmedi", sysmodel.PlaybookInput{Title: "Faturam gelmedi",
		Instructions: "Müşteri faturasının gitmediğini söylerse Fatura'ya bak; durum Hata ise hata metnini oku ve müşteriye açıkla.",
		ConceptKeys:  []string{"fatura"}, ActionKeys: []string{"fatura_tekrar_gonder"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// 7. The customer writes from the widget's invoice screen.
	attrs, _ := json.Marshal(map[string]any{"source": "widget", "page_url": "https://app.example.com/faturalar/F-2026-101?token=abc",
		"events": []map[string]any{{"name": "fatura_goruntulendi", "properties": map[string]any{"no": "F-2026-101"}}}})
	var msgUUID uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_contact_id,kind,status,content,content_attributes) VALUES($1,$2,'in','contact',$3,'text','received','Geçen ayki ödeme belgem bana hiç ulaşmadı',$4) RETURNING uuid",
		org, f.Conversation, f.Contact, attrs).Scan(&msgUUID); err != nil {
		t.Fatal(err)
	}
	fake.responses = []string{
		`{"language":"tr","intent":"fatura_gelmedi","category":"billing","sentiment":"neutral","urgency":"normal","priority":"normal","summary":"Fatura ulaşmamış","suggested_tags":[],"suggested_team":"","confidence":0.9}`,
		`{"reply":"F-2026-101 numaralı faturanız alıcı e-posta adresi reddettiği için gönderilemedi.","summary":"Fatura hatası açıklandı","confidence":0.95,"handoff":false,"citation_ids":[]}`,
	}
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	svc.Systems = sys
	cfg := model.DefaultConfig()
	cfg.Mode, cfg.TriageEnabled, cfg.Channels = "draft", true, []string{"widget"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.Inbound(ctx, org, msgUUID); err != nil {
		t.Fatal(err)
	}

	if len(fake.inputs) != 2 {
		t.Fatalf("model calls: %d", len(fake.inputs))
	}
	triage, reply := fake.inputs[0], fake.inputs[1]
	if !strings.Contains(triage, "fatura_gelmedi") || !strings.Contains(triage, "by meaning, not by keywords") {
		t.Fatalf("triage did not get the playbook intents: %s", triage)
	}
	for _, want := range []string{"F-2026-101", "Alıcı e-posta adresi reddetti", "Hata", "fatura_goruntulendi", "/faturalar/F-2026-101", "Müşteri faturasının gitmediğini"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply input lacks %q:\n%s", want, reply)
		}
	}
	for _, leak := range []string{"token=abc", "x-991", systemstest.Token} {
		if strings.Contains(reply, leak) {
			t.Fatalf("reply input leaks %q", leak)
		}
	}
	if len(acc.Queries) == 0 || acc.Queries[len(acc.Queries)-1] != "CUST-42" {
		t.Fatalf("concept not fetched by external id: %v", acc.Queries)
	}
	jobs, err := svc.Repo.Q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: f.ConvUUID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, j := range jobs {
		status[j.Task] = j.Status
	}
	if status["triage"] != "completed" || status["reply"] != "draft" {
		t.Fatalf("jobs: %+v", jobs)
	}

	// The AI-allowed action runs as a tool for this conversation's customer.
	out, err := sys.InvokeTool(ctx, org, f.Conversation, systems.ToolPrefix+"fatura_tekrar_gonder", json.RawMessage(`{"fatura_no":"F-2026-101"}`))
	if err != nil || !strings.Contains(out, `"ok":true`) || len(acc.Resent) != 1 {
		t.Fatalf("tool: %v %s %v", err, out, acc.Resent)
	}
}

package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	airepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type agentFakeAI struct {
	calls    atomic.Int32
	response string
	input    string
	err      error
	hook     func()
}

func (f *agentFakeAI) Generate(_ context.Context, req ai.Request) (ai.Result, error) {
	f.calls.Add(1)
	f.input = req.Turns[0].Text
	if f.hook != nil {
		f.hook()
	}
	return ai.Result{Text: f.response, Provider: ai.ProviderAnthropic, Model: req.Model, FinalUsage: true, InputTokens: 12, OutputTokens: 5, Calls: []ai.ToolCall{}}, f.err
}
func (f *agentFakeAI) Stream(ctx context.Context, req ai.Request, _ ai.Delta) (ai.Result, error) {
	return f.Generate(ctx, req)
}
func agentCoreFixture(t *testing.T) (*core.Service, *agentFakeAI, int64, int64) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var actor, org int64
	var orgUUID uuid.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','AI','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'AI test') RETURNING id,uuid", uuid.NewString()).Scan(&org, &orgUUID); err != nil {
		t.Fatal(err)
	}
	repo := billingrepo.New(pool, db.New(pool))
	billing := billingusecase.New(repo)
	plan, err := billing.Create(ctx, billingmodel.PlanInput{Code: "ai-" + uuid.NewString(), Name: "AI", Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	limit := int64(1000000)
	features, err := billing.Features(ctx)
	if err != nil {
		t.Fatal(err)
	}
	grants := []billingmodel.Grant{}
	for _, feature := range features {
		g := billingmodel.Grant{Feature: feature.Key, Enforcement: "hard", WarnPercent: 80}
		if feature.Kind == "boolean" {
			g.Enabled = &enabled
		} else {
			g.Limit = &limit
			if feature.Key == "ai.credits" {
				credits := int64(1000)
				g.Limit = &credits
			}
		}
		grants = append(grants, g)
	}
	version, err := billing.SaveVersion(ctx, plan.UUID, uuid.Nil, billingmodel.VersionInput{Prices: []billingmodel.Price{{Interval: "monthly", Currency: "TRY", Amount: 10000, VAT: 2000}}, Entitlements: grants})
	if err != nil {
		t.Fatal(err)
	}
	version, err = billing.Publish(ctx, plan.UUID, version.UUID, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = billing.CreateSubscription(ctx, billingmodel.CreateSubscription{OrganizationUUID: orgUUID, VersionUUID: version.UUID, PriceUUID: version.Prices[0].UUID, Status: "active", PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now().AddDate(0, 1, 0)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	ent := entitlements.New(repo, nil, "ai-test")
	store := airepo.New(repo, ent)
	_, err = store.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true, DailyCalls: 100, Concurrency: 1, DailyBudgetMicros: 10000000})
	if err != nil {
		t.Fatal(err)
	}
	fake := &agentFakeAI{}
	routes := map[string]ai.Route{}
	for _, task := range []string{"triage", "reply", "complex"} {
		routes[task] = ai.Route{Provider: fake, ProviderName: ai.ProviderAnthropic, Model: ai.ModelForTask(task), InputPrice: 1000000, OutputPrice: 2000000}
	}
	return &core.Service{Repo: store, Routes: routes}, fake, org, actor
}

func conversationFixture(t *testing.T, svc *core.Service, org int64) (db.Conversation, db.Message) {
	t.Helper()
	ctx := context.Background()
	pool := svc.Repo.Billing.Pool
	var contact int64
	if err := pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Customer') RETURNING id", org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	var convUUID uuid.UUID
	var convID int64
	if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_type,source) VALUES($1,1,$2,'whatsapp','inbound') RETURNING id,uuid", org, contact).Scan(&convID, &convUUID); err != nil {
		t.Fatal(err)
	}
	var message uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_contact_id,kind,status,content) VALUES($1,$2,'in','contact',$3,'text','received','Question from alice@example.com') RETURNING uuid", org, convID, contact).Scan(&message); err != nil {
		t.Fatal(err)
	}
	c, err := svc.Repo.Billing.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: convUUID})
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.Repo.Billing.Q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: org, Uuid: message})
	if err != nil {
		t.Fatal(err)
	}
	return c, m
}
func TestTriageUsesCheapProviderCreditsAndDedup(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	conv, msg := conversationFixture(t, engine, org)
	fake.response = `{"language":"en","intent":"billing","category":"payments","sentiment":"neutral","urgency":"normal","priority":"normal","summary":"Payment question","suggested_tags":["billing"],"suggested_team":"Finance","confidence":0.9}`
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	cfg := model.DefaultConfig()
	cfg.TriageEnabled = true
	if _, err := svc.Repo.SaveConfig(context.Background(), org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.Inbound(context.Background(), org, msg.Uuid); err != nil {
			t.Fatal(err)
		}
	}
	if fake.calls.Load() != 1 || strings.Contains(fake.input, "alice@example.com") {
		t.Fatalf("calls=%d input=%s", fake.calls.Load(), fake.input)
	}
	jobs, err := svc.Repo.Q.AIAgentJobs(context.Background(), db.AIAgentJobsParams{OrganizationID: org, Uuid: conv.Uuid, Limit: 20})
	if err != nil || len(jobs) != 1 || jobs[0].Status != "completed" || !jobs[0].RunUuid.Valid {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
	balance, err := engine.Repo.Billing.Q.AICreditBalance(context.Background(), org)
	if err != nil || balance != 999 {
		t.Fatalf("balance=%d err=%v", balance, err)
	}
}

type fakeSources struct{ citation kbmodel.Citation }

func (f fakeSources) Search(context.Context, int64, string, string, bool) ([]kbmodel.Citation, error) {
	return []kbmodel.Citation{f.citation}, nil
}

type fakeBot struct {
	calls int
	err   error
}

func (f *fakeBot) PostAI(context.Context, int64, uuid.UUID, string, int32) (uuid.UUID, error) {
	f.calls++
	return uuid.New(), f.err
}
func TestReplyThresholdSourcesAndPolicyChanges(t *testing.T) {
	for _, tc := range []struct {
		name            string
		confidence      float64
		foreignCitation bool
		human           bool
		policyChange    bool
		want            string
		sent            int
	}{
		{name: "grounded automatic reply", confidence: 0.95, want: "completed", sent: 1},
		{name: "low confidence", confidence: 0.2, want: "handoff"},
		{name: "invented citation", confidence: 0.95, foreignCitation: true, want: "handoff"},
		{name: "human intervened", confidence: 0.95, human: true, want: "handoff", sent: 1},
		{name: "policy disabled during call", confidence: 0.95, policyChange: true, want: "handoff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, fake, org, actor := agentCoreFixture(t)
			c, msg := conversationFixture(t, engine, org)
			source := kbmodel.Citation{ChunkUUID: uuid.New(), ArticleUUID: uuid.New(), Version: 1, Content: "Supported answer", Locale: "tr"}
			engine.RAG = fakeSources{citation: source}
			bot := &fakeBot{}
			if tc.human {
				bot.err = errors.New("human wrote")
			}
			svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, bot)
			cfg := model.DefaultConfig()
			cfg.Mode = "autonomous"
			cfg.Channels = []string{"whatsapp"}
			if _, err := svc.Repo.SaveConfig(context.Background(), org, actor, cfg); err != nil {
				t.Fatal(err)
			}
			citation := source.ChunkUUID
			if tc.foreignCitation {
				citation = uuid.New()
			}
			raw, _ := json.Marshal(model.Reply{Text: "Supported answer", Summary: "Question answered", Confidence: tc.confidence, Citations: []uuid.UUID{citation}})
			fake.response = string(raw)
			if tc.policyChange {
				fake.hook = func() {
					cfg.Mode = "off"
					if _, err := svc.Repo.SaveConfig(context.Background(), org, actor, cfg); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := svc.Inbound(context.Background(), org, msg.Uuid); err != nil {
				t.Fatal(err)
			}
			rows, err := svc.Repo.Q.AIAgentJobs(context.Background(), db.AIAgentJobsParams{OrganizationID: org, Uuid: c.Uuid, Limit: 20})
			if err != nil || len(rows) != 1 || rows[0].Status != tc.want || bot.calls != tc.sent {
				t.Fatalf("jobs=%+v calls=%d err=%v", rows, bot.calls, err)
			}
		})
	}
}

func TestToolApprovalDryRunAndCrossTenant(t *testing.T) {
	engine, _, org, actor := agentCoreFixture(t)
	c, _ := conversationFixture(t, engine, org)
	box, err := crypto.NewSecretBox("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	tools := &ToolService{Repo: &repository.Store{Repository: engine.Repo.Billing}, Core: engine, Box: box}
	ctx := context.Background()
	invoke := Invocation{Name: "create_note", Arguments: json.RawMessage(`{"text":"Approved note"}`), Key: uuid.New()}
	if _, err = tools.Invoke(ctx, org, 0, c.Uuid, invoke); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("model approval bypass: %v", err)
	}
	invoke.DryRun = true
	if _, err = tools.Invoke(ctx, org, actor, c.Uuid, invoke); err != nil {
		t.Fatal(err)
	}
	var notes int
	if err = engine.Repo.Billing.Pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE organization_id=$1 AND conversation_id=$2 AND private", org, c.ID).Scan(&notes); err != nil || notes != 0 {
		t.Fatalf("dry run mutated %d %v", notes, err)
	}
	if _, err = tools.Invoke(ctx, org, actor, c.Uuid, invoke); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("replay: %v", err)
	}
	invoke.Key = uuid.New()
	if _, err = tools.Invoke(ctx, org+100000, actor, c.Uuid, invoke); err == nil {
		t.Fatal("cross-tenant tool executed")
	}
	_, err = tools.Save(ctx, org, ToolInput{Name: "order_lookup", URL: "https://127.0.0.1/private", Schema: objectSchema(map[string]any{}), DailyLimit: 10})
	if !errors.Is(err, model.ErrInvalid) {
		t.Fatal("private tool URL accepted")
	}
}

func TestCopilotOperationAndOrganizationScope(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	c, _ := conversationFixture(t, engine, org)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	fake.response = `{"text":"Short customer summary"}`
	result, err := svc.Copilot(context.Background(), org, actor, c.Uuid, model.CopilotInput{Operation: "summarize", Locale: "en"})
	if err != nil || result.Credits != 1 || result.Text != "Short customer summary" {
		t.Fatalf("result=%+v %v", result, err)
	}
	if _, err = svc.Copilot(context.Background(), org+100000, actor, c.Uuid, model.CopilotInput{Operation: "summarize"}); err == nil {
		t.Fatal("foreign conversation read")
	}
	if _, err = svc.Copilot(context.Background(), org, actor, c.Uuid, model.CopilotInput{Operation: "execute_payment"}); !errors.Is(err, model.ErrInvalid) {
		t.Fatal("unknown operation accepted")
	}
	// Schema-invalid model output is rejected while provider usage stays charged.
	fake.response = `{"language":42}`
	_, err = engine.RunAgent(context.Background(), org, actor, coremodel.RunInput{Task: "triage", Input: "question"}, core.AgentOptions{Schema: triageSchema})
	if !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("invalid output: %v", err)
	}
}

func TestProcedurePublishedSnapshotAndOptimisticLock(t *testing.T) {
	engine, _, org, _ := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	in := Procedure{Intent: "refund", Locale: "tr", Title: "Refund procedure", Steps: []string{"Ask for order number", "Hand off after 14 days"}, Tools: []string{"lookup_contact"}}
	row, err := svc.SaveProcedure(ctx, org, uuid.Nil, in)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.Repo.Q.AIProcedurePublish(ctx, db.AIProcedurePublishParams{OrganizationID: org, Uuid: row.Uuid, ExpectedRevision: row.Revision, Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	in.Revision = row.Revision
	in.Steps = []string{"Different draft instructions"}
	edited, err := svc.SaveProcedure(ctx, org, row.Uuid, in)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Revision != 2 || string(edited.Published) != string(published.Published) {
		t.Fatal("editing draft changed published procedure")
	}
	if _, err = svc.SaveProcedure(ctx, org, row.Uuid, in); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("stale write: %v", err)
	}
	if _, err = svc.Repo.Q.AIProcedurePublished(ctx, db.AIProcedurePublishedParams{OrganizationID: org + 100000, Intent: "refund", Locale: "tr"}); err == nil {
		t.Fatal("foreign procedure read")
	}
}

func TestAIResolutionDefinitionAndIdempotency(t *testing.T) {
	engine, _, org, _ := agentCoreFixture(t)
	c, msg := conversationFixture(t, engine, org)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	pool := engine.Repo.Billing.Pool
	if _, err := pool.Exec(ctx, "INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task,status) VALUES($1,$2,$3,'reply','completed')", org, c.ID, msg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE conversations SET status='resolved' WHERE organization_id=$1 AND id=$2", org, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.recordResolution(ctx, org, c.Uuid); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ai_resolutions WHERE organization_id=$1", org).Scan(&count); err != nil || count != 0 {
		t.Fatalf("undelivered counted: %d %v", count, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'out','bot','sent','Answer')", org, c.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.recordResolution(ctx, org, c.Uuid); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ai_resolutions WHERE organization_id=$1", org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestOrganizationToolAuthEncryptedAndDryRunNoNetwork(t *testing.T) {
	engine, _, org, actor := agentCoreFixture(t)
	c, _ := conversationFixture(t, engine, org)
	box, err := crypto.NewSecretBox("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	tools := &ToolService{Repo: &repository.Store{Repository: engine.Repo.Billing}, Core: engine, Box: box}
	ctx := context.Background()
	_, err = tools.Save(ctx, org, ToolInput{Name: "order_lookup", Description: "Order lookup", URL: "https://orders.example.com/lookup", Schema: objectSchema(map[string]any{"number": textSchema(100)}), Bearer: "fake-bearer-secret", HMAC: "fake-hmac-secret", RequiresApproval: true, Enabled: true, DailyLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := tools.Repo.Q.AIToolByName(ctx, db.AIToolByNameParams{OrganizationID: org, Name: "order_lookup"})
	if err != nil || strings.Contains(saved.AuthEnc, "fake-") {
		t.Fatalf("auth not encrypted: %v", err)
	}
	result, err := tools.Invoke(ctx, org, actor, c.Uuid, Invocation{Name: "order_lookup", Arguments: json.RawMessage(`{"number":"123"}`), Key: uuid.New(), DryRun: true})
	if err != nil || !strings.Contains(result, "no action executed") {
		t.Fatalf("dry run=%s err=%v", result, err)
	}
}

package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	memmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	"github.com/google/uuid"
)

// fakeMemory is the knowledge layer seen from the AI agent (CF-216).
type fakeMemory struct {
	mu      sync.Mutex
	result  memmodel.Result
	lookups []memmodel.Query
	gaps    []memmodel.Query
}

func (f *fakeMemory) Ready() bool { return true }
func (f *fakeMemory) Lookup(_ context.Context, q memmodel.Query) (memmodel.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, q)
	return f.result, nil
}
func (f *fakeMemory) RecordGap(_ context.Context, q memmodel.Query, _ int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gaps = append(f.gaps, q)
	return nil
}

func TestReplyFromKnowledgeUnitSkipsGeneration(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	c, msg := conversationFixture(t, engine, org)
	bot := &fakeBot{}
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, bot)
	unit := uuid.New()
	memory := &fakeMemory{result: memmodel.Result{Outcome: memmodel.OutcomeExact, Unit: &memmodel.UnitHit{UUID: unit, Answer: "Siparişler 2 iş gününde teslim edilir.", Visibility: "public", Score: 1}}}
	svc.Memory = memory
	cfg := model.DefaultConfig()
	cfg.Mode = "autonomous"
	cfg.Channels = []string{"whatsapp"}
	if _, err := svc.Repo.SaveConfig(context.Background(), org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.Inbound(context.Background(), org, msg.Uuid); err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 0 || bot.calls != 1 {
		t.Fatalf("generation calls=%d sent=%d", fake.calls.Load(), bot.calls)
	}
	if len(memory.lookups) != 1 || memory.lookups[0].Audience != memmodel.AudienceCustomer || memory.lookups[0].ExcludeConversationID != c.ID {
		t.Fatalf("lookups=%+v", memory.lookups)
	}
	jobs, err := svc.Repo.Q.AIAgentJobs(context.Background(), db.AIAgentJobsParams{OrganizationID: org, Uuid: c.Uuid, Limit: 20})
	if err != nil || len(jobs) != 1 || jobs[0].Status != "completed" {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
	var reply model.Reply
	if err = json.Unmarshal(jobs[0].Result, &reply); err != nil || reply.UnitUUID == nil || *reply.UnitUUID != unit || reply.Memory != memmodel.OutcomeExact {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
}

func TestReplyMissUsesCasesAndRecordsGap(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	_, msg := conversationFixture(t, engine, org)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	memory := &fakeMemory{result: memmodel.Result{Outcome: memmodel.OutcomeMiss, CustomerCases: []memmodel.CustomerCase{{Problem: "Portala giriş yapılamıyor", Resolution: "Önbellek temizlendi"}}}}
	svc.Memory = memory
	engine.RAG = noSources{}
	cfg := model.DefaultConfig()
	cfg.Mode = "draft"
	cfg.Channels = []string{"whatsapp"}
	if _, err := svc.Repo.SaveConfig(context.Background(), org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	// The model answers without a valid source: a knowledge gap.
	fake.response = `{"reply":"Bilmiyorum","summary":"","confidence":0.9,"handoff":false,"citation_ids":[]}`
	if err := svc.Inbound(context.Background(), org, msg.Uuid); err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 || !strings.Contains(fake.input, "Önbellek temizlendi") {
		t.Fatalf("calls=%d case hint missing from input", fake.calls.Load())
	}
	if len(memory.gaps) != 1 || memory.gaps[0].Audience != memmodel.AudienceCustomer {
		t.Fatalf("gaps=%+v", memory.gaps)
	}
}

func TestCopilotAnswersFromKnowledgeUnit(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	c, _ := conversationFixture(t, engine, org)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	memory := &fakeMemory{result: memmodel.Result{Outcome: memmodel.OutcomeSemantic, Unit: &memmodel.UnitHit{UUID: uuid.New(), Answer: "İç not: 45 güne kadar.", Visibility: "internal"}}}
	svc.Memory = memory
	out, err := svc.Copilot(context.Background(), org, actor, c.Uuid, model.CopilotInput{Operation: "suggest_reply", Locale: "tr"})
	if err != nil || out.Text != "İç not: 45 güne kadar." || fake.calls.Load() != 0 {
		t.Fatalf("out=%+v calls=%d err=%v", out, fake.calls.Load(), err)
	}
	// suggest_reply looks up the customer's latest message, as the agent.
	if len(memory.lookups) != 1 || memory.lookups[0].Audience != memmodel.AudienceAgent || !strings.Contains(memory.lookups[0].Question, "Question from") {
		t.Fatalf("lookups=%+v", memory.lookups)
	}
	// A miss with no source and no case records a gap for the agent side.
	memory.result = memmodel.Result{Outcome: memmodel.OutcomeMiss}
	engine.RAG = noSources{}
	fake.response = `{"text":"Taslak"}`
	if _, err = svc.Copilot(context.Background(), org, actor, c.Uuid, model.CopilotInput{Operation: "ask", Text: "Toplu fatura nasıl kesilir?", Locale: "tr"}); err != nil {
		t.Fatal(err)
	}
	if len(memory.gaps) != 1 || memory.gaps[0].Audience != memmodel.AudienceAgent || memory.gaps[0].Question != "Toplu fatura nasıl kesilir?" {
		t.Fatalf("gaps=%+v", memory.gaps)
	}
}

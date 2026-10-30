package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kb "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestBusinessContextVersionsIsolationAndPublicReplyExclusion(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	engine.Context = svc
	ctx := context.Background()
	row, err := svc.SaveBusinessContext(ctx, org, actor, uuid.Nil, BusinessContextInput{Kind: "pricing", Title: "Private policy", Body: "Never disclose internal margin 73 percent", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveBusinessContext(ctx, org, actor, row.Uuid, BusinessContextInput{Kind: "pricing", Title: "Private policy", Body: "New policy", Active: true, Revision: 0}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("stale revision %v", err)
	}
	if _, err = svc.SaveBusinessContext(ctx, org+999999, actor, row.Uuid, BusinessContextInput{Kind: "pricing", Title: "Private policy", Body: "Stolen", Active: true, Revision: 1}); err == nil {
		t.Fatal("cross-org update succeeded")
	}
	versions, err := svc.Repo.Q.BusinessContextVersions(ctx, db.BusinessContextVersionsParams{OrganizationID: org, Uuid: row.Uuid, Limit: 10})
	if err != nil || len(versions) != 1 || versions[0].Revision != 1 {
		t.Fatalf("history %v %v", versions, err)
	}
	instructions := ""
	capture := &captureProvider{base: fake, capture: &instructions}
	for key, route := range engine.Routes {
		route.Provider = capture
		engine.Routes[key] = route
	}
	fake.response = "Internal analysis"
	if _, err = engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "complex", Input: "Analyze organization"}, core.AgentOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, "internal margin 73") {
		t.Fatal("private context not loaded for internal analysis")
	}
	if _, err = engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "reply", Input: "Public answer"}, core.AgentOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(instructions, "internal margin 73") {
		t.Fatal("private context entered public reply instructions")
	}
}

type captureProvider struct {
	base    *agentFakeAI
	capture *string
}

func (p *captureProvider) Generate(ctx context.Context, in ai.Request) (ai.Result, error) {
	*p.capture = in.Instructions
	return p.base.Generate(ctx, in)
}
func (p *captureProvider) Stream(ctx context.Context, in ai.Request, _ ai.Delta) (ai.Result, error) {
	return p.Generate(ctx, in)
}

func TestEvaluationRegressionSnapshotsAndNoConversationWrites(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	ctx := context.Background()
	source := kbmodel.Citation{ChunkUUID: uuid.New(), Content: "Refund within 14 days", Locale: "en"}
	engine.RAG = fakeSources{citation: source}
	cfg := model.DefaultConfig()
	cfg.Mode = "draft"
	cfg.Channels = []string{"whatsapp"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	c, err := svc.SaveEvaluationCase(ctx, org, actor, EvaluationCaseInput{Name: "Refund", Channel: "whatsapp", Transcript: "When can I return an item?", Locale: "en", Expected: Expectations{Contains: []string{"14 days"}}})
	if err != nil {
		t.Fatal(err)
	}
	fake.response = `{"reply":"Return within 14 days","summary":"Refund","confidence":0.95,"handoff":false,"citation_ids":["` + source.ChunkUUID.String() + `"]}`
	baseline, err := svc.Evaluate(ctx, org, actor, EvaluationInput{})
	if err != nil {
		t.Fatal(err)
	}
	fake.response = `{"reply":"Ask support","summary":"Refund","confidence":0.95,"handoff":false,"citation_ids":["` + source.ChunkUUID.String() + `"]}`
	run, err := svc.Evaluate(ctx, org, actor, EvaluationInput{Baseline: &baseline.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	var report EvaluationReport
	if err = json.Unmarshal(run.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.MeanScore != 0 || len(report.Regressions) != 1 || report.Regressions[0] != c.Uuid {
		t.Fatalf("regression %+v", report)
	}
	if _, err = svc.Evaluate(ctx, org+999999, actor, EvaluationInput{Baseline: &baseline.Uuid}); err == nil {
		t.Fatal("foreign evaluation read")
	}
	var count int
	if err = engine.Repo.Billing.Pool.QueryRow(ctx, "SELECT count(*) FROM conversations WHERE organization_id=$1", org).Scan(&count); err != nil || count != 0 {
		t.Fatalf("simulation wrote live conversations %d %v", count, err)
	}
}

func TestSourceImportIdempotencyManualEditAndPrivateRAG(t *testing.T) {
	engine, _, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	sources := &SourceService{Agents: svc}
	source, err := sources.CreateURL(ctx, org, actor, SourceInput{URL: "https://docs.example.com/guide", Title: "Guide", Locale: "en", IntervalHours: 24, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	doc := CrawledDocument{Location: "https://docs.example.com/guide", Title: "Guide", Body: "Reusable support instructions"}
	for range 2 {
		if err = sources.importDocument(ctx, source, doc); err != nil {
			t.Fatal(err)
		}
	}
	old, err := svc.Repo.Q.AIKnowledgeSourceDocument(ctx, db.AIKnowledgeSourceDocumentParams{OrganizationID: org, SourceID: source.ID, Location: doc.Location})
	if err != nil || old.ArticleVersion != 1 {
		t.Fatalf("source version %+v %v", old, err)
	}
	article, err := svc.Repo.Q.KnowledgeSourceArticleLock(ctx, db.KnowledgeSourceArticleLockParams{OrganizationID: org, ID: old.ArticleID})
	if err != nil {
		t.Fatal(err)
	}
	if article.Visibility != "internal" || !article.PublishedVersion.Valid {
		t.Fatal("source not indexed as internal snapshot")
	}
	knowledge := kb.New(kbrepo.New(engine.Repo.Billing.Pool))
	_, err = knowledge.SaveArticle(ctx, org, actor, article.Uuid, kbmodel.ArticleInput{Slug: article.Slug, Locale: "en", Title: "Manual change", Body: "Editor content", Visibility: "internal", Version: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	doc.Body = "Changed source"
	if err = sources.importDocument(ctx, source, doc); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("manual edit overwritten: %v", err)
	}
	var events int
	if err = engine.Repo.Billing.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='knowledge.article_published'", org).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("expected one index event, got %d", events)
	}
	if _, err = svc.Repo.Q.AIKnowledgeSourceGet(ctx, db.AIKnowledgeSourceGetParams{OrganizationID: org + 999999, Uuid: source.Uuid}); err == nil {
		t.Fatal("cross-org source read")
	}
}

func TestTranslationStalenessAndSuggestionApproval(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	knowledge := kb.New(kbrepo.New(engine.Repo.Billing.Pool))
	a, err := knowledge.SaveArticle(ctx, org, actor, uuid.New(), kbmodel.ArticleInput{Slug: "original", Locale: "tr", Title: "İade", Body: "14 gün", Visibility: "public"}, true)
	if err != nil {
		t.Fatal(err)
	}
	fake.response = `{"title":"Refund","body":"14 days"}`
	translated, err := svc.TranslateArticle(ctx, org, actor, a.Uuid, "en")
	if err != nil {
		t.Fatal(err)
	}
	if translated.PublishedVersion.Valid {
		t.Fatal("translation published without review")
	}
	_, err = knowledge.SaveArticle(ctx, org, actor, a.Uuid, kbmodel.ArticleInput{Slug: a.Slug, Locale: a.Locale, Title: a.Title, Body: "30 gün", Visibility: "public", Version: a.Version}, false)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = svc.SweepKnowledgeReviews(ctx, org, 6); err != nil {
			t.Fatal(err)
		}
	}
	reviews, err := svc.Repo.Q.KnowledgeReviews(ctx, db.KnowledgeReviewsParams{OrganizationID: org, Limit: 20})
	if err != nil || len(reviews) != 1 || reviews[0].Reason != "translation_outdated" || reviews[0].OwnerID != actor {
		t.Fatalf("reviews %v %v", reviews, err)
	}
	conversation, _ := conversationFixture(t, engine, org)
	if _, err = engine.Repo.Billing.Pool.Exec(ctx, "UPDATE conversations SET status='resolved',subject='Returns' WHERE organization_id=$1 AND id=$2", org, conversation.ID); err != nil {
		t.Fatal(err)
	}
	suggestion, err := svc.SuggestKnowledge(ctx, org, actor, conversation.Uuid, "en")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := svc.ReviewSuggestion(ctx, org, actor, suggestion.Uuid, true, "en")
	if err != nil || !approved.ArticleID.Valid || approved.Status != "approved" {
		t.Fatalf("approval %+v %v", approved, err)
	}
	if _, err = svc.ReviewSuggestion(ctx, org, actor, suggestion.Uuid, true, "en"); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("duplicate approval %v", err)
	}
}

func TestPerformanceEmptyAndConversationDenominator(t *testing.T) {
	engine, _, org, _ := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	now := time.Now()
	report, err := svc.Performance(ctx, org, now.Add(-time.Hour), now, 10)
	if err != nil || report.CSAT != nil || report.CostPerResolutionMicros != nil {
		t.Fatalf("empty report %+v %v", report, err)
	}
	c, msg := conversationFixture(t, engine, org)
	if _, err = engine.Repo.Billing.Pool.Exec(ctx, "INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task,status,reason) VALUES($1,$2,$3,'reply','handoff','Insufficient knowledge')", org, c.ID, msg.ID); err != nil {
		t.Fatal(err)
	}
	report, err = svc.Performance(ctx, org, now.Add(-time.Hour), time.Now(), 10)
	if err != nil || report.Conversations != 1 || report.HandoffRate != 1 || report.Resolutions != 0 {
		t.Fatalf("report %+v %v", report, err)
	}
	gaps, err := svc.Repo.Q.AIKnowledgeGapTopics(ctx, db.AIKnowledgeGapTopicsParams{OrganizationID: org, Since: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}, Limit: 20})
	if err != nil || len(gaps) != 1 || gaps[0].Conversations != 1 {
		t.Fatalf("gaps %+v %v", gaps, err)
	}
}

type fakeIssueGateway struct {
	organization int64
	created      int
	linked       int
	issue        uuid.UUID
}

func (f *fakeIssueGateway) MatchOpen(_ context.Context, org int64, _ string) ([]IssueMatch, error) {
	f.organization = org
	return []IssueMatch{{UUID: f.issue, Title: "Open payment incident", Score: 0.9}}, nil
}
func (f *fakeIssueGateway) Create(_ context.Context, org, _ int64, _ uuid.UUID, _, _ string) (uuid.UUID, error) {
	f.organization = org
	f.created++
	return f.issue, nil
}
func (f *fakeIssueGateway) Link(_ context.Context, org, _ int64, _, _ uuid.UUID) error {
	f.organization = org
	f.linked++
	return nil
}
func TestTriageSimilarLinkAndIssueGatewayApproval(t *testing.T) {
	engine, _, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	c, msg := conversationFixture(t, engine, org)
	pool := engine.Repo.Billing.Pool
	var other int64
	if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_type,source,subject) VALUES($1,2,$2,'web','inbound','Payment failure') RETURNING id", org, c.ContactID).Scan(&other); err != nil {
		t.Fatal(err)
	}
	c.Subject = "Payment"
	gateway := &fakeIssueGateway{issue: uuid.New()}
	svc.Tools = &ToolService{Repo: svc.Repo, Issues: gateway}
	similar, issues, err := svc.enrichTriage(ctx, org, c, "Payment")
	if err != nil || len(similar) != 1 || len(issues) != 1 || gateway.organization != org {
		t.Fatalf("matching %v %v %v", similar, issues, err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM ai_conversation_links WHERE organization_id=$1 AND conversation_id=$2 AND similar_id=$3", org, c.ID, other).Scan(&count); err != nil || count != 1 {
		t.Fatalf("similar link %d %v", count, err)
	}
	invocation := Invocation{Name: "create_issue", Key: uuid.New(), Arguments: json.RawMessage(`{"title":"Payment failure","body":"Observed support problem"}`)}
	if _, err = svc.Tools.Invoke(ctx, org, 0, c.Uuid, invocation); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("AI approved issue creation %v", err)
	}
	invocation.Approve = true
	if _, err = svc.Tools.Invoke(ctx, org, actor, c.Uuid, invocation); err != nil || gateway.created != 1 {
		t.Fatalf("approved issue creation %v", err)
	}
	if _, err = svc.Tools.Invoke(ctx, org, actor, c.Uuid, invocation); !errors.Is(err, model.ErrConflict) || gateway.created != 1 {
		t.Fatal("issue invocation replayed")
	}
	_ = msg
}

func TestPerformanceCSATUsesOnlyAIResolvedConversations(t *testing.T) {
	engine, _, org, _ := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	c, msg := conversationFixture(t, engine, org)
	pool := engine.Repo.Billing.Pool
	if _, err := pool.Exec(ctx, "INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task,status) VALUES($1,$2,$3,'reply','completed')", org, c.ID, msg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE conversations SET status='resolved' WHERE organization_id=$1 AND id=$2", org, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'out','bot','sent','Answer')", org, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.recordResolution(ctx, org, c.Uuid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO conversation_ratings(organization_id,conversation_id,score) VALUES($1,$2,5)", org, c.ID); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Performance(ctx, org, time.Now().Add(-time.Hour), time.Now(), 12)
	if err != nil || report.CSAT == nil || *report.CSAT != 5 || report.CSATSamples != 1 || report.ResolutionRate != 1 || report.EstimatedSavedHours != 0.2 {
		t.Fatalf("AI CSAT %+v %v", report, err)
	}
}

func TestCopilotAndGoldenCaptureRespectConversationVisibility(t *testing.T) {
	engine, fake, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	c, _ := conversationFixture(t, engine, org)
	if _, err := engine.Repo.Billing.Pool.Exec(ctx, "UPDATE conversations SET assignee_user_id=$3 WHERE organization_id=$1 AND id=$2", org, c.ID, actor); err != nil {
		t.Fatal(err)
	}
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: actor + 999999})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantConversationsRead}})
	if _, err := svc.Copilot(ctx, org, actor+999999, c.Uuid, model.CopilotInput{Operation: "summarize", Locale: "en"}); err == nil {
		t.Fatal("hidden conversation reached copilot")
	}
	if _, err := svc.SaveEvaluationCase(ctx, org, actor+999999, EvaluationCaseInput{Name: "Hidden", Conversation: &c.Uuid, Locale: "en", Expected: Expectations{Contains: []string{"answer"}}}); err == nil {
		t.Fatal("hidden conversation reached golden set")
	}
	if fake.calls.Load() != 0 {
		t.Fatal("provider called with hidden conversation")
	}
}

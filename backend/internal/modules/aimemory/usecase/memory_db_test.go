package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeEngine stands in for the AI core: it answers by purpose and counts calls.
type fakeEngine struct {
	calls  atomic.Int32
	units  string
	note   string
	inputs []string
}

func (f *fakeEngine) RunAgent(_ context.Context, _ int64, _ int64, in coremodel.RunInput, o core.AgentOptions) (core.RunResult, error) {
	f.calls.Add(1)
	f.inputs = append(f.inputs, in.Input)
	text := f.units
	if o.Purpose == "case_memory" {
		text = f.note
	}
	return core.RunResult{Result: ai.Result{Text: text}}, nil
}

// bagEmbedder is a deterministic bag-of-words embedder: equal word sets give
// cosine 1, unrelated texts give ~0.
type bagEmbedder struct{ calls atomic.Int32 }

func (b *bagEmbedder) Model() string { return "bag-test" }
func (b *bagEmbedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	b.calls.Add(1)
	out := make([][]float32, len(in))
	for i, s := range in {
		v := make([]float32, ai.EmbeddingDimensions)
		for _, w := range strings.Fields(Normalize(s)) {
			h := fnv.New32a()
			_, _ = h.Write([]byte(w))
			v[h.Sum32()%ai.EmbeddingDimensions]++
		}
		v[ai.EmbeddingDimensions-1] += 0.01 // never a zero vector
		out[i] = v
	}
	return ai.Embeddings{Vectors: out, InputTokens: int64(len(in))}, nil
}

type fixture struct {
	pool   *pgxpool.Pool
	svc    *Service
	engine *fakeEngine
	embed  *bagEmbedder
	org    int64
	user   int64
	brandA uuid.UUID
	brandB uuid.UUID
}

func newFixture(t *testing.T) *fixture {
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
	f := &fixture{pool: pool, engine: &fakeEngine{}, embed: &bagEmbedder{}}
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','Memory','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.user); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Memory test') RETURNING id", uuid.NewString()).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	for _, b := range []*uuid.UUID{&f.brandA, &f.brandB} {
		if err = pool.QueryRow(ctx, "INSERT INTO brands(organization_id,name) VALUES($1,$2) RETURNING uuid", f.org, "Brand "+uuid.NewString()[:8]).Scan(b); err != nil {
			t.Fatal(err)
		}
	}
	f.svc = New(pool, f.engine, f.embed)
	return f
}

// article inserts and publishes an article version.
func (f *fixture) article(t *testing.T, brand *uuid.UUID, visibility, body string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id int64
	var u uuid.UUID
	b := pgtype.UUID{}
	if brand != nil {
		b = pgtype.UUID{Bytes: *brand, Valid: true}
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO knowledge_articles(organization_id,slug,locale,title,body,visibility,author_id,brand_id) VALUES($1,$2,'tr','Makale',$3,$4,$5,$6) RETURNING id,uuid", f.org, "a-"+uuid.NewString()[:8], body, visibility, f.user, b).Scan(&id, &u); err != nil {
		t.Fatal(err)
	}
	f.publish(t, id, 1, body, visibility)
	return u
}

func (f *fixture) publish(t *testing.T, id int64, version int, body, visibility string) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, "INSERT INTO knowledge_article_versions(organization_id,article_id,version,title,body,visibility,author_id) VALUES($1,$2,$3,'Makale',$4,$5,$6)", f.org, id, version, body, visibility, f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE knowledge_articles SET version=$3,published_version=$3,body=$4 WHERE organization_id=$1 AND id=$2", f.org, id, version, body); err != nil {
		t.Fatal(err)
	}
}

func unitsJSON(units ...Unit) string {
	raw, _ := json.Marshal(map[string]any{"units": units})
	return string(raw)
}

func (f *fixture) lookup(t *testing.T, audience string, brand *uuid.UUID, question string) model.Result {
	t.Helper()
	out, err := f.svc.Lookup(context.Background(), model.Query{Organization: f.org, Audience: audience, Locale: "tr", Scope: model.Scope{BrandID: brand}, Question: question})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnitsCacheHitAndInvalidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.engine.units = unitsJSON(Unit{Question: "Kargo ne zaman gelir", Variants: []string{"Siparişim kaç günde teslim edilir"}, Answer: "Siparişler 2 iş gününde teslim edilir.", Category: "kargo"})
	articleUUID := f.article(t, nil, "public", "Siparişler 2 iş gününde teslim edilir.")
	if err := f.svc.SyncArticle(ctx, f.org, articleUUID); err != nil {
		t.Fatal(err)
	}
	// Same published version again: no AI call.
	if err := f.svc.SyncArticle(ctx, f.org, articleUUID); err != nil {
		t.Fatal(err)
	}
	if f.engine.calls.Load() != 1 || f.embed.calls.Load() != 1 {
		t.Fatalf("extraction calls=%d embeddings=%d", f.engine.calls.Load(), f.embed.calls.Load())
	}

	// A variant phrasing hits semantically (one embedding call), the exact
	// repeat hits the cache with no call at all.
	first := f.lookup(t, model.AudienceCustomer, nil, "Siparişim kaç günde teslim edilir?")
	if first.Outcome != model.OutcomeSemantic || first.Unit == nil || first.EmbeddingCalls != 1 || first.Unit.ArticleUUID == nil || *first.Unit.ArticleUUID != articleUUID {
		t.Fatalf("first=%+v", first)
	}
	before := f.embed.calls.Load()
	second := f.lookup(t, model.AudienceCustomer, nil, "  siparişim KAÇ günde teslim edilir ")
	if second.Outcome != model.OutcomeExact || second.EmbeddingCalls != 0 || f.embed.calls.Load() != before {
		t.Fatalf("second=%+v", second)
	}
	miss := f.lookup(t, model.AudienceCustomer, nil, "Faturamı nasıl indiririm")
	if miss.Outcome != model.OutcomeMiss || miss.Unit != nil {
		t.Fatalf("miss=%+v", miss)
	}
	stats, err := f.svc.Stats(ctx, f.org, f.svc.Now().AddDate(0, 0, -1))
	if err != nil || len(stats.Audiences) != 1 || stats.Audiences[0].Lookups != 3 || math.Abs(stats.Audiences[0].HitRate-2.0/3.0) > 1e-9 || stats.Cached != 1 || stats.Units != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}

	// The source changes: units are rebuilt and every cached answer for the
	// old units is gone, so the old answer is never served again.
	var articleID int64
	if err = f.pool.QueryRow(ctx, "SELECT id FROM knowledge_articles WHERE uuid=$1", articleUUID).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	f.publish(t, articleID, 2, "Siparişler 1 iş gününde teslim edilir.", "public")
	f.engine.units = unitsJSON(Unit{Question: "Kargo ne zaman gelir", Variants: []string{"Siparişim kaç günde teslim edilir"}, Answer: "Siparişler 1 iş gününde teslim edilir.", Category: "kargo"})
	if err = f.svc.SyncArticle(ctx, f.org, articleUUID); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.svc.Q.AnswerCacheCount(ctx, f.org); n != 0 {
		t.Fatalf("cache survived a source change: %d", n)
	}
	again := f.lookup(t, model.AudienceCustomer, nil, "siparişim kaç günde teslim edilir")
	if again.Unit == nil || again.Outcome != model.OutcomeSemantic || !strings.Contains(again.Unit.Answer, "1 iş günü") {
		t.Fatalf("again=%+v", again)
	}

	// Unpublishing removes the units and their cache.
	if _, err = f.pool.Exec(ctx, "UPDATE knowledge_articles SET published_version=NULL WHERE id=$1", articleID); err != nil {
		t.Fatal(err)
	}
	if err = f.svc.SyncArticle(ctx, f.org, articleUUID); err != nil {
		t.Fatal(err)
	}
	if gone := f.lookup(t, model.AudienceCustomer, nil, "siparişim kaç günde teslim edilir"); gone.Unit != nil {
		t.Fatalf("unpublished unit served: %+v", gone)
	}
}

func TestUnitScopeAndAudience(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sync := func(brand *uuid.UUID, visibility, answer string) {
		f.engine.units = unitsJSON(Unit{Question: "İade süresi kaç gün", Answer: answer})
		if err := f.svc.SyncArticle(ctx, f.org, f.article(t, brand, visibility, answer)); err != nil {
			t.Fatal(err)
		}
	}
	sync(nil, "public", "Genel: 14 gün.")
	sync(&f.brandA, "public", "Marka A: 30 gün.")
	sync(nil, "internal", "İç not: 45 güne kadar esnetilebilir.")

	q := "İade süresi kaç gün?"
	if got := f.lookup(t, model.AudienceCustomer, &f.brandA, q); got.Unit == nil || got.Unit.Answer != "Marka A: 30 gün." {
		t.Fatalf("brand A should get its own unit: %+v", got.Unit)
	}
	// Brand B and a conversation without a brand fall back to the
	// organization-wide unit and never see brand A's.
	for _, brand := range []*uuid.UUID{&f.brandB, nil} {
		if got := f.lookup(t, model.AudienceCustomer, brand, q); got.Unit == nil || got.Unit.Answer != "Genel: 14 gün." {
			t.Fatalf("fallback: %+v", got.Unit)
		}
	}
	// The customer AI never gets an internal unit; the agent assistant may.
	if _, err := f.pool.Exec(ctx, "DELETE FROM knowledge_units WHERE organization_id=$1 AND visibility='public'", f.org); err != nil {
		t.Fatal(err)
	}
	if got := f.lookup(t, model.AudienceCustomer, nil, q); got.Unit != nil {
		t.Fatalf("internal unit reached the customer: %+v", got.Unit)
	}
	if got := f.lookup(t, model.AudienceAgent, nil, q); got.Unit == nil || got.Unit.Visibility != "internal" {
		t.Fatalf("agent should see the internal unit: %+v", got.Unit)
	}
	// Another organization sees nothing.
	other := newFixture(t)
	other.svc = f.svc
	if got, err := f.svc.Lookup(ctx, model.Query{Organization: other.org, Audience: model.AudienceAgent, Locale: "tr", Question: q}); err != nil || got.Unit != nil {
		t.Fatalf("cross-tenant: %+v %v", got.Unit, err)
	}
}

// conversation creates a resolved conversation between a named contact at a
// named company and an agent.
func (f *fixture) conversation(t *testing.T, contact, email, phone, company, domain string, lines ...string) (int64, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var companyID, contactID, id int64
	var u uuid.UUID
	if err := f.pool.QueryRow(ctx, "INSERT INTO companies(organization_id,name,domain) VALUES($1,$2,$3) RETURNING id", f.org, company, domain).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,company_id,name,email,phone) VALUES($1,$2,$3,$4,$5) RETURNING id", f.org, companyID, contact, email, phone).Scan(&contactID); err != nil {
		t.Fatal(err)
	}
	var number int64
	_ = f.pool.QueryRow(ctx, "SELECT coalesce(max(number),0)+1 FROM conversations WHERE organization_id=$1", f.org).Scan(&number)
	if err := f.pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_type,source,status,subject) VALUES($1,$2,$3,'email','inbound','resolved',$4) RETURNING id,uuid", f.org, number, contactID, contact+" fatura sorunu").Scan(&id, &u); err != nil {
		t.Fatal(err)
	}
	for i, line := range lines {
		dir, sender := "in", "contact"
		if i%2 == 1 {
			dir, sender = "out", "agent"
		}
		if _, err := f.pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_contact_id,kind,status,content) VALUES($1,$2,$3,$4,CASE WHEN $4='contact' THEN $5::bigint END,'text','received',$6)", f.org, id, dir, sender, contactID, line); err != nil {
			t.Fatal(err)
		}
	}
	return id, u
}

// TestCustomerCaseMemoryNeverLeaksAnotherCustomer is the CF-216 leak test.
// The model is made to misbehave and echo the customer's identity; the
// customer AI must still never receive the source customer's text, name,
// contact details, company or conversation.
func TestCustomerCaseMemoryNeverLeaksAnotherCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	secretLine := "Merhaba ben Ayşe Yılmaz, Kuzey Lojistik muhasebesinden. e-fatura portalına giremiyorum, ayse.yilmaz@kuzeylojistik.com.tr ve 0532 123 45 67"
	_, sourceUUID := f.conversation(t, "Ayşe Yılmaz", "ayse.yilmaz@kuzeylojistik.com.tr", "+90 532 123 45 67", "Kuzey Lojistik Ltd. Şti.", "kuzeylojistik.com.tr",
		secretLine, "Şifre sıfırlama bağlantısı gönderdim, tarayıcı önbelleğini temizleyip tekrar deneyin.", "Oldu, teşekkürler!")
	// Worst case: the summarizer copies identity into the note.
	f.engine.note = `{"problem":"Ayşe Yılmaz (Kuzey Lojistik, ayse.yilmaz@kuzeylojistik.com.tr, 0532 123 45 67) e-fatura portalına giremiyor","resolution":"Yılmaz Hanım'a şifre sıfırlama bağlantısı gönderildi, kuzeylojistik.com.tr kullanıcısı tarayıcı önbelleğini temizleyince giriş yaptı","category":"e-fatura giriş","resolved":true}`
	if err := f.svc.Remember(ctx, f.org, sourceUUID); err != nil {
		t.Fatal(err)
	}
	// The model never saw the identity either.
	for _, in := range f.engine.inputs {
		assertClean(t, in)
	}
	var problem, resolution, status string
	var anonymized bool
	var caseUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid,problem,resolution,status,anonymized FROM ai_case_memories WHERE organization_id=$1", f.org).Scan(&caseUUID, &problem, &resolution, &status, &anonymized); err != nil {
		t.Fatal(err)
	}
	assertClean(t, problem+resolution)
	if status != "pending" || !anonymized {
		t.Fatalf("status=%s anonymized=%v", status, anonymized)
	}

	asker := "e-fatura portalına giremiyorum ne yapmalıyım"
	// The bag-of-words test embedder scores a short question against a long
	// summary low; the real threshold is calibrated for a real model.
	f.svc.CaseThreshold = 0.2
	// Pending summaries never reach the customer AI.
	if got := f.lookup(t, model.AudienceCustomer, nil, asker); len(got.CustomerCases) != 0 {
		t.Fatalf("pending case reached the customer: %+v", got.CustomerCases)
	}
	if _, err := f.svc.Review(ctx, f.org, f.user, caseUUID, true); err != nil {
		t.Fatal(err)
	}
	got := f.lookup(t, model.AudienceCustomer, nil, asker)
	if len(got.CustomerCases) != 1 || !strings.Contains(got.CustomerCases[0].Resolution, "şifre sıfırlama") {
		t.Fatalf("approved case missing: %+v", got)
	}
	everything, _ := json.Marshal(struct {
		R model.Result
		C []model.CustomerCase
	}{got, got.CustomerCases})
	assertClean(t, string(everything))
	if strings.Contains(string(everything), sourceUUID.String()) || strings.Contains(string(everything), caseUUID.String()) || strings.Contains(string(everything), "giremiyorum, ayse") {
		t.Fatalf("customer result references the source conversation: %s", everything)
	}

	// Even if an approved summary is later edited to contain the identity
	// (or the anonymizer missed something), the customer path scrubs it.
	if _, err := f.pool.Exec(ctx, "UPDATE ai_case_memories SET resolution=resolution||' Ayşe Yılmaz, Kuzey Lojistik' WHERE uuid=$1", caseUUID); err != nil {
		t.Fatal(err)
	}
	got = f.lookup(t, model.AudienceCustomer, nil, asker)
	everything, _ = json.Marshal(got.CustomerCases)
	assertClean(t, string(everything))

	// A summary that fails the anonymizer cannot be approved.
	if _, err := f.pool.Exec(ctx, "UPDATE ai_case_memories SET status='pending',anonymized=false WHERE uuid=$1", caseUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Review(ctx, f.org, f.user, caseUUID, true); err != model.ErrConflict {
		t.Fatalf("non-anonymized approval: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE ai_case_memories SET status='approved' WHERE uuid=$1", caseUUID); err == nil {
		t.Fatal("database accepted an approved, non-anonymized case")
	}
	if _, err := f.pool.Exec(ctx, "UPDATE ai_case_memories SET anonymized=true WHERE uuid=$1", caseUUID); err != nil {
		t.Fatal(err)
	}

	// The agent assistant sees pending and approved cases with the source
	// conversation; the conversation that asks is excluded from its own results.
	if _, err := f.pool.Exec(ctx, "UPDATE ai_case_memories SET resolution='Şifre sıfırlama bağlantısı gönderildi' WHERE uuid=$1", caseUUID); err != nil {
		t.Fatal(err)
	}
	agent := f.lookup(t, model.AudienceAgent, nil, asker)
	if len(agent.AgentCases) != 1 || agent.AgentCases[0].ConversationUUID == nil || *agent.AgentCases[0].ConversationUUID != sourceUUID {
		t.Fatalf("agent cases=%+v", agent.AgentCases)
	}
	var sourceID int64
	_ = f.pool.QueryRow(ctx, "SELECT id FROM conversations WHERE uuid=$1", sourceUUID).Scan(&sourceID)
	self, err := f.svc.Lookup(ctx, model.Query{Organization: f.org, Audience: model.AudienceAgent, Locale: "tr", Question: asker, ExcludeConversationID: sourceID})
	if err != nil || len(self.AgentCases) != 0 {
		t.Fatalf("own conversation offered: %+v %v", self.AgentCases, err)
	}
}

func TestUnansweredQuestionsBecomeKnowledgeGaps(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	convID, _ := f.conversation(t, "Bora", "bora@example.com", "", "Acme", "acme.test", "Toplu fatura nasıl kesilir?", "Kontrol edip döneceğim.")
	q := model.Query{Organization: f.org, Audience: model.AudienceAgent, Locale: "tr", Question: "Toplu fatura nasıl kesilir? bora@example.com"}
	for range 2 {
		if err := f.svc.RecordGap(ctx, q, convID, "fatura"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := f.svc.Q.KnowledgeGapQuestions(ctx, db.KnowledgeGapQuestionsParams{OrganizationID: f.org, Since: pgtype.Timestamptz{Time: f.svc.Now().AddDate(0, 0, -1), Valid: true}, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].Occurrences != 2 || strings.Contains(rows[0].Question, "bora@example.com") {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	topics, err := f.svc.Q.AIKnowledgeGapTopics(ctx, db.AIKnowledgeGapTopicsParams{OrganizationID: f.org, Since: pgtype.Timestamptz{Time: f.svc.Now().AddDate(0, 0, -1), Valid: true}, Limit: 10})
	if err != nil || len(topics) != 1 || topics[0].Topic != "fatura" || topics[0].Conversations != 1 || topics[0].ConversationIds[0] != convID {
		t.Fatalf("topics=%+v err=%v", topics, err)
	}
	if n, err := f.svc.Q.AIKnowledgeGapTopicCount(ctx, db.AIKnowledgeGapTopicCountParams{OrganizationID: f.org, Since: pgtype.Timestamptz{Time: f.svc.Now().AddDate(0, 0, -1), Valid: true}}); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}

// TestMeasureCallsAndCost replays a realistic question mix and reports AI
// calls and cost per question before (every question: retrieval embedding +
// generation) and after (knowledge layer in front). Numbers go into the PR.
func TestMeasureCallsAndCost(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	topics := []Unit{
		{Question: "Kargo ne zaman gelir", Variants: []string{"Siparişim kaç günde teslim edilir", "Teslimat süresi nedir"}, Answer: "2 iş günü."},
		{Question: "İade nasıl yapılır", Variants: []string{"Ürünü nasıl iade ederim", "İade süreci nedir"}, Answer: "Panelden iade talebi açın."},
		{Question: "Şifremi nasıl sıfırlarım", Variants: []string{"Şifremi unuttum", "Parola sıfırlama"}, Answer: "Giriş ekranında Şifremi unuttum."},
		{Question: "Fatura nereden indirilir", Variants: []string{"Faturamı nasıl indiririm", "E-fatura nerede"}, Answer: "Hesabım > Faturalar."},
		{Question: "Ödeme yöntemleri neler", Variants: []string{"Kredi kartı kabul ediyor musunuz", "Havale ile ödeme"}, Answer: "Kart ve havale."},
	}
	f.engine.units = unitsJSON(topics...)
	if err := f.svc.SyncArticle(ctx, f.org, f.article(t, nil, "public", "SSS")); err != nil {
		t.Fatal(err)
	}
	extractionCalls, extractionEmbeds := f.engine.calls.Load(), f.embed.calls.Load()

	// 200 questions: 70% repeat a known phrasing, 15% are new wordings of a
	// known question (semantic), 15% are unknown (miss → normal AI path).
	// The bag-of-words test embedder only matches reworded questions that keep
	// the same words; a real embedding model also matches looser paraphrases,
	// so the semantic share here is a lower bound.
	questions := []string{}
	unknown := []string{"Bayi olmak istiyorum", "Kurumsal indirim var mı", "Hafta sonu açık mısınız"}
	for i := 0; len(questions) < 200; i++ {
		u := topics[i%len(topics)]
		switch {
		case i%20 < 14:
			questions = append(questions, append([]string{u.Question}, u.Variants...)[i%3])
		case i%20 < 17:
			// Same words in another order: a new cache key, matched by vector.
			words := strings.Fields(u.Variants[i%2])
			slices.Reverse(words)
			questions = append(questions, strings.Join(words, " ")+"?")
		default:
			questions = append(questions, fmt.Sprintf("%s %d", unknown[i%3], i))
		}
	}
	const (
		embedMicros = 2   // retrieval/lookup embedding of one question
		genMicros   = 900 // one reply generation (≈1.5k in / 300 out tokens)
	)
	var exact, semantic, miss int
	after := struct{ embeds, gens int }{}
	for _, q := range questions {
		before := f.embed.calls.Load()
		out := f.lookup(t, model.AudienceCustomer, nil, q)
		after.embeds += int(f.embed.calls.Load() - before)
		switch out.Outcome {
		case model.OutcomeExact:
			exact++
		case model.OutcomeSemantic:
			semantic++
		default:
			miss++
			after.embeds++ // the normal path still embeds for retrieval
			after.gens++
		}
	}
	n := len(questions)
	beforeCalls, beforeCost := 2*n, n*(embedMicros+genMicros)
	afterCalls, afterCost := after.embeds+after.gens, after.embeds*embedMicros+after.gens*genMicros
	t.Logf("questions=%d exact=%d semantic=%d miss=%d hit_rate=%.1f%%", n, exact, semantic, miss, 100*float64(exact+semantic)/float64(n))
	t.Logf("before: calls=%d (%.2f/question) cost=%d micros (%.1f/question)", beforeCalls, float64(beforeCalls)/float64(n), beforeCost, float64(beforeCost)/float64(n))
	t.Logf("after:  calls=%d (%.2f/question) cost=%d micros (%.1f/question); one-off extraction: %d generation + %d embedding calls", afterCalls, float64(afterCalls)/float64(n), afterCost, float64(afterCost)/float64(n), extractionCalls, extractionEmbeds)
	if exact+semantic < n*80/100 || miss == 0 || afterCalls >= beforeCalls/2 {
		t.Fatalf("unexpected mix: exact=%d semantic=%d miss=%d calls=%d", exact, semantic, miss, afterCalls)
	}
}

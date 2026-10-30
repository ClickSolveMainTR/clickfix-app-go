package usecase

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeEmbedding struct{ input []string }

func (f *fakeEmbedding) Model() string { return "fake-vector-v1" }
func (f *fakeEmbedding) Embed(_ context.Context, input []string) (ai.Embeddings, error) {
	f.input = append(f.input, input...)
	out := [][]float32{}
	for range input {
		out = append(out, []float32{1, 0})
	}
	return ai.Embeddings{Vectors: out, InputTokens: int64(len(input))}, nil
}

type untrustedLexical struct {
	ids  []uuid.UUID
	docs []Document
}

func (l *untrustedLexical) Search(context.Context, int64, string, string, bool) ([]uuid.UUID, error) {
	return l.ids, nil
}

func (l *untrustedLexical) Replace(_ context.Context, _ int64, _ uuid.UUID, docs []Document) error {
	l.docs = docs
	return nil
}

func TestChunkHTMLAndMarkdown(t *testing.T) {
	parts := Chunk("<h1>Başlık</h1><script>steal()</script><p>Ödeme bilgisi</p><pre>code()</pre>", 128, 20)
	all := strings.Join(parts, "\n")
	if strings.Contains(all, "steal") || !strings.Contains(all, "Başlık") || !strings.Contains(all, "code()") {
		t.Fatal(parts)
	}
	body := "# Başlık\n\n```go\nhello()\n```\n\n" + strings.Repeat("Türkçe🙂", 70)
	parts = Chunk(body, 128, 20)
	if len(parts) < 3 {
		t.Fatal(parts)
	}
	for _, p := range parts {
		if !utf8.ValidString(p) || utf8.RuneCountInString(p) > 128 {
			t.Fatal("invalid UTF-8 or chunk bound")
		}
	}
	if !strings.Contains(strings.Join(parts, "\n"), "```go\nhello()\n```") {
		t.Fatal("split small fenced block")
	}
}

func kbFixture(t *testing.T) (*Service, *RAG, int64, int64, int64) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var org, other, actor int64
	for _, p := range []struct {
		target *int64
		sql    string
		arg    string
	}{
		{target: &org, sql: "INSERT INTO organizations(slug,name) VALUES($1,'KB test') RETURNING id", arg: uuid.NewString()},
		{target: &other, sql: "INSERT INTO organizations(slug,name) VALUES($1,'KB other') RETURNING id", arg: uuid.NewString()},
		{target: &actor, sql: "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','KB','Test') RETURNING id", arg: uuid.NewString() + "@example.test"},
	} {
		if err = pool.QueryRow(ctx, p.sql, p.arg).Scan(p.target); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(repository.New(pool))
	rag := &RAG{KB: svc, Embedder: &fakeEmbedding{}}
	return svc, rag, org, other, actor
}

func TestKnowledgeVersionsPublicationAndTenantIsolation(t *testing.T) {
	svc, rag, org, other, actor := kbFixture(t)
	ctx := context.Background()
	collection, err := svc.SaveCollection(ctx, org, uuid.New(), model.CollectionInput{Name: "Help", Slug: "help", Kind: "collection"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := svc.SaveCollection(ctx, org, uuid.New(), model.CollectionInput{Name: "Payments", Slug: "payments", Kind: "category", Parent: &collection.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveCollection(ctx, other, uuid.New(), model.CollectionInput{Name: "Cross", Slug: "cross", Kind: "category", Parent: &collection.Uuid}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("cross org parent %v", err)
	}
	in := model.ArticleInput{Collection: &category.Uuid, Slug: "payment-help", Locale: "tr", Title: "Ödeme", Body: "Kart sorunu için alice@example.com ile iletişime geçin.", Visibility: "public"}
	article, err := svc.SaveArticle(ctx, org, actor, uuid.New(), in, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Article(ctx, other, article.Uuid); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("cross org read %v", err)
	}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	hits, err := rag.Search(ctx, org, "Ödeme", "tr", true)
	if err != nil || len(hits) != 0 {
		t.Fatalf("draft leaked %+v %v", hits, err)
	}
	if err = svc.Publish(ctx, org, article.Uuid, article.Version, true); err != nil {
		t.Fatal(err)
	}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	hits, err = rag.Search(ctx, org, "Ödeme", "tr", true)
	if err != nil || len(hits) != 1 {
		t.Fatalf("published %+v %v", hits, err)
	}
	if strings.Contains(hits[0].Content, "alice@example.com") {
		t.Fatal("PII in embeddings/citations")
	}
	original := hits[0]
	in.Version = article.Version
	in.Body = "New draft body"
	in.Visibility = "internal"
	revised, err := svc.SaveArticle(ctx, org, actor, article.Uuid, in, false)
	if err != nil {
		t.Fatal(err)
	}
	hits, err = rag.Search(ctx, org, "Ödeme", "tr", true)
	if err != nil || len(hits) != 1 || hits[0].Version != 1 || strings.Contains(hits[0].Content, "New draft") {
		t.Fatalf("draft affected published snapshot %+v %v", hits, err)
	}
	if _, err = svc.SaveArticle(ctx, org, actor, article.Uuid, in, false); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("lost update %v", err)
	}
	if err = svc.Publish(ctx, org, article.Uuid, revised.Version, true); err != nil {
		t.Fatal(err)
	}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	lexical := &untrustedLexical{ids: []uuid.UUID{original.ChunkUUID}}
	rag.Lexical = lexical
	hits, err = rag.Search(ctx, org, "Ödeme", "tr", true)
	if err != nil || len(hits) != 0 {
		t.Fatalf("private/stale leak %+v %v", hits, err)
	}
	hits, err = rag.Search(ctx, other, "Ödeme", "tr", false)
	if err != nil || len(hits) != 0 {
		t.Fatalf("cross tenant stale index %+v %v", hits, err)
	}
	hits, err = rag.Search(ctx, org, "Ödeme", "en", false)
	if err != nil || len(hits) != 0 {
		t.Fatalf("locale leak %+v %v", hits, err)
	}
	versions, total, err := svc.Versions(ctx, org, article.Uuid, 100, 0)
	if err != nil || len(versions) != 2 || total != 2 {
		t.Fatalf("versions %+v %v", versions, err)
	}
	if _, err = svc.Repo.Pool.Exec(ctx, "UPDATE knowledge_article_versions SET title='oops' WHERE organization_id=$1 AND article_id=$2", org, article.ID); err == nil {
		t.Fatal("version mutable")
	}
	if err = svc.Publish(ctx, org, article.Uuid, revised.Version, false); err != nil {
		t.Fatal(err)
	}
	if err = svc.Publish(ctx, org, article.Uuid, revised.Version, true); err != nil {
		t.Fatal(err)
	}
	var events int
	if err = svc.Repo.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='knowledge.article_published'", org).Scan(&events); err != nil || events != 3 {
		t.Fatalf("republish deduped events=%d %v", events, err)
	}
	if err = svc.DeleteArticle(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	hits, err = rag.Search(ctx, org, "Ödeme", "tr", false)
	if err != nil || len(hits) != 0 {
		t.Fatalf("deleted leak %+v %v", hits, err)
	}
	if _, err = svc.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: article.Uuid}); err == nil {
		t.Fatal("deleted readable")
	}
}

type failingEmbedding struct{ err error }

func (f failingEmbedding) Model() string { return "fake-vector-v1" }
func (f failingEmbedding) Embed(context.Context, []string) (ai.Embeddings, error) {
	return ai.Embeddings{}, f.err
}

func TestSkipReasonTreatsAIStatesAsSkips(t *testing.T) {
	for err, want := range map[error]string{
		aimodel.ErrDisabled: SkipAIDisabled,
		ai.ErrNotConfigured: SkipAINotConfigured,
		&entitlement.Error{Code: entitlement.CodeFeatureRequired}: SkipAINotEntitled,
		aimodel.ErrCredits: SkipAICreditsExhausted,
		aimodel.ErrDaily:   "",
		ai.ErrUnavailable:  "",
	} {
		if got := SkipReason(err); got != want {
			t.Errorf("%v: got %q want %q", err, got, want)
		}
	}
}

// BACK-END-7: with AI off the indexer must not fail (and be retried until
// dead-lettered); the article is marked skipped and indexed once AI works.
func TestIndexSkipsQuietlyWhenAIDisabled(t *testing.T) {
	svc, rag, org, _, actor := kbFixture(t)
	ctx := context.Background()
	in := model.ArticleInput{Slug: "skip-" + uuid.NewString()[:8], Locale: "tr", Title: "Kapalı", Body: "Yapay zekâ kapalıyken yayınlanan makale.", Visibility: "public"}
	article, err := svc.SaveArticle(ctx, org, actor, uuid.New(), in, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Publish(ctx, org, article.Uuid, article.Version, true); err != nil {
		t.Fatal(err)
	}
	rag.Embedder = failingEmbedding{err: aimodel.ErrDisabled}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatalf("disabled AI must be a quiet skip, got %v", err)
	}
	skipped, err := svc.Repo.Q.KnowledgeSkippedArticles(ctx, org)
	if err != nil || len(skipped) != 1 || skipped[0].Uuid != article.Uuid {
		t.Fatalf("skipped = %+v %v", skipped, err)
	}
	if _, err = rag.Search(ctx, org, "makale", "tr", true); err != nil {
		t.Fatalf("search must fall back to the lexical index while AI is off, got %v", err)
	}
	rag.Embedder = failingEmbedding{err: aimodel.ErrDaily}
	if err = rag.Index(ctx, org, article.Uuid); !errors.Is(err, aimodel.ErrDaily) {
		t.Fatalf("a daily limit is retried, got %v", err)
	}
	rag.Embedder = &fakeEmbedding{}
	if err = rag.Index(ctx, org, article.Uuid); err != nil {
		t.Fatal(err)
	}
	if skipped, err = svc.Repo.Q.KnowledgeSkippedArticles(ctx, org); err != nil || len(skipped) != 0 {
		t.Fatalf("still skipped after indexing: %+v %v", skipped, err)
	}
}

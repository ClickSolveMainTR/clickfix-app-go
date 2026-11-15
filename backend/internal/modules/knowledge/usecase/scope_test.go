package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func newBrand(t *testing.T, rag *RAG, org int64, name string) pgtype.UUID {
	t.Helper()
	var id uuid.UUID
	if err := rag.KB.Repo.Pool.QueryRow(context.Background(), "INSERT INTO brands(organization_id,name) VALUES($1,$2) RETURNING uuid", org, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// publishArticle creates, publishes and indexes a public tr article, optionally of a brand.
func publishArticle(t *testing.T, svc *Service, rag *RAG, org, actor int64, slug, body string, brand pgtype.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	a, err := svc.SaveArticle(ctx, org, actor, uuid.New(), model.ArticleInput{Slug: slug, Locale: "tr", Title: slug, Body: body, Visibility: "public"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if brand.Valid {
		if _, err = rag.KB.Repo.Pool.Exec(ctx, "UPDATE knowledge_articles SET brand_id=$1 WHERE organization_id=$2 AND uuid=$3", brand, org, a.Uuid); err != nil {
			t.Fatal(err)
		}
	}
	if err = svc.Publish(ctx, org, a.Uuid, a.Version, true); err != nil {
		t.Fatal(err)
	}
	if err = rag.Index(ctx, org, a.Uuid); err != nil {
		t.Fatal(err)
	}
	return a.Uuid
}

func articles(hits []model.Citation) []uuid.UUID {
	out := []uuid.UUID{}
	for _, h := range hits {
		if !slices.Contains(out, h.ArticleUUID) {
			out = append(out, h.ArticleUUID)
		}
	}
	return out
}

// CF-214: a conversation of brand A sees brand A knowledge first, then
// organization-wide knowledge, never brand B; a brandless conversation sees
// organization-wide knowledge only; another tenant sees nothing.
func TestSearchScopedBrandFallbackAndTenantIsolation(t *testing.T) {
	svc, rag, org, other, actor := kbFixture(t)
	ctx := context.Background()
	brandA, brandB := newBrand(t, rag, org, "Marka A"), newBrand(t, rag, org, "Marka B")
	generic := publishArticle(t, svc, rag, org, actor, "iade-genel", "İade süresi genel kural on dört gündür.", pgtype.UUID{})
	onlyA := publishArticle(t, svc, rag, org, actor, "iade-a", "İade A markasında otuz gündür.", brandA)
	onlyB := publishArticle(t, svc, rag, org, actor, "iade-b", "İade B markasında yedi gündür.", brandB)

	for _, withLexical := range []bool{false, true} {
		// Vector candidates alone (an empty lexical index) and vector + SQL
		// lexical candidates follow the same scope rules.
		rag.Lexical = &untrustedLexical{}
		if withLexical {
			rag.Lexical = nil
		}
		hits, err := rag.SearchScoped(ctx, vectorstore.Conversation(org, brandA, pgtype.Int8{}), "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		got := articles(hits)
		if len(got) != 2 || got[0] != onlyA || got[1] != generic {
			t.Fatalf("brand A scope: got %v want [%v %v]", got, onlyA, generic)
		}
		if !hits[0].Specific || hits[len(hits)-1].Specific {
			t.Fatalf("specific flags %+v", hits)
		}
		hits, err = rag.SearchScoped(ctx, vectorstore.Conversation(org, pgtype.UUID{}, pgtype.Int8{}), "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		if got = articles(hits); len(got) != 1 || got[0] != generic {
			t.Fatalf("brandless scope: %v", got)
		}
		hits, err = rag.Search(ctx, org, "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		if got = articles(hits); len(got) != 3 || !slices.Contains(got, onlyB) {
			t.Fatalf("organization scope: %v", got)
		}
		for _, scope := range []vectorstore.Scope{vectorstore.Organization(other), vectorstore.Conversation(other, brandA, pgtype.Int8{})} {
			hits, err = rag.SearchScoped(ctx, scope, "İade", "tr", true)
			if err != nil || len(hits) != 0 {
				t.Fatalf("cross tenant %+v %v", hits, err)
			}
		}
	}

	// Moving an article to another brand moves its chunks without re-indexing.
	if _, err := rag.KB.Repo.Pool.Exec(ctx, "UPDATE knowledge_articles SET brand_id=$1 WHERE organization_id=$2 AND uuid=$3", brandA, org, onlyB); err != nil {
		t.Fatal(err)
	}
	hits, err := rag.SearchScoped(ctx, vectorstore.Conversation(org, brandA, pgtype.Int8{}), "İade", "tr", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := articles(hits); len(got) != 3 || got[2] != generic {
		t.Fatalf("after brand move: %v", got)
	}
	// Another organization's brand can never be attached to this organization's chunks.
	foreign := newBrand(t, rag, other, "Yabancı")
	if _, err = rag.KB.Repo.Pool.Exec(ctx, "UPDATE knowledge_chunks SET brand_id=$1 WHERE organization_id=$2", foreign, org); err == nil {
		t.Fatal("cross-tenant brand accepted")
	}
}

// CF-214: after a model change search keeps the old vectors (queries use the
// old model), new articles wait in pending_*, and the background job switches
// the organization to the new model in one step.
func TestReembedKeepsOldVectorsUntilSwitch(t *testing.T) {
	svc, rag, org, _, actor := kbFixture(t)
	ctx := context.Background()
	old := publishArticle(t, svc, rag, org, actor, "kargo", "Kargo iki gün sürer.", pgtype.UUID{})

	next := &fakeEmbedding{model: "fake-vector-v2"}
	rag.Embedder = next
	rag.Lexical = &untrustedLexical{} // vector results only
	hits, err := rag.Search(ctx, org, "Kargo", "tr", true)
	if err != nil || len(hits) != 1 || hits[0].ArticleUUID != old {
		t.Fatalf("old vectors not searched during transition %+v %v", hits, err)
	}
	if next.models[len(next.models)-1] != "fake-vector-v1" {
		t.Fatalf("query embedded with %v", next.models)
	}
	fresh := publishArticle(t, svc, rag, org, actor, "kargo-yeni", "Kargo ücretsizdir.", pgtype.UUID{})
	var pending int
	if err = rag.KB.Repo.Pool.QueryRow(ctx, "SELECT count(*) FROM knowledge_chunks c JOIN knowledge_articles a ON a.id=c.article_id WHERE a.uuid=$1 AND c.embedding IS NULL AND c.pending_model='fake-vector-v2'", fresh).Scan(&pending); err != nil || pending == 0 {
		t.Fatalf("new article during transition not pending: %d %v", pending, err)
	}

	job := &vectorstore.Reembedder{Pool: rag.KB.Repo.Pool, Embedder: next, Sources: []vectorstore.Source{ChunkSource}, Batch: 1}
	if err = job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var stale int
	if err = rag.KB.Repo.Pool.QueryRow(ctx, "SELECT count(*) FROM knowledge_chunks WHERE organization_id=$1 AND (embedding IS NULL OR embedding_model<>'fake-vector-v2' OR pending_model IS NOT NULL)", org).Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("not switched: %d %v", stale, err)
	}
	hits, err = rag.Search(ctx, org, "Kargo", "tr", true)
	if err != nil || len(hits) != 2 {
		t.Fatalf("after switch %+v %v", hits, err)
	}
	if next.models[len(next.models)-1] != "fake-vector-v2" {
		t.Fatalf("query embedded with %v", next.models)
	}
}

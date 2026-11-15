package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func newDepartment(t *testing.T, rag *RAG, org int64, name string) int64 {
	t.Helper()
	var id int64
	if err := rag.KB.Repo.Pool.QueryRow(context.Background(), "INSERT INTO departments(organization_id,name) VALUES($1,$2) RETURNING id", org, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func dept(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: true} }

// publishInDepartment creates a public tr article of the department (0 =
// organization wide), publishes and indexes it.
func publishInDepartment(t *testing.T, svc *Service, rag *RAG, org, actor int64, slug, body string, department int64) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	a, err := svc.SaveArticle(ctx, org, actor, uuid.New(), model.ArticleInput{Slug: slug, Locale: "tr", Title: slug, Body: body, Visibility: "public"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if department > 0 {
		if _, err = rag.KB.Repo.Pool.Exec(ctx, "UPDATE knowledge_articles SET department_id=$1 WHERE organization_id=$2 AND uuid=$3", department, org, a.Uuid); err != nil {
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

func sameSet(got, want []uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			return false
		}
	}
	return true
}

// CF-222: chunks carry their article's BIGINT department. A conversation of
// department A sees A's knowledge first, then organization-wide knowledge
// and articles shared with A; never department B's own knowledge. A
// conversation without a department sees organization-wide knowledge only;
// another tenant sees nothing.
func TestSearchScopedDepartmentFallbackAndTenantIsolation(t *testing.T) {
	svc, rag, org, other, actor := kbFixture(t)
	ctx := context.Background()
	pool := rag.KB.Repo.Pool
	deptA, deptB := newDepartment(t, rag, org, "Teknik"), newDepartment(t, rag, org, "Satış")
	foreign := newDepartment(t, rag, other, "Yabancı")
	generic := publishInDepartment(t, svc, rag, org, actor, "iade-genel", "İade süresi genel kural on dört gündür.", 0)
	onlyA := publishInDepartment(t, svc, rag, org, actor, "iade-teknik", "İade teknik destekte otuz gündür.", deptA)
	onlyB := publishInDepartment(t, svc, rag, org, actor, "iade-satis", "İade satışta yedi gündür.", deptB)
	sharedB := publishInDepartment(t, svc, rag, org, actor, "iade-ortak", "İade ortak kuralı bir yıldır.", deptB)
	if _, err := pool.Exec(ctx, `INSERT INTO department_knowledge_shares(organization_id,department_id,resource_kind,resource_id)
 SELECT organization_id,$1,'article',id FROM knowledge_articles WHERE organization_id=$2 AND uuid=$3`, deptA, org, sharedB); err != nil {
		t.Fatal(err)
	}

	// The chunk copies the article's department when it is written.
	var copied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM knowledge_chunks c JOIN knowledge_articles a ON a.organization_id=c.organization_id AND a.id=c.article_id
 WHERE a.uuid=$1 AND c.department_id=$2`, onlyA, deptA).Scan(&copied); err != nil || copied == 0 {
		t.Fatalf("department not copied to chunks: %d %v", copied, err)
	}

	for _, withLexical := range []bool{false, true} {
		rag.Lexical = &untrustedLexical{}
		if withLexical {
			rag.Lexical = nil
		}
		hits, err := rag.SearchScoped(ctx, vectorstore.Conversation(org, pgtype.UUID{}, dept(deptA)), "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		got := articles(hits)
		if !sameSet(got, []uuid.UUID{onlyA, generic, sharedB}) || got[0] != onlyA {
			t.Fatalf("department A scope (lexical %v): got %v want %v first, then %v %v", withLexical, got, onlyA, generic, sharedB)
		}
		if !hits[0].Specific || hits[len(hits)-1].Specific {
			t.Fatalf("specific flags %+v", hits)
		}
		// Without a department only organization-wide knowledge.
		hits, err = rag.SearchScoped(ctx, vectorstore.Conversation(org, pgtype.UUID{}, pgtype.Int8{}), "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		if got = articles(hits); !sameSet(got, []uuid.UUID{generic}) {
			t.Fatalf("department-less scope: %v", got)
		}
		// Internal search follows the viewer's departments.
		hits, err = rag.Search(deptscope.With(ctx, deptscope.All()), org, "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		if got = articles(hits); len(got) != 4 {
			t.Fatalf("read_all scope: %v", got)
		}
		hits, err = rag.Search(deptscope.With(ctx, deptscope.Only(deptB)), org, "İade", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		if got = articles(hits); !sameSet(got, []uuid.UUID{generic, onlyB, sharedB}) {
			t.Fatalf("member of B: %v", got)
		}
		for _, scope := range []vectorstore.Scope{vectorstore.Organization(other), vectorstore.Conversation(other, pgtype.UUID{}, dept(deptA)), vectorstore.Conversation(other, pgtype.UUID{}, dept(foreign))} {
			hits, err = rag.SearchScoped(deptscope.With(ctx, deptscope.All()), scope, "İade", "tr", true)
			if err != nil || len(hits) != 0 {
				t.Fatalf("cross tenant %+v %v", hits, err)
			}
		}
	}

	// The filter runs inside the vector query (chunk level): B's own chunks
	// are not even candidates for department A.
	vec := unitVector(0)
	restrict, viewer := deptscope.Only(deptA).Args()
	raw, err := vectorstore.Search(ctx, pool, ChunkSource, vectorstore.Conversation(org, pgtype.UUID{}, dept(deptA)).WithDepartments(restrict, viewer), vectorstore.Query{
		Vector: vec, Model: "fake-vector-v1", MinSimilarity: KnowledgeMinSimilarity, Limit: 40, Args: pgx.NamedArgs{"locale": "tr", "public_only": true},
	})
	if err != nil || len(raw) == 0 {
		t.Fatalf("raw vector search %v %v", raw, err)
	}
	ids := make([]int64, len(raw))
	for i, h := range raw {
		ids[i] = h.ID
	}
	var leaked int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM knowledge_chunks c JOIN knowledge_articles a ON a.organization_id=c.organization_id AND a.id=c.article_id
 WHERE c.id=ANY($1) AND a.uuid=$2`, ids, onlyB).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("department B chunks among vector candidates: %d %v", leaked, err)
	}

	// The article's department stays the visibility rule: a chunk whose
	// copy is out of date (organization wide) does not reveal B's article.
	if _, err = pool.Exec(ctx, `UPDATE knowledge_chunks c SET department_id=NULL FROM knowledge_articles a
 WHERE a.organization_id=c.organization_id AND a.id=c.article_id AND a.uuid=$1`, onlyB); err != nil {
		t.Fatal(err)
	}
	rag.Lexical = &untrustedLexical{}
	hits, err := rag.SearchScoped(ctx, vectorstore.Conversation(org, pgtype.UUID{}, dept(deptA)), "İade", "tr", true)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(articles(hits), onlyB) {
		t.Fatalf("stale chunk department revealed B's article: %v", articles(hits))
	}

	// Moving an article to another department moves its chunks without re-indexing.
	if _, err = pool.Exec(ctx, "UPDATE knowledge_articles SET department_id=$1 WHERE organization_id=$2 AND uuid=$3", deptA, org, onlyB); err != nil {
		t.Fatal(err)
	}
	hits, err = rag.SearchScoped(ctx, vectorstore.Conversation(org, pgtype.UUID{}, dept(deptA)), "İade", "tr", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := articles(hits); len(got) != 4 || !slices.Contains(got[:2], onlyB) {
		t.Fatalf("after department move: %v", got)
	}

	// Another organization's department can never be attached to this
	// organization's chunks (composite foreign key).
	if _, err = pool.Exec(ctx, "UPDATE knowledge_chunks SET department_id=$1 WHERE organization_id=$2", foreign, org); err == nil {
		t.Fatal("cross-tenant department accepted")
	}
}

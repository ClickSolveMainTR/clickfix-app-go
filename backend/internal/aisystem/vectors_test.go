package aisystem

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeEmbedder returns a deterministic unit-ish vector per (model, text).
type fakeEmbedder struct {
	model string
	calls int
	texts []string
}

func (f *fakeEmbedder) Model() string { return f.model }

func (f *fakeEmbedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	f.calls++
	f.texts = append(f.texts, in...)
	out := ai.Embeddings{Vectors: make([][]float32, len(in))}
	for i, text := range in {
		out.Vectors[i] = fakeVector(f.model, text)
	}
	return out, nil
}

func fakeVector(model, text string) []float32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(model + "\x00" + text))
	v := make([]float32, ai.EmbeddingDimensions)
	v[h.Sum32()%uint32(ai.EmbeddingDimensions)] = 1
	v[0] += 0.5
	return v
}

func TestVectorTablesValid(t *testing.T) {
	seen := map[string]bool{}
	for _, table := range VectorTables {
		if seen[table.Table] {
			t.Fatalf("duplicate %s", table.Table)
		}
		seen[table.Table] = true
		if table.Centroid != nil && !seen[table.Centroid.Members] {
			t.Fatalf("%s listed before its members %s", table.Table, table.Centroid.Members)
		}
	}
}

// CF-223: after a model change every vector table is re-embedded (batch by
// batch, resuming across runs), centroids are rebuilt from their members,
// and until then target-model queries never return old-model rows.
func TestReembedAllVectorTables(t *testing.T) {
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
	const oldModel, newModel = "fake-v1", "fake-v2"
	vec := func(text string) vectorstore.Vector { return vectorstore.Vector(fakeVector(oldModel, text)) }
	one := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	org := one(`INSERT INTO organizations(slug,name) VALUES($1,'Reembed') RETURNING id`, uuid.NewString())
	user := one(`INSERT INTO users(email,password_hash,name,surname) VALUES($1,'x','R','E') RETURNING id`, uuid.NewString()+"@example.test")
	contact := one(`INSERT INTO contacts(organization_id) VALUES($1) RETURNING id`, org)
	conv := one(`INSERT INTO conversations(organization_id,number,channel_type,contact_id) VALUES($1,1,'email',$2) RETURNING id`, org, contact)
	other := one(`INSERT INTO conversations(organization_id,number,channel_type,contact_id) VALUES($1,2,'email',$2) RETURNING id`, org, contact)

	sugg := one(`INSERT INTO knowledge_suggestions(organization_id,conversation_id,topic,title,body) VALUES($1,$2,'kargo','Kargo','Kargo iki gün') RETURNING id`, org, conv)
	unit := one(`INSERT INTO knowledge_units(organization_id,suggestion_id,source_version,source_hash,ordinal,locale,visibility,question,answer,embedding_model)
 VALUES($1,$2,1,repeat('a',64),0,'tr','public','Kargo ne zaman gelir?','İki gün.',$3) RETURNING id`, org, sugg, oldModel)
	for i, q := range []string{"Kargo ne zaman gelir?", "Kargom nerede?"} {
		if _, err = pool.Exec(ctx, `INSERT INTO knowledge_unit_questions(organization_id,unit_id,ordinal,content,embedding) VALUES($1,$2,$3,$4,$5::vector)`, org, unit, i, q, vec(q)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO ai_answer_cache(organization_id,unit_id,audience,locale,scope_key,question_hash,score) VALUES($1,$2,'agent','tr','org',repeat('b',64),0.9)`, org, unit); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO ai_case_memories(organization_id,conversation_id,locale,problem,resolution,embedding,embedding_model) VALUES($1,$2,'tr','Paket gelmedi','Yeniden gönderildi',$3::vector,$4)`,
		org, conv, vec("Paket gelmedi\nYeniden gönderildi"), oldModel); err != nil {
		t.Fatal(err)
	}
	category := one(`INSERT INTO categories(organization_id,name,slug) VALUES($1,'Kargo','kargo') RETURNING id`, org)
	for _, model := range []string{oldModel, newModel} { // the v2 twin was already written lazily
		if _, err = pool.Exec(ctx, `INSERT INTO category_examples(organization_id,category_id,content,embedding,embedding_model) VALUES($1,$2,'Kargo',$3::vector,$4)`, org, category, vec("Kargo"), model); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO category_examples(organization_id,category_id,content,embedding,embedding_model) VALUES($1,$2,'Teslimat',$3::vector,$4)`, org, category, vec("Teslimat"), oldModel); err != nil {
		t.Fatal(err)
	}
	for _, label := range []struct {
		text  string
		model string
	}{{"Paketim gelmedi", oldModel}, {"Sipariş gecikti", ""}} {
		var v any
		if label.model != "" {
			v = vec(label.text)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO category_labels(organization_id,subject_type,subject_uuid,action,content,embedding,embedding_model,category_id,labeled_by)
 VALUES($1,'conversation',gen_random_uuid(),'confirm',$2,$3::vector,$4,$5,$6)`, org, label.text, v, label.model, category, user); err != nil {
			t.Fatal(err)
		}
	}
	run := one(`INSERT INTO history_imports(organization_id,source,consent_analysis) VALUES($1,'upload',true) RETURNING id`, org)
	cluster := one(`INSERT INTO history_clusters(organization_id,centroid,embedding_model,size) VALUES($1,$2::vector,$3,2) RETURNING id`, org, vec("centroid"), oldModel)
	empty := one(`INSERT INTO history_clusters(organization_id,centroid,embedding_model,size) VALUES($1,$2::vector,$3,0) RETURNING id`, org, vec("empty"), oldModel)
	for i, c := range []struct{ subject, problem, resolution string }{{"Kargo", "Paket gelmedi", "Yeniden gönderildi"}, {"Kargo", "Paket gecikti", ""}} {
		if _, err = pool.Exec(ctx, `INSERT INTO history_cases(organization_id,import_id,source,external_ref,subject,problem,resolution,embedding,resolution_embedding,embedding_model,cluster_id)
 VALUES($1,$2,'upload',$3,$4,$5,$6,$7::vector,$8::vector,$9,$10)`, org, run, fmt.Sprintf("%064x", i+1),
			c.subject, c.problem, c.resolution, vec(c.problem), vec(c.resolution), oldModel, cluster); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO history_decisions(organization_id,cluster_id,decision,reason,title,body,embedding,embedding_model) VALUES($1,$2,'rejected','Yanlış','Kargo','Ücretli',$3::vector,$4)`,
		org, cluster, vec("Kargo\nÜcretli"), oldModel); err != nil {
		t.Fatal(err)
	}

	// Mid-transition: target-model queries see none of the old rows.
	q := db.New(pool)
	query := fakeVector(newModel, "Kargo ne zaman gelir?")
	assertQueries := func(want bool) {
		t.Helper()
		units, err := q.KnowledgeUnitMatch(ctx, db.KnowledgeUnitMatchParams{OrganizationID: org, Locale: "tr", Embedding: query, EmbeddingModel: newModel})
		if err != nil || (len(units) > 0) != want {
			t.Fatalf("units %v %v", units, err)
		}
		cases, err := q.CaseMemoryMatch(ctx, db.CaseMemoryMatchParams{OrganizationID: org, Locale: "tr", Embedding: query, EmbeddingModel: newModel, ExcludeConversationID: other})
		if err != nil || (len(cases) > 0) != want {
			t.Fatalf("case memories %v %v", cases, err)
		}
		cands, err := q.HistoryClusterCandidates(ctx, db.HistoryClusterCandidatesParams{OrganizationID: org, EmbeddingModel: newModel})
		if err != nil || (len(cands) > 0) != want {
			t.Fatalf("clusters %v %v", cands, err)
		}
		_, err = q.HistoryNearestRejection(ctx, db.HistoryNearestRejectionParams{OrganizationID: org, EmbeddingModel: newModel, Embedding: query})
		if (err == nil) != want {
			t.Fatalf("decisions %v", err)
		}
		var labels int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM category_labels WHERE organization_id=$1 AND embedding_model=$2`, org, newModel).Scan(&labels); err != nil || (labels > 0) != want {
			t.Fatalf("labels %d %v", labels, err)
		}
	}
	assertQueries(false)
	_, err = q.AnswerCacheGet(ctx, db.AnswerCacheGetParams{OrganizationID: org, Audience: "agent", Locale: "tr", ScopeKey: "org", QuestionHash: strings.Repeat("b", 64), EmbeddingModel: newModel})
	if err == nil {
		t.Fatal("answer cache served an old-model unit")
	}

	next := &fakeEmbedder{model: newModel}
	job := &vectorstore.Reembedder{Pool: pool, Embedder: next, Tables: VectorTables, Batch: 1, MaxBatches: 1, Organizations: []int64{org}}
	if err = job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var stale int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM history_cases WHERE organization_id=$1 AND embedding_model<>$2`, org, newModel).Scan(&stale); err != nil || stale != 1 {
		t.Fatalf("first run should stop after one case batch: %d %v", stale, err)
	}
	for range 5 { // later runs resume where the first stopped
		if err = job.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"knowledge_units", "ai_case_memories", "category_examples", "category_labels", "history_cases", "history_decisions"} {
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE organization_id=$1 AND embedding_model<>$2`, org, newModel).Scan(&stale); err != nil || stale != 0 {
			t.Fatalf("%s not re-embedded: %d %v", table, stale, err)
		}
	}
	assertQueries(true)

	var ok bool
	// Vectors equal a fresh embedding of the stored text.
	if err = pool.QueryRow(ctx, `SELECT bool_and(q.embedding=$2::vector) FROM knowledge_unit_questions q WHERE q.organization_id=$1 AND q.content='Kargom nerede?'`,
		org, vectorstore.Vector(fakeVector(newModel, "Kargom nerede?"))).Scan(&ok); err != nil || !ok {
		t.Fatalf("unit question vector %v %v", ok, err)
	}
	if err = pool.QueryRow(ctx, `SELECT embedding=$2::vector AND resolution_embedding=$3::vector FROM history_cases WHERE organization_id=$1 AND problem='Paket gelmedi'`,
		org, vectorstore.Vector(fakeVector(newModel, "Kargo\nPaket gelmedi")), vectorstore.Vector(fakeVector(newModel, "Yeniden gönderildi"))).Scan(&ok); err != nil || !ok {
		t.Fatalf("history case vectors %v %v", ok, err)
	}
	var nullResolution bool
	if err = pool.QueryRow(ctx, `SELECT resolution_embedding IS NULL FROM history_cases WHERE organization_id=$1 AND problem='Paket gecikti'`, org).Scan(&nullResolution); err != nil || !nullResolution {
		t.Fatalf("empty resolution must have no vector %v %v", nullResolution, err)
	}
	// Centroid: mean of the members' new vectors; a memberless cluster is invalid.
	if err = pool.QueryRow(ctx, `SELECT c.embedding_model=$3 AND c.centroid=(SELECT avg(embedding) FROM history_cases WHERE organization_id=$1 AND cluster_id=$2) FROM history_clusters c WHERE c.organization_id=$1 AND c.id=$2`,
		org, cluster, newModel).Scan(&ok); err != nil || !ok {
		t.Fatalf("centroid %v %v", ok, err)
	}
	var model string
	if err = pool.QueryRow(ctx, `SELECT embedding_model FROM history_clusters WHERE id=$1`, empty).Scan(&model); err != nil || model != "" {
		t.Fatalf("empty cluster %q %v", model, err)
	}
	var examples, cache int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM category_examples WHERE organization_id=$1),(SELECT count(*) FROM ai_answer_cache WHERE organization_id=$1)`, org).Scan(&examples, &cache); err != nil || examples != 2 || cache != 0 {
		t.Fatalf("twin example kept or old cache kept: examples=%d cache=%d %v", examples, cache, err)
	}
	// Nothing is left: another run makes no provider call.
	calls := next.calls
	if err = job.Run(ctx); err != nil || next.calls != calls {
		t.Fatalf("idle run embedded again: %d→%d %v", calls, next.calls, err)
	}
	// Label immutability still holds for anything but the vector.
	if _, err = pool.Exec(ctx, `UPDATE category_labels SET content='x' WHERE organization_id=$1`, org); err == nil {
		t.Fatal("label content changed")
	}
}

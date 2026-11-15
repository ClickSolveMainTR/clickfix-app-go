package aisystem

import "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"

// VectorTables is the re-embedding registry (CF-223, bilgi-katmani.md §4):
// every vector table outside knowledge_chunks with the text its vectors are
// built from. The text expressions mirror the code that writes each vector,
// so a re-embedded row matches a freshly written one. Order matters: a
// centroid table comes after its members.
var VectorTables = []vectorstore.Table{
	{
		// Knowledge units (CF-216): the question and variant vectors live in
		// knowledge_unit_questions, the model on the unit. Cached answers keep
		// an old-model score, so they are dropped and refilled on demand.
		Name: "knowledge_units", Table: "knowledge_units",
		Child: &vectorstore.Child{Table: "knowledge_unit_questions", Key: "unit_id", Column: "embedding", Text: "c.content"},
		After: "DELETE FROM ai_answer_cache WHERE organization_id=@org AND unit_id=ANY(@ids)",
	},
	{
		// Case memories (CF-216): aimemory embeds problem + "\n" + resolution.
		Name: "ai_case_memories", Table: "ai_case_memories",
		Vectors: []vectorstore.Column{{Column: "embedding", Text: "v.problem||E'\\n'||v.resolution"}},
	},
	{
		// Category example vectors (CF-218): unique per content and model, so
		// a row whose target-model copy was already written lazily is dropped.
		Name: "category_examples", Table: "category_examples",
		Vectors: []vectorstore.Column{{Column: "embedding", Text: "v.content"}},
		Twin: "EXISTS(SELECT 1 FROM category_examples t WHERE t.organization_id=v.organization_id AND t.category_id=v.category_id" +
			" AND t.content=v.content AND t.embedding_model=@target)",
	},
	{
		// Label store (CF-218): the masked label text, first 6000 runes. A
		// label saved without a vector (embedding_model '') is vectorized too.
		Name: "category_labels", Table: "category_labels",
		Vectors: []vectorstore.Column{{Column: "embedding", Text: "left(btrim(v.content),6000)"}},
	},
	{
		// History cases (CF-217): subject + problem (4000 runes) and the
		// resolution; an empty resolution has no vector.
		Name: "history_cases", Table: "history_cases",
		Vectors: []vectorstore.Column{
			{Column: "embedding", Text: "left(btrim(v.subject||E'\\n'||v.problem),4000)"},
			{Column: "resolution_embedding", Text: "btrim(v.resolution)", Nullable: true},
		},
	},
	{
		// History clusters (CF-217): the centroid has no text. It is the mean
		// of the member cases' problem vectors, so it is recomputed from them
		// once every case is on the new model; a cluster with no member left
		// is marked invalid (embedding_model '') and never matches again.
		Name: "history_clusters", Table: "history_clusters",
		Centroid: &vectorstore.Centroid{Column: "centroid", Members: "history_cases", Key: "cluster_id", Vector: "embedding"},
	},
	{
		// Decision memory (CF-217): title + "\n" + body, 6000 runes.
		Name: "history_decisions", Table: "history_decisions",
		Vectors: []vectorstore.Column{{Column: "embedding", Text: "left(btrim(v.title||E'\\n'||v.body),6000)"}},
	},
}

package vectorstore

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Source describes one vector table. Table, Join and Filter are trusted SQL
// written in code (never user input); Filter may reference named args given
// in Query.Args. The table must follow the package column convention.
type Source struct {
	// Name is the stable label used in logs.
	Name  string
	Table string
	// Join adds tables the Filter needs, e.g. the article of a chunk. The
	// vector table's alias is always "v".
	Join   string
	Filter string
	// TextColumn holds the (already masked) text the vector was built from;
	// the re-embedding job embeds it again.
	TextColumn string
}

var identifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func (s Source) valid() error {
	if !identifier.MatchString(s.Table) || !identifier.MatchString(s.TextColumn) {
		return fmt.Errorf("vectorstore: invalid source %q", s.Name)
	}
	return nil
}

// Query is one similarity search.
type Query struct {
	Vector []float32
	// Model is the model Vector was built with; only rows of that model are compared.
	Model string
	// MinSimilarity is the cosine similarity threshold (-1..1).
	MinSimilarity float64
	// Limit bounds the candidates (1..200, default 40).
	Limit int
	// Args are the named args of Source.Filter.
	Args pgx.NamedArgs
}

// Hit is one matching row. Specific rows carry the scope's brand or department;
// the others are organization-wide fallbacks.
type Hit struct {
	ID         int64
	Similarity float64
	Specific   bool
}

// Beginner is satisfied by *pgxpool.Pool and pgx.Tx.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Querier runs single queries (*pgxpool.Pool, pgx.Tx, *pgx.Conn).
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Search returns rows in scope whose similarity reaches the threshold: scope
// specific rows first, then organization-wide rows, each by similarity. The
// HNSW index is used with iterative scanning so scope filters do not starve
// the result.
func Search(ctx context.Context, db Beginner, src Source, scope Scope, q Query) ([]Hit, error) {
	if err := src.valid(); err != nil {
		return nil, err
	}
	if scope.OrganizationID <= 0 || q.Model == "" {
		return nil, fmt.Errorf("vectorstore: organization and model are required")
	}
	if err := Check(q.Vector); err != nil {
		return nil, err
	}
	if q.Limit <= 0 {
		q.Limit = 40
	}
	q.Limit = min(q.Limit, 200)
	args := pgx.NamedArgs{}
	for k, v := range q.Args {
		args[k] = v
	}
	where, specific := scope.predicate("v", args)
	if src.Filter != "" {
		where += " AND (" + src.Filter + ")"
	}
	args["query_vector"] = Vector(q.Vector)
	args["query_model"] = q.Model
	args["query_limit"] = q.Limit
	args["min_similarity"] = q.MinSimilarity
	// One ordered, limited candidate set per tier: with a brand/department
	// scope the specific rows get their own LIMIT, so organization-wide rows
	// cannot crowd them out of the candidates.
	inner := func(extra string) string {
		// #nosec G201 -- table, join and filter are code constants validated above; values are bound.
		return fmt.Sprintf(`(SELECT v.id,(1-(v.embedding <=> @query_vector::vector))::float8 AS similarity,%s AS specific
 FROM %s v %s
 WHERE %s%s AND v.embedding IS NOT NULL AND v.embedding_model=@query_model
 ORDER BY v.embedding <=> @query_vector::vector LIMIT @query_limit)`, specific, src.Table, src.Join, where, extra)
	}
	candidates := inner("")
	if specific != "false" {
		candidates = inner(" AND "+specific) + " UNION ALL " + inner(" AND NOT "+specific)
	}
	sql := `SELECT id,similarity,specific FROM (` + candidates + `) h
 WHERE similarity>=@min_similarity ORDER BY specific DESC,similarity DESC,id`
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// relaxed_order keeps scanning the index until LIMIT rows pass the filters.
	if _, err = tx.Exec(ctx, `SELECT set_config('hnsw.iterative_scan','relaxed_order',true),set_config('hnsw.ef_search','100',true)`); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	hits, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Hit, error) {
		var h Hit
		err := r.Scan(&h.ID, &h.Similarity, &h.Specific)
		return h, err
	})
	if err != nil {
		return nil, err
	}
	return hits, tx.Commit(ctx)
}

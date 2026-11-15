package vectorstore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
)

// Table is one in-place entry of the re-embedding registry (CF-223). Unlike a
// Source (pending_* columns, one switch per organization), these tables keep
// several models side by side and every query compares only rows of its own
// query model (`embedding_model=@model`). A re-embedded row therefore becomes
// searchable on its own, and a row still on the old model is simply not found
// by a target-model query: there is never a mixed-model comparison.
//
// Table, the SQL expressions and the column names are trusted code constants
// (never user input). The row table's alias is always "v"; it must have id,
// organization_id and embedding_model columns.
type Table struct {
	// Name is the stable label used in logs.
	Name  string
	Table string
	// Vectors are the row's own vector columns, each rebuilt from a text
	// expression.
	Vectors []Column
	// Child holds vectors kept in a child table whose rows share the row's
	// embedding_model (knowledge unit questions of a unit).
	Child *Child
	// Centroid is a vector with no stored text (cluster centroid). It is
	// recomputed as the mean of its members' target-model vectors once the
	// members table has no stale row left in the organization; a row without
	// any member is marked invalid with embedding_model ''.
	Centroid *Centroid
	// Twin is a condition (alias v, arg @target) that is true when a
	// target-model copy of the row already exists; the stale row is deleted
	// instead of re-embedded (tables unique on content and model).
	Twin string
	// After runs in each batch transaction with @org and @ids (bigint[]),
	// e.g. to drop cache rows whose score came from the old model.
	After string
}

// Column is a vector column and the SQL expression of the text it embeds.
type Column struct {
	Column string
	Text   string
	// Nullable columns get NULL for an empty text instead of a provider call.
	Nullable bool
}

// Child is a vector column of a child table; Key references the row id.
type Child struct {
	Table, Key, Column, Text string
}

// Centroid is a vector rebuilt from a members table: Key on the members
// references the row id and Vector is the member column that is averaged.
type Centroid struct {
	Column, Members, Key, Vector string
}

func (t Table) valid() error {
	names := []string{t.Table}
	for _, c := range t.Vectors {
		names = append(names, c.Column)
		if strings.TrimSpace(c.Text) == "" {
			return fmt.Errorf("vectorstore: table %q column %q has no text", t.Name, c.Column)
		}
	}
	kinds := 0
	if len(t.Vectors) > 0 {
		kinds++
	}
	if t.Child != nil {
		kinds++
		names = append(names, t.Child.Table, t.Child.Key, t.Child.Column)
		if strings.TrimSpace(t.Child.Text) == "" {
			return fmt.Errorf("vectorstore: table %q child has no text", t.Name)
		}
	}
	if t.Centroid != nil {
		kinds++
		names = append(names, t.Centroid.Column, t.Centroid.Members, t.Centroid.Key, t.Centroid.Vector)
	}
	if kinds != 1 {
		return fmt.Errorf("vectorstore: table %q needs exactly one of vectors, child or centroid", t.Name)
	}
	for _, n := range names {
		if !identifier.MatchString(n) {
			return fmt.Errorf("vectorstore: invalid table %q", t.Name)
		}
	}
	return nil
}

// staleRow selects rows of the row table (alias v) not yet on the target
// model. ” marks an invalid centroid, which waits for members instead.
func (t Table) staleRow() string {
	if t.Centroid != nil {
		return `v.embedding_model<>@target AND v.embedding_model<>''`
	}
	return `v.embedding_model<>@target`
}

func (r *Reembedder) table(ctx context.Context, t Table) error {
	if err := t.valid(); err != nil {
		return err
	}
	target := r.Embedder.Model()
	// #nosec G201 -- table name and condition are validated code constants.
	rows, err := r.Pool.Query(ctx, fmt.Sprintf(`SELECT DISTINCT v.organization_id FROM %s v WHERE %s AND %s ORDER BY 1 LIMIT 100`, t.Table, t.staleRow(), orgFilter),
		pgx.NamedArgs{"target": target, "orgs": r.orgs()})
	if err != nil {
		return err
	}
	orgs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	for _, org := range orgs {
		if err = ctx.Err(); err != nil {
			return err
		}
		octx := orgctx.WithOrganizationID(ctx, org)
		if t.Centroid != nil {
			err = r.centroid(octx, t, org, target)
		} else {
			err = r.tableOrganization(octx, t, org, target)
		}
		if err != nil {
			if r.Skip != nil && r.Skip(err) {
				slog.InfoContext(ctx, "vector_reembed_postponed", "source", t.Name, "organization_id", org, "error", err)
				continue
			}
			return err
		}
	}
	return nil
}

type tableRow struct {
	id    int64
	model string
	texts []string // one per Vectors column
	child []childRow
}

type childRow struct {
	id   int64
	text string
}

func (r *Reembedder) tableOrganization(ctx context.Context, t Table, org int64, target string) error {
	batch, maxBatches := r.limits()
	args := pgx.NamedArgs{"target": target, "org": org, "batch": batch}
	done := 0
	for range maxBatches {
		if t.Twin != "" {
			// #nosec G201 -- table name and conditions are validated code constants.
			if _, err := r.Pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s v WHERE v.organization_id=@org AND %s AND (%s)`, t.Table, t.staleRow(), t.Twin), args); err != nil {
				return err
			}
		}
		items, err := r.staleRows(ctx, t, args)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		texts := []string{}
		for _, it := range items {
			for i, text := range it.texts {
				if text != "" || !t.Vectors[i].Nullable {
					texts = append(texts, text)
				}
			}
			for _, c := range it.child {
				texts = append(texts, c.text)
			}
		}
		vectors := [][]float32{}
		if len(texts) > 0 {
			out, err := r.Embedder.Embed(ctx, texts)
			if err != nil {
				return err
			}
			if len(out.Vectors) != len(texts) {
				return ai.ErrInvalidOutput
			}
			for _, v := range out.Vectors {
				if err = Check(v); err != nil {
					return err
				}
			}
			vectors = out.Vectors
		}
		if err = r.writeRows(ctx, t, org, target, items, vectors); err != nil {
			return err
		}
		done += len(items)
		if len(items) < batch {
			break
		}
	}
	if done > 0 {
		slog.InfoContext(ctx, "vector_reembed_rows", "source", t.Name, "organization_id", org, "model", target, "rows", done)
	}
	return nil
}

func (r *Reembedder) staleRows(ctx context.Context, t Table, args pgx.NamedArgs) ([]tableRow, error) {
	cols := make([]string, len(t.Vectors))
	for i, c := range t.Vectors {
		cols[i] = fmt.Sprintf(",coalesce((%s)::text,'')", c.Text)
	}
	// #nosec G201 -- table name and expressions are validated code constants.
	rows, err := r.Pool.Query(ctx, fmt.Sprintf(`SELECT v.id,v.embedding_model%s FROM %s v WHERE v.organization_id=@org AND %s ORDER BY v.id LIMIT @batch`,
		strings.Join(cols, ""), t.Table, t.staleRow()), args)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tableRow, error) {
		it := tableRow{texts: make([]string, len(t.Vectors))}
		dest := []any{&it.id, &it.model}
		for i := range it.texts {
			dest = append(dest, &it.texts[i])
		}
		return it, row.Scan(dest...)
	})
	if err != nil || t.Child == nil || len(items) == 0 {
		return items, err
	}
	ids := make([]int64, len(items))
	at := make(map[int64]int, len(items))
	for i, it := range items {
		ids[i], at[it.id] = it.id, i
	}
	// #nosec G201 -- table, column names and expression are validated code constants.
	rows, err = r.Pool.Query(ctx, fmt.Sprintf(`SELECT c.id,c.%[2]s,coalesce((%[3]s)::text,'') FROM %[1]s c WHERE c.organization_id=@org AND c.%[2]s=ANY(@ids) ORDER BY c.%[2]s,c.id`,
		t.Child.Table, t.Child.Key, t.Child.Text), pgx.NamedArgs{"org": args["org"], "ids": ids})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c childRow
		var parent int64
		if err = rows.Scan(&c.id, &parent, &c.text); err != nil {
			return nil, err
		}
		items[at[parent]].child = append(items[at[parent]].child, c)
	}
	return items, rows.Err()
}

func (r *Reembedder) writeRows(ctx context.Context, t Table, org int64, target string, items []tableRow, vectors [][]float32) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	next := 0
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		args := pgx.NamedArgs{"org": org, "id": it.id, "target": target, "old": it.model}
		set := []string{"embedding_model=@target"}
		for i, c := range t.Vectors {
			key := fmt.Sprintf("v%d", i)
			if it.texts[i] == "" && c.Nullable {
				args[key] = nil
			} else {
				args[key] = Vector(vectors[next])
				next++
			}
			set = append(set, fmt.Sprintf("%s=@%s::vector", c.Column, key))
		}
		for _, c := range it.child {
			// #nosec G201 -- table and column names are validated code constants.
			if _, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$1::vector WHERE organization_id=$2 AND id=$3`, t.Child.Table, t.Child.Column), Vector(vectors[next]), org, c.id); err != nil {
				return err
			}
			next++
		}
		guard := ""
		if t.Twin != "" {
			guard = " AND NOT (" + t.Twin + ")"
		}
		// The old model guards against a row rebuilt by its owner meanwhile.
		// #nosec G201 -- table, column names and conditions are validated code constants.
		tag, execErr := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s v SET %s WHERE v.organization_id=@org AND v.id=@id AND v.embedding_model=@old%s`, t.Table, strings.Join(set, ","), guard), args)
		if execErr != nil {
			return execErr
		}
		if tag.RowsAffected() > 0 {
			ids = append(ids, it.id)
		}
	}
	if t.After != "" && len(ids) > 0 {
		if _, err = tx.Exec(ctx, t.After, pgx.NamedArgs{"org": org, "ids": ids}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// centroid rebuilds mean vectors once every member is on the target model.
func (r *Reembedder) centroid(ctx context.Context, t Table, org int64, target string) error {
	c := t.Centroid
	args := pgx.NamedArgs{"org": org, "target": target}
	var waiting bool
	// #nosec G201 -- table name is a validated code constant.
	if err := r.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE organization_id=@org AND embedding_model<>@target)`, c.Members), args).Scan(&waiting); err != nil || waiting {
		return err
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// #nosec G201 -- table, column names and condition are validated code constants.
	rebuilt, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %[1]s v SET %[2]s=m.mean,embedding_model=@target
 FROM (SELECT %[4]s AS owner,avg(%[5]s) AS mean FROM %[3]s WHERE organization_id=@org AND embedding_model=@target AND %[4]s IS NOT NULL AND %[5]s IS NOT NULL GROUP BY %[4]s) m
 WHERE v.organization_id=@org AND v.id=m.owner AND %[6]s`, t.Table, c.Column, c.Members, c.Key, c.Vector, t.staleRow()), args)
	if err != nil {
		return err
	}
	// #nosec G201 -- table name and condition are validated code constants.
	invalid, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s v SET embedding_model='' WHERE v.organization_id=@org AND %s`, t.Table, t.staleRow()), args)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	slog.InfoContext(ctx, "vector_reembed_centroids", "source", t.Name, "organization_id", org, "model", target, "rebuilt", rebuilt.RowsAffected(), "invalid", invalid.RowsAffected())
	return nil
}

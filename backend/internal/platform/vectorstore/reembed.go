package vectorstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reembedder rebuilds vectors whose model is not the configured one. New
// vectors go to pending_* columns, so search keeps the old ones; when no row
// of an organization is left, one UPDATE switches that organization to the
// new model. Embedding calls go through the metered embedder (organization
// wallet and policy), so an organization with AI off simply waits.
type Reembedder struct {
	Pool     *pgxpool.Pool
	Embedder ai.Embedder
	// Sources use pending_* columns and switch an organization at once
	// (knowledge chunks); Tables are re-embedded row by row in place. Both
	// run in registry order, so a centroid table listed after its members
	// sees them done.
	Sources []Source
	Tables  []Table
	// Skip reports product states (AI off, no credits) that postpone an
	// organization instead of failing the run.
	Skip func(error) bool
	// Batch is the rows per embedding call (default 64); MaxBatches bounds one
	// run per organization and source (default 20).
	Batch, MaxBatches int
	// Organizations, when set, limits a run to these organizations (tests
	// and manual runs); empty means every organization.
	Organizations []int64
}

// orgFilter is the organization predicate of the discovery queries.
const orgFilter = `(cardinality(@orgs::bigint[])=0 OR organization_id=ANY(@orgs::bigint[]))`

func (r *Reembedder) orgs() []int64 {
	if r.Organizations == nil {
		return []int64{}
	}
	return r.Organizations
}

// stale selects rows that still need a target-model vector.
const stale = `(pending_model IS DISTINCT FROM @target) AND (embedding IS NULL OR embedding_model<>@target)`

// Run visits every source once. It is safe to run concurrently with indexing:
// a row replaced meanwhile is simply not found by id.
func (r *Reembedder) Run(ctx context.Context) error {
	if r == nil || r.Pool == nil || r.Embedder == nil || r.Embedder.Model() == "" {
		return nil
	}
	var errs []error
	for _, src := range r.Sources {
		if err := r.source(ctx, src); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.Name, err))
		}
	}
	for _, t := range r.Tables {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := r.table(ctx, t); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Reembedder) source(ctx context.Context, src Source) error {
	if err := src.valid(); err != nil {
		return err
	}
	target := r.Embedder.Model()
	args := pgx.NamedArgs{"target": target, "orgs": r.orgs()}
	// #nosec G201 -- table name is a validated code constant.
	rows, err := r.Pool.Query(ctx, fmt.Sprintf(`SELECT DISTINCT organization_id FROM %[1]s WHERE %[2]s AND %[3]s
 UNION SELECT DISTINCT organization_id FROM %[1]s WHERE pending_model=@target AND %[3]s LIMIT 100`, src.Table, stale, orgFilter), args)
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
		if err = r.organization(orgctx.WithOrganizationID(ctx, org), src, org, target); err != nil {
			if r.Skip != nil && r.Skip(err) {
				slog.InfoContext(ctx, "vector_reembed_postponed", "source", src.Name, "organization_id", org, "error", err)
				continue
			}
			return err
		}
	}
	return nil
}

func (r *Reembedder) organization(ctx context.Context, src Source, org int64, target string) error {
	batch, maxBatches := r.limits()
	args := pgx.NamedArgs{"target": target, "org": org, "batch": batch}
	for range maxBatches {
		// #nosec G201 -- table and column names are validated code constants.
		rows, err := r.Pool.Query(ctx, fmt.Sprintf(`SELECT id,%s FROM %s WHERE organization_id=@org AND %s ORDER BY id LIMIT @batch`, src.TextColumn, src.Table, stale), args)
		if err != nil {
			return err
		}
		type item struct {
			ID   int64
			Text string
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (item, error) {
			var it item
			err := row.Scan(&it.ID, &it.Text)
			return it, err
		})
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		texts := make([]string, len(items))
		for i, it := range items {
			texts[i] = it.Text
		}
		out, err := r.Embedder.Embed(ctx, texts)
		if err != nil {
			return err
		}
		if len(out.Vectors) != len(items) {
			return ai.ErrInvalidOutput
		}
		tx, err := r.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		for i, it := range items {
			if err = Check(out.Vectors[i]); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			// #nosec G201 -- table name is a validated code constant.
			if _, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET pending_embedding=$1::vector,pending_model=$2 WHERE organization_id=$3 AND id=$4`, src.Table), Vector(out.Vectors[i]), target, org, it.ID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return r.swap(ctx, src, org, target)
}

// limits are the rows per embedding call and the calls per organization and
// table in one run; the next run resumes with the rows still stale.
func (r *Reembedder) limits() (int, int) {
	batch, maxBatches := r.Batch, r.MaxBatches
	if batch <= 0 || batch > 256 {
		batch = 64
	}
	if maxBatches <= 0 {
		maxBatches = 20
	}
	return batch, maxBatches
}

// swap switches the organization to the target model once nothing is left.
func (r *Reembedder) swap(ctx context.Context, src Source, org int64, target string) error {
	// #nosec G201 -- table name is a validated code constant.
	tag, err := r.Pool.Exec(ctx, fmt.Sprintf(`UPDATE %[1]s SET embedding=pending_embedding,embedding_model=pending_model,pending_embedding=NULL,pending_model=NULL
 WHERE organization_id=@org AND pending_model=@target AND NOT EXISTS(SELECT 1 FROM %[1]s s WHERE s.organization_id=@org AND (s.pending_model IS DISTINCT FROM @target) AND (s.embedding IS NULL OR s.embedding_model<>@target))`, src.Table),
		pgx.NamedArgs{"org": org, "target": target})
	if err == nil && tag.RowsAffected() > 0 {
		slog.InfoContext(ctx, "vector_reembed_switched", "source", src.Name, "organization_id", org, "model", target, "rows", tag.RowsAffected())
	}
	return err
}

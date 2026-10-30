package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Netgsm serializes browser/CDR merging inside a per-organization, per-instance transaction.
type Netgsm struct{ Pool *pgxpool.Pool }

func (r *Netgsm) InTx(ctx context.Context, org, id int64, fn func(*db.Queries) error) error {
	tx, e := r.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if e = q.LockNetgsmInstance(ctx, db.LockNetgsmInstanceParams{OrganizationID: org, ChannelInstanceID: id}); e != nil {
		return e
	}
	if e = fn(q); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

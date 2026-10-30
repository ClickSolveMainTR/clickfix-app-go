package repository

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(p *pgxpool.Pool, q *db.Queries) *Repository { return &Repository{p, q} }
func (r *Repository) InTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(r.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

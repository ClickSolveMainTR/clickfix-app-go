// Package repository provides scoped sqlc access and transactional outbox writes.
package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(pool *pgxpool.Pool, q *db.Queries) *Repository { return &Repository{Pool: pool, Q: q} }
func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(&Repository{Pool: r.Pool, Q: r.Q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Emit(ctx context.Context, e events.Event) error {
	return outbox.EnqueueQueries(ctx, r.Q, e)
}

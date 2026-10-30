// Package repository owns engineering persistence and transaction boundaries.
package repository

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(pool *pgxpool.Pool, q *db.Queries) *Store { return &Store{Pool: pool, Q: q} }
func (s *Store) InTx(ctx context.Context, fn func(context.Context, *db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(dbtx.WithTx(ctx, tx), s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

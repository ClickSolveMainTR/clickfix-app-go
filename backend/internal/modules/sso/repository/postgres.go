// Package repository owns SSO transaction boundaries.
package repository

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(pool *pgxpool.Pool, q *db.Queries) *Postgres { return &Postgres{Pool: pool, Q: q} }
func (p *Postgres) Transaction(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(p.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

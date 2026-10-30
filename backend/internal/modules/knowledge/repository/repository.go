// Package repository implements transactional sqlc persistence for knowledge articles.
package repository

import (
	"context"
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool, Q: db.New(pool)} }
func Normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23503", "23514":
			return model.ErrConflict
		}
	}
	return err
}

func (s *Store) Within(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(s.Q.WithTx(tx)); err != nil {
		return Normalize(err)
	}
	return Normalize(tx.Commit(ctx))
}

func Emit(ctx context.Context, q *db.Queries, ev events.Event) error {
	return outbox.EnqueueQueries(ctx, q, ev)
}

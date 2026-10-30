package repository

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkflowStore serializes quota decisions and mutations on the owning organization.
type WorkflowStore struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func (s *WorkflowStore) Atomic(ctx context.Context, org int64, fn func(*db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)
	if err = q.WorkflowOrgLock(ctx, org); err != nil {
		return err
	}
	if err = fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

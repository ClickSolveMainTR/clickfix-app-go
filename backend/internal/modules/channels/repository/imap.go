// Package repository provides persistence and locking for mailbox workers.
package repository

import (
	"context"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IMAP holds a transaction-scoped advisory lock on a dedicated connection.
// Cursor writes use the regular pool and survive a later fetch failure.
type IMAP struct{ Pool *pgxpool.Pool }

// WithLock prevents overlapping polling by workers across processes. Rollback
// releases the advisory lock even after context cancellation or a panic.
func (r *IMAP) WithLock(ctx context.Context, instanceID int64, fn func() error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Negative keys reserve a separate namespace from inbox locks.
	ok, err := db.New(tx).TryEmailIMAPLock(ctx, -(560000000000 + instanceID))
	if err != nil || !ok {
		return err
	}
	return fn()
}

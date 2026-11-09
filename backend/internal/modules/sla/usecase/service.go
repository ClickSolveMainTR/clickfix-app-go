// Package usecase is the SLA module (CF-46): business hours, holidays, SLA
// policies and the conversation clocks. Clocks follow the conversation
// through the durable event consumer (Sync) and a periodic sweep (Sweep)
// raises sla.warning / sla.breached through the outbox.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the service.
var (
	ErrNotFound = errors.New("sla: not found")
	ErrInvalid  = errors.New("sla: invalid request")
	ErrConflict = errors.New("sla: conflict")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Outbox writes domain events into an open transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Service is the SLA module.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	outbox   Outbox
	resyncer Resyncer
	log      *slog.Logger
	now      func() time.Time
	fields   FieldSource
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, outbox Outbox, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, outbox: outbox, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// inTx runs fn in a transaction. When ctx already carries one (an event
// consumer) fn joins it instead.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx, q *db.Queries) error) error {
	if tx, ok := dbtx.Tx(ctx); ok {
		return fn(ctx, tx, s.q.WithTx(tx))
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("sla: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(dbtx.WithTx(ctx, tx), tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("sla: commit: %w", err)
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

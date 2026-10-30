// Package repository owns recording transcript persistence through sqlc.
package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	*db.Queries
	Pool *pgxpool.Pool
}

// Finish commits the derived result, voice status and durable signal together.
func (s *Store) Finish(ctx context.Context, in db.TranscriptFinishParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.WithTx(tx)
	n, err := q.TranscriptFinish(ctx, in)
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit(ctx)
	}
	if err = q.TranscriptSyncVoice(ctx, db.TranscriptSyncVoiceParams{OrganizationID: in.OrganizationID, Uuid: in.Uuid}); err != nil {
		return err
	}
	ev := events.New("recording.transcribed").WithOrganization(in.OrganizationID).WithPayload(map[string]any{"transcript_uuid": in.Uuid.String(), "status": in.Status}).WithIdempotencyKey("transcript:" + in.Uuid.String())
	if err = outbox.EnqueueQueries(ctx, q, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

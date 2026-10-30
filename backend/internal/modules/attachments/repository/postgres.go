// Package repository persists attachment metadata.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound means no live attachment matched in the organization.
	ErrNotFound = errors.New("attachments: not found")
	// ErrQuotaExceeded means inserting would pass the organization quota.
	ErrQuotaExceeded = errors.New("attachments: organization quota exceeded")
)

// quotaLockNamespace separates the attachment quota advisory lock from other
// per-organization locks (the organization id is XORed into the low bits).
const quotaLockNamespace int64 = 0x41545441 << 32

// Postgres is the sqlc backed repository.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds a repository on pool.
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// InsertWithQuota inserts a row unless it would push the organization's live
// bytes past quotaBytes (0 = unlimited). Concurrent uploads of one
// organization are serialized by a transaction-scoped advisory lock, so the
// quota cannot be overshot by racing requests.
func (r *Postgres) InsertWithQuota(ctx context.Context, params db.InsertAttachmentParams, quotaBytes int64) (db.Attachment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return db.Attachment{}, fmt.Errorf("attachments: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if quotaBytes > 0 {
		if err := q.LockAttachmentQuota(ctx, quotaLockNamespace^params.OrganizationID); err != nil {
			return db.Attachment{}, fmt.Errorf("attachments: lock quota: %w", err)
		}
		usage, err := q.SumAttachmentUsage(ctx, params.OrganizationID)
		if err != nil {
			return db.Attachment{}, fmt.Errorf("attachments: sum usage: %w", err)
		}
		if usage.UsedBytes+params.SizeBytes > quotaBytes {
			return db.Attachment{}, ErrQuotaExceeded
		}
	}
	row, err := q.InsertAttachment(ctx, params)
	if err != nil {
		return db.Attachment{}, fmt.Errorf("attachments: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Attachment{}, fmt.Errorf("attachments: finish tx: %w", err)
	}
	return row, nil
}

// Get loads one live attachment of an organization.
func (r *Postgres) Get(ctx context.Context, orgID int64, id uuid.UUID) (db.Attachment, error) {
	row, err := r.q.GetAttachmentByUUID(ctx, db.GetAttachmentByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Attachment{}, ErrNotFound
	}
	return row, err
}

// List returns one page plus the total count.
func (r *Postgres) List(ctx context.Context, list db.ListAttachmentsParams, count db.CountAttachmentsParams) ([]db.Attachment, int64, error) {
	rows, err := r.q.ListAttachments(ctx, list)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountAttachments(ctx, count)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SoftDelete marks a live attachment deleted and returns it.
func (r *Postgres) SoftDelete(ctx context.Context, orgID int64, id uuid.UUID) (db.Attachment, error) {
	row, err := r.q.SoftDeleteAttachment(ctx, db.SoftDeleteAttachmentParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Attachment{}, ErrNotFound
	}
	return row, err
}

// Usage returns live bytes and file count of an organization.
func (r *Postgres) Usage(ctx context.Context, orgID int64) (db.SumAttachmentUsageRow, error) {
	return r.q.SumAttachmentUsage(ctx, orgID)
}

// UserUUIDs maps internal user ids to public uuids.
func (r *Postgres) UserUUIDs(ctx context.Context, ids []int64) (map[int64]uuid.UUID, error) {
	out := make(map[int64]uuid.UUID, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.ListUserUUIDsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Uuid
	}
	return out, nil
}

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(pool *pgxpool.Pool, q *db.Queries) *Repository { return &Repository{pool, q} }
func (r *Repository) Sync(ctx context.Context, org, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err = q.LockAnalyticsConversation(ctx, db.LockAnalyticsConversationParams{OrganizationID: fmt.Sprint(org), ConversationID: fmt.Sprint(id)}); err != nil {
		return err
	}
	if err = q.ProjectAnalyticsConversation(ctx, db.ProjectAnalyticsConversationParams{OrganizationID: org, ConversationID: id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *Repository) Backfill(ctx context.Context, org int64) error {
	var after int64
	for {
		ids, err := r.q.AnalyticsBackfillIDs(ctx, db.AnalyticsBackfillIDsParams{OrganizationID: org, AfterID: after})
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err = r.Sync(ctx, org, id); err != nil {
				return err
			}
			after = id
		}
	}
}
func nullable(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
func (r *Repository) Dashboard(ctx context.Context, org int64, f model.Filter) (map[string]any, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	tz, err := q.AnalyticsTimezone(ctx, org)
	if err != nil {
		return nil, err
	}
	if f.Timezone != "" {
		tz = f.Timezone
	}
	p := db.AnalyticsDashboardParams{OrganizationID: org, FromAt: pgtype.Timestamptz{Time: f.From, Valid: true}, ToAt: pgtype.Timestamptz{Time: f.To, Valid: true}, Channel: f.Channel, Team: nullable(f.Team), Agent: nullable(f.Agent), Tag: nullable(f.Tag), Bucket: f.Bucket, Timezone: tz}
	raw, err := q.AnalyticsDashboard(ctx, p)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	p.ToAt.Time = f.From
	p.FromAt.Time = f.From.Add(-f.To.Sub(f.From))
	prev, err := q.AnalyticsDashboard(ctx, p)
	if err != nil {
		return nil, err
	}
	out["comparison"] = json.RawMessage(prev)
	out["timezone"] = tz
	out["from"] = f.From
	out["to"] = f.To
	out["cohort"] = "created_at"
	out["backlog_basis"] = "current_status"
	return out, tx.Commit(ctx)
}

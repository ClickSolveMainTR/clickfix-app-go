// Package repository persists widget tokens, Live Activity push tokens and
// the Live Activity switch (CF-152) with sqlc.
package repository

import (
	"context"
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres implements usecase.Store.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the repository.
func New(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

func (r *Postgres) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if r.pool == nil {
		return fn(r.q)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error { return fn(r.q.WithTx(tx)) })
}

// IssueWidgetToken revokes the user's tokens on the device and stores the
// new one in one transaction.
func (r *Postgres) IssueWidgetToken(ctx context.Context, p db.CreateWidgetTokenParams) (db.WidgetToken, error) {
	var out db.WidgetToken
	err := r.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.RevokeWidgetTokensForDevice(ctx, db.RevokeWidgetTokensForDeviceParams{UserID: p.UserID, DeviceID: p.DeviceID}); err != nil {
			return err
		}
		row, err := q.CreateWidgetToken(ctx, p)
		out = row
		return err
	})
	return out, err
}

// RevokeWidgetTokens revokes the user's tokens on the device.
func (r *Postgres) RevokeWidgetTokens(ctx context.Context, userID int64, deviceID string) error {
	_, err := r.q.RevokeWidgetTokensForDevice(ctx, db.RevokeWidgetTokensForDeviceParams{UserID: userID, DeviceID: deviceID})
	return err
}

// ValidWidgetToken finds a usable token by hash; ok is false when there is none.
func (r *Postgres) ValidWidgetToken(ctx context.Context, hash string) (db.GetValidWidgetTokenRow, bool, error) {
	row, err := r.q.GetValidWidgetToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

// TouchWidgetToken records use (at most once a minute).
func (r *Postgres) TouchWidgetToken(ctx context.Context, id int64) error {
	return r.q.TouchWidgetToken(ctx, id)
}

// RebindWidgetTokens moves tokens bound to a rotated refresh session.
func (r *Postgres) RebindWidgetTokens(ctx context.Context, userID int64, from, to uuid.UUID) error {
	_, err := r.q.RebindWidgetTokensSession(ctx, db.RebindWidgetTokensSessionParams{
		UserID: userID, FromSession: pgUUID(from), ToSession: pgUUID(to),
	})
	return err
}

// UpsertLiveActivityToken stores a token; a new push-to-start token
// replaces the device's previous one of the same activity type.
func (r *Postgres) UpsertLiveActivityToken(ctx context.Context, p db.UpsertLiveActivityTokenParams) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.UpsertLiveActivityToken(ctx, p); err != nil {
			return err
		}
		if p.Kind != "push_to_start" {
			return nil
		}
		return q.DisableSupersededPushToStartTokens(ctx, db.DisableSupersededPushToStartTokensParams{
			UserID: p.UserID, DeviceID: p.DeviceID, ActivityType: p.ActivityType, Token: p.Token,
		})
	})
}

// DeleteLiveActivityToken removes one of the user's tokens.
func (r *Postgres) DeleteLiveActivityToken(ctx context.Context, userID int64, token string) error {
	_, err := r.q.DeleteLiveActivityToken(ctx, db.DeleteLiveActivityTokenParams{UserID: userID, Token: token})
	return err
}

// PushToStartTokens lists the user's active push-to-start tokens.
func (r *Postgres) PushToStartTokens(ctx context.Context, userID int64, activityType string) ([]db.LiveActivityToken, error) {
	return r.q.ListPushToStartLiveActivityTokens(ctx, db.ListPushToStartLiveActivityTokensParams{UserID: userID, ActivityType: activityType})
}

// UpdateTokensByRef lists active update tokens of running activities about ref.
func (r *Postgres) UpdateTokensByRef(ctx context.Context, activityType, ref string) ([]db.LiveActivityToken, error) {
	return r.q.ListUpdateLiveActivityTokensByRef(ctx, db.ListUpdateLiveActivityTokensByRefParams{ActivityType: activityType, Ref: ref})
}

// DisableLiveActivityToken stops using a token.
func (r *Postgres) DisableLiveActivityToken(ctx context.Context, id int64, reason string) error {
	return r.q.DisableLiveActivityToken(ctx, db.DisableLiveActivityTokenParams{ID: id, Reason: reason})
}

// LiveActivitiesEnabled reads the switch (absent row = on).
func (r *Postgres) LiveActivitiesEnabled(ctx context.Context, userID int64) (bool, error) {
	row, err := r.q.GetMobileSurfacePreferences(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return row.LiveActivitiesEnabled, nil
}

// SetLiveActivitiesEnabled stores the switch; turning it off deletes the
// user's Live Activity tokens in the same transaction.
func (r *Postgres) SetLiveActivitiesEnabled(ctx context.Context, userID int64, enabled bool) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.UpsertMobileSurfacePreferences(ctx, db.UpsertMobileSurfacePreferencesParams{UserID: userID, LiveActivitiesEnabled: enabled}); err != nil {
			return err
		}
		if enabled {
			return nil
		}
		_, err := q.DeleteLiveActivityTokensForUser(ctx, userID)
		return err
	})
}

// Recipient loads a user's push gates and locale.
func (r *Postgres) Recipient(ctx context.Context, userID int64) (db.GetLiveActivityRecipientRow, error) {
	return r.q.GetLiveActivityRecipient(ctx, userID)
}

// Conversation loads what a Live Activity shows about a conversation.
func (r *Postgres) Conversation(ctx context.Context, organizationID, id int64) (db.GetLiveActivityConversationRow, error) {
	return r.q.GetLiveActivityConversation(ctx, db.GetLiveActivityConversationParams{OrganizationID: organizationID, ID: id})
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

package usecase

import (
	"context"
	"sync"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	mu        sync.Mutex
	issued    []db.CreateWidgetTokenParams
	revoked   []string
	valid     map[string]db.GetValidWidgetTokenRow
	touched   []int64
	rebinds   [][2]uuid.UUID
	upserts   []db.UpsertLiveActivityTokenParams
	tokens    []db.LiveActivityToken
	disabled  map[int64]string
	enabled   map[int64]bool
	recipient map[int64]db.GetLiveActivityRecipientRow
	conv      map[int64]db.GetLiveActivityConversationRow
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		valid: map[string]db.GetValidWidgetTokenRow{}, disabled: map[int64]string{}, enabled: map[int64]bool{},
		recipient: map[int64]db.GetLiveActivityRecipientRow{}, conv: map[int64]db.GetLiveActivityConversationRow{},
	}
}

func (f *fakeStore) IssueWidgetToken(_ context.Context, p db.CreateWidgetTokenParams) (db.WidgetToken, error) {
	f.issued = append(f.issued, p)
	return db.WidgetToken{UserID: p.UserID, TokenHash: p.TokenHash}, nil
}

func (f *fakeStore) RevokeWidgetTokens(_ context.Context, _ int64, deviceID string) error {
	f.revoked = append(f.revoked, deviceID)
	return nil
}

func (f *fakeStore) ValidWidgetToken(_ context.Context, hash string) (db.GetValidWidgetTokenRow, bool, error) {
	row, ok := f.valid[hash]
	return row, ok, nil
}

func (f *fakeStore) TouchWidgetToken(_ context.Context, id int64) error {
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeStore) RebindWidgetTokens(_ context.Context, _ int64, from, to uuid.UUID) error {
	f.rebinds = append(f.rebinds, [2]uuid.UUID{from, to})
	return nil
}

func (f *fakeStore) UpsertLiveActivityToken(_ context.Context, p db.UpsertLiveActivityTokenParams) error {
	f.upserts = append(f.upserts, p)
	return nil
}

func (f *fakeStore) DeleteLiveActivityToken(context.Context, int64, string) error { return nil }

func (f *fakeStore) active(match func(db.LiveActivityToken) bool) []db.LiveActivityToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.LiveActivityToken
	for _, t := range f.tokens {
		if _, off := f.disabled[t.ID]; !off && match(t) {
			out = append(out, t)
		}
	}
	return out
}

func (f *fakeStore) PushToStartTokens(_ context.Context, userID int64, activityType string) ([]db.LiveActivityToken, error) {
	return f.active(func(t db.LiveActivityToken) bool {
		return t.UserID == userID && t.Kind == KindPushToStart && t.ActivityType == activityType
	}), nil
}

func (f *fakeStore) UpdateTokensByRef(_ context.Context, activityType, ref string) ([]db.LiveActivityToken, error) {
	return f.active(func(t db.LiveActivityToken) bool {
		return t.Kind == KindUpdate && t.ActivityType == activityType && t.Ref == ref
	}), nil
}

func (f *fakeStore) DisableLiveActivityToken(_ context.Context, id int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disabled[id] = reason
	return nil
}

func (f *fakeStore) LiveActivitiesEnabled(_ context.Context, userID int64) (bool, error) {
	on, ok := f.enabled[userID]
	return on || !ok, nil
}

func (f *fakeStore) SetLiveActivitiesEnabled(_ context.Context, userID int64, enabled bool) error {
	f.enabled[userID] = enabled
	return nil
}

func (f *fakeStore) Recipient(_ context.Context, userID int64) (db.GetLiveActivityRecipientRow, error) {
	r, ok := f.recipient[userID]
	if !ok {
		return r, pgx.ErrNoRows
	}
	return r, nil
}

func (f *fakeStore) Conversation(_ context.Context, _ int64, id int64) (db.GetLiveActivityConversationRow, error) {
	c, ok := f.conv[id]
	if !ok {
		return c, pgx.ErrNoRows
	}
	return c, nil
}

// fakeSender records pushes and answers with the result set per token.
type fakeSender struct {
	mu      sync.Mutex
	sent    []apns.Notification
	results map[string]apns.Result
	errs    map[string]error
}

func (s *fakeSender) SendLiveActivity(_ context.Context, n apns.Notification) (apns.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, n)
	if err := s.errs[n.Token]; err != nil {
		return apns.Result{}, err
	}
	if r, ok := s.results[n.Token]; ok {
		return r, nil
	}
	return apns.Result{Status: 200}, nil
}

// Package usecase implements agent presence, typing indicators and realtime
// channel authorization for organization channels (CF-10).
//
// Liveness follows the legacy inbox: an open panel heartbeats every 30
// seconds; a dropped socket never takes anyone offline by itself, the idle
// sweep does after IdleTimeout without a heartbeat. Transitions are
// idempotent and only real changes are published (org:{uuid} channel and
// the agent.presence_changed domain event).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Statuses.
const (
	StatusOnline  = "online"
	StatusAway    = "away"
	StatusBusy    = "busy"
	StatusOffline = "offline"
)

// Timing.
const (
	HeartbeatInterval = 30 * time.Second
	IdleTimeout       = 3 * time.Minute
	// TypingWindow: one typing "start" per user and conversation per window.
	TypingWindow = 2 * time.Second
)

// Errors mapped by the handler.
var (
	ErrInvalid     = errors.New("presence: invalid request")
	ErrNotFound    = errors.New("presence: not found")
	ErrRateLimited = errors.New("presence: rate limited")
)

// Limiter is the platform/ratelimit surface used for typing.
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Actor identifies the caller inside one organization.
type Actor struct {
	OrganizationID   int64
	OrganizationUUID uuid.UUID
	UserID           int64
	UserUUID         uuid.UUID
}

// Member is one organization member with presence.
type Member struct {
	UserUUID   uuid.UUID  `json:"user_uuid"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	ChangedAt  *time.Time `json:"changed_at"`
}

// Me is the caller's own presence.
type Me struct {
	Status       string    `json:"status"`
	ManualStatus string    `json:"manual_status"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	// HeartbeatSeconds tells the client how often to call heartbeat.
	HeartbeatSeconds int `json:"heartbeat_seconds"`
}

// Service is the presence use case.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	pub     realtime.Publisher
	limiter Limiter
	outbox  *outbox.Store
	log     *slog.Logger
	now     func() time.Time
}

// New builds the service; pub and limiter may be nil (no realtime / no limit).
func New(pool *pgxpool.Pool, q *db.Queries, pub realtime.Publisher, limiter Limiter, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if pub == nil {
		pub = realtime.NoopPublisher{}
	}
	return &Service{pool: pool, q: q, pub: pub, limiter: limiter, outbox: outbox.NewStore(pool, q), log: log, now: time.Now}
}

func validStatus(s string) bool {
	switch s {
	case StatusOnline, StatusAway, StatusBusy, StatusOffline:
		return true
	}
	return false
}

func toMe(r db.AgentPresence) Me {
	return Me{Status: r.Status, ManualStatus: r.ManualStatus, LastSeenAt: r.LastSeenAt.Time.UTC(), HeartbeatSeconds: int(HeartbeatInterval / time.Second)}
}

// List returns every member of the organization with presence. Rows whose
// heartbeat is older than IdleTimeout read as offline even before the sweep.
func (s *Service) List(ctx context.Context, orgID int64) ([]Member, error) {
	rows, err := s.q.ListOrganizationPresence(ctx, orgID)
	if err != nil {
		return nil, err
	}
	cutoff := s.now().Add(-IdleTimeout)
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		m := Member{UserUUID: r.UserUuid, Name: joinName(r.Name, r.Surname), Status: r.Status}
		if r.LastSeenAt.Valid {
			t := r.LastSeenAt.Time.UTC()
			m.LastSeenAt = &t
			if t.Before(cutoff) {
				m.Status = StatusOffline
			}
		}
		if r.ChangedAt.Valid {
			t := r.ChangedAt.Time.UTC()
			m.ChangedAt = &t
		}
		out = append(out, m)
	}
	return out, nil
}

func joinName(name, surname string) string {
	if surname == "" {
		return name
	}
	return name + " " + surname
}

// SetStatus stores the status the agent picked.
func (s *Service) SetStatus(ctx context.Context, a Actor, status string) (Me, error) {
	if !validStatus(status) {
		return Me{}, fmt.Errorf("%w: status must be online, away, busy or offline", ErrInvalid)
	}
	return s.transition(ctx, a, func(cur *db.AgentPresence) (string, string, string) {
		return status, status, "manual"
	})
}

// Idle is the automatic away (CF-54): the client reports that the agent has
// been inactive (idle=true) or is back (idle=false). Only an agent who picked
// "online" is moved; the status they picked (manual_status) is kept, so
// coming back restores it. Busy, away and offline agents are left alone.
func (s *Service) Idle(ctx context.Context, a Actor, idle bool) (Me, error) {
	return s.transition(ctx, a, func(cur *db.AgentPresence) (string, string, string) {
		if cur == nil {
			return StatusOffline, StatusOnline, "auto_away"
		}
		switch {
		case idle && cur.Status == StatusOnline && cur.ManualStatus == StatusOnline:
			return StatusAway, cur.ManualStatus, "auto_away"
		case !idle && cur.Status == StatusAway && cur.ManualStatus == StatusOnline:
			return StatusOnline, cur.ManualStatus, "auto_back"
		}
		return cur.Status, cur.ManualStatus, "auto_noop"
	})
}

// Heartbeat keeps the agent present. The first heartbeat (or the first
// after an idle sweep) restores the status the agent picked last.
func (s *Service) Heartbeat(ctx context.Context, a Actor) (Me, error) {
	row, err := s.q.GetAgentPresence(ctx, db.GetAgentPresenceParams{OrganizationID: a.OrganizationID, UserID: a.UserID})
	if err == nil && row.Status != StatusOffline {
		if s.now().Sub(row.LastSeenAt.Time) >= HeartbeatInterval/2 {
			if err := s.q.TouchAgentPresence(ctx, db.TouchAgentPresenceParams{OrganizationID: a.OrganizationID, UserID: a.UserID}); err != nil {
				return Me{}, err
			}
			row.LastSeenAt = pgtype.Timestamptz{Time: s.now().UTC(), Valid: true}
		}
		return toMe(row), nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Me{}, err
	}
	return s.transition(ctx, a, func(cur *db.AgentPresence) (string, string, string) {
		manual := StatusOnline
		if cur != nil {
			manual = cur.ManualStatus
		}
		return manual, manual, "heartbeat"
	})
}

// transition locks the row, applies next (status, manual, reason), writes
// the domain event in the same transaction and publishes after commit.
func (s *Service) transition(ctx context.Context, a Actor, next func(cur *db.AgentPresence) (status, manual, reason string)) (Me, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Me{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	var cur *db.AgentPresence
	if row, err := q.LockAgentPresence(ctx, db.LockAgentPresenceParams{OrganizationID: a.OrganizationID, UserID: a.UserID}); err == nil {
		cur = &row
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Me{}, err
	}
	status, manual, reason := next(cur)
	previous := StatusOffline
	if cur != nil {
		previous = cur.Status
	}
	row, err := q.UpsertAgentPresence(ctx, db.UpsertAgentPresenceParams{
		OrganizationID: a.OrganizationID, UserID: a.UserID, Status: status, ManualStatus: manual,
	})
	if err != nil {
		return Me{}, err
	}
	changed := previous != status
	if changed {
		if err := s.emit(ctx, tx, a, status, previous, reason); err != nil {
			return Me{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Me{}, err
	}
	if changed {
		s.publish(ctx, a.OrganizationUUID, a.UserUUID, status, row.LastSeenAt.Time)
	}
	return toMe(row), nil
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, a Actor, status, previous, reason string) error {
	ev, err := events.NewDomain(events.AgentPresenceChanged, a.OrganizationID, events.PresencePayload{
		UserID: a.UserID, UserUUID: a.UserUUID, Status: status, Previous: previous, Reason: reason,
	})
	if err != nil {
		return err
	}
	if reason != "idle" {
		ev = ev.WithActor(a.UserID)
	}
	return s.outbox.Enqueue(ctx, tx, ev)
}

func (s *Service) publish(ctx context.Context, orgUUID, userUUID uuid.UUID, status string, lastSeen time.Time) {
	msg := realtime.Message{Type: realtime.EventPresenceChanged, Data: map[string]any{
		"user_uuid":    userUUID.String(),
		"status":       status,
		"last_seen_at": lastSeen.UTC().Format(time.RFC3339),
	}}
	if err := s.pub.Publish(ctx, realtime.OrgChannel(orgUUID), msg); err != nil {
		s.log.Warn("presence_publish_failed", "error", err)
	}
}

// SweepIdle turns agents without a heartbeat for IdleTimeout offline.
func (s *Service) SweepIdle(ctx context.Context) (int, error) {
	cutoff := pgtype.Timestamptz{Time: s.now().Add(-IdleTimeout).UTC(), Valid: true}
	rows, err := s.q.ListIdleAgentPresence(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return n, err
		}
		q := s.q.WithTx(tx)
		cur, err := q.LockAgentPresence(ctx, db.LockAgentPresenceParams{OrganizationID: r.OrganizationID, UserID: r.UserID})
		if err != nil {
			_ = tx.Rollback(ctx)
			continue
		}
		changed, err := q.SetAgentPresenceOffline(ctx, db.SetAgentPresenceOfflineParams{OrganizationID: r.OrganizationID, UserID: r.UserID, Cutoff: cutoff})
		if err != nil || changed == 0 {
			_ = tx.Rollback(ctx)
			continue
		}
		a := Actor{OrganizationID: r.OrganizationID, OrganizationUUID: r.OrganizationUuid, UserID: r.UserID, UserUUID: r.UserUuid}
		if err := s.emit(ctx, tx, a, StatusOffline, cur.Status, "idle"); err != nil {
			_ = tx.Rollback(ctx)
			return n, err
		}
		if err := tx.Commit(ctx); err != nil {
			return n, err
		}
		s.publish(ctx, r.OrganizationUuid, r.UserUuid, StatusOffline, cur.LastSeenAt.Time)
		n++
	}
	return n, nil
}

// Typing publishes a typing indicator to a conversation of the
// organization. "start" is limited to one per TypingWindow; "stop" to a
// handful, so a misbehaving client cannot flood the channel.
func (s *Service) Typing(ctx context.Context, a Actor, name string, conversationUUID uuid.UUID, state string) error {
	if state != "start" && state != "stop" {
		return fmt.Errorf("%w: state must be start or stop", ErrInvalid)
	}
	conv, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: a.OrganizationID, Uuid: conversationUUID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	// Team visibility (CF-44): typing into a hidden conversation is a 404.
	if v, ok := access.From(ctx); ok {
		visible, err := v.CanSee(ctx, s.q, a.OrganizationID, conv.ID)
		if err != nil {
			return err
		}
		if !visible {
			return ErrNotFound
		}
	}
	if s.limiter != nil {
		limit := 1
		if state == "stop" {
			limit = 3
		}
		subject := a.UserUUID.String() + ":" + conversationUUID.String()
		if ok, _ := s.limiter.Allow(ctx, "typing_"+state, subject, limit, TypingWindow); !ok {
			return ErrRateLimited
		}
	}
	return s.pub.Publish(ctx, realtime.ConversationChannel(conversationUUID), realtime.Message{
		Type: realtime.EventConversationTyping,
		Data: map[string]any{"user_uuid": a.UserUUID.String(), "name": name, "state": state},
	})
}

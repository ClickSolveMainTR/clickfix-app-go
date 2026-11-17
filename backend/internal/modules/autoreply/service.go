package autoreply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Errors.
var (
	ErrInvalid  = errors.New("autoreply: invalid input")
	ErrNotFound = errors.New("autoreply: not found")
)

// ConsumerName is the durable consumer of inbound messages.
const ConsumerName = "auto_replies"

// OfflineRepeat is how long after an out-of-hours notice a conversation
// stays quiet: further messages in the same closed window get no second
// notice.
const OfflineRepeat = 8 * time.Hour

// Poster posts a bot message on a conversation (the inbox core).
type Poster interface {
	PostAuto(ctx context.Context, org, conversationID int64, kind, content string) (uuid.UUID, error)
}

// ETASource is the SLA module: the first-response target of a conversation.
type ETASource interface {
	FirstResponseTarget(ctx context.Context, orgID, conversationID int64) (*time.Duration, error)
}

// Service reads and writes the settings and reacts to inbound messages.
type Service struct {
	q      *db.Queries
	poster Poster
	hours  businesshours.Source
	eta    ETASource
	log    *slog.Logger
	now    func() time.Time
}

// New builds the service. hours and eta may be nil (always open, no ETA).
func New(q *db.Queries, poster Poster, hours businesshours.Source, eta ETASource, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: q, poster: poster, hours: hours, eta: eta, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// WithPoster sets the poster (the worker builds the inbox core after the
// service).
func (s *Service) WithPoster(p Poster) *Service { s.poster = p; return s }

// Get returns the organization's settings (defaults when never saved).
func (s *Service) Get(ctx context.Context, org int64) (Settings, error) {
	raw, err := s.q.GetAutoReplySettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return Defaults(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	out := Defaults()
	if err := json.Unmarshal(raw, &out); err != nil {
		return Settings{}, fmt.Errorf("autoreply: settings of organization %d: %w", org, err)
	}
	out, _ = Normalize(out)
	return out, nil
}

// Put replaces the organization's settings.
func (s *Service) Put(ctx context.Context, org int64, in Settings) (Settings, error) {
	in, err := Normalize(in)
	if err != nil {
		return Settings{}, err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return Settings{}, err
	}
	if err := s.q.UpsertAutoReplySettings(ctx, db.UpsertAutoReplySettingsParams{OrganizationID: org, Settings: raw}); err != nil {
		return Settings{}, err
	}
	return in, nil
}

// Consumer reacts to every inbound message within seconds of the event
// (the outbox relays in well under the 10 s the product promises).
func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name: ConsumerName, Events: []string{events.MessagesReceived}, MaxRetry: 3, Timeout: 30 * time.Second,
		Handle: func(ctx context.Context, ev events.Event) error {
			if ev.TenantID == nil {
				return nil
			}
			raw, _ := ev.Payload["message_uuid"].(string)
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil
			}
			return s.OnMessage(ctx, *ev.TenantID, id)
		},
	}
}

// Decision is what an inbound message gets: "" (nothing), KindAck or
// KindOffline.
type Decision string

// Decide is the rule, kept pure for tests: the first message of a
// conversation gets the acknowledgement inside working hours and the
// out-of-hours notice outside them; later messages outside hours get the
// notice again only when the last one is older than OfflineRepeat.
func Decide(first, open bool, lastOffline *time.Time, now time.Time) Decision {
	if open {
		if first {
			return KindAck
		}
		return ""
	}
	if lastOffline != nil && now.Sub(*lastOffline) < OfflineRepeat {
		return ""
	}
	return KindOffline
}

// OnMessage handles one inbound message. Business refusals (closed window,
// disabled kind) are not errors: the event is consumed once.
func (s *Service) OnMessage(ctx context.Context, org int64, message uuid.UUID) error {
	if s.poster == nil {
		return nil
	}
	msg, err := s.q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: org, Uuid: message})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if msg.Direction != "in" || msg.SenderType != "contact" || msg.Private {
		return nil
	}
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: msg.ConversationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	settings, err := s.Get(ctx, org)
	if err != nil {
		return err
	}
	count, err := s.q.CountContactMessages(ctx, db.CountContactMessagesParams{OrganizationID: org, ConversationID: conv.ID})
	if err != nil {
		return err
	}
	now := s.now()
	open := true
	if s.hours != nil {
		st, err := s.hours.HoursState(ctx, org, now)
		if err != nil {
			return err
		}
		open = st.Open
	}
	var lastOffline *time.Time
	if !open {
		last, err := s.q.LastAutoReplyAt(ctx, db.LastAutoReplyAtParams{OrganizationID: org, ConversationID: conv.ID, Kind: KindOffline})
		if err != nil {
			return err
		}
		if last.Valid {
			t := last.Time
			lastOffline = &t
		}
	}
	kind := Decide(count == 1, open, lastOffline, now)
	if kind == "" {
		return nil
	}
	// At-least-once delivery: a replayed event must not acknowledge twice.
	if kind == KindAck {
		last, err := s.q.LastAutoReplyAt(ctx, db.LastAutoReplyAtParams{OrganizationID: org, ConversationID: conv.ID, Kind: KindAck})
		if err != nil {
			return err
		}
		if last.Valid {
			return nil
		}
	}
	var eta *time.Duration
	if kind == KindAck && s.eta != nil {
		if eta, err = s.eta.FirstResponseTarget(ctx, org, conv.ID); err != nil {
			s.log.Warn("auto_reply_eta_failed", "organization_id", org, "error", err)
			eta = nil
		}
	}
	text := settings.Render(string(kind), s.locale(ctx, org), eta)
	if text == "" {
		return nil
	}
	if _, err := s.poster.PostAuto(ctx, org, conv.ID, string(kind), text); err != nil {
		s.log.Warn("auto_reply_post_failed", "organization_id", org, "conversation_id", conv.ID, "kind", kind, "error", err)
	}
	return nil
}

// NotifyHandoff tells the customer that a person takes over (the AI agent
// calls it when it hands off a conversation it was answering, could not
// answer safely, or the customer asked for a person). At most once per
// conversation; sent reports whether this call posted the notice.
func (s *Service) NotifyHandoff(ctx context.Context, org, conversationID int64) (bool, error) {
	if s.poster == nil {
		return false, nil
	}
	last, err := s.q.LastAutoReplyAt(ctx, db.LastAutoReplyAtParams{OrganizationID: org, ConversationID: conversationID, Kind: KindHandoff})
	if err != nil {
		return false, err
	}
	if last.Valid {
		return false, nil
	}
	settings, err := s.Get(ctx, org)
	if err != nil {
		return false, err
	}
	text := settings.Render(KindHandoff, s.locale(ctx, org), nil)
	if text == "" {
		return false, nil
	}
	if _, err := s.poster.PostAuto(ctx, org, conversationID, KindHandoff, text); err != nil {
		s.log.Warn("auto_reply_post_failed", "organization_id", org, "conversation_id", conversationID, "kind", KindHandoff, "error", err)
		return false, nil
	}
	return true, nil
}

// locale is the organization's locale ("tr" when unknown).
func (s *Service) locale(ctx context.Context, org int64) string {
	o, err := s.q.GetOrganizationByID(ctx, org)
	if err != nil || o.Locale == "" {
		return "tr"
	}
	return o.Locale
}

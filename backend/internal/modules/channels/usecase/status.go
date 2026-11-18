package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// StatusEvent is one conversation status transition read from the domain
// event (CF-245). To may be "snoozed|<RFC3339>" as the inbox writes it.
type StatusEvent struct {
	EventID        uuid.UUID
	OrganizationID int64
	ConversationID int64
	From, To       string
	At             time.Time
}

// NotifyStatus tells the conversation's channel about a status change when
// its adapter implements channels.StatusNotifier (REST API channel). Other
// channels, conversations without an instance and removed instances are
// skipped. Permanent delivery errors are logged and dropped; others are
// returned so the event consumer retries with the same event id.
func (s *Service) NotifyStatus(ctx context.Context, ev StatusEvent) error {
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: ev.OrganizationID, ID: ev.ConversationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !conv.ChannelInstanceID.Valid {
		return nil
	}
	inst, adapter, err := s.Instance(ctx, ev.OrganizationID, conv.ChannelInstanceID.Int64)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnknownType) {
		return nil
	}
	if err != nil {
		return err
	}
	notifier, ok := adapter.(channels.StatusNotifier)
	if !ok {
		return nil
	}
	change := channels.StatusChange{
		EventID: ev.EventID, ConversationUUID: conv.Uuid, ThreadID: conv.ExternalThreadID.String,
		From: statusName(ev.From), To: ev.To, At: ev.At,
	}
	if name, until, ok := strings.Cut(ev.To, "|"); ok {
		change.To = name
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			change.SnoozedUntil = &t
		}
	}
	if err := notifier.NotifyStatus(ctx, inst, change); err != nil {
		if channels.IsPermanent(err) {
			s.log.WarnContext(ctx, "channel_status_notify_dropped", "organization_id", ev.OrganizationID, "conversation_id", ev.ConversationID, "error", err)
			return nil
		}
		return err
	}
	return nil
}

func statusName(v string) string {
	name, _, _ := strings.Cut(v, "|")
	return name
}

// MessageRef is the public reference of a stored inbound message.
type MessageRef struct {
	ConversationUUID uuid.UUID
	MessageUUID      uuid.UUID
}

// MessageRef finds the message an instance stored for a provider message id
// and the conversation it joined (CF-245: the REST API channel's webhook
// answer). Not stored yet (paused instance) is the zero ref, not an error.
func (s *Service) MessageRef(ctx context.Context, inst channels.Instance, externalID string) (MessageRef, error) {
	m, err := s.q.GetMessageByExternal(ctx, db.GetMessageByExternalParams{
		OrganizationID: inst.OrganizationID, ChannelInstanceID: pgtype.Int8{Int64: inst.ID, Valid: true}, ExternalID: text(externalID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MessageRef{}, nil
	}
	if err != nil {
		return MessageRef{}, err
	}
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: inst.OrganizationID, ID: m.ConversationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return MessageRef{}, nil
	}
	if err != nil {
		return MessageRef{}, err
	}
	return MessageRef{ConversationUUID: conv.Uuid, MessageUUID: m.Uuid}, nil
}

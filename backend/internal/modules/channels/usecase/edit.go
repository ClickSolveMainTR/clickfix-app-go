package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EditEvent is an agent's correction of a delivered reply read from the
// domain event (CF-248).
type EditEvent struct {
	EventID        uuid.UUID
	OrganizationID int64
	ConversationID int64
	MessageUUID    uuid.UUID
	At             time.Time
}

// NotifyEdit tells the conversation's channel about an edited reply when
// its adapter implements channels.MessageEditor (REST API channel, web
// widget). It sends the message's current text, so a retry or a later
// edit never delivers an outdated one. Other channels, removed messages
// and removed instances are skipped. Permanent delivery errors are logged
// and dropped; others are returned so the consumer retries with the same
// event id.
func (s *Service) NotifyEdit(ctx context.Context, ev EditEvent) error {
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
	m, err := s.q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: ev.OrganizationID, Uuid: ev.MessageUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if m.ConversationID != conv.ID || m.Content == "" {
		return nil
	}
	inst, adapter, err := s.Instance(ctx, ev.OrganizationID, conv.ChannelInstanceID.Int64)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnknownType) {
		return nil
	}
	if err != nil {
		return err
	}
	editor, ok := adapter.(channels.MessageEditor)
	if !ok {
		return nil
	}
	to, err := s.recipient(ctx, conv, inst)
	if err != nil {
		return err
	}
	at := ev.At
	if m.EditedAt.Valid {
		at = m.EditedAt.Time
	}
	edit := channels.MessageEdit{
		EventID: ev.EventID, MessageUUID: m.Uuid, ConversationUUID: conv.Uuid, ExternalID: m.ExternalID.String,
		ThreadID: conv.ExternalThreadID.String, To: to, Text: m.Content, EditedAt: at.UTC(),
	}
	if err := editor.EditMessage(ctx, inst, edit); err != nil {
		if channels.IsPermanent(err) {
			s.log.WarnContext(ctx, "channel_message_edit_dropped", "organization_id", ev.OrganizationID, "message", m.Uuid, "error", err)
			return nil
		}
		return err
	}
	return nil
}

// recipient is the contact's identity on the conversation's channel: one
// scoped to the instance first, else a global one of the type. None is
// the empty recipient (the adapter decides whether it needs one).
func (s *Service) recipient(ctx context.Context, conv db.Conversation, inst channels.Instance) (channels.Recipient, error) {
	ids, err := s.q.ListContactIdentities(ctx, db.ListContactIdentitiesParams{OrganizationID: conv.OrganizationID, ContactID: conv.ContactID})
	if err != nil {
		return channels.Recipient{}, err
	}
	out := channels.Recipient{}
	for _, id := range ids {
		if id.ChannelType != inst.Type {
			continue
		}
		if id.ChannelInstanceID.Valid && id.ChannelInstanceID.Int64 == inst.ID {
			return channels.Recipient{ExternalID: id.ExternalID}, nil
		}
		if !id.ChannelInstanceID.Valid && out.ExternalID == "" {
			out.ExternalID = id.ExternalID
		}
	}
	return out, nil
}

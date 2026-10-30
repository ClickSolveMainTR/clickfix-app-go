package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

type CreateInput struct {
	Contact uuid.UUID `json:"contact_uuid"`
	Inbox   uuid.UUID `json:"inbox_uuid"`
	Subject string    `json:"subject"`
}

// Create opens an API-sourced conversation assigned to its creator. Messages use Post and its visibility checks.
func (s *Service) Create(ctx context.Context, org int64, actor Actor, in CreateInput) (db.Conversation, error) {
	var out db.Conversation
	if in.Contact == uuid.Nil || in.Inbox == uuid.Nil || strings.TrimSpace(in.Subject) == "" || len(in.Subject) > 500 {
		return out, ErrInvalid
	}
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		contact, err := tx.Q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: org, Uuid: in.Contact})
		if err != nil {
			return err
		}
		inbox, err := tx.Q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: org, Uuid: in.Inbox})
		if err != nil {
			return err
		}
		if inbox.Status != "active" {
			return ErrChannelClosed
		}
		out, err = tx.CreateConversation(ctx, repository.NewConversation{OrganizationID: org, ChannelType: inbox.ChannelType, InstanceID: inbox.ID, ContactID: contact.ID, CompanyID: contact.CompanyID.Int64, Subject: in.Subject, Source: "outbound", AssigneeUserID: actor.ID, At: s.now()})
		if err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ConversationsCreated, org, actor.ID, out, map[string]any{"source": "api"})
	})
	return out, err
}

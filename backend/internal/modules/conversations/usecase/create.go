package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CreateInput struct {
	Contact uuid.UUID `json:"contact_uuid"`
	// ContactExternalID picks the contact by the customer's own user id
	// instead of contact_uuid (CF-195).
	ContactExternalID string    `json:"contact_external_id"`
	Inbox             uuid.UUID `json:"inbox_uuid"`
	Subject           string    `json:"subject"`
}

// Create opens an API-sourced conversation assigned to its creator. Messages use Post and its visibility checks.
func (s *Service) Create(ctx context.Context, org int64, actor Actor, in CreateInput) (db.Conversation, error) {
	var out db.Conversation
	ext := strings.TrimSpace(in.ContactExternalID)
	if (in.Contact == uuid.Nil) == (ext == "") || in.Inbox == uuid.Nil || strings.TrimSpace(in.Subject) == "" || len(in.Subject) > 500 {
		return out, ErrInvalid
	}
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var contact db.Contact
		var err error
		if ext != "" {
			contact, err = tx.Q.GetContactByExternalID(ctx, db.GetContactByExternalIDParams{OrganizationID: org, ExternalID: ext})
		} else {
			contact, err = tx.Q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: org, Uuid: in.Contact})
		}
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
		if err := ensureWidgetUserIdentity(ctx, tx, org, inbox, contact); err != nil {
			return err
		}
		out, err = tx.CreateConversation(ctx, repository.NewConversation{OrganizationID: org, ChannelType: inbox.ChannelType, InstanceID: inbox.ID, ContactID: contact.ID, CompanyID: contact.CompanyID.Int64, Subject: in.Subject, Source: "outbound", AssigneeUserID: actor.ID, At: s.now()})
		if err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ConversationsCreated, org, actor.ID, out, map[string]any{"source": "api"})
	})
	return out, err
}

// ensureWidgetUserIdentity lets an agent write first to a customer who has
// not opened the site yet (CF-195): on a widget inbox the contact's
// external id becomes its widget identity ("user:<id>"), the id identify
// later resolves the signed-in visitor to, so the visitor finds the
// conversation. An identity another contact already holds is left alone.
func ensureWidgetUserIdentity(ctx context.Context, tx *repository.Tx, org int64, inbox db.ChannelInstance, contact db.Contact) error {
	if inbox.ChannelType != channels.TypeWidget || !contact.ExternalID.Valid {
		return nil
	}
	instance := pgtype.Int8{Int64: inbox.ID, Valid: true}
	id := widget.UserExternalID(contact.ExternalID.String)
	_, err := tx.Q.FindContactIdentity(ctx, db.FindContactIdentityParams{
		OrganizationID: org, ChannelType: channels.TypeWidget, ChannelInstanceID: instance, ExternalID: id,
	})
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = tx.Q.UpsertContactIdentity(ctx, db.UpsertContactIdentityParams{
		OrganizationID: org, ContactID: contact.ID, ChannelType: channels.TypeWidget, ChannelInstanceID: instance,
		ExternalID: id, DisplayName: contact.Name, Meta: []byte(`{"source":"external_id"}`),
	})
	return err
}

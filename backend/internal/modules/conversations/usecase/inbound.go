package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReceiveResult counts what a webhook delivery changed.
type ReceiveResult struct {
	Stored     int
	Duplicates int
	Statuses   int
}

// maxExternalID bounds provider ids stored for idempotency.
const maxExternalID = 255

// Receive runs the inbound pipeline for one verified webhook delivery of an
// instance: every message resolves its contact, joins (or opens) its
// conversation and is stored once; receipts move outbound messages forward.
// A failing message aborts the delivery so the provider retries it; messages
// already stored are skipped on the retry by their external id.
func (s *Service) Receive(ctx context.Context, inst channels.Instance, res channels.WebhookResult) (ReceiveResult, error) {
	var out ReceiveResult
	for _, m := range res.Messages {
		stored, err := s.receiveMessage(ctx, inst, m)
		if err != nil {
			return out, err
		}
		if stored {
			out.Stored++
		} else {
			out.Duplicates++
		}
	}
	for _, st := range res.Statuses {
		changed, err := s.ApplyStatus(ctx, inst, st)
		if err != nil {
			return out, err
		}
		if changed {
			out.Statuses++
		}
	}
	if len(res.Messages) > 0 {
		if err := s.store.Q().TouchChannelInstanceInbound(ctx, db.TouchChannelInstanceInboundParams{At: pgTime(s.now()), OrganizationID: inst.OrganizationID, ID: inst.ID}); err != nil {
			s.log.Warn("channel_instance_touch_failed", "instance", inst.UUID, "error", err)
		}
	}
	return out, nil
}

type inboundOutcome struct {
	conv     db.Conversation
	msg      db.Message
	contact  db.Contact
	created  bool
	reopened bool
	dup      bool
}

func (s *Service) receiveMessage(ctx context.Context, inst channels.Instance, m channels.InboundMessage) (bool, error) {
	if strings.TrimSpace(m.ExternalID) == "" || strings.TrimSpace(m.From.ExternalID) == "" {
		return false, fmt.Errorf("%w: inbound message needs an id and a sender", ErrInvalid)
	}
	if len(m.ExternalID) > maxExternalID {
		m.ExternalID = m.ExternalID[:maxExternalID]
	}
	if inst.Type == channels.TypeEmail && isSideThread(m.ThreadID) {
		// An answer to a side conversation (CF-98), not a customer message.
		if handled, stored, err := s.receiveSide(ctx, inst, m); handled {
			return stored, err
		}
	}
	at := m.At
	if at.IsZero() || at.After(s.now()) {
		at = s.now()
	}
	kind := m.Kind
	if !model.Valid(kind, []string{model.KindText, model.KindMedia, model.KindInteractive, model.KindEmail, model.KindUnsupported, model.KindTemplate}) {
		kind = model.KindText
		if len(m.Attachments) > 0 && strings.TrimSpace(m.Text) == "" {
			kind = model.KindMedia
		}
	}
	attrs, err := jsonObject(m.Attributes, map[string]any{"subject": m.Subject, "thread_id": m.ThreadID})
	if err != nil {
		return false, err
	}
	identityMeta, err := jsonObject(m.From.Meta, nil)
	if err != nil {
		return false, err
	}
	var o inboundOutcome
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		scoped := int64(0)
		if m.From.InstanceScoped {
			scoped = inst.ID
		}
		rc, err := tx.ResolveContact(ctx, repository.IdentityInput{
			OrganizationID: inst.OrganizationID, ChannelType: inst.Type, InstanceID: scoped,
			ExternalID: m.From.ExternalID, DisplayName: m.From.Name, Username: m.From.Username,
			Email: m.From.Email, Phone: m.From.Phone, Meta: identityMeta, SeenAt: at,
		})
		if err != nil {
			return err
		}
		o.contact = rc.Contact
		conv, created, err := s.conversationFor(ctx, tx, inst, rc.Contact, m, at)
		if err != nil {
			return err
		}
		o.created = created
		var replyTo int64
		if m.ReplyToExternalID != "" {
			if ref, err := tx.Q.GetMessageByExternal(ctx, db.GetMessageByExternalParams{
				OrganizationID: inst.OrganizationID, ChannelInstanceID: pgInt8(inst.ID),
				ExternalID: pgtype.Text{String: m.ReplyToExternalID, Valid: true},
			}); err == nil {
				replyTo = ref.ID
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		app, err := tx.AppendMessage(ctx, repository.NewMessage{
			OrganizationID: inst.OrganizationID, ConversationID: conv.ID, InstanceID: inst.ID,
			Direction: model.DirectionIn, SenderType: model.SenderContact, SenderContactID: rc.Contact.ID,
			Kind: kind, Content: m.Text, Attributes: attrs, ExternalID: m.ExternalID, ReplyToID: replyTo, At: at,
		})
		if err != nil {
			return err
		}
		o.conv, o.msg, o.dup, o.reopened = app.Conversation, app.Message, app.Duplicate, app.Reopened
		if app.Duplicate {
			return nil
		}
		if rc.Created {
			if err := outbox.EnqueueQueries(ctx, tx.Q, events.New(events.ContactCreated).WithOrganization(inst.OrganizationID).WithEntity("contact", &rc.Contact.ID, &rc.Contact.Uuid).WithPayload(map[string]any{"contact_uuid": rc.Contact.Uuid}).WithIdempotencyKey(rc.Contact.Uuid.String())); err != nil {
				return err
			}
		}
		if created {
			if err := s.emit(ctx, tx, events.ConversationsCreated, inst.OrganizationID, 0, app.Conversation, map[string]any{
				"channel_type": inst.Type, "channel_instance_uuid": inst.UUID.String(), "contact_uuid": rc.Contact.Uuid.String(),
				"number": app.Conversation.Number, "source": model.SourceInbound,
			}); err != nil {
				return err
			}
		}
		if app.Reopened {
			if err := s.emit(ctx, tx, events.ConversationsReopened, inst.OrganizationID, 0, app.Conversation, map[string]any{"by": "contact"}); err != nil {
				return err
			}
		}
		return s.emit(ctx, tx, events.MessagesReceived, inst.OrganizationID, 0, app.Conversation, map[string]any{
			"message_uuid": app.Message.Uuid.String(), "kind": kind, "channel_type": inst.Type,
			"contact_uuid": rc.Contact.Uuid.String(), "attachments": len(m.Attachments),
		})
	})
	if err != nil {
		return false, err
	}
	if !o.dup {
		s.log.InfoContext(ctx, "ai_stage", "stage", "message_received", "message_uuid", o.msg.Uuid, "organization_id", inst.OrganizationID)
	}
	if o.dup {
		return false, nil
	}
	s.storeAttachments(ctx, inst, o.msg, m.Attachments)
	if o.created {
		s.notify(ctx, inst.OrganizationID, model.LiveEvent{Type: model.EventConversationCreated, ConversationUUID: o.conv.Uuid})
	} else if o.reopened {
		s.notify(ctx, inst.OrganizationID, model.LiveEvent{Type: model.EventConversationUpdated, ConversationUUID: o.conv.Uuid})
	}
	contactUUID := o.contact.Uuid
	msg := toMessage(o.msg, o.conv.Uuid, model.Sender{Type: model.SenderContact, UUID: &contactUUID, Name: o.contact.Name})
	s.notify(ctx, inst.OrganizationID, model.LiveEvent{Type: model.EventMessageCreated, ConversationUUID: o.conv.Uuid, Message: &msg})
	s.indexed(ctx, o.conv.Uuid, o.msg.Uuid, o.contact.Uuid)
	return true, nil
}

// conversationFor picks the conversation an inbound message joins: the
// e-mail thread when the channel threads, else the contact's latest
// conversation on this inbox unless it is closed or was resolved longer
// than the reopen window ago; otherwise a new conversation.
func (s *Service) conversationFor(ctx context.Context, tx *repository.Tx, inst channels.Instance, contact db.Contact, m channels.InboundMessage, at time.Time) (db.Conversation, bool, error) {
	usable := func(c db.Conversation) bool {
		switch c.Status {
		case model.StatusClosed:
			return false
		case model.StatusResolved:
			return c.ResolvedAt.Valid && s.now().Sub(c.ResolvedAt.Time) < s.settings.ReopenWindow
		}
		return true
	}
	if m.ThreadID != "" {
		c, err := tx.Q.FindConversationByThread(ctx, db.FindConversationByThreadParams{
			OrganizationID: inst.OrganizationID, ChannelInstanceID: pgInt8(inst.ID),
			ExternalThreadID: pgtype.Text{String: m.ThreadID, Valid: true},
		})
		if err == nil {
			// A reply on a merged conversation's thread joins the target (CF-98).
			if c, err = followMerged(ctx, tx, c); err != nil {
				return db.Conversation{}, false, err
			}
		}
		if err == nil && usable(c) {
			return c, false, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.Conversation{}, false, err
		}
	} else {
		c, err := tx.Q.FindLatestConversationForContact(ctx, db.FindLatestConversationForContactParams{
			OrganizationID: inst.OrganizationID, ContactID: contact.ID, ChannelInstanceID: pgInt8(inst.ID),
		})
		if err == nil && usable(c) {
			return c, false, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.Conversation{}, false, err
		}
	}
	c, err := tx.CreateConversation(ctx, repository.NewConversation{
		OrganizationID: inst.OrganizationID, ChannelType: inst.Type, InstanceID: inst.ID,
		ContactID: contact.ID, CompanyID: contact.CompanyID.Int64, Subject: m.Subject,
		Source: model.SourceInbound, ExternalThreadID: m.ThreadID, At: at,
	})
	return c, true, err
}

// storeAttachments downloads inbound files into attachments owned by the
// message. Failures are logged: the text of the message is already stored
// and a provider retry would not bring the file back.
func (s *Service) storeAttachments(ctx context.Context, inst channels.Instance, msg db.Message, files []channels.InboundAttachment) {
	if s.files == nil {
		return
	}
	for _, f := range files {
		if f.Open == nil {
			continue
		}
		if err := func() error {
			body, err := f.Open(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = body.Close() }()
			name := f.Filename
			if name == "" {
				name = "attachment"
			}
			return s.files.StoreInbound(ctx, inst.OrganizationID, msg.Uuid, name, f.ContentType, body)
		}(); err != nil {
			s.log.Warn("inbound_attachment_failed", "instance", inst.UUID, "message", msg.Uuid, "error", err)
		}
	}
}

// ApplyStatus applies a delivery receipt. It reports whether the message
// moved (unknown ids and stale receipts are ignored).
func (s *Service) ApplyStatus(ctx context.Context, inst channels.Instance, st channels.StatusUpdate) (bool, error) {
	if st.ExternalID == "" || !model.Valid(st.Status, []string{channels.StatusDelivered, channels.StatusRead, channels.StatusFailed}) {
		return false, nil
	}
	at := st.At
	if at.IsZero() {
		at = s.now()
	}
	row, err := s.store.Q().UpdateMessageDeliveryByExternal(ctx, db.UpdateMessageDeliveryByExternalParams{
		Status: st.Status, Error: truncate(st.Error, 500), At: pgTime(at), OrganizationID: inst.OrganizationID,
		ChannelInstanceID: pgInt8(inst.ID), ExternalID: st.ExternalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	conv, err := s.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: inst.OrganizationID, ID: row.ConversationID})
	if err != nil {
		return true, nil
	}
	msg := toMessage(row, conv.Uuid, model.Sender{Type: row.SenderType})
	s.notify(ctx, inst.OrganizationID, model.LiveEvent{Type: model.EventMessageUpdated, ConversationUUID: conv.Uuid, Message: &msg})
	return true, nil
}

func jsonObject(m map[string]any, extra map[string]any) ([]byte, error) {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for k, v := range extra {
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		out[k] = v
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("conversations: attributes: %w", err)
	}
	return raw, nil
}

// ArchiveInbound retries a deferred provider recording without duplicating its message.
func (s *Service) ArchiveInbound(ctx context.Context, inst channels.Instance, external string, files []channels.InboundAttachment) error {
	if s.files == nil {
		return errors.New("attachment storage unavailable")
	}
	msg, err := s.store.Q().GetMessageByExternal(ctx, db.GetMessageByExternalParams{OrganizationID: inst.OrganizationID, ChannelInstanceID: pgInt8(inst.ID), ExternalID: pgtype.Text{String: external, Valid: true}})
	if err != nil {
		return err
	}
	count, err := s.store.Q().CountAttachments(ctx, db.CountAttachmentsParams{OrganizationID: inst.OrganizationID, OwnerType: pgtype.Text{String: "message", Valid: true}, OwnerID: pgtype.UUID{Bytes: msg.Uuid, Valid: true}})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, f := range files {
		body, err := f.Open(ctx)
		if err != nil {
			return err
		}
		err = s.files.StoreInbound(ctx, inst.OrganizationID, msg.Uuid, f.Filename, f.ContentType, body)
		_ = body.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxMessageLength bounds one message text (characters).
const MaxMessageLength = 10000

// PostInput is an agent's message or private note.
type PostInput struct {
	// EngineeringDelivery rechecks follow-up opt-outs at channel delivery.
	EngineeringDelivery uuid.UUID
	OrganizationID      int64
	ConversationUUID    uuid.UUID
	AuthorID            int64
	AuthorUUID          uuid.UUID
	AuthorName          string
	Content             string
	// Private posts an internal note (never sent to the contact).
	Private bool
	// ClientID makes a retried optimistic send idempotent.
	ClientID         uuid.UUID
	ReplyToUUID      uuid.UUID
	MentionedUserIDs []int64
	Template         *channels.Template
	// Attachments are files the author uploaded (ownerless) for this message.
	Attachments []uuid.UUID
	// Closing appends a closing text to a reply (CF-250); the zero value adds none.
	Closing Closing
}

// Posted is the stored message of Post.
type Posted struct {
	Message      model.Message
	Conversation db.Conversation
	Duplicate    bool
}

// Post stores an agent message (queued for delivery) or a private note.
// Replies need a channel; the reply window of the channel is enforced at
// delivery so the composer can still record the attempt.
func (s *Service) Post(ctx context.Context, in PostInput) (Posted, error) {
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" && in.Template == nil && len(in.Attachments) == 0 {
		return Posted{}, fmt.Errorf("%w: message is empty", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Content) > MaxMessageLength {
		return Posted{}, fmt.Errorf("%w: message is too long", ErrInvalid)
	}
	conv, err := s.store.Q().GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.ConversationUUID})
	if err != nil {
		return Posted{}, notFound(err)
	}
	if err := s.mustSee(ctx, in.OrganizationID, conv.ID); err != nil {
		return Posted{}, err
	}
	if !in.Private && !conv.ChannelInstanceID.Valid {
		return Posted{}, ErrNoChannel
	}
	var replyTo int64
	if in.ReplyToUUID != uuid.Nil {
		ref, err := s.store.Q().GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.ReplyToUUID})
		if err != nil || ref.ConversationID != conv.ID {
			return Posted{}, fmt.Errorf("%w: reply_to is not a message of this conversation", ErrInvalid)
		}
		replyTo = ref.ID
	}
	var closingAttrs map[string]any
	if !in.Private && in.Template == nil {
		in.Content, closingAttrs, err = s.applyClosing(ctx, conv, in.AuthorID, in.Closing, in.Content, nil)
		if err != nil {
			return Posted{}, err
		}
		if utf8.RuneCountInString(in.Content) > MaxMessageLength {
			return Posted{}, fmt.Errorf("%w: message is too long with the closing text", ErrInvalid)
		}
	}
	nm := repository.NewMessage{
		OrganizationID: in.OrganizationID, ConversationID: conv.ID, SenderType: model.SenderAgent,
		SenderUserID: in.AuthorID, Content: in.Content, ReplyToID: replyTo, At: s.now(),
	}
	if in.ClientID != uuid.Nil {
		nm.ClientID = pgtype.UUID{Bytes: in.ClientID, Valid: true}
	}
	if in.Private {
		nm.Direction, nm.Kind, nm.Private = model.DirectionInternal, model.KindNote, true
		nm.MentionedUserIDs = in.MentionedUserIDs
	} else {
		nm.Direction, nm.Kind, nm.Status = model.DirectionOut, model.KindText, model.MessageQueued
		nm.InstanceID = conv.ChannelInstanceID.Int64
		if in.Template != nil {
			nm.Kind = model.KindTemplate
			attrs, err := jsonObject(map[string]any{"template": map[string]any{
				"name": in.Template.Name, "language": in.Template.Language, "parameters": in.Template.Parameters,
			}}, nil)
			if err != nil {
				return Posted{}, err
			}
			nm.Attributes = attrs
		}
		if closingAttrs != nil {
			raw, err := json.Marshal(closingAttrs)
			if err != nil {
				return Posted{}, err
			}
			nm.Attributes = raw
		}
	}
	if in.EngineeringDelivery != uuid.Nil {
		var attrs map[string]any
		if len(nm.Attributes) > 0 {
			if err := json.Unmarshal(nm.Attributes, &attrs); err != nil {
				return Posted{}, err
			}
		}
		if attrs == nil {
			attrs = map[string]any{}
		}
		attrs["engineering_delivery_uuid"] = in.EngineeringDelivery.String()
		raw, err := json.Marshal(attrs)
		if err != nil {
			return Posted{}, err
		}
		nm.Attributes = raw
	}
	var app repository.Appended
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		app, err = tx.AppendMessage(ctx, nm)
		if err != nil || app.Duplicate {
			return err
		}
		if len(in.Attachments) > 0 {
			n, err := tx.Q.AttachToMessage(ctx, db.AttachToMessageParams{
				MessageUuid: pgtype.UUID{Bytes: app.Message.Uuid, Valid: true}, OrganizationID: in.OrganizationID,
				Uuids: in.Attachments, UploadedBy: pgInt8(in.AuthorID),
			})
			if err != nil {
				return err
			}
			if n != int64(len(in.Attachments)) {
				return fmt.Errorf("%w: attachments must be your own unattached uploads", ErrInvalid)
			}
		}
		name := events.MessagesSent
		if in.Private {
			name = events.ConversationsUpdated
		}
		return s.emit(ctx, tx, name, in.OrganizationID, in.AuthorID, app.Conversation, map[string]any{
			"message_uuid": app.Message.Uuid.String(), "private": in.Private, "kind": nm.Kind,
			"mentions": len(in.MentionedUserIDs),
		})
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return Posted{}, err
		}
		return Posted{}, notFound(err)
	}
	author := in.AuthorUUID
	msg := toMessage(app.Message, conv.Uuid, model.Sender{Type: model.SenderAgent, UUID: &author, Name: in.AuthorName})
	if app.Duplicate {
		if in.EngineeringDelivery != uuid.Nil && app.Message.Status == model.MessageQueued {
			if err := s.queueSend(ctx, in.OrganizationID, app.Message.ID, 0); err != nil {
				return Posted{}, err
			}
		}
		return Posted{Message: msg, Conversation: app.Conversation, Duplicate: true}, nil
	}
	if !in.Private {
		if err := s.queueSend(ctx, in.OrganizationID, app.Message.ID, 0); err != nil {
			if in.EngineeringDelivery != uuid.Nil {
				return Posted{}, err
			}
			s.log.Error("conversation_send_enqueue_failed", "message", app.Message.Uuid, "error", err)
		}
	}
	if len(in.Attachments) > 0 {
		files, _, err := s.messageExtras(ctx, in.OrganizationID, []int64{app.Message.ID}, []uuid.UUID{app.Message.Uuid})
		if err == nil {
			msg.Attachments = append(msg.Attachments, files[app.Message.Uuid]...)
		}
	}
	if len(in.MentionedUserIDs) > 0 && in.Private && s.mentions != nil {
		s.mentions.Mentioned(ctx, in.OrganizationID, in.MentionedUserIDs, Actor{ID: in.AuthorID, UUID: in.AuthorUUID, Name: in.AuthorName}, app.Conversation, model.Preview(in.Content))
	}
	s.notify(ctx, in.OrganizationID, model.LiveEvent{Type: model.EventMessageCreated, ConversationUUID: conv.Uuid, Message: &msg, ActorUUID: &author})
	s.indexed(ctx, conv.Uuid, app.Message.Uuid, uuid.Nil)
	return Posted{Message: msg, Conversation: app.Conversation}, nil
}

func (s *Service) queueSend(ctx context.Context, orgID, messageID int64, delay time.Duration) error {
	if s.queue == nil {
		return errors.New("conversations: no send queue configured")
	}
	return s.queue.EnqueueSend(ctx, orgID, messageID, delay)
}

// queueSendPriority queues an AI or automatic message on the AI lane when
// the queue has one (CF-251); otherwise it is an ordinary send.
func (s *Service) queueSendPriority(ctx context.Context, orgID, messageID int64) error {
	if fast, ok := s.queue.(interface {
		EnqueueSendPriority(ctx context.Context, orgID, messageID int64) error
	}); ok {
		return fast.EnqueueSendPriority(ctx, orgID, messageID)
	}
	return s.queueSend(ctx, orgID, messageID, 0)
}

// Retry queues a failed outbound message again.
func (s *Service) Retry(ctx context.Context, orgID int64, messageUUID uuid.UUID) error {
	m, err := s.store.Q().GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: orgID, Uuid: messageUUID})
	if err != nil {
		return notFound(err)
	}
	if err := s.mustSee(ctx, orgID, m.ConversationID); err != nil {
		return err
	}
	if m.Direction != model.DirectionOut || m.Status != model.MessageFailed {
		return fmt.Errorf("%w: only failed outbound messages can be retried", ErrInvalid)
	}
	if _, err := s.store.Q().MarkMessageQueued(ctx, db.MarkMessageQueuedParams{OrganizationID: orgID, ID: m.ID}); err != nil {
		return notFound(err)
	}
	return s.queueSend(ctx, orgID, m.ID, 0)
}

// DeliverAttempt carries the queue's retry bookkeeping into Deliver.
type DeliverAttempt struct {
	// Retried is how many times the task already ran.
	Retried int
	// MaxRetry is how many retries the task has in total.
	MaxRetry int
}

// ErrRetry asks the queue to retry a delivery later.
var ErrRetry = errors.New("conversations: delivery will be retried")

// Deliver sends one queued outbound message through its channel adapter.
// Permanent failures (no recipient, window closed, rejected content) and
// the last attempt mark the message failed and return nil; temporary
// failures return an error wrapping ErrRetry so the queue retries.
func (s *Service) Deliver(ctx context.Context, orgID, messageID int64, attempt DeliverAttempt) error {
	q := s.store.Q()
	m, err := q.GetMessageByID(ctx, db.GetMessageByIDParams{OrganizationID: orgID, ID: messageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // deleted meanwhile
	}
	if err != nil {
		return err
	}
	if m.Direction != model.DirectionOut || m.Status != model.MessageQueued || m.DeletedAt.Valid {
		return nil // already delivered, failed or claimed by another attempt
	}
	var guard struct {
		Delivery string `json:"engineering_delivery_uuid"`
	}
	if json.Unmarshal(m.ContentAttributes, &guard) == nil && guard.Delivery != "" {
		id, e := uuid.Parse(guard.Delivery)
		if e != nil {
			return e
		}
		allowed, e := q.EngineeringDeliveryAllowed(ctx, db.EngineeringDeliveryAllowedParams{OrganizationID: orgID, Uuid: id})
		if e != nil {
			return e
		}
		if !allowed {
			_, e = q.MarkMessageFailed(ctx, db.MarkMessageFailedParams{OrganizationID: orgID, ID: m.ID, Error: "customer_opt_out"})
			return e
		}
	}
	// Claim the message so overlapping or retried tasks never send it twice.
	if m, err = q.ClaimMessageForSend(ctx, db.ClaimMessageForSendParams{OrganizationID: orgID, ID: messageID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	conv, err := q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: m.ConversationID})
	if err != nil {
		return s.fail(ctx, m, conv, "conversation_missing", err)
	}
	if !conv.ChannelInstanceID.Valid {
		return s.fail(ctx, m, conv, "no_channel", ErrNoChannel)
	}
	inst, adapter, err := s.instances.Instance(ctx, orgID, conv.ChannelInstanceID.Int64)
	if err != nil {
		return s.fail(ctx, m, conv, "channel_unavailable", err)
	}
	if inst.Status != "active" && inst.Status != "error" {
		return s.fail(ctx, m, conv, "channel_inactive", ErrChannelClosed)
	}
	caps := adapter.Capabilities()
	if m.Kind != model.KindTemplate && !channels.WindowOpen(caps, conv.LastInboundAt.Time, s.now()) {
		return s.fail(ctx, m, conv, "window_closed", channels.ErrWindowClosed)
	}
	to, err := s.recipient(ctx, orgID, conv, inst)
	if err != nil {
		return s.fail(ctx, m, conv, "no_recipient", err)
	}
	if s.limiter != nil {
		if ok, wait := s.limiter.AllowSend(ctx, inst); !ok {
			if wait <= 0 {
				wait = time.Second
			}
			if err := s.release(ctx, m, ""); err != nil {
				return err
			}
			return s.queueSend(ctx, orgID, m.ID, wait)
		}
	}
	out := channels.OutboundMessage{
		MessageUUID: m.Uuid, ConversationUUID: conv.Uuid, To: to, Kind: m.Kind, Text: m.Content,
		ThreadID: conv.ExternalThreadID.String, Subject: conv.Subject,
	}
	if attrs, ok := decodeJSON(m.ContentAttributes).(map[string]any); ok {
		out.Attributes = attrs
		out.Template = templateFrom(attrs)
	}
	if m.Kind != model.KindTemplate {
		if text, added := s.aiClosing(ctx, conv, m, out.Attributes, out.Text); added {
			out.Text = text
			attrs := make(map[string]any, len(out.Attributes)+1)
			for k, v := range out.Attributes {
				attrs[k] = v
			}
			attrs[AttrClosingText] = true
			out.Attributes = attrs
		}
	}
	inboundReceivedAt := time.Time{}
	if m.ReplyToID.Valid {
		if ref, err := q.GetMessageByID(ctx, db.GetMessageByIDParams{OrganizationID: orgID, ID: m.ReplyToID.Int64}); err == nil {
			out.ReplyToExternalID = ref.ExternalID.String
			if ref.CreatedAt.Valid {
				inboundReceivedAt = ref.CreatedAt.Time
			}
		}
	}
	if err := s.applyEmailBrand(ctx, conv, &inst, &out); err != nil {
		return s.fail(ctx, m, conv, "brand_configuration", err)
	}
	sendStarted := time.Now()
	res, sendErr := adapter.Send(ctx, inst, out)
	s.log.InfoContext(ctx, "message_delivery_timing", "message_id", messageID, "organization_id", orgID, "duration_ms", time.Since(sendStarted).Milliseconds(), "failed", sendErr != nil)
	if sendErr != nil {
		_ = q.SetChannelInstanceError(ctx, db.SetChannelInstanceErrorParams{Error: truncate(sendErr.Error(), 500), OrganizationID: inst.OrganizationID, ID: inst.ID})
		if channels.IsPermanent(sendErr) || attempt.Retried >= attempt.MaxRetry {
			return s.fail(ctx, m, conv, "send_failed", sendErr)
		}
		s.log.Warn("conversation_send_retry", "message", m.Uuid, "attempt", attempt.Retried+1, "error", sendErr)
		if err := s.release(ctx, m, truncate(sendErr.Error(), 500)); err != nil {
			return err
		}
		return fmt.Errorf("%w: %w", ErrRetry, sendErr)
	}
	if m.SenderType == model.SenderBot {
		endToEnd := int64(0)
		if !inboundReceivedAt.IsZero() {
			endToEnd = time.Since(inboundReceivedAt).Milliseconds()
		}
		s.log.InfoContext(ctx, "ai_stage", "stage", "message_sent", "message_uuid", m.Uuid, "trigger_message_id", m.ReplyToID.Int64,
			"organization_id", orgID, "received_to_sent_ms", endToEnd, "delivery_ms", time.Since(sendStarted).Milliseconds())
	}
	// The provider accepted the message: recording it is retried here, never
	// by the queue (a queue retry would send it again). If recording keeps
	// failing the message stays pending and is not resent.
	var sent db.Message
	for try := 0; try < 3; try++ {
		err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
			var err error
			sent, err = tx.Q.MarkMessageSent(ctx, db.MarkMessageSentParams{
				ExternalID: pgText(truncate(res.ExternalID, maxExternalID)), At: pgTime(s.now()), OrganizationID: orgID, ID: m.ID,
			})
			if err != nil {
				return err
			}
			if res.Usage != nil && res.Usage.Quantity > 0 && s.outbox != nil {
				key := "channel-send:" + m.Uuid.String()
				ev := events.New(events.BillingUsageRecorded).WithTenant(orgID).WithIdempotencyKey("usage:" + key).WithPayload(map[string]any{
					"feature": res.Usage.Feature, "quantity": res.Usage.Quantity, "event_key": key, "occurred_at": s.now(),
				})
				if err := s.outbox.Enqueue(ctx, tx.Raw, ev); err != nil {
					return err
				}
			}
			return s.emit(ctx, tx, events.MessagesStatusChanged, orgID, 0, conv, map[string]any{
				"message_uuid": m.Uuid.String(), "status": model.MessageSent,
			})
		})
		if err == nil || errors.Is(err, pgx.ErrNoRows) || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(try+1) * 200 * time.Millisecond)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		s.log.Error("conversation_send_record_failed", "message", m.Uuid, "external_id", res.ExternalID, "error", err)
		return nil
	}
	_ = q.TouchChannelInstanceOutbound(ctx, db.TouchChannelInstanceOutboundParams{At: pgTime(s.now()), OrganizationID: inst.OrganizationID, ID: inst.ID})
	msg := toMessage(sent, conv.Uuid, model.Sender{Type: sent.SenderType})
	s.notify(ctx, orgID, model.LiveEvent{Type: model.EventMessageUpdated, ConversationUUID: conv.Uuid, Message: &msg})
	return nil
}

// release hands a claimed message back to the queue.
func (s *Service) release(ctx context.Context, m db.Message, reason string) error {
	return s.store.Q().ReleaseMessageClaim(ctx, db.ReleaseMessageClaimParams{Error: reason, OrganizationID: m.OrganizationID, ID: m.ID})
}

func (s *Service) fail(ctx context.Context, m db.Message, conv db.Conversation, code string, cause error) error {
	reason := code
	if cause != nil {
		reason = code + ": " + cause.Error()
	}
	var failed db.Message
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		failed, err = tx.Q.MarkMessageFailed(ctx, db.MarkMessageFailedParams{Error: truncate(reason, 500), OrganizationID: m.OrganizationID, ID: m.ID})
		if err != nil {
			return err
		}
		if conv.ID == 0 {
			return nil
		}
		return s.emit(ctx, tx, events.MessagesStatusChanged, m.OrganizationID, 0, conv, map[string]any{
			"message_uuid": m.Uuid.String(), "status": model.MessageFailed, "code": code,
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	s.log.Warn("conversation_send_failed", "message", m.Uuid, "code", code, "error", cause)
	if conv.ID != 0 {
		msg := toMessage(failed, conv.Uuid, model.Sender{Type: failed.SenderType})
		s.notify(ctx, m.OrganizationID, model.LiveEvent{Type: model.EventMessageUpdated, ConversationUUID: conv.Uuid, Message: &msg})
	}
	return nil
}

// recipient finds the contact's identity on the conversation's channel:
// an identity scoped to this instance first, else a global one of the type.
func (s *Service) recipient(ctx context.Context, orgID int64, conv db.Conversation, inst channels.Instance) (channels.Recipient, error) {
	contact, err := s.store.Q().GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: orgID, ID: conv.ContactID})
	if err != nil {
		return channels.Recipient{}, err
	}
	ids, err := s.store.Q().ListContactIdentities(ctx, db.ListContactIdentitiesParams{OrganizationID: orgID, ContactID: conv.ContactID})
	if err != nil {
		return channels.Recipient{}, err
	}
	var global *db.ListContactIdentitiesRow
	for i := range ids {
		id := ids[i]
		if id.ChannelType != inst.Type {
			continue
		}
		if id.ChannelInstanceID.Valid && id.ChannelInstanceID.Int64 == inst.ID {
			return channels.Recipient{ExternalID: id.ExternalID, Name: contact.Name, Email: contact.Email.String, Phone: contact.Phone.String}, nil
		}
		if !id.ChannelInstanceID.Valid && global == nil {
			global = &id
		}
	}
	if global != nil {
		return channels.Recipient{ExternalID: global.ExternalID, Name: contact.Name, Email: contact.Email.String, Phone: contact.Phone.String}, nil
	}
	return channels.Recipient{}, ErrNoRecipient
}

func templateFrom(attrs map[string]any) *channels.Template {
	raw, ok := attrs["template"].(map[string]any)
	if !ok {
		return nil
	}
	t := &channels.Template{}
	t.Name, _ = raw["name"].(string)
	t.Language, _ = raw["language"].(string)
	if params, ok := raw["parameters"].([]any); ok {
		for _, p := range params {
			if v, ok := p.(string); ok {
				t.Parameters = append(t.Parameters, v)
			}
		}
	}
	if t.Name == "" {
		return nil
	}
	return t
}

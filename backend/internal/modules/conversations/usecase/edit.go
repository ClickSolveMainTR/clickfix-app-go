package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MessageEditWindow is how long after sending an agent may correct a reply
// (CF-248). The frontend shows "Düzenle" for the same time.
const MessageEditWindow = 15 * time.Minute

// Edit errors.
var (
	// ErrNotEditable: the message is not a public text reply that can be
	// edited (note, template, contact message, being sent right now).
	ErrNotEditable = errors.New("conversations: message cannot be edited")
	// ErrNotAuthor: only the agent who wrote a reply may edit it.
	ErrNotAuthor = errors.New("conversations: only the author can edit this message")
	// ErrEditWindowClosed: the edit window of the message has passed.
	ErrEditWindowClosed = errors.New("conversations: the edit window of this message has passed")
)

// Channel edit outcomes of an edit (Edited.ChannelEdit).
const (
	// ChannelEditNotified: the channel will update the contact's copy.
	ChannelEditNotified = "notified"
	// ChannelEditUnsupported: the channel cannot update a delivered
	// message; the contact keeps the first text.
	ChannelEditUnsupported = "unsupported"
	// ChannelEditNotSent: the reply was not delivered yet (queued or
	// failed); the next delivery carries the new text.
	ChannelEditNotSent = "not_sent"
)

// EditInput is an agent's correction of their own reply.
type EditInput struct {
	OrganizationID   int64
	ConversationUUID uuid.UUID
	MessageUUID      uuid.UUID
	EditorID         int64
	EditorUUID       uuid.UUID
	EditorName       string
	Content          string
}

// Edited is the result of EditMessage.
type Edited struct {
	Message      model.Message   `json:"message"`
	ChannelEdit  string          `json:"channel_edit"`
	Conversation db.Conversation `json:"-"`
	Previous     string          `json:"-"`
}

// MessageEditEntry is one earlier version of an edited message.
type MessageEditEntry struct {
	UUID uuid.UUID `json:"uuid"`
	// PreviousContent is the text before this edit.
	PreviousContent string     `json:"previous_content"`
	Editor          *model.Ref `json:"editor"`
	EditedAt        time.Time  `json:"edited_at"`
}

// delivered statuses: the channel already has the reply.
var deliveredStatuses = map[string]bool{model.MessageSent: true, model.MessageDelivered: true, model.MessageRead: true}

// EditMessage rewrites an agent's own public text reply within
// MessageEditWindow, keeps the previous text in the edit history and sets
// edited_at. When the channel already has the reply, a conversations
// event with change=message_edited asks channels that can update it
// (channels.MessageEditor) to do so; the outcome says what the contact
// will see.
func (s *Service) EditMessage(ctx context.Context, in EditInput) (Edited, error) {
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return Edited{}, fmt.Errorf("%w: message is empty", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Content) > MaxMessageLength {
		return Edited{}, fmt.Errorf("%w: message is too long", ErrInvalid)
	}
	q := s.store.Q()
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.ConversationUUID})
	if err != nil {
		return Edited{}, notFound(err)
	}
	if err := s.mustSee(ctx, in.OrganizationID, conv.ID); err != nil {
		return Edited{}, err
	}
	m, err := q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.MessageUUID})
	if err != nil {
		return Edited{}, notFound(err)
	}
	if m.ConversationID != conv.ID {
		return Edited{}, ErrNotFound
	}
	if err := editable(m, in.EditorID, s.now()); err != nil {
		return Edited{}, err
	}
	if in.Content == m.Content {
		return Edited{}, fmt.Errorf("%w: message is unchanged", ErrInvalid)
	}
	now := s.now()
	notify := deliveredStatuses[m.Status]
	var edited db.Message
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		edited, err = tx.Q.EditOutboundMessage(ctx, db.EditOutboundMessageParams{
			Content: in.Content, EditedAt: pgTime(now), OrganizationID: in.OrganizationID, ID: m.ID,
			EditorUserID: pgInt8(in.EditorID), PreviousContent: m.Content,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Sent, edited or deleted meanwhile.
			return fmt.Errorf("%w: the message changed meanwhile, reload and try again", ErrNotEditable)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Q.InsertMessageEdit(ctx, db.InsertMessageEditParams{
			OrganizationID: in.OrganizationID, MessageID: m.ID, EditorUserID: pgInt8(in.EditorID),
			PreviousContent: m.Content, CreatedAt: pgTime(now),
		}); err != nil {
			return err
		}
		if err := tx.Q.SetConversationPreviewIfLatest(ctx, db.SetConversationPreviewIfLatestParams{
			Preview: model.PreviewOutbound(in.Content), OrganizationID: in.OrganizationID, ConversationID: conv.ID, MessageID: m.ID,
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ConversationsUpdated, in.OrganizationID, in.EditorID, conv, map[string]any{
			"change": "message_edited", "message_uuid": m.Uuid.String(), "notify_channel": notify,
			"edited_at": now.Format(time.RFC3339Nano),
		})
	})
	if err != nil {
		if errors.Is(err, ErrNotEditable) {
			return Edited{}, err
		}
		return Edited{}, notFound(err)
	}
	editor := in.EditorUUID
	msg := toMessage(edited, conv.Uuid, model.Sender{Type: model.SenderAgent, UUID: &editor, Name: in.EditorName})
	if edited.ReplyToID.Valid {
		if r, err := q.GetMessageByID(ctx, db.GetMessageByIDParams{OrganizationID: in.OrganizationID, ID: edited.ReplyToID.Int64}); err == nil {
			msg.ReplyTo = &r.Uuid
		}
	}
	if files, _, err := s.messageExtras(ctx, in.OrganizationID, []int64{edited.ID}, []uuid.UUID{edited.Uuid}); err == nil {
		msg.Attachments = append(msg.Attachments, files[edited.Uuid]...)
	}
	out := Edited{Message: msg, Conversation: conv, Previous: m.Content, ChannelEdit: ChannelEditNotSent}
	if notify {
		out.ChannelEdit = ChannelEditUnsupported
		if s.registry.CanEdit(conv.ChannelType) {
			out.ChannelEdit = ChannelEditNotified
		}
	}
	s.notify(ctx, in.OrganizationID, model.LiveEvent{Type: model.EventMessageUpdated, ConversationUUID: conv.Uuid, Message: &msg, ActorUUID: &editor})
	s.indexed(ctx, conv.Uuid, edited.Uuid, uuid.Nil)
	return out, nil
}

// editable checks that a stored message is an agent's own public text
// reply inside the edit window.
func editable(m db.Message, editorID int64, now time.Time) error {
	if m.DeletedAt.Valid || m.Direction != model.DirectionOut || m.SenderType != model.SenderAgent || m.Private {
		return fmt.Errorf("%w: only agent replies can be edited", ErrNotEditable)
	}
	if m.Kind != model.KindText || m.Content == "" {
		return fmt.Errorf("%w: only text replies can be edited", ErrNotEditable)
	}
	if !m.SenderUserID.Valid || m.SenderUserID.Int64 != editorID {
		return ErrNotAuthor
	}
	if m.Status == model.MessagePending {
		return fmt.Errorf("%w: the message is being sent, try again in a moment", ErrNotEditable)
	}
	if m.CreatedAt.Valid && now.Sub(m.CreatedAt.Time) > MessageEditWindow {
		return ErrEditWindowClosed
	}
	return nil
}

// MessageEdits returns the earlier versions of a message, oldest first:
// the first entry's previous_content is the original text.
func (s *Service) MessageEdits(ctx context.Context, orgID int64, convID, messageID uuid.UUID) ([]MessageEditEntry, error) {
	q := s.store.Q()
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: convID})
	if err != nil {
		return nil, notFound(err)
	}
	if err := s.mustSee(ctx, orgID, conv.ID); err != nil {
		return nil, err
	}
	m, err := q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: orgID, Uuid: messageID})
	if err != nil {
		return nil, notFound(err)
	}
	if m.ConversationID != conv.ID {
		return nil, ErrNotFound
	}
	rows, err := q.ListMessageEdits(ctx, db.ListMessageEditsParams{OrganizationID: orgID, MessageID: m.ID})
	if err != nil {
		return nil, err
	}
	out := make([]MessageEditEntry, 0, len(rows))
	for _, r := range rows {
		e := MessageEditEntry{UUID: r.Uuid, PreviousContent: r.PreviousContent, EditedAt: r.CreatedAt.Time.UTC()}
		if r.EditorUuid.Valid {
			e.Editor = &model.Ref{UUID: uuid.UUID(r.EditorUuid.Bytes), Name: fullName(r.EditorName, r.EditorSurname)}
		}
		out = append(out, e)
	}
	return out, nil
}

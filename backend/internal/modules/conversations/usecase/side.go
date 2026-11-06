package usecase

// CF-98: side conversations. An agent starts an e-mail thread with someone
// who is not the customer (another team, a supplier) from inside a
// conversation. Mail goes out through one of the organization's e-mail
// inboxes with a synthetic thread root (<sc.{uuid}@side.clickfix>) that our
// Message-ID carries, so the answer comes back to the same inbox with that
// root as its thread id; Receive hands it here instead of opening a customer
// conversation. Every start and every answer is also written into the parent
// conversation's timeline as an internal event.
//
// Sending is synchronous (one mail per request) and the outcome is stored on
// the side message (sent / failed with the provider error); a failed message
// can be retried by sending again.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Side conversation limits and markers.
const (
	maxSideRecipients = 10
	maxSideBody       = 65536
	sideRootPrefix    = "<sc."
	sideRootDomain    = "side.clickfix>"
	// attrNewThread: the e-mail adapter sends the subject without "Re:"
	// (email.AttrNewThread).
	attrNewThread = "new_thread"
)

// ErrNoEmailInbox: the organization has no e-mail inbox to send from.
var ErrNoEmailInbox = errors.New("conversations: no e-mail inbox to send side conversations from")

// SideMessage is one mail of a side conversation.
type SideMessage struct {
	UUID        uuid.UUID  `json:"uuid"`
	Direction   string     `json:"direction"`
	Sender      *model.Ref `json:"sender"`
	FromAddress string     `json:"from_address"`
	FromName    string     `json:"from_name"`
	Recipients  []string   `json:"recipients"`
	Content     string     `json:"content"`
	Status      string     `json:"status"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SideConversation is a side thread of a conversation.
type SideConversation struct {
	UUID             uuid.UUID     `json:"uuid"`
	ConversationUUID uuid.UUID     `json:"conversation_uuid"`
	Subject          string        `json:"subject"`
	Participants     []string      `json:"participants"`
	Status           string        `json:"status"`
	Inbox            *model.Ref    `json:"inbox"`
	CreatedBy        *model.Ref    `json:"created_by"`
	MessageCount     int32         `json:"message_count"`
	LastMessageAt    time.Time     `json:"last_message_at"`
	CreatedAt        time.Time     `json:"created_at"`
	Messages         []SideMessage `json:"messages,omitempty"`
}

// SideInbox is an e-mail inbox side conversations can be sent from.
type SideInbox struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// SideList is the side conversations of a conversation plus the inboxes a
// new one can use.
type SideList struct {
	Items   []SideConversation `json:"items"`
	Inboxes []SideInbox        `json:"inboxes"`
}

// StartSideInput starts a side conversation.
type StartSideInput struct {
	OrganizationID   int64
	Actor            Actor
	ConversationUUID uuid.UUID
	InboxUUID        *uuid.UUID
	To               []string
	Subject          string
	Body             string
}

// ListSide lists the side conversations of a conversation.
func (s *Service) ListSide(ctx context.Context, orgID int64, convID uuid.UUID) (SideList, error) {
	q := s.store.Q()
	conv, err := s.loadVisible(ctx, q, orgID, convID)
	if err != nil {
		return SideList{}, err
	}
	rows, err := q.ListSideConversations(ctx, db.ListSideConversationsParams{OrganizationID: orgID, ConversationID: conv.ID})
	if err != nil {
		return SideList{}, err
	}
	out := SideList{Items: make([]SideConversation, 0, len(rows)), Inboxes: []SideInbox{}}
	inboxes, err := q.ListEmailInstances(ctx, orgID)
	if err != nil {
		return SideList{}, err
	}
	names := map[int64]SideInbox{}
	for _, i := range inboxes {
		names[i.ID] = SideInbox{UUID: i.Uuid, Name: i.Name}
		out.Inboxes = append(out.Inboxes, SideInbox{UUID: i.Uuid, Name: i.Name})
	}
	for _, r := range rows {
		sc := sideOf(db.SideConversation{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, ConversationID: r.ConversationID,
			ChannelInstanceID: r.ChannelInstanceID, Subject: r.Subject, Participants: r.Participants, Status: r.Status,
			MessageCount: r.MessageCount, LastMessageAt: r.LastMessageAt, CreatedAt: r.CreatedAt,
		}, conv.Uuid)
		if in, ok := names[r.ChannelInstanceID.Int64]; ok && r.ChannelInstanceID.Valid {
			sc.Inbox = &model.Ref{UUID: in.UUID, Name: in.Name}
		}
		if r.CreatedByUuid.Valid {
			sc.CreatedBy = &model.Ref{UUID: uuid.UUID(r.CreatedByUuid.Bytes), Name: fullName(r.CreatedByName, r.CreatedBySurname)}
		}
		out.Items = append(out.Items, sc)
	}
	return out, nil
}

// GetSide returns a side conversation with its messages.
func (s *Service) GetSide(ctx context.Context, orgID int64, id uuid.UUID) (SideConversation, error) {
	q := s.store.Q()
	sc, conv, err := s.sideVisible(ctx, q, orgID, id)
	if err != nil {
		return SideConversation{}, err
	}
	out := sideOf(sc, conv.Uuid)
	if sc.ChannelInstanceID.Valid {
		if inboxes, err := q.ListEmailInstances(ctx, orgID); err == nil {
			for _, i := range inboxes {
				if i.ID == sc.ChannelInstanceID.Int64 {
					out.Inbox = &model.Ref{UUID: i.Uuid, Name: i.Name}
				}
			}
		}
	}
	rows, err := q.ListSideConversationMessages(ctx, db.ListSideConversationMessagesParams{OrganizationID: orgID, SideConversationID: sc.ID})
	if err != nil {
		return SideConversation{}, err
	}
	out.Messages = make([]SideMessage, 0, len(rows))
	for _, r := range rows {
		m := SideMessage{
			UUID: r.Uuid, Direction: r.Direction, FromAddress: r.FromAddress, FromName: r.FromName,
			Recipients: nonNilStrings(r.Recipients), Content: r.Content, Status: r.Status, Error: r.Error, CreatedAt: r.CreatedAt.Time,
		}
		if r.SenderUserUuid.Valid {
			m.Sender = &model.Ref{UUID: uuid.UUID(r.SenderUserUuid.Bytes), Name: fullName(r.SenderUserName, r.SenderUserSurname)}
		}
		out.Messages = append(out.Messages, m)
	}
	return out, nil
}

// sideVisible loads a side conversation whose parent the viewer may see.
func (s *Service) sideVisible(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.SideConversation, db.Conversation, error) {
	sc, err := q.GetSideConversationByUUID(ctx, db.GetSideConversationByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.SideConversation{}, db.Conversation{}, notFound(err)
	}
	conv, err := q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: sc.ConversationID})
	if err != nil {
		return db.SideConversation{}, db.Conversation{}, notFound(err)
	}
	if err := s.mustSee(ctx, orgID, conv.ID); err != nil {
		return db.SideConversation{}, db.Conversation{}, err
	}
	return sc, conv, nil
}

// StartSide opens a side conversation and sends its first mail.
func (s *Service) StartSide(ctx context.Context, in StartSideInput) (SideConversation, error) {
	to, err := cleanAddresses(in.To)
	if err != nil {
		return SideConversation{}, err
	}
	subject := strings.TrimSpace(in.Subject)
	body := strings.TrimSpace(in.Body)
	if subject == "" || len(subject) > 500 {
		return SideConversation{}, fmt.Errorf("%w: subject is required (at most 500 characters)", ErrInvalid)
	}
	if body == "" || len(body) > maxSideBody {
		return SideConversation{}, fmt.Errorf("%w: message is required", ErrInvalid)
	}
	actor := s.Named(ctx, in.Actor)
	q := s.store.Q()
	conv, err := s.loadVisible(ctx, q, in.OrganizationID, in.ConversationUUID)
	if err != nil {
		return SideConversation{}, err
	}
	instID, err := s.sideInbox(ctx, q, in.OrganizationID, in.InboxUUID)
	if err != nil {
		return SideConversation{}, err
	}
	sideID := uuid.New()
	root := sideRootPrefix + sideID.String() + "@" + sideRootDomain
	var sc db.SideConversation
	var msg db.SideConversationMessage
	var timeline []db.Message
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		sc, err = tx.Q.InsertSideConversation(ctx, db.InsertSideConversationParams{
			OrganizationID: in.OrganizationID, ConversationID: conv.ID, ChannelInstanceID: pgInt8(instID),
			Subject: subject, Participants: to, ThreadRoot: root, CreatedBy: pgInt8(actor.ID),
		})
		if err != nil {
			return err
		}
		if msg, err = s.appendSideOut(ctx, tx, sc, actor, to, body); err != nil {
			return err
		}
		m, err := s.sideEvent(ctx, tx, in.OrganizationID, conv.ID, actor, "side_conversation_started", sc, strings.Join(to, ", "), body)
		if err != nil {
			return err
		}
		timeline = append(timeline, m)
		return nil
	})
	if err != nil {
		return SideConversation{}, notFound(err)
	}
	s.publishTimeline(ctx, in.OrganizationID, conv, timeline)
	s.deliverSide(ctx, sc, msg, to, subject, body, "", true)
	return s.GetSide(ctx, in.OrganizationID, sc.Uuid)
}

// ReplySide sends another mail on a side conversation (to every participant).
func (s *Service) ReplySide(ctx context.Context, orgID int64, actor Actor, id uuid.UUID, body string) (SideConversation, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > maxSideBody {
		return SideConversation{}, fmt.Errorf("%w: message is required", ErrInvalid)
	}
	actor = s.Named(ctx, actor)
	sc, conv, err := s.sideVisible(ctx, s.store.Q(), orgID, id)
	if err != nil {
		return SideConversation{}, err
	}
	if !sc.ChannelInstanceID.Valid {
		return SideConversation{}, ErrNoEmailInbox
	}
	to := nonNilStrings(sc.Participants)
	if len(to) == 0 {
		return SideConversation{}, fmt.Errorf("%w: the side conversation has no participants", ErrInvalid)
	}
	var msg db.SideConversationMessage
	var replyTo string
	var timeline []db.Message
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		cur, err := tx.Q.LockSideConversation(ctx, db.LockSideConversationParams{OrganizationID: orgID, ID: sc.ID})
		if err != nil {
			return err
		}
		if replyTo, err = tx.Q.LastSideMessageExternalID(ctx, db.LastSideMessageExternalIDParams{OrganizationID: orgID, SideConversationID: sc.ID}); err != nil {
			return err
		}
		if msg, err = s.appendSideOut(ctx, tx, cur, actor, to, body); err != nil {
			return err
		}
		m, err := s.sideEvent(ctx, tx, orgID, conv.ID, actor, "side_conversation_sent", cur, strings.Join(to, ", "), body)
		if err != nil {
			return err
		}
		timeline = append(timeline, m)
		return nil
	})
	if err != nil {
		return SideConversation{}, notFound(err)
	}
	s.publishTimeline(ctx, orgID, conv, timeline)
	s.deliverSide(ctx, sc, msg, to, sc.Subject, body, replyTo, false)
	return s.GetSide(ctx, orgID, sc.Uuid)
}

// SetSideStatus opens or closes a side conversation.
func (s *Service) SetSideStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (SideConversation, error) {
	if status != "open" && status != "closed" {
		return SideConversation{}, fmt.Errorf("%w: status must be open or closed", ErrInvalid)
	}
	sc, _, err := s.sideVisible(ctx, s.store.Q(), orgID, id)
	if err != nil {
		return SideConversation{}, err
	}
	if _, err := s.store.Q().UpdateSideConversationStatus(ctx, db.UpdateSideConversationStatusParams{OrganizationID: orgID, ID: sc.ID, Status: status}); err != nil {
		return SideConversation{}, err
	}
	return s.GetSide(ctx, orgID, id)
}

func (s *Service) appendSideOut(ctx context.Context, tx *repository.Tx, sc db.SideConversation, actor Actor, to []string, body string) (db.SideConversationMessage, error) {
	at := s.now()
	msg, err := tx.Q.InsertSideConversationMessage(ctx, db.InsertSideConversationMessageParams{
		OrganizationID: sc.OrganizationID, SideConversationID: sc.ID, Direction: model.DirectionOut,
		SenderUserID: pgInt8(actor.ID), FromName: actor.Name, Recipients: to, Content: body,
		Status: model.MessagePending, CreatedAt: pgTime(at),
	})
	if err != nil {
		return db.SideConversationMessage{}, err
	}
	_, err = tx.Q.TouchSideConversation(ctx, db.TouchSideConversationParams{
		OrganizationID: sc.OrganizationID, ID: sc.ID, At: pgTime(at), Participants: []string{}, Reopen: true,
	})
	return msg, err
}

// deliverSide sends one side mail and stores the outcome on the message.
func (s *Service) deliverSide(ctx context.Context, sc db.SideConversation, msg db.SideConversationMessage, to []string, subject, body, replyTo string, fresh bool) {
	fail := func(cause error) {
		s.log.Warn("side_conversation_send_failed", "side_conversation", sc.Uuid, "error", cause)
		if _, err := s.store.Q().MarkSideMessageFailed(ctx, db.MarkSideMessageFailedParams{
			OrganizationID: sc.OrganizationID, ID: msg.ID, Error: truncate(cause.Error(), 500),
		}); err != nil {
			s.log.Warn("side_conversation_mark_failed", "error", err)
		}
	}
	if s.instances == nil || !sc.ChannelInstanceID.Valid {
		fail(ErrNoEmailInbox)
		return
	}
	inst, adapter, err := s.instances.Instance(ctx, sc.OrganizationID, sc.ChannelInstanceID.Int64)
	if err != nil {
		fail(err)
		return
	}
	if inst.Type != channels.TypeEmail {
		fail(ErrNoEmailInbox)
		return
	}
	attrs := map[string]any{}
	if len(to) > 1 {
		attrs["cc"] = to[1:]
	}
	if fresh {
		attrs[attrNewThread] = true
	}
	out := channels.OutboundMessage{
		MessageUUID: msg.Uuid, To: channels.Recipient{Email: to[0], ExternalID: to[0]}, Kind: model.KindEmail,
		Text: body, ThreadID: sc.ThreadRoot, Subject: subject, ReplyToExternalID: replyTo, Attributes: attrs,
	}
	parent, err := s.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: sc.OrganizationID, ID: sc.ConversationID})
	if err != nil {
		fail(err)
		return
	}
	if err := s.applyEmailBrand(ctx, parent, &inst, &out); err != nil {
		fail(err)
		return
	}
	res, err := adapter.Send(ctx, inst, out)
	if err != nil {
		fail(err)
		return
	}
	ext := pgtype.Text{}
	if res.ExternalID != "" {
		ext = pgtype.Text{String: truncate(res.ExternalID, maxExternalID), Valid: true}
	}
	if _, err := s.store.Q().MarkSideMessageSent(ctx, db.MarkSideMessageSentParams{OrganizationID: sc.OrganizationID, ID: msg.ID, ExternalID: ext}); err != nil {
		s.log.Warn("side_conversation_mark_sent", "error", err)
	}
}

// sideInbox picks the e-mail inbox: the requested one or the first one.
func (s *Service) sideInbox(ctx context.Context, q *db.Queries, orgID int64, want *uuid.UUID) (int64, error) {
	inboxes, err := q.ListEmailInstances(ctx, orgID)
	if err != nil {
		return 0, err
	}
	for _, i := range inboxes {
		if want == nil || i.Uuid == *want {
			return i.ID, nil
		}
	}
	if want != nil {
		return 0, fmt.Errorf("%w: unknown e-mail inbox", ErrInvalid)
	}
	return 0, ErrNoEmailInbox
}

// sideEvent writes the parent conversation's internal timeline entry.
func (s *Service) sideEvent(ctx context.Context, tx *repository.Tx, orgID, convID int64, actor Actor, event string, sc db.SideConversation, who, body string) (db.Message, error) {
	attrs, err := json.Marshal(map[string]any{
		"event": event, "to": who, "side_conversation_uuid": sc.Uuid.String(), "subject": sc.Subject,
		"preview": model.PreviewOutbound(body), "actor_uuid": actor.UUID, "actor_name": actor.Name,
	})
	if err != nil {
		return db.Message{}, err
	}
	app, err := tx.AppendMessage(ctx, repository.NewMessage{
		OrganizationID: orgID, ConversationID: convID, Direction: model.DirectionInternal, SenderType: model.SenderSystem,
		SenderUserID: actor.ID, Kind: model.KindEvent, Attributes: attrs, At: s.now(),
	})
	return app.Message, err
}

// isSideThread reports whether an inbound thread id is a side conversation root.
func isSideThread(threadID string) bool {
	return strings.HasPrefix(threadID, sideRootPrefix) && strings.HasSuffix(threadID, "@"+sideRootDomain)
}

// receiveSide stores an inbound answer to a side conversation. handled is
// false when no side conversation has the thread (the normal pipeline runs).
func (s *Service) receiveSide(ctx context.Context, inst channels.Instance, m channels.InboundMessage) (handled, stored bool, err error) {
	q := s.store.Q()
	sc, err := q.FindSideConversationByThread(ctx, db.FindSideConversationByThreadParams{OrganizationID: inst.OrganizationID, ThreadRoot: m.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return true, false, err
	}
	from := strings.ToLower(strings.TrimSpace(firstNonEmpty(m.From.Email, m.From.ExternalID)))
	at := m.At
	if at.IsZero() || at.After(s.now()) {
		at = s.now()
	}
	var conv db.Conversation
	var timeline []db.Message
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		cur, err := tx.Q.LockSideConversation(ctx, db.LockSideConversationParams{OrganizationID: inst.OrganizationID, ID: sc.ID})
		if err != nil {
			return err
		}
		_, err = tx.Q.InsertSideConversationMessage(ctx, db.InsertSideConversationMessageParams{
			OrganizationID: inst.OrganizationID, SideConversationID: cur.ID, Direction: model.DirectionIn,
			FromAddress: from, FromName: m.From.Name, Recipients: []string{}, Content: truncate(m.Text, maxSideBody),
			Status: model.MessageReceived, ExternalID: pgtype.Text{String: m.ExternalID, Valid: m.ExternalID != ""}, CreatedAt: pgTime(at),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already stored (provider retry)
		}
		if err != nil {
			return err
		}
		stored = true
		var add []string
		if _, err := mail.ParseAddress(from); err == nil {
			add = []string{from}
		}
		if _, err := tx.Q.TouchSideConversation(ctx, db.TouchSideConversationParams{
			OrganizationID: inst.OrganizationID, ID: cur.ID, At: pgTime(at), Participants: add, Reopen: true,
		}); err != nil {
			return err
		}
		if conv, err = tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: inst.OrganizationID, ID: cur.ConversationID}); err != nil {
			return err
		}
		ev, err := s.sideEvent(ctx, tx, inst.OrganizationID, conv.ID, Actor{Name: firstNonEmpty(m.From.Name, from)}, "side_conversation_reply", cur, from, m.Text)
		if err != nil {
			return err
		}
		timeline = append(timeline, ev)
		return nil
	})
	if err != nil {
		return true, false, err
	}
	if stored {
		s.publishTimeline(ctx, inst.OrganizationID, conv, timeline)
	}
	return true, stored, nil
}

func cleanAddresses(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			addr, err := mail.ParseAddress(part)
			if err != nil {
				return nil, fmt.Errorf("%w: %q is not an e-mail address", ErrInvalid, part)
			}
			a := strings.ToLower(addr.Address)
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 || len(out) > maxSideRecipients {
		return nil, fmt.Errorf("%w: between 1 and %d recipients", ErrInvalid, maxSideRecipients)
	}
	return out, nil
}

func sideOf(r db.SideConversation, conv uuid.UUID) SideConversation {
	return SideConversation{
		UUID: r.Uuid, ConversationUUID: conv, Subject: r.Subject, Participants: nonNilStrings(r.Participants),
		Status: r.Status, MessageCount: r.MessageCount, LastMessageAt: r.LastMessageAt.Time, CreatedAt: r.CreatedAt.Time,
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

package usecase

// CF-98: merge, split and linked conversations.
//
// Merge folds a source conversation into a target of the same organization:
// its messages (with their attachments, which hang off the message), tags,
// participants and side conversations move, the source is closed with
// merged_into_id pointing at the target and both timelines get a system
// event. Inbound mail still threaded on the source follows merged_into_id.
//
// Split moves one message (optionally with every later one) into a new
// conversation of the same contact and inbox and links it as a child.
//
// Links are parent/child (one parent per conversation, no cycles) or
// related (symmetric).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Link relations as seen from one conversation.
const (
	RelationParent  = "parent"
	RelationChild   = "child"
	RelationRelated = "related"
	RelationMerged  = "merged" // the other conversation was merged into this one
)

// Link kinds stored in conversation_links.
const (
	linkParent  = "parent"
	linkRelated = "related"
)

// maxParentDepth bounds the walk up a parent chain (cycle check).
const maxParentDepth = 64

// ErrLinkConflict: the link would give a conversation a second parent or a
// parent cycle.
var ErrLinkConflict = errors.New("conversations: link conflicts with an existing parent")

// LinkedConversation is one conversation related to another.
type LinkedConversation struct {
	Relation      string    `json:"relation"`
	UUID          uuid.UUID `json:"uuid"`
	Number        int64     `json:"number"`
	Subject       string    `json:"subject"`
	Status        string    `json:"status"`
	ChannelType   string    `json:"channel_type"`
	LastMessageAt time.Time `json:"last_message_at"`
	LinkedAt      time.Time `json:"linked_at"`
}

// Links is everything linked to a conversation.
type Links struct {
	// MergedInto is set when this conversation was merged into another one.
	MergedInto *LinkedConversation  `json:"merged_into"`
	Items      []LinkedConversation `json:"items"`
}

// MergeResult is what a merge moved.
type MergeResult struct {
	Target   db.Conversation
	Source   db.Conversation
	Messages int64
}

// load returns a conversation of the org the request may see.
func (s *Service) loadVisible(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.Conversation, error) {
	c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.Conversation{}, notFound(err)
	}
	if err := s.mustSee(ctx, orgID, c.ID); err != nil {
		return db.Conversation{}, err
	}
	return c, nil
}

// linkEvent writes a system event naming another conversation.
func (s *Service) linkEvent(ctx context.Context, tx *repository.Tx, orgID, convID int64, actor Actor, event string, other db.Conversation, extra map[string]any) (db.Message, error) {
	attrs := map[string]any{
		"event": event, "to": "#" + strconv.FormatInt(other.Number, 10),
		"conversation_uuid": other.Uuid.String(), "conversation_number": other.Number,
		"actor_uuid": actor.UUID, "actor_name": actor.Name,
	}
	for k, v := range extra {
		attrs[k] = v
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return db.Message{}, err
	}
	app, err := tx.AppendMessage(ctx, repository.NewMessage{
		OrganizationID: orgID, ConversationID: convID, Direction: model.DirectionInternal, SenderType: model.SenderSystem,
		SenderUserID: actor.ID, Kind: model.KindEvent, Attributes: raw, At: s.now(),
	})
	return app.Message, err
}

// Merge folds source into target.
func (s *Service) Merge(ctx context.Context, orgID int64, actor Actor, targetID, sourceID uuid.UUID) (MergeResult, error) {
	if targetID == sourceID {
		return MergeResult{}, fmt.Errorf("%w: a conversation cannot be merged into itself", ErrInvalid)
	}
	actor = s.Named(ctx, actor)
	var out MergeResult
	var tTimeline, sTimeline []db.Message
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		target, err := s.loadVisible(ctx, tx.Q, orgID, targetID)
		if err != nil {
			return err
		}
		source, err := s.loadVisible(ctx, tx.Q, orgID, sourceID)
		if err != nil {
			return err
		}
		if source.MergedIntoID.Valid {
			return fmt.Errorf("%w: the conversation is already merged", ErrInvalid)
		}
		if target.MergedIntoID.Valid {
			return fmt.Errorf("%w: the target conversation was merged into another one", ErrInvalid)
		}
		// Lock both in id order so two crossing merges cannot deadlock.
		first, second := target.ID, source.ID
		if second < first {
			first, second = second, first
		}
		for _, id := range []int64{first, second} {
			if _, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: orgID, ID: id}); err != nil {
				return err
			}
		}
		moved, err := tx.Q.MoveConversationMessages(ctx, db.MoveConversationMessagesParams{OrganizationID: orgID, IntoID: target.ID, FromID: source.ID})
		if err != nil {
			return err
		}
		out.Messages = moved
		for _, step := range []func() error{
			func() error {
				return tx.Q.CopyConversationTags(ctx, db.CopyConversationTagsParams{OrganizationID: orgID, IntoID: target.ID, FromID: source.ID})
			},
			func() error {
				return tx.Q.CopyConversationParticipants(ctx, db.CopyConversationParticipantsParams{OrganizationID: orgID, IntoID: target.ID, FromID: source.ID})
			},
			func() error {
				return tx.Q.MoveSideConversations(ctx, db.MoveSideConversationsParams{OrganizationID: orgID, IntoID: target.ID, FromID: source.ID})
			},
			func() error {
				return tx.Q.MoveConversationLinks(ctx, db.MoveConversationLinksParams{OrganizationID: orgID, ID: source.ID})
			},
			func() error {
				return tx.Q.RepointMergedConversations(ctx, db.RepointMergedConversationsParams{OrganizationID: orgID, IntoID: pgInt8(target.ID), FromID: pgInt8(source.ID)})
			},
		} {
			if err := step(); err != nil {
				return err
			}
		}
		if _, err := tx.Q.MarkConversationMerged(ctx, db.MarkConversationMergedParams{OrganizationID: orgID, IntoID: pgInt8(target.ID), ID: source.ID}); err != nil {
			return err
		}
		if _, err := tx.Q.RecountConversation(ctx, db.RecountConversationParams{OrganizationID: orgID, ID: source.ID}); err != nil {
			return err
		}
		if _, err := tx.Q.RecountConversation(ctx, db.RecountConversationParams{OrganizationID: orgID, ID: target.ID}); err != nil {
			return err
		}
		m, err := s.linkEvent(ctx, tx, orgID, target.ID, actor, "merged_from", source, map[string]any{"messages": moved})
		if err != nil {
			return err
		}
		tTimeline = append(tTimeline, m)
		if m, err = s.linkEvent(ctx, tx, orgID, source.ID, actor, "merged_into", target, nil); err != nil {
			return err
		}
		sTimeline = append(sTimeline, m)
		if out.Target, err = tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: target.ID}); err != nil {
			return err
		}
		if out.Source, err = tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: source.ID}); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.ConversationsUpdated, orgID, actor.ID, out.Target, map[string]any{
			"change": "merged", "from": source.Uuid.String(), "to": target.Uuid.String(), "messages": moved,
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ConversationsResolved, orgID, actor.ID, out.Source, map[string]any{
			"change": "merged_into", "from": source.Status, "to": model.StatusClosed, "merged_into": target.Uuid.String(),
		})
	})
	if err != nil {
		return MergeResult{}, notFound(err)
	}
	s.publishTimeline(ctx, orgID, out.Target, tTimeline)
	s.publishTimeline(ctx, orgID, out.Source, sTimeline)
	s.indexed(ctx, out.Target.Uuid, uuid.Nil, uuid.Nil)
	s.indexed(ctx, out.Source.Uuid, uuid.Nil, uuid.Nil)
	return out, nil
}

// SplitInput moves messages of a conversation into a new one.
type SplitInput struct {
	OrganizationID   int64
	Actor            Actor
	ConversationUUID uuid.UUID
	MessageUUID      uuid.UUID
	// Following also moves every later message.
	Following bool
	Subject   string
}

// Split creates a child conversation from a message.
func (s *Service) Split(ctx context.Context, in SplitInput) (db.Conversation, error) {
	actor := s.Named(ctx, in.Actor)
	var child, parent db.Conversation
	var pTimeline, cTimeline []db.Message
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		src, err := s.loadVisible(ctx, tx.Q, in.OrganizationID, in.ConversationUUID)
		if err != nil {
			return err
		}
		if src.MergedIntoID.Valid {
			return fmt.Errorf("%w: the conversation was merged", ErrInvalid)
		}
		if _, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: in.OrganizationID, ID: src.ID}); err != nil {
			return err
		}
		msg, err := tx.Q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.MessageUUID})
		if err != nil || msg.ConversationID != src.ID {
			return ErrNotFound
		}
		if msg.Kind == model.KindEvent {
			return fmt.Errorf("%w: a system event cannot start a conversation", ErrInvalid)
		}
		subject := strings.TrimSpace(in.Subject)
		if subject == "" {
			subject = src.Subject
		}
		child, err = tx.CreateConversation(ctx, repository.NewConversation{
			OrganizationID: in.OrganizationID, ChannelType: src.ChannelType, InstanceID: src.ChannelInstanceID.Int64,
			ContactID: src.ContactID, CompanyID: src.CompanyID.Int64, Subject: subject,
			Priority: src.Priority, Source: src.Source, AssigneeUserID: src.AssigneeUserID.Int64,
			AssigneeTeamID: src.AssigneeTeamID.Int64, At: msg.CreatedAt.Time,
		})
		if err != nil {
			return err
		}
		moved, err := tx.Q.MoveMessagesFrom(ctx, db.MoveMessagesFromParams{
			OrganizationID: in.OrganizationID, IntoID: child.ID, FromID: src.ID, FirstID: msg.ID, Following: in.Following,
		})
		if err != nil {
			return err
		}
		if moved == 0 {
			return ErrNotFound
		}
		if _, err := tx.Q.InsertConversationLink(ctx, db.InsertConversationLinkParams{
			OrganizationID: in.OrganizationID, FromID: src.ID, ToID: child.ID, Kind: linkParent, CreatedBy: pgInt8(actor.ID),
		}); err != nil {
			return err
		}
		if _, err := tx.Q.RecountConversation(ctx, db.RecountConversationParams{OrganizationID: in.OrganizationID, ID: src.ID}); err != nil {
			return err
		}
		if _, err := tx.Q.RecountConversation(ctx, db.RecountConversationParams{OrganizationID: in.OrganizationID, ID: child.ID}); err != nil {
			return err
		}
		m, err := s.linkEvent(ctx, tx, in.OrganizationID, src.ID, actor, "split_to", child, map[string]any{"messages": moved})
		if err != nil {
			return err
		}
		pTimeline = append(pTimeline, m)
		if m, err = s.linkEvent(ctx, tx, in.OrganizationID, child.ID, actor, "split_from", src, map[string]any{"messages": moved}); err != nil {
			return err
		}
		cTimeline = append(cTimeline, m)
		if parent, err = tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: in.OrganizationID, ID: src.ID}); err != nil {
			return err
		}
		if child, err = tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: in.OrganizationID, ID: child.ID}); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.ConversationsCreated, in.OrganizationID, actor.ID, child, map[string]any{
			"channel_type": child.ChannelType, "number": child.Number, "source": child.Source,
			"split_from": src.Uuid.String(), "messages": moved,
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ConversationsUpdated, in.OrganizationID, actor.ID, parent, map[string]any{
			"change": "split", "to": child.Uuid.String(), "messages": moved,
		})
	})
	if err != nil {
		return db.Conversation{}, notFound(err)
	}
	s.notify(ctx, in.OrganizationID, model.LiveEvent{Type: model.EventConversationCreated, ConversationUUID: child.Uuid})
	s.publishTimeline(ctx, in.OrganizationID, parent, pTimeline)
	s.publishTimeline(ctx, in.OrganizationID, child, cTimeline)
	s.indexed(ctx, parent.Uuid, uuid.Nil, uuid.Nil)
	s.indexed(ctx, child.Uuid, uuid.Nil, uuid.Nil)
	return child, nil
}

// LinkInput links two conversations. Relation is from the conversation's
// point of view: "parent" makes Other its parent, "child" its child.
type LinkInput struct {
	OrganizationID   int64
	Actor            Actor
	ConversationUUID uuid.UUID
	OtherUUID        uuid.UUID
	Relation         string
}

// Link adds (or changes) the link between two conversations.
func (s *Service) Link(ctx context.Context, in LinkInput) error {
	if in.ConversationUUID == in.OtherUUID {
		return fmt.Errorf("%w: a conversation cannot be linked to itself", ErrInvalid)
	}
	if !model.Valid(in.Relation, []string{RelationParent, RelationChild, RelationRelated}) {
		return fmt.Errorf("%w: relation must be parent, child or related", ErrInvalid)
	}
	actor := s.Named(ctx, in.Actor)
	var conv, other db.Conversation
	var cTimeline, oTimeline []db.Message
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		if conv, err = s.loadVisible(ctx, tx.Q, in.OrganizationID, in.ConversationUUID); err != nil {
			return err
		}
		if other, err = s.loadVisible(ctx, tx.Q, in.OrganizationID, in.OtherUUID); err != nil {
			return err
		}
		// Changing the kind of an existing pair replaces it.
		if _, err := tx.Q.DeleteConversationLink(ctx, db.DeleteConversationLinkParams{OrganizationID: in.OrganizationID, AID: conv.ID, BID: other.ID}); err != nil {
			return err
		}
		p := db.InsertConversationLinkParams{OrganizationID: in.OrganizationID, CreatedBy: pgInt8(actor.ID)}
		switch in.Relation {
		case RelationRelated:
			p.Kind, p.FromID, p.ToID = linkRelated, min64(conv.ID, other.ID), max64(conv.ID, other.ID)
		case RelationParent:
			p.Kind, p.FromID, p.ToID = linkParent, other.ID, conv.ID
		default:
			p.Kind, p.FromID, p.ToID = linkParent, conv.ID, other.ID
		}
		if p.Kind == linkParent {
			if err := s.checkParent(ctx, tx.Q, in.OrganizationID, p.FromID, p.ToID); err != nil {
				return err
			}
		}
		if _, err := tx.Q.InsertConversationLink(ctx, p); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				return ErrLinkConflict
			}
			return err
		}
		m, err := s.linkEvent(ctx, tx, in.OrganizationID, conv.ID, actor, "linked", other, map[string]any{"relation": in.Relation})
		if err != nil {
			return err
		}
		cTimeline = append(cTimeline, m)
		if m, err = s.linkEvent(ctx, tx, in.OrganizationID, other.ID, actor, "linked", conv, map[string]any{"relation": inverse(in.Relation)}); err != nil {
			return err
		}
		oTimeline = append(oTimeline, m)
		return s.emit(ctx, tx, events.ConversationsUpdated, in.OrganizationID, actor.ID, conv, map[string]any{
			"change": "linked", "relation": in.Relation, "to": other.Uuid.String(),
		})
	})
	if err != nil {
		return notFound(err)
	}
	s.publishTimeline(ctx, in.OrganizationID, conv, cTimeline)
	s.publishTimeline(ctx, in.OrganizationID, other, oTimeline)
	return nil
}

// checkParent refuses a second parent for child and a parent cycle.
func (s *Service) checkParent(ctx context.Context, q *db.Queries, orgID, parentID, childID int64) error {
	has, err := q.ConversationHasParent(ctx, db.ConversationHasParentParams{OrganizationID: orgID, ID: childID})
	if err != nil {
		return err
	}
	if has {
		return ErrLinkConflict
	}
	cur := parentID
	for i := 0; i < maxParentDepth; i++ {
		if cur == childID {
			return ErrLinkConflict
		}
		up, err := q.GetConversationParentID(ctx, db.GetConversationParentIDParams{OrganizationID: orgID, ID: cur})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		cur = up
	}
	return ErrLinkConflict
}

// Unlink removes the link between two conversations.
func (s *Service) Unlink(ctx context.Context, orgID int64, actor Actor, convID, otherID uuid.UUID) error {
	actor = s.Named(ctx, actor)
	var conv, other db.Conversation
	var cTimeline, oTimeline []db.Message
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		if conv, err = s.loadVisible(ctx, tx.Q, orgID, convID); err != nil {
			return err
		}
		if other, err = s.loadVisible(ctx, tx.Q, orgID, otherID); err != nil {
			return err
		}
		n, err := tx.Q.DeleteConversationLink(ctx, db.DeleteConversationLinkParams{OrganizationID: orgID, AID: conv.ID, BID: other.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		m, err := s.linkEvent(ctx, tx, orgID, conv.ID, actor, "unlinked", other, nil)
		if err != nil {
			return err
		}
		cTimeline = append(cTimeline, m)
		if m, err = s.linkEvent(ctx, tx, orgID, other.ID, actor, "unlinked", conv, nil); err != nil {
			return err
		}
		oTimeline = append(oTimeline, m)
		return s.emit(ctx, tx, events.ConversationsUpdated, orgID, actor.ID, conv, map[string]any{
			"change": "unlinked", "to": other.Uuid.String(),
		})
	})
	if err != nil {
		return notFound(err)
	}
	s.publishTimeline(ctx, orgID, conv, cTimeline)
	s.publishTimeline(ctx, orgID, other, oTimeline)
	return nil
}

// Links lists the conversations linked to one conversation, those merged
// into it and the one it was merged into. Linked conversations the viewer
// may not see (team visibility) are left out.
func (s *Service) Links(ctx context.Context, orgID int64, id uuid.UUID) (Links, error) {
	q := s.store.Q()
	conv, err := s.loadVisible(ctx, q, orgID, id)
	if err != nil {
		return Links{}, err
	}
	out := Links{Items: []LinkedConversation{}}
	visible := func(u uuid.UUID) bool {
		c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: u})
		return err == nil && s.mustSee(ctx, orgID, c.ID) == nil
	}
	if conv.MergedIntoID.Valid {
		b, err := q.GetConversationBrief(ctx, db.GetConversationBriefParams{OrganizationID: orgID, ID: conv.MergedIntoID.Int64})
		if err == nil && visible(b.Uuid) {
			out.MergedInto = &LinkedConversation{
				Relation: "merged_into", UUID: b.Uuid, Number: b.Number, Subject: b.Subject, Status: b.Status,
				ChannelType: b.ChannelType, LastMessageAt: b.LastMessageAt.Time, LinkedAt: conv.MergedAt.Time,
			}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Links{}, err
		}
	}
	rows, err := q.ListConversationLinks(ctx, db.ListConversationLinksParams{OrganizationID: orgID, ID: conv.ID})
	if err != nil {
		return Links{}, err
	}
	for _, r := range rows {
		if !visible(r.Uuid) {
			continue
		}
		out.Items = append(out.Items, LinkedConversation{
			Relation: r.Relation, UUID: r.Uuid, Number: r.Number, Subject: r.Subject, Status: r.Status,
			ChannelType: r.ChannelType, LastMessageAt: r.LastMessageAt.Time, LinkedAt: r.CreatedAt.Time,
		})
	}
	merged, err := q.ListMergedIntoConversation(ctx, db.ListMergedIntoConversationParams{OrganizationID: orgID, ID: pgInt8(conv.ID)})
	if err != nil {
		return Links{}, err
	}
	for _, r := range merged {
		if !visible(r.Uuid) {
			continue
		}
		out.Items = append(out.Items, LinkedConversation{
			Relation: RelationMerged, UUID: r.Uuid, Number: r.Number, Subject: r.Subject, Status: r.Status,
			ChannelType: r.ChannelType, LastMessageAt: r.LastMessageAt.Time, LinkedAt: r.MergedAt.Time,
		})
	}
	return out, nil
}

// followMerged returns the conversation a merged one was folded into (the
// pointer is kept flat by RepointMergedConversations, one hop suffices).
func followMerged(ctx context.Context, tx *repository.Tx, c db.Conversation) (db.Conversation, error) {
	if !c.MergedIntoID.Valid {
		return c, nil
	}
	return tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: c.OrganizationID, ID: c.MergedIntoID.Int64})
}

func inverse(rel string) string {
	switch rel {
	case RelationParent:
		return RelationChild
	case RelationChild:
		return RelationParent
	}
	return rel
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

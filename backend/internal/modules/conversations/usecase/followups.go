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
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
)

// Reminder conditions (CF-51).
const (
	ReminderAlways  = "always"
	ReminderNoReply = "no_reply"
)

// ReminderNotifier is implemented by notifiers that can tell an agent a
// reminder fired.
type ReminderNotifier interface {
	ReminderDue(ctx context.Context, orgID, userID int64, conv db.Conversation, note string)
}

// Reminder is a follow-up an agent set on a conversation.
type Reminder struct {
	UUID      uuid.UUID `json:"uuid"`
	DueAt     time.Time `json:"due_at"`
	Condition string    `json:"condition"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// AddReminder schedules a follow-up for actor on a conversation.
func (s *Service) AddReminder(ctx context.Context, orgID int64, conv uuid.UUID, actor Actor, due time.Time, condition, note string) (Reminder, error) {
	if condition == "" {
		condition = ReminderAlways
	}
	if condition != ReminderAlways && condition != ReminderNoReply {
		return Reminder{}, fmt.Errorf("%w: condition must be always or no_reply", ErrInvalid)
	}
	if !due.After(s.now()) || due.After(s.now().Add(366*24*time.Hour)) {
		return Reminder{}, fmt.Errorf("%w: due_at must be in the next year", ErrInvalid)
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 500 {
		return Reminder{}, fmt.Errorf("%w: note is too long", ErrInvalid)
	}
	c, err := s.store.Q().GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: conv})
	if err != nil {
		return Reminder{}, notFound(err)
	}
	r, err := s.store.Q().InsertConversationReminder(ctx, db.InsertConversationReminderParams{
		OrganizationID: orgID, ConversationID: c.ID, UserID: actor.ID, DueAt: pgTime(due), Condition: condition, Note: note,
	})
	if err != nil {
		return Reminder{}, err
	}
	return reminderOf(r), nil
}

// Reminders lists the pending reminders of a conversation.
func (s *Service) Reminders(ctx context.Context, orgID int64, conv uuid.UUID) ([]Reminder, error) {
	c, err := s.store.Q().GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: conv})
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := s.store.Q().ListConversationReminders(ctx, db.ListConversationRemindersParams{OrganizationID: orgID, ConversationID: c.ID})
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(rows))
	for _, r := range rows {
		out = append(out, reminderOf(r))
	}
	return out, nil
}

// CancelReminder cancels a pending reminder.
func (s *Service) CancelReminder(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.store.Q().CancelConversationReminder(ctx, db.CancelConversationReminderParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TickResult is what one follow-up tick did.
type TickResult struct {
	Woken      int
	Reminded   int
	Superseded int
}

// Tick wakes snoozed conversations whose time came and fires due
// reminders (the worker runs it every minute).
func (s *Service) Tick(ctx context.Context) (TickResult, error) {
	var res TickResult
	var woken []db.Conversation
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		woken, err = tx.Q.WakeSnoozedConversations(ctx, pgTime(s.now()))
		if err != nil {
			return err
		}
		for _, c := range woken {
			if _, err := s.timelineEvent(ctx, tx, c.OrganizationID, c.ID, Actor{}, "status_changed", model.StatusSnoozed, model.StatusOpen); err != nil {
				return err
			}
			if err := s.emit(ctx, tx, events.ConversationsReopened, c.OrganizationID, 0, c, map[string]any{"by": "snooze"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("conversations: wake snoozed: %w", err)
	}
	res.Woken = len(woken)
	for _, c := range woken {
		s.notify(ctx, c.OrganizationID, model.LiveEvent{Type: model.EventConversationUpdated, ConversationUUID: c.Uuid})
	}

	due, err := s.store.Q().ClaimDueReminders(ctx, pgTime(s.now()))
	if err != nil {
		return res, fmt.Errorf("conversations: claim reminders: %w", err)
	}
	for _, r := range due {
		c, err := s.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: r.OrganizationID, ID: r.ConversationID})
		if err != nil {
			continue
		}
		if r.Condition == ReminderNoReply && c.LastInboundAt.Valid && c.LastInboundAt.Time.After(r.CreatedAt.Time) {
			res.Superseded++
			continue
		}
		if c.Status != model.StatusOpen {
			open := model.StatusOpen
			actor := s.Named(ctx, Actor{ID: r.UserID, UUID: s.userUUID(ctx, r.UserID)})
			if _, err := s.Update(ctx, UpdateInput{OrganizationID: r.OrganizationID, ConversationUUID: c.Uuid, Actor: actor, Status: &open}); err != nil {
				s.log.Warn("reminder_reopen_failed", "conversation", c.Uuid, "error", err)
			}
		}
		if rn, ok := s.mentions.(ReminderNotifier); ok {
			rn.ReminderDue(ctx, r.OrganizationID, r.UserID, c, r.Note)
		}
		res.Reminded++
	}
	return res, nil
}

func (s *Service) userUUID(ctx context.Context, id int64) uuid.UUID {
	rows, err := s.store.Q().ListUserUUIDsByIDs(ctx, []int64{id})
	if err != nil || len(rows) == 0 {
		return uuid.Nil
	}
	return rows[0].Uuid
}

func reminderOf(r db.ConversationReminder) Reminder {
	return Reminder{UUID: r.Uuid, DueAt: r.DueAt.Time.UTC(), Condition: r.Condition, Note: r.Note, CreatedAt: r.CreatedAt.Time.UTC()}
}

// ---------------------------------------------------------------------------
// Bulk actions

// Bulk actions on conversations.
const (
	BulkAssign   = "assign"
	BulkStatus   = "status"
	BulkPriority = "priority"
	BulkSnooze   = "snooze"
	BulkTag      = "tag"
	BulkUntag    = "untag"
)

// MaxBulk bounds one bulk request.
const MaxBulk = 200

// BulkPermissions are the caller's conversation permissions.
type BulkPermissions struct{ Manage, Assign bool }

// BulkInput is one bulk request.
type BulkInput struct {
	OrganizationID int64
	Actor          Actor
	Perms          BulkPermissions
	Conversations  []uuid.UUID
	Action         string
	// Value: agent uuid / "me" / "" (assign), status, priority, RFC3339
	// time (snooze), tag name (tag, untag).
	Value string
}

// BulkResult reports a bulk run; Undo is the operation to revert.
type BulkResult struct {
	Operation uuid.UUID `json:"operation"`
	Applied   int       `json:"applied"`
	Failed    int       `json:"failed"`
}

type bulkItem struct {
	Conversation uuid.UUID  `json:"conversation"`
	Status       string     `json:"status"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
	Priority     string     `json:"priority"`
	Assignee     *uuid.UUID `json:"assignee,omitempty"`
	Changed      bool       `json:"changed"`
}

// Bulk applies one action to many conversations; every change lands on
// each timeline. The previous state is stored so Undo can revert it.
func (s *Service) Bulk(ctx context.Context, in BulkInput) (BulkResult, error) {
	if len(in.Conversations) == 0 || len(in.Conversations) > MaxBulk {
		return BulkResult{}, fmt.Errorf("%w: select 1-%d conversations", ErrInvalid, MaxBulk)
	}
	switch in.Action {
	case BulkAssign:
		if !in.Perms.Assign && !in.Perms.Manage {
			return BulkResult{}, ErrForbidden
		}
	case BulkStatus, BulkPriority, BulkSnooze, BulkTag, BulkUntag:
		if !in.Perms.Manage {
			return BulkResult{}, ErrForbidden
		}
	default:
		return BulkResult{}, fmt.Errorf("%w: unknown action", ErrInvalid)
	}
	var snoozeUntil time.Time
	switch in.Action {
	case BulkStatus:
		if !model.Valid(in.Value, model.Statuses) || in.Value == model.StatusSnoozed {
			return BulkResult{}, fmt.Errorf("%w: status must be open, pending, resolved or closed", ErrInvalid)
		}
	case BulkPriority:
		if !model.Valid(in.Value, model.Priorities) {
			return BulkResult{}, fmt.Errorf("%w: unknown priority", ErrInvalid)
		}
	case BulkSnooze:
		t, err := time.Parse(time.RFC3339, in.Value)
		if err != nil || !t.After(s.now()) {
			return BulkResult{}, fmt.Errorf("%w: snooze needs a future RFC3339 time", ErrInvalid)
		}
		snoozeUntil = t
	case BulkTag, BulkUntag:
		if strings.TrimSpace(in.Value) == "" {
			return BulkResult{}, fmt.Errorf("%w: tag name is required", ErrInvalid)
		}
	}
	convs, err := s.store.Q().GetConversationsByUUIDs(ctx, db.GetConversationsByUUIDsParams{OrganizationID: in.OrganizationID, Uuids: in.Conversations})
	if err != nil {
		return BulkResult{}, err
	}
	actor := s.Named(ctx, in.Actor)
	var tagID uuid.UUID
	if in.Action == BulkUntag {
		if tags, err := s.Tags(ctx, in.OrganizationID); err == nil {
			for _, t := range tags {
				if strings.EqualFold(t.Name, strings.TrimSpace(in.Value)) {
					tagID = t.UUID
				}
			}
		}
	}
	items := make([]bulkItem, 0, len(convs))
	var res BulkResult
	for _, c := range convs {
		item := bulkItem{Conversation: c.Uuid, Status: c.Status, Priority: c.Priority, SnoozedUntil: timePtr(c.SnoozedUntil)}
		if c.AssigneeUserID.Valid {
			u := s.userUUID(ctx, c.AssigneeUserID.Int64)
			item.Assignee = &u
		}
		up := UpdateInput{OrganizationID: in.OrganizationID, ConversationUUID: c.Uuid, Actor: actor}
		var err error
		switch in.Action {
		case BulkAssign:
			var target *uuid.UUID
			switch in.Value {
			case "":
			case "me":
				target = &actor.UUID
			default:
				id, perr := uuid.Parse(in.Value)
				if perr != nil {
					return BulkResult{}, fmt.Errorf("%w: assignee must be me, an agent uuid or empty", ErrInvalid)
				}
				target = &id
			}
			up.Assignee = &target
			_, err = s.Update(ctx, up)
		case BulkStatus, BulkSnooze:
			st := in.Value
			if in.Action == BulkSnooze {
				st = model.StatusSnoozed
				up.SnoozedUntil = &snoozeUntil
			}
			up.Status = &st
			_, err = s.Update(ctx, up)
		case BulkPriority:
			p := in.Value
			up.Priority = &p
			_, err = s.Update(ctx, up)
		case BulkTag:
			_, err = s.TagConversation(ctx, in.OrganizationID, c.Uuid, actor, uuid.Nil, in.Value, true)
		case BulkUntag:
			if tagID != uuid.Nil {
				_, err = s.TagConversation(ctx, in.OrganizationID, c.Uuid, actor, tagID, "", false)
			}
		}
		if err != nil {
			s.log.Warn("conversation_bulk_item_failed", "conversation", c.Uuid, "action", in.Action, "error", err)
			res.Failed++
			continue
		}
		item.Changed = true
		items = append(items, item)
		res.Applied++
	}
	res.Failed += len(in.Conversations) - len(convs)
	raw, err := json.Marshal(items)
	if err != nil {
		return BulkResult{}, err
	}
	op, err := s.store.Q().InsertConversationBulkOp(ctx, db.InsertConversationBulkOpParams{
		OrganizationID: in.OrganizationID, ActorID: pgInt8(in.Actor.ID), Action: in.Action, Value: in.Value,
		Items: raw, Applied: safeconv.ToInt32(res.Applied), Failed: safeconv.ToInt32(res.Failed),
	})
	if err != nil {
		return BulkResult{}, err
	}
	res.Operation = op.Uuid
	return res, nil
}

// BulkUndoWindow is how long a bulk operation can be reverted.
const BulkUndoWindow = 24 * time.Hour

// ErrUndoExpired: the operation is older than the undo window or already reverted.
var ErrUndoExpired = errors.New("conversations: bulk operation can no longer be undone")

// UndoBulk restores the previous state of every conversation a bulk
// operation changed.
func (s *Service) UndoBulk(ctx context.Context, orgID int64, actor Actor, perms BulkPermissions, id uuid.UUID) (BulkResult, error) {
	op, err := s.store.Q().GetConversationBulkOp(ctx, db.GetConversationBulkOpParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return BulkResult{}, notFound(err)
	}
	if op.RevertedAt.Valid || s.now().Sub(op.CreatedAt.Time) > BulkUndoWindow {
		return BulkResult{}, ErrUndoExpired
	}
	if !perms.Manage && (op.Action != BulkAssign || !perms.Assign) {
		return BulkResult{}, ErrForbidden
	}
	var items []bulkItem
	if err := json.Unmarshal(op.Items, &items); err != nil {
		return BulkResult{}, err
	}
	n, err := s.store.Q().MarkConversationBulkOpReverted(ctx, db.MarkConversationBulkOpRevertedParams{OrganizationID: orgID, ID: op.ID})
	if err != nil {
		return BulkResult{}, err
	}
	if n == 0 {
		return BulkResult{}, ErrUndoExpired
	}
	actor = s.Named(ctx, actor)
	res := BulkResult{Operation: op.Uuid}
	for _, it := range items {
		up := UpdateInput{OrganizationID: orgID, ConversationUUID: it.Conversation, Actor: actor}
		var err error
		switch op.Action {
		case BulkAssign:
			target := it.Assignee
			up.Assignee = &target
			_, err = s.Update(ctx, up)
		case BulkStatus, BulkSnooze:
			st := it.Status
			up.Status = &st
			if st == model.StatusSnoozed {
				if it.SnoozedUntil == nil || !it.SnoozedUntil.After(s.now()) {
					st = model.StatusOpen
				} else {
					up.SnoozedUntil = it.SnoozedUntil
				}
			}
			_, err = s.Update(ctx, up)
		case BulkPriority:
			p := it.Priority
			up.Priority = &p
			_, err = s.Update(ctx, up)
		case BulkTag:
			if t, terr := s.tagByName(ctx, orgID, op.Value); terr == nil {
				_, err = s.TagConversation(ctx, orgID, it.Conversation, actor, t, "", false)
			}
		case BulkUntag:
			_, err = s.TagConversation(ctx, orgID, it.Conversation, actor, uuid.Nil, op.Value, true)
		}
		if err != nil {
			res.Failed++
			continue
		}
		res.Applied++
	}
	return res, nil
}

func (s *Service) tagByName(ctx context.Context, orgID int64, name string) (uuid.UUID, error) {
	tags, err := s.Tags(ctx, orgID)
	if err != nil {
		return uuid.Nil, err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, strings.TrimSpace(name)) {
			return t.UUID, nil
		}
	}
	return uuid.Nil, ErrNotFound
}

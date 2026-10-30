package inbox

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

// Authorizer guards the inbox channels (CF-44) and defers every other
// channel to next:
//
//   - inbox:{org} carries every event of the organization: members with
//     tenant.conversations.read_all only;
//   - inbox:{org}:{user} carries the events of the conversations that member
//     may see (LiveNotifier fans out): that member only, with
//     tenant.conversations.read.
type Authorizer struct {
	Next     realtime.ChannelAuthorizer
	Resolver orgctx.Resolver
	Queries  *db.Queries
}

// CanSubscribeChannel implements realtime.ChannelAuthorizer.
func (a Authorizer) CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error) {
	if orgUUID, forUser, ok := realtime.ParseInboxUserChannel(channel); ok {
		if forUser != userUUID {
			return false, nil
		}
		scope, ok := a.scope(ctx, userUUID, orgUUID)
		return ok && access.CanRead(scope), nil
	}
	orgUUID, ok := realtime.ParseInboxChannel(channel)
	if !ok {
		return a.Next.CanSubscribeChannel(ctx, userUUID, isSuperAdmin, channel)
	}
	scope, ok := a.scope(ctx, userUUID, orgUUID)
	return ok && scope.CanSeeAll(access.Resource), nil
}

func (a Authorizer) scope(ctx context.Context, userUUID, orgUUID uuid.UUID) (orgctx.Scope, bool) {
	if a.Resolver == nil || a.Queries == nil {
		return orgctx.Scope{}, false
	}
	user, err := a.Queries.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return orgctx.Scope{}, false
	}
	scope, err := a.Resolver.Resolve(ctx, orgUUID.String(), user.ID)
	if err != nil {
		return orgctx.Scope{}, false
	}
	return scope, true
}

// Enqueuer is the notifications service slice the inbox uses.
type Enqueuer interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
	NotifyEvent(ctx context.Context, in notifusecase.NotifyInput) error
}

// ConversationLookup loads the facts a conversation notification shows.
type ConversationLookup interface {
	Conversation(ctx context.Context, organizationID, conversationID int64) (notifmodule.ConversationInfo, error)
}

// MentionNotifier notifies @mentions and assignments through the
// notification catalog (CF-157): the member's per-event preferences, quiet
// hours, e-mail and push apply as for every other event.
type MentionNotifier struct {
	N      Enqueuer
	Lookup ConversationLookup
	Log    *slog.Logger
}

func (m MentionNotifier) send(ctx context.Context, orgID int64, userIDs []int64, event string, by convusecase.Actor, conv db.Conversation, preview string) {
	info := notifmodule.ConversationInfo{UUID: conv.Uuid, Number: conv.Number, Subject: conv.Subject, ChannelType: conv.ChannelType, LastMessage: conv.LastMessagePreview}
	if m.Lookup != nil {
		if got, err := m.Lookup.Conversation(ctx, orgID, conv.ID); err == nil {
			info = got
		}
	}
	vars := info.Vars()
	vars["actor"] = by.Name
	if event == events.ConversationMentioned || event == notifusecase.EventReminderDue {
		vars["preview"] = preview // the note, not the last customer message
	}
	org := orgID
	if err := m.N.NotifyEvent(ctx, notifusecase.NotifyInput{
		Event: event, OrganizationID: &org, UserIDs: userIDs, Vars: vars, ActionURL: vars["url"],
		Payload: map[string]any{"conversation_uuid": conv.Uuid.String(), "number": conv.Number},
	}); err != nil && m.Log != nil {
		m.Log.Warn("inbox_notification_failed", "event", event, "error", err)
	}
}

// Mentioned implements usecase.MentionNotifier.
func (m MentionNotifier) Mentioned(ctx context.Context, orgID int64, userIDs []int64, by convusecase.Actor, conv db.Conversation, preview string) {
	ids := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id != by.ID {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		m.send(ctx, orgID, ids, events.ConversationMentioned, by, conv, preview)
	}
}

// Assigned implements usecase.MentionNotifier.
func (m MentionNotifier) Assigned(ctx context.Context, orgID int64, userID int64, by convusecase.Actor, conv db.Conversation) {
	m.send(ctx, orgID, []int64{userID}, events.ConversationAssigned, by, conv, "")
}

// ReminderDue implements usecase.ReminderNotifier.
func (m MentionNotifier) ReminderDue(ctx context.Context, orgID, userID int64, conv db.Conversation, note string) {
	m.send(ctx, orgID, []int64{userID}, notifusecase.EventReminderDue, convusecase.Actor{}, conv, note)
}

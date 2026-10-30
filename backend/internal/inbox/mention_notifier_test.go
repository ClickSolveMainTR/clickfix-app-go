package inbox_test

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

type recordingNotifier struct{ got []notifusecase.NotifyInput }

func (r *recordingNotifier) Enqueue(context.Context, notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	return nil, nil
}

func (r *recordingNotifier) NotifyEvent(_ context.Context, in notifusecase.NotifyInput) error {
	r.got = append(r.got, in)
	return nil
}

type staticLookup struct{ info notifmodule.ConversationInfo }

func (s staticLookup) Conversation(context.Context, int64, int64) (notifmodule.ConversationInfo, error) {
	return s.info, nil
}

// TestMentionNotifierUsesCatalog: assignments, mentions and reminders go
// through the notification catalog (preferences, quiet hours, e-mail, push)
// with the conversation facts, not as bare in-app rows (CF-157).
func TestMentionNotifierUsesCatalog(t *testing.T) {
	conv := db.Conversation{ID: 9, Uuid: uuid.New(), Number: 42}
	rec := &recordingNotifier{}
	n := inbox.MentionNotifier{N: rec, Lookup: staticLookup{notifmodule.ConversationInfo{
		UUID: conv.Uuid, Number: 42, Customer: "Deniz Kaya", ChannelType: "whatsapp", LastMessage: "Kargo nerede?",
	}}}
	ctx := context.Background()
	by := convusecase.Actor{ID: 3, Name: "Ali Veli"}

	n.Assigned(ctx, 1, 7, by, conv)
	n.Mentioned(ctx, 1, []int64{3, 8}, by, conv, "@Ayşe bakar mısın?")
	n.ReminderDue(ctx, 1, 7, conv, "Müşteriyi ara")
	if len(rec.got) != 3 {
		t.Fatalf("calls %d", len(rec.got))
	}
	a, m, r := rec.got[0], rec.got[1], rec.got[2]
	if a.Event != events.ConversationAssigned || a.UserIDs[0] != 7 || a.Vars["actor"] != "Ali Veli" || a.Vars["customer"] != "Deniz Kaya" || a.Vars["preview"] != "Kargo nerede?" {
		t.Fatalf("assigned %+v", a)
	}
	if a.ActionURL != "/inbox?c="+conv.Uuid.String() || a.OrganizationID == nil || *a.OrganizationID != 1 {
		t.Fatalf("assigned target %+v", a)
	}
	if m.Event != events.ConversationMentioned || len(m.UserIDs) != 1 || m.UserIDs[0] != 8 || m.Vars["preview"] != "@Ayşe bakar mısın?" {
		t.Fatalf("mentioned %+v (the author is never notified; the note is the preview)", m)
	}
	if r.Event != notifusecase.EventReminderDue || r.Vars["preview"] != "Müşteriyi ara" {
		t.Fatalf("reminder %+v", r)
	}
}

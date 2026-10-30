package inbox_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
)

func TestInboxStateChangesLandOnTimeline(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"s-1","from":{"id":"v-7","name":"Selin"},"text":"İade istiyorum"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND slug = 'member' AND organization_id IS NULL", e.org, e.agent); err != nil {
		t.Fatal(err)
	}
	var agentUUID uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	list, total, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{Assignee: "unassigned"})
	if err != nil || total != 1 || list[0].UnreadCount != 1 || list[0].Contact.Name != "Selin" {
		t.Fatalf("unassigned list = %+v total=%d err=%v", list, total, err)
	}
	conv := list[0].UUID
	actor := e.ib.Conversations.Named(ctx, convusecase.Actor{ID: e.agent, UUID: agentUUID})
	if actor.Name != "Elif Şahin" {
		t.Fatalf("actor name = %q", actor.Name)
	}
	target := &agentUUID
	resolved, urgent := convmodel.StatusResolved, convmodel.PriorityUrgent
	if _, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{
		OrganizationID: e.org, ConversationUUID: conv, Actor: actor, Assignee: &target, Status: &resolved, Priority: &urgent,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ib.Conversations.TagConversation(ctx, e.org, conv, actor, uuid.Nil, "İade", true); err != nil {
		t.Fatal(err)
	}
	mine, _, _ := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{Assignee: "me", Statuses: []string{"resolved"}})
	if len(mine) != 1 || mine[0].Priority != urgent || len(mine[0].TagUUIDs) != 1 || mine[0].Assignee == nil {
		t.Fatalf("mine = %+v", mine)
	}
	page, err := e.ib.Conversations.Messages(ctx, e.org, conv, uuid.Nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	events := []string{}
	for _, m := range page.Items {
		if m.Kind == convmodel.KindEvent {
			events = append(events, m.Attributes.(map[string]any)["event"].(string))
		}
	}
	if len(page.Items) != 5 || len(events) != 4 || page.Items[0].Kind != convmodel.KindText {
		t.Fatalf("timeline = %d items, events %v", len(page.Items), events)
	}
	var outbox int
	_ = e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name IN ('conversations.assigned','conversations.resolved') AND payload->>'entity_uuid' = $1`, conv.String()).Scan(&outbox)
	if outbox != 2 {
		t.Fatalf("assigned/resolved outbox rows = %d", outbox)
	}
	// Snoozing needs a future time; an inbound message wakes it.
	snoozed := convmodel.StatusSnoozed
	if _, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: conv, Actor: actor, Status: &snoozed}); err == nil {
		t.Fatal("snooze without a time must fail")
	}
	until := time.Now().Add(time.Hour)
	if _, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: conv, Actor: actor, Status: &snoozed, SnoozedUntil: &until}); err != nil {
		t.Fatal(err)
	}
	_ = e.post(t, `{"messages":[{"id":"s-2","from":{"id":"v-7"},"text":"Hâlâ bekliyorum"}]}`, "")
	d, err := e.ib.Conversations.Get(ctx, e.org, e.agent, conv)
	if err != nil || d.Status != convmodel.StatusOpen || len(d.Tags) != 1 || len(d.Participants) != 1 {
		t.Fatalf("after wake: %+v (%v)", d, err)
	}
	if err := e.ib.Conversations.MarkRead(ctx, e.org, e.agent, conv); err != nil {
		t.Fatal(err)
	}
	if d, _ = e.ib.Conversations.Get(ctx, e.org, e.agent, conv); d.UnreadCount != 0 {
		t.Fatalf("unread after read = %d", d.UnreadCount)
	}
	counts, err := e.ib.Conversations.Counts(ctx, e.org, e.agent)
	if err != nil || counts.Mine != 1 || counts.ByStatus["open"] != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

package inbox_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
)

// CF-247: an agent marks a conversation unread again, one by one and in
// bulk; undo puts its read marker back.
func TestMarkUnreadAndBulk(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, id := range []string{"ur-1", "ur-2"} {
		if code := e.post(t, `{"messages":[{"id":"`+id+`","from":{"id":"`+id+`-v"},"text":"selam"}]}`, ""); code != http.StatusOK {
			t.Fatalf("webhook = %d", code)
		}
	}
	// A second customer message in the first conversation.
	_ = e.post(t, `{"messages":[{"id":"ur-1b","from":{"id":"ur-1-v"},"text":"orada misiniz"}]}`, "")
	var agentUUID uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	var convs []uuid.UUID
	rows, _ := e.pool.Query(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1 ORDER BY number", e.org)
	for rows.Next() {
		var u uuid.UUID
		_ = rows.Scan(&u)
		convs = append(convs, u)
	}
	rows.Close()
	if len(convs) != 2 {
		t.Fatalf("conversations = %d", len(convs))
	}
	svc := e.ib.Conversations
	actor := convusecase.Actor{ID: e.agent, UUID: agentUUID}
	unread := func(u uuid.UUID) int {
		t.Helper()
		d, err := svc.Get(ctx, e.org, e.agent, u)
		if err != nil {
			t.Fatal(err)
		}
		return d.UnreadCount
	}

	if unread(convs[0]) != 2 {
		t.Fatalf("new conversation unread = %d", unread(convs[0]))
	}
	if err := svc.MarkRead(ctx, e.org, e.agent, convs[0]); err != nil || unread(convs[0]) != 0 {
		t.Fatalf("read: %v, unread = %d", err, unread(convs[0]))
	}
	// Unread leaves only the latest customer message unread.
	if err := svc.MarkUnread(ctx, e.org, actor, convs[0]); err != nil {
		t.Fatal(err)
	}
	if n := unread(convs[0]); n != 1 {
		t.Fatalf("after unread = %d", n)
	}
	// The marker never moves forward: unread again keeps the count.
	if err := svc.MarkUnread(ctx, e.org, actor, convs[0]); err != nil || unread(convs[0]) != 1 {
		t.Fatalf("second unread: %v, unread = %d", err, unread(convs[0]))
	}
	last := e.live.events[len(e.live.events)-1]
	if last.Type != convmodel.EventReadState || last.ConversationUUID != convs[0] || last.ActorUUID == nil || *last.ActorUUID != agentUUID {
		t.Fatalf("read state event = %+v", last)
	}
	if err := svc.MarkUnread(ctx, e.org, actor, uuid.New()); !errors.Is(err, convusecase.ErrNotFound) {
		t.Fatalf("unknown conversation = %v", err)
	}

	// Bulk read needs no manage permission; undo restores the markers.
	both := []uuid.UUID{convs[0], convs[1]}
	br, err := svc.Bulk(ctx, convusecase.BulkInput{OrganizationID: e.org, Actor: actor, Conversations: both, Action: convusecase.BulkRead})
	if err != nil || br.Applied != 2 {
		t.Fatalf("bulk read = %+v (%v)", br, err)
	}
	if unread(convs[0]) != 0 || unread(convs[1]) != 0 {
		t.Fatalf("bulk read left %d / %d", unread(convs[0]), unread(convs[1]))
	}
	bu, err := svc.Bulk(ctx, convusecase.BulkInput{OrganizationID: e.org, Actor: actor, Conversations: both, Action: convusecase.BulkUnread})
	if err != nil || bu.Applied != 2 {
		t.Fatalf("bulk unread = %+v (%v)", bu, err)
	}
	if unread(convs[0]) != 1 || unread(convs[1]) != 1 {
		t.Fatalf("bulk unread left %d / %d", unread(convs[0]), unread(convs[1]))
	}
	// Another member may not undo someone else's read markers.
	other := convusecase.Actor{ID: e.agent + 1_000_000, UUID: uuid.New()}
	if _, err := svc.UndoBulk(ctx, e.org, other, convusecase.BulkPermissions{Manage: true}, bu.Operation); !errors.Is(err, convusecase.ErrForbidden) {
		t.Fatalf("foreign undo = %v", err)
	}
	undo, err := svc.UndoBulk(ctx, e.org, actor, convusecase.BulkPermissions{}, bu.Operation)
	if err != nil || undo.Applied != 2 {
		t.Fatalf("undo = %+v (%v)", undo, err)
	}
	if unread(convs[0]) != 0 || unread(convs[1]) != 0 {
		t.Fatalf("undo left %d / %d", unread(convs[0]), unread(convs[1]))
	}
}

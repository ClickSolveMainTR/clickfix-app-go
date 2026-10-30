package inbox_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
)

func TestSnoozeWakeRemindersAndBulk(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, id := range []string{"fu-1", "fu-2", "fu-3"} {
		if code := e.post(t, `{"messages":[{"id":"`+id+`","from":{"id":"`+id+`-v"},"text":"selam"}]}`, ""); code != http.StatusOK {
			t.Fatalf("webhook = %d", code)
		}
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND slug = 'member' AND organization_id IS NULL", e.org, e.agent); err != nil {
		t.Fatal(err)
	}
	var agentUUID uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	convs := map[int64]uuid.UUID{}
	rows, _ := e.pool.Query(ctx, "SELECT number, uuid FROM conversations WHERE organization_id = $1", e.org)
	for rows.Next() {
		var n int64
		var u uuid.UUID
		_ = rows.Scan(&n, &u)
		convs[n] = u
	}
	rows.Close()
	svc := e.ib.Conversations
	actor := convusecase.Actor{ID: e.agent, UUID: agentUUID}

	// 1. An expired snooze wakes up on the tick.
	if _, err := e.pool.Exec(ctx, "UPDATE conversations SET status = 'snoozed', snoozed_until = NOW() - interval '1 minute' WHERE uuid = $1", convs[1]); err != nil {
		t.Fatal(err)
	}
	// 2. Reminders: 'always' on a resolved conversation reopens it; 'no_reply'
	// is superseded because the contact writes after it was set.
	resolved := "resolved"
	if _, err := svc.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: convs[2], Actor: actor, Status: &resolved}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddReminder(ctx, e.org, convs[2], actor, time.Now().Add(time.Hour), "always", "Ara"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddReminder(ctx, e.org, convs[3], actor, time.Now().Add(time.Hour), "no_reply", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddReminder(ctx, e.org, convs[3], actor, time.Now().Add(-time.Hour), "always", ""); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("past reminder = %v", err)
	}
	_ = e.post(t, `{"messages":[{"id":"fu-3b","from":{"id":"fu-3-v"},"text":"cevap"}]}`, "")
	if _, err := e.pool.Exec(ctx, "UPDATE conversation_reminders SET due_at = NOW() - interval '1 second' WHERE organization_id = $1", e.org); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Tick(ctx)
	if err != nil || res.Woken != 1 || res.Reminded != 1 || res.Superseded != 1 {
		t.Fatalf("tick = %+v (%v)", res, err)
	}
	status := func(u uuid.UUID) string {
		var s string
		_ = e.pool.QueryRow(ctx, "SELECT status FROM conversations WHERE uuid = $1", u).Scan(&s)
		return s
	}
	if status(convs[1]) != "open" || status(convs[2]) != "open" {
		t.Fatalf("after tick: %s %s", status(convs[1]), status(convs[2]))
	}

	// 3. Bulk resolve + undo.
	all := []uuid.UUID{convs[1], convs[2], convs[3]}
	if _, err := svc.Bulk(ctx, convusecase.BulkInput{OrganizationID: e.org, Actor: actor, Conversations: all, Action: "status", Value: "resolved"}); !errors.Is(err, convusecase.ErrForbidden) {
		t.Fatalf("bulk without permission = %v", err)
	}
	perms := convusecase.BulkPermissions{Manage: true}
	br, err := svc.Bulk(ctx, convusecase.BulkInput{OrganizationID: e.org, Actor: actor, Perms: perms, Conversations: all, Action: "status", Value: "resolved"})
	if err != nil || br.Applied != 3 {
		t.Fatalf("bulk = %+v (%v)", br, err)
	}
	for _, u := range all {
		if status(u) != "resolved" {
			t.Fatalf("bulk did not resolve %s", u)
		}
	}
	if _, err := svc.Bulk(ctx, convusecase.BulkInput{OrganizationID: e.org, Actor: actor, Perms: perms, Conversations: all, Action: "tag", Value: "kampanya"}); err != nil {
		t.Fatal(err)
	}
	undo, err := svc.UndoBulk(ctx, e.org, actor, perms, br.Operation)
	if err != nil || undo.Applied != 3 {
		t.Fatalf("undo = %+v (%v)", undo, err)
	}
	for _, u := range all {
		if status(u) != "open" {
			t.Fatalf("undo did not restore %s: %s", u, status(u))
		}
	}
	if _, err := svc.UndoBulk(ctx, e.org, actor, perms, br.Operation); !errors.Is(err, convusecase.ErrUndoExpired) {
		t.Fatalf("second undo = %v", err)
	}
}

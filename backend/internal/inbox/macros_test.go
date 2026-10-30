package inbox_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	macros "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros/usecase"
	"github.com/google/uuid"
)

func TestCannedResponsesAndMacros(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"mc-1","from":{"id":"v-m","name":"Ayşe Kara","email":"ayse@firma.com"},"text":"Fatura?"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND slug = 'member' AND organization_id IS NULL", e.org, e.agent); err != nil {
		t.Fatal(err)
	}
	var agentUUID, conv uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1", e.org).Scan(&conv)
	svc := macros.New(db.New(e.pool), e.ib.Conversations)
	agent := macros.Viewer{OrganizationID: e.org, UserID: e.agent, UserUUID: agentUUID}

	if _, err := svc.CreateCanned(ctx, agent, macros.CannedInput{Scope: "organization", Title: "x", Shortcut: "x", Content: "x"}); !errors.Is(err, macros.ErrForbidden) {
		t.Fatalf("organization canned without write = %v", err)
	}
	c, err := svc.CreateCanned(ctx, agent, macros.CannedInput{Title: "Selam", Shortcut: "/Selam", Content: "Merhaba {{contact.first_name}}, ben {{agent.first_name}} (#{{conversation.number}})."})
	if err != nil || c.Shortcut != "selam" || c.Scope != "personal" {
		t.Fatalf("personal canned: %+v %v", c, err)
	}
	if _, err := svc.CreateCanned(ctx, agent, macros.CannedInput{Title: "Again", Shortcut: "selam", Content: "y"}); !errors.Is(err, macros.ErrShortcutExists) {
		t.Fatalf("duplicate shortcut = %v", err)
	}
	r, err := svc.Render(ctx, agent, c.UUID, conv)
	if err != nil || r.Content != "Merhaba Ayşe, ben Elif (#1)." {
		t.Fatalf("render = %q (%v)", r.Content, err)
	}
	list, _ := svc.ListCanned(ctx, agent, "sel")
	if len(list) != 1 || list[0].UsageCount != 1 {
		t.Fatalf("list after render = %+v", list)
	}

	admin := agent
	admin.CanWrite = true
	m, err := svc.CreateMacro(ctx, admin, macros.MacroInput{Scope: "organization", Name: "Fatura çözüldü", Actions: []macros.Action{
		{Type: "reply", Value: "Faturanız gönderildi {{contact.first_name}}."},
		{Type: "tag", Value: "fatura"},
		{Type: "assign", Value: "me"},
		{Type: "status", Value: "resolved"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, agent, macros.Permissions{Note: true}, m.UUID, conv); !errors.Is(err, macros.ErrForbidden) {
		t.Fatalf("macro without permissions = %v", err)
	}
	res, err := svc.Apply(ctx, agent, macros.Permissions{Reply: true, Manage: true, Assign: true}, m.UUID, conv)
	if err != nil || len(res.Applied) != 4 {
		t.Fatalf("apply = %+v %v", res, err)
	}
	d, err := e.ib.Conversations.Get(ctx, e.org, e.agent, conv)
	if err != nil || d.Status != "resolved" || d.Assignee == nil || len(d.Tags) != 1 || d.Tags[0].Name != "fatura" {
		t.Fatalf("after macro: %+v", d)
	}
	page, _ := e.ib.Conversations.Messages(ctx, e.org, conv, uuid.Nil, 50)
	found := false
	for _, msg := range page.Items {
		if strings.Contains(msg.Content, "Faturanız gönderildi Ayşe.") {
			found = true
		}
	}
	if !found {
		t.Fatal("macro reply missing")
	}
}

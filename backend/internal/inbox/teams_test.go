package inbox_test

import (
	"context"
	"net/http"
	"testing"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
)

func TestTeamAssignment(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, id := range []string{"tm-1", "tm-2", "tm-3"} {
		if code := e.post(t, `{"messages":[{"id":"`+id+`","from":{"id":"`+id+`-v"},"text":"merhaba"}]}`, ""); code != http.StatusOK {
			t.Fatalf("webhook = %d", code)
		}
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND slug = 'member' AND organization_id IS NULL", e.org, e.agent); err != nil {
		t.Fatal(err)
	}
	var mine, other uuid.UUID
	if err := e.pool.QueryRow(ctx, "INSERT INTO teams (organization_id, name) VALUES ($1, 'L1') RETURNING uuid", e.org).Scan(&mine); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, "INSERT INTO teams (organization_id, name) VALUES ($1, 'Faturalama') RETURNING uuid", e.org).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO team_members (organization_id, team_id, user_id) SELECT $1, id, $2 FROM teams WHERE uuid = $3", e.org, e.agent, mine); err != nil {
		t.Fatal(err)
	}
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
	var agentUUID uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	actor := svc.Named(ctx, convusecase.Actor{ID: e.agent, UUID: agentUUID})
	route := func(conv, team uuid.UUID) {
		t.Helper()
		target := &team
		if _, err := svc.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: conv, Actor: actor, Team: &target}); err != nil {
			t.Fatal(err)
		}
	}
	route(convs[1], mine)
	route(convs[2], other)

	d, err := svc.Get(ctx, e.org, e.agent, convs[1])
	if err != nil || d.Team == nil || d.Team.Name != "L1" {
		t.Fatalf("team on detail: %+v (%v)", d.Team, err)
	}
	page, _ := svc.Messages(ctx, e.org, convs[1], uuid.Nil, 50)
	last := page.Items[len(page.Items)-1]
	if last.Attributes.(map[string]any)["event"] != "team_assigned" {
		t.Fatalf("timeline event = %+v", last.Attributes)
	}

	teams, total, _ := svc.List(ctx, e.org, e.agent, convusecase.ListFilter{MyTeams: true})
	if total != 1 || teams[0].UUID != convs[1] {
		t.Fatalf("my teams = %d", total)
	}
	if _, err := e.pool.Exec(ctx, "DELETE FROM teams WHERE uuid = $1", other); err != nil {
		t.Fatal(err)
	}
	var team *string
	_ = e.pool.QueryRow(ctx, "SELECT assignee_team_id::text FROM conversations WHERE uuid = $1", convs[2]).Scan(&team)
	if team != nil {
		t.Fatal("deleting a team must clear its conversations")
	}
}

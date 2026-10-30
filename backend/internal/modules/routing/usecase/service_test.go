package usecase_test

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sqlAssigner writes the assignee directly (the real one goes through the
// conversations service).
type sqlAssigner struct{ pool *pgxpool.Pool }

func (a sqlAssigner) Assign(ctx context.Context, orgID int64, conv uuid.UUID, user *uuid.UUID) error {
	_, err := a.pool.Exec(ctx, `UPDATE conversations SET assignee_user_id = (SELECT id FROM users WHERE uuid = $3)
		WHERE organization_id = $1 AND uuid = $2`, orgID, conv, user)
	return err
}

type fixture struct {
	pool    *pgxpool.Pool
	svc     *usecase.Service
	org     int64
	agents  []int64
	uuids   []uuid.UUID
	contact int64
	num     int64
}

func scalar(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func setup(t *testing.T, n int) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for routing tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &fixture{pool: pool, svc: usecase.New(pool, db.New(pool), sqlAssigner{pool}, nil)}
	f.org = scalar(t, pool, "INSERT INTO organizations (slug, name) VALUES ($1, 'Routing') RETURNING id", "rt-"+uuid.NewString()[:12])
	role := scalar(t, pool, "SELECT id FROM roles WHERE slug = 'agent' AND scope = 'tenant' AND organization_id IS NULL")
	for range n {
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ajan', 'Test', 'active') RETURNING id, uuid",
			uuid.NewString()+"@example.test").Scan(&id, &u); err != nil {
			t.Fatal(err)
		}
		f.exec(t, "INSERT INTO organization_members (organization_id, user_id, role_id) VALUES ($1, $2, $3)", f.org, id, role)
		f.exec(t, "INSERT INTO agent_presence (organization_id, user_id, status, manual_status, last_seen_at) VALUES ($1, $2, 'online', 'online', NOW())", f.org, id)
		f.agents, f.uuids = append(f.agents, id), append(f.uuids, u)
	}
	f.contact = scalar(t, pool, "INSERT INTO contacts (organization_id) VALUES ($1) RETURNING id", f.org)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", f.agents)
	})
	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) conversation(t *testing.T, channel string) int64 {
	t.Helper()
	f.num++
	return scalar(t, f.pool, "INSERT INTO conversations (organization_id, number, channel_type, contact_id, waiting_since) VALUES ($1, $2, $3, $4, NOW()) RETURNING id",
		f.org, f.num, channel, f.contact)
}

func (f *fixture) assignee(t *testing.T, conv int64) int64 {
	t.Helper()
	var id *int64
	_ = f.pool.QueryRow(context.Background(), "SELECT assignee_user_id FROM conversations WHERE id = $1", conv).Scan(&id)
	if id == nil {
		return 0
	}
	return *id
}

func (f *fixture) enable(t *testing.T, strategy string, unanswered *int32) {
	t.Helper()
	if _, err := f.svc.SaveSettings(context.Background(), f.org, model.Settings{
		Enabled: true, Strategy: strategy, RespectCapacity: true, VIPAccountOwner: true, ReassignOnOffline: true, UnansweredMinutes: unanswered,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRoundRobinCapacityAndQueue(t *testing.T) {
	f := setup(t, 2)
	ctx := context.Background()
	c1 := f.conversation(t, "widget")
	// Routing off: nothing happens.
	if pick, _ := f.svc.Route(ctx, f.org, c1, usecase.ReasonNew, 0); pick != 0 {
		t.Fatal("routed while disabled")
	}
	f.enable(t, "round_robin", nil) // drains the queue: c1 → agent 0
	if got := f.assignee(t, c1); got != f.agents[0] {
		t.Fatalf("c1 → %d", got)
	}
	c2 := f.conversation(t, "widget")
	if pick, err := f.svc.Route(ctx, f.org, c2, usecase.ReasonNew, 0); err != nil || pick != f.agents[1] {
		t.Fatalf("c2 → %d %v", pick, err)
	}
	// Capacity: one open conversation each.
	if _, err := f.svc.SetCapacityLimits(ctx, f.org, model.CapacityInput{Limits: model.Limits{"*": 1}}); err != nil {
		t.Fatal(err)
	}
	c3 := f.conversation(t, "widget")
	if pick, _ := f.svc.Route(ctx, f.org, c3, usecase.ReasonNew, 0); pick != 0 {
		t.Fatal("over capacity")
	}
	view, err := f.svc.Capacity(ctx, f.org)
	if err != nil || view.Queue != 1 || view.Online != 2 || view.Agents[0].OpenTotal != 1 || view.Agents[0].Available {
		t.Fatalf("view %+v err=%v", view, err)
	}
	// Agent 0 gets more room: the queue drains to them.
	if _, err := f.svc.SetCapacityLimits(ctx, f.org, model.CapacityInput{UserUUID: &f.uuids[0], Limits: model.Limits{"*": 3}}); err != nil {
		t.Fatal(err)
	}
	if got := f.assignee(t, c3); got != f.agents[0] {
		t.Fatalf("c3 → %d", got)
	}
	evs, _ := f.svc.Events(ctx, f.org, nil, 10)
	if len(evs) != 3 || evs[0].To == nil {
		t.Fatalf("audit %+v", evs)
	}
}

func TestOfflineHandoverVIPAndUnanswered(t *testing.T) {
	f := setup(t, 2)
	ctx := context.Background()
	ten := int32(10)
	f.enable(t, "load_balanced", &ten)
	// VIP: the company's account owner (agent 1) gets the conversation.
	company := scalar(t, f.pool, "INSERT INTO companies (organization_id, name, account_owner_id) VALUES ($1, 'VIP', $2) RETURNING id", f.org, f.agents[1])
	c1 := f.conversation(t, "email")
	f.exec(t, "UPDATE conversations SET company_id = $2 WHERE id = $1", c1, company)
	if pick, _ := f.svc.Route(ctx, f.org, c1, usecase.ReasonNew, 0); pick != f.agents[1] {
		t.Fatalf("vip → %d", pick)
	}
	// Agent 1 goes offline: the waiting conversation moves to agent 0.
	f.exec(t, "UPDATE agent_presence SET status = 'offline' WHERE user_id = $1", f.agents[1])
	if err := f.svc.AgentOffline(ctx, f.org, f.agents[1]); err != nil {
		t.Fatal(err)
	}
	if got := f.assignee(t, c1); got != f.agents[0] {
		t.Fatalf("handover → %d", got)
	}
	// Unanswered: back online, agent 0 left it waiting 15 minutes.
	f.exec(t, "UPDATE agent_presence SET status = 'online', last_seen_at = NOW() WHERE user_id = $1", f.agents[1])
	f.exec(t, "UPDATE conversations SET waiting_since = NOW() - interval '15 minutes' WHERE id = $1", c1)
	f.exec(t, "UPDATE routing_events SET created_at = NOW() - interval '12 minutes' WHERE conversation_id = $1", c1)
	if n, err := f.svc.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v", n, err)
	}
	if got := f.assignee(t, c1); got != f.agents[1] {
		t.Fatalf("unanswered → %d", got)
	}
	evs, _ := f.svc.Events(ctx, f.org, nil, 10)
	// The handover goes back to the VIP account owner, logged as vip.
	if evs[0].Reason != "vip" || evs[0].From == nil || evs[1].Reason != "offline" || evs[2].Reason != "vip" {
		t.Fatalf("reasons %v %v %v", evs[0].Reason, evs[1].Reason, evs[2].Reason)
	}
}

func TestRuleTeamQueueAndSkills(t *testing.T) {
	f := setup(t, 3)
	ctx := context.Background()
	team := scalar(t, f.pool, "INSERT INTO teams (organization_id, name) VALUES ($1, 'Fatura') RETURNING id", f.org)
	var teamUUID uuid.UUID
	_ = f.pool.QueryRow(ctx, "SELECT uuid FROM teams WHERE id = $1", team).Scan(&teamUUID)
	f.exec(t, "INSERT INTO team_members (organization_id, team_id, user_id) VALUES ($1, $2, $3), ($1, $2, $4)", f.org, team, f.agents[1], f.agents[2])
	f.exec(t, "UPDATE organization_members SET skills = '{en}' WHERE user_id = $1", f.agents[2])
	if _, err := f.svc.CreateRule(ctx, f.org, model.RuleInput{Name: "WhatsApp → Fatura", TeamUUID: &teamUUID,
		Conditions: model.Conditions{ChannelTypes: []string{"whatsapp"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateRule(ctx, f.org, model.RuleInput{Name: "İngilizce", Skills: []string{"EN"},
		Conditions: model.Conditions{ChannelTypes: []string{"email"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateRule(ctx, f.org, model.RuleInput{Name: "Boş"}); err == nil {
		t.Fatal("rule without team or skills accepted")
	}
	f.enable(t, "round_robin", nil)
	wa := f.conversation(t, "whatsapp")
	if pick, _ := f.svc.Route(ctx, f.org, wa, usecase.ReasonNew, 0); pick != f.agents[1] {
		t.Fatalf("team pool → %d", pick)
	}
	var teamID *int64
	_ = f.pool.QueryRow(ctx, "SELECT assignee_team_id FROM conversations WHERE id = $1", wa).Scan(&teamID)
	if teamID == nil || *teamID != team {
		t.Fatal("team queue not set")
	}
	em := f.conversation(t, "email")
	if pick, _ := f.svc.Route(ctx, f.org, em, usecase.ReasonNew, 0); pick != f.agents[2] {
		t.Fatalf("skill → %d", pick)
	}
}

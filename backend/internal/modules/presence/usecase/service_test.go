package usecase

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recPub struct {
	mu   sync.Mutex
	msgs []struct {
		channel string
		msg     realtime.Message
	}
}

func (p *recPub) Publish(_ context.Context, channel string, data any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, struct {
		channel string
		msg     realtime.Message
	}{channel, data.(realtime.Message)})
	return nil
}

func (p *recPub) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.msgs)
}

type onceLimiter struct{ seen map[string]bool }

func (l *onceLimiter) Allow(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	k := action + subject
	if l.seen[k] {
		return false, time.Second
	}
	l.seen[k] = true
	return true, 0
}

type fixture struct {
	pool *pgxpool.Pool
	q    *db.Queries
	a    Actor
	conv uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "pr-" + suffix, Name: "Presence"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID) })
	var userID int64
	var userUUID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ece', 'Agent', 'active') RETURNING id, uuid`,
		"pr-"+suffix+"@example.test").Scan(&userID, &userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	if _, err := pool.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
		SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'member'`, org.ID, userID); err != nil {
		t.Fatal(err)
	}
	var contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, name) VALUES ($1, 'Müşteri') RETURNING id`, org.ID).Scan(&contactID); err != nil {
		t.Fatal(err)
	}
	conv, err := q.InsertConversation(ctx, db.InsertConversationParams{
		OrganizationID: org.ID, Number: 1, ChannelType: "web", ContactID: contactID, Subject: "x",
		Status: "open", Priority: "normal", Source: "inbound", CustomFields: []byte("{}"),
		LastMessageAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{pool: pool, q: q, conv: conv.Uuid, a: Actor{OrganizationID: org.ID, OrganizationUUID: org.Uuid, UserID: userID, UserUUID: userUUID}}
}

func TestPresenceLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pub := &recPub{}
	svc := New(f.pool, f.q, pub, nil, nil)

	list, err := svc.List(ctx, f.a.OrganizationID)
	if err != nil || len(list) != 1 || list[0].Status != StatusOffline || list[0].Name != "Ece Agent" {
		t.Fatalf("initial list = %+v %v", list, err)
	}
	me, err := svc.Heartbeat(ctx, f.a)
	if err != nil || me.Status != StatusOnline || pub.count() != 1 {
		t.Fatalf("first heartbeat = %+v %v pubs=%d", me, err, pub.count())
	}
	if pub.msgs[0].channel != realtime.OrgChannel(f.a.OrganizationUUID) || pub.msgs[0].msg.Type != realtime.EventPresenceChanged {
		t.Fatalf("publication = %+v", pub.msgs[0])
	}
	// Repeated heartbeats and same-status sets publish nothing.
	if _, err := svc.Heartbeat(ctx, f.a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetStatus(ctx, f.a, StatusOnline); err != nil || pub.count() != 1 {
		t.Fatalf("idempotent set published: %d %v", pub.count(), err)
	}
	if _, err := svc.SetStatus(ctx, f.a, "lunch"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad status: %v", err)
	}
	me, err = svc.SetStatus(ctx, f.a, StatusBusy)
	if err != nil || me.Status != StatusBusy || pub.count() != 2 {
		t.Fatalf("busy = %+v %v", me, err)
	}
	var events int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_name = 'agent.presence_changed'`, f.a.OrganizationID).Scan(&events)
	if events != 2 {
		t.Fatalf("outbox events = %d, want 2 (online, busy)", events)
	}

	// Idle: back-date the heartbeat, sweep, then a heartbeat restores busy.
	if _, err := f.pool.Exec(ctx, `UPDATE agent_presence SET last_seen_at = NOW() - INTERVAL '10 minutes' WHERE user_id = $1`, f.a.UserID); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.List(ctx, f.a.OrganizationID); list[0].Status != StatusOffline {
		t.Fatalf("stale heartbeat must read offline: %+v", list[0])
	}
	n, err := svc.SweepIdle(ctx)
	if err != nil || n < 1 || pub.count() != 3 {
		t.Fatalf("sweep n=%d err=%v pubs=%d", n, err, pub.count())
	}
	if n, _ := svc.SweepIdle(ctx); n != 0 {
		t.Fatalf("second sweep changed %d", n)
	}
	me, err = svc.Heartbeat(ctx, f.a)
	if err != nil || me.Status != StatusBusy {
		t.Fatalf("heartbeat after idle must restore the picked status: %+v %v", me, err)
	}
	// Manual offline sticks through heartbeats.
	if _, err := svc.SetStatus(ctx, f.a, StatusOffline); err != nil {
		t.Fatal(err)
	}
	if me, _ := svc.Heartbeat(ctx, f.a); me.Status != StatusOffline {
		t.Fatalf("manual offline overridden: %+v", me)
	}
}

func TestTyping(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pub := &recPub{}
	svc := New(f.pool, f.q, pub, &onceLimiter{seen: map[string]bool{}}, nil)
	if err := svc.Typing(ctx, f.a, "Ece", f.conv, "start"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Typing(ctx, f.a, "Ece", f.conv, "start"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second start within the window: %v", err)
	}
	if err := svc.Typing(ctx, f.a, "Ece", f.conv, "stop"); err != nil {
		t.Fatal(err)
	}
	if pub.count() != 2 || pub.msgs[0].channel != realtime.ConversationChannel(f.conv) || pub.msgs[0].msg.Data["state"] != "start" {
		t.Fatalf("typing publications = %+v", pub.msgs)
	}
	if err := svc.Typing(ctx, f.a, "Ece", uuid.New(), "start"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign conversation: %v", err)
	}
	if err := svc.Typing(ctx, f.a, "Ece", f.conv, "dance"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad state: %v", err)
	}
}

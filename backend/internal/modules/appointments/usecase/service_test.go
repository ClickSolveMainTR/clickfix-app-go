package usecase_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type allowAll struct{}

func (allowAll) Require(context.Context, int64, string) error { return nil }
func (allowAll) CheckLimit(context.Context, int64, string, int64, int64) (entitlement.Decision, error) {
	return entitlement.Decision{Allowed: true}, nil
}
func (allowAll) RequireWritable(context.Context, int64) error { return nil }

type captured struct {
	mu   sync.Mutex
	sent []notif.EnqueueInput
}

func (c *captured) Enqueue(_ context.Context, in notif.EnqueueInput) ([]notif.Notification, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, in)
	return nil, nil
}

func (c *captured) count(source string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, in := range c.sent {
		if in.SourceEvent == source {
			n++
		}
	}
	return n
}

type fixture struct {
	pool   *pgxpool.Pool
	svc    *usecase.Service
	notes  *captured
	org    int64
	slug   string
	hosts  []uuid.UUID
	hostID []int64
	now    time.Time
	loc    *time.Location
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for appointment tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	f := &fixture{pool: pool, notes: &captured{}, slug: "appt-" + uuid.NewString()[:12]}
	f.loc, _ = time.LoadLocation("Europe/Istanbul")
	// Monday 2 November 2026, 08:00 Istanbul.
	f.now = time.Date(2026, 11, 2, 8, 0, 0, 0, f.loc)
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Randevu test') RETURNING id", f.slug).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM appointments WHERE organization_id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
	})
	var role int64
	if err := pool.QueryRow(ctx, "SELECT id FROM roles WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'agent'").Scan(&role); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		var id int64
		var u uuid.UUID
		email := uuid.NewString()[:8] + "@example.com"
		if err := pool.QueryRow(ctx, "INSERT INTO users (email, name, surname, password_hash) VALUES ($1, 'Agent', $2, 'x') RETURNING id, uuid",
			email, []string{"Bir", "Iki"}[i]).Scan(&id, &u); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id) })
		if _, err := pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) VALUES ($1, $2, $3)", f.org, id, role); err != nil {
			t.Fatal(err)
		}
		f.hosts = append(f.hosts, u)
		f.hostID = append(f.hostID, id)
	}
	f.svc = usecase.New(usecase.Deps{Pool: pool, Queries: q, Entitlements: allowAll{}, Notifier: f.notes, Secret: "s3cret", BaseURL: "https://app.example.com"})
	f.svc.SetClock(func() time.Time { return f.now })
	return f
}

func (f *fixture) demoType(t *testing.T, mode string, maxPerDay int32) usecase.Type {
	t.Helper()
	typ, err := f.svc.CreateType(context.Background(), f.org, usecase.TypeInput{
		Name: "Ürün demosu", DurationOptions: []int32{30, 60}, DefaultDuration: 30, SlotInterval: 30,
		BufferAfter: 15, LocationKind: usecase.LocationVideo, HostMode: mode, HostUUIDs: f.hosts,
		BookingWindowDays: 30, MinNoticeMinutes: 60, MaxPerDay: maxPerDay, AllowCancel: true, AllowReschedule: true,
		IsPublic: true, IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func TestPublicBookingRoundRobinCollisionAndLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	typ := f.demoType(t, usecase.HostRoundRobin, 0)
	if typ.Slug != "urun-demosu" || typ.BookingURL != "https://app.example.com/book/"+f.slug+"/urun-demosu" {
		t.Fatalf("type %+v", typ)
	}

	slots, err := f.svc.PublicSlots(ctx, f.slug, typ.Slug, "2026-11-02", "2026-11-03", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Default week 09:00-18:00 and one hour notice from 08:00: first 09:00,
	// last 17:30.
	if len(slots) == 0 || slots[0].StartsAt.In(f.loc).Format("15:04") != "09:00" || slots[len(slots)-1].StartsAt.In(f.loc).Format("15:04") != "17:30" {
		t.Fatalf("slots %v", slots)
	}
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, f.loc)
	book := func(email string) (usecase.PublicManage, error) {
		return f.svc.PublicBook(ctx, f.slug, typ.Slug, usecase.PublicBookInput{StartsAt: at, Name: "Ayşe", Email: email})
	}
	first, err := book("ayse@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := book("mehmet@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first.HostName == second.HostName {
		t.Fatalf("round robin gave both bookings to %s", first.HostName)
	}
	if _, err := book("zeynep@example.com"); !errors.Is(err, usecase.ErrSlotTaken) {
		t.Fatalf("third booking at a full slot: %v", err)
	}
	// 10:30 is blocked by the 15 minute after-buffer of both 10:00 bookings.
	slots, _ = f.svc.PublicSlots(ctx, f.slug, typ.Slug, "2026-11-02", "2026-11-03", 0)
	for _, s := range slots {
		if c := s.StartsAt.In(f.loc).Format("15:04"); c == "10:00" || c == "10:30" {
			t.Fatalf("slot %s should be taken", c)
		}
	}
	if f.notes.count("appointment.booked") < 2 {
		t.Fatalf("no confirmation e-mails: %+v", f.notes.sent)
	}

	// Invitee reschedules with the manage token; old links stop working.
	moved, err := f.svc.PublicReschedule(ctx, first.Token, usecase.PublicRescheduleInput{StartsAt: time.Date(2026, 11, 3, 14, 0, 0, 0, f.loc)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PublicManageGet(ctx, first.Token); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("stale token still works: %v", err)
	}
	cancelled, err := f.svc.PublicCancel(ctx, moved.Token, "plan değişti")
	if err != nil || cancelled.Status != usecase.StatusCancelled || cancelled.CanCancel {
		t.Fatalf("cancel %+v err=%v", cancelled, err)
	}
	ics, err := f.svc.ICSFor(ctx, f.org, moved.UUID)
	if err != nil || len(ics) == 0 {
		t.Fatalf("ics err=%v", err)
	}

	// Agent lifecycle on the second booking.
	if _, err := f.svc.SetStatus(ctx, f.org, f.hostID[0], second.UUID, usecase.StatusInput{Status: usecase.StatusCompleted}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("completed before start: %v", err)
	}
	f.now = time.Date(2026, 11, 2, 11, 0, 0, 0, f.loc)
	done, err := f.svc.SetStatus(ctx, f.org, f.hostID[0], second.UUID, usecase.StatusInput{Status: usecase.StatusNoShow})
	if err != nil || done.Status != usecase.StatusNoShow {
		t.Fatalf("no-show %+v err=%v", done, err)
	}
	if _, err := f.svc.SetStatus(ctx, f.org, f.hostID[0], second.UUID, usecase.StatusInput{Status: usecase.StatusConfirmed}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("no-show back to confirmed: %v", err)
	}
}

func TestInternalBookingTimeOffMaxPerDayAndReminders(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	typ := f.demoType(t, usecase.HostAgents, 1)
	// Host 0 is off on Tuesday morning.
	if _, err := f.pool.Exec(ctx, `INSERT INTO agent_time_off (organization_id, user_id, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, 'approved')`, f.org, f.hostID[0],
		time.Date(2026, 11, 3, 9, 0, 0, 0, f.loc), time.Date(2026, 11, 3, 12, 0, 0, 0, f.loc)); err != nil {
		t.Fatal(err)
	}
	host := f.hosts[0]
	if _, err := f.svc.Book(ctx, f.org, f.hostID[1], false, usecase.BookInput{
		TypeUUID: typ.UUID, StartsAt: time.Date(2026, 11, 3, 10, 0, 0, 0, f.loc), HostUUID: &host, InviteeName: "Ali",
	}, nil); !errors.Is(err, usecase.ErrSlotTaken) {
		t.Fatalf("booking during time off: %v", err)
	}
	a, err := f.svc.Book(ctx, f.org, f.hostID[1], false, usecase.BookInput{
		TypeUUID: typ.UUID, StartsAt: time.Date(2026, 11, 3, 13, 0, 0, 0, f.loc), Duration: 60, HostUUID: &host,
		InviteeName: "Ali", InviteeEmail: "ali@example.com",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Host == nil || a.Host.UUID != host || a.EndsAt.Sub(a.StartsAt) != time.Hour {
		t.Fatalf("booking %+v", a)
	}
	// max_per_day=1 hides the host's other Tuesday slots from the public.
	slots, err := f.svc.PublicSlots(ctx, f.slug, typ.Slug, "2026-11-03", "2026-11-04", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 {
		t.Fatal("the second host should still be free")
	}
	// Outside working hours needs the override (manage permission).
	late := time.Date(2026, 11, 3, 20, 0, 0, 0, f.loc)
	if _, err := f.svc.Reschedule(ctx, f.org, f.hostID[1], false, a.UUID, usecase.RescheduleInput{StartsAt: late}); !errors.Is(err, usecase.ErrSlotTaken) {
		t.Fatalf("reschedule after hours without override: %v", err)
	}
	if _, err := f.svc.Reschedule(ctx, f.org, f.hostID[1], false, a.UUID, usecase.RescheduleInput{StartsAt: late, OverrideAvailability: true}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("override without manage: %v", err)
	}
	moved, err := f.svc.Reschedule(ctx, f.org, f.hostID[1], true, a.UUID, usecase.RescheduleInput{StartsAt: late, OverrideAvailability: true})
	if err != nil || !moved.StartsAt.Equal(late) {
		t.Fatalf("override reschedule %+v err=%v", moved, err)
	}

	// Reminders: 24h before (default offsets 1440, 60) goes out once.
	f.now = late.Add(-24 * time.Hour).Add(time.Minute)
	n, err := f.svc.SweepReminders(ctx)
	if err != nil || n < 1 {
		t.Fatalf("sweep sent %d err=%v", n, err)
	}
	if again, _ := f.svc.SweepReminders(ctx); again != 0 {
		t.Fatalf("reminder sent twice (%d)", again)
	}
	if f.notes.count("appointment.reminder") == 0 {
		t.Fatal("no reminder notification")
	}

	list, err := f.svc.List(ctx, f.org, usecase.ListFilter{HostUUIDs: []uuid.UUID{host}})
	if err != nil || len(list) != 1 {
		t.Fatalf("list by host %d err=%v", len(list), err)
	}
}

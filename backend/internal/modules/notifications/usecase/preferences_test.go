package usecase

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/content"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQuietUntilWrapsMidnight(t *testing.T) {
	t.Parallel()
	q := model.QuietHours{Enabled: true, Start: "22:00", End: "08:00", Timezone: "Europe/Istanbul"}
	ist, _ := time.LoadLocation("Europe/Istanbul")
	inside := time.Date(2026, 10, 28, 23, 30, 0, 0, ist)
	end, ok := QuietUntil(q, inside)
	if !ok || !end.Equal(time.Date(2026, 10, 29, 8, 0, 0, 0, ist).UTC()) {
		t.Fatalf("23:30 -> %v %v", end, ok)
	}
	early := time.Date(2026, 10, 29, 7, 0, 0, 0, ist)
	if end, ok := QuietUntil(q, early); !ok || !end.Equal(time.Date(2026, 10, 29, 8, 0, 0, 0, ist).UTC()) {
		t.Fatalf("07:00 -> %v %v", end, ok)
	}
	if _, ok := QuietUntil(q, time.Date(2026, 10, 29, 12, 0, 0, 0, ist)); ok {
		t.Fatal("noon must not be quiet")
	}
	q.Enabled = false
	if _, ok := QuietUntil(q, inside); ok {
		t.Fatal("disabled quiet hours")
	}
}

func TestPlanChannel(t *testing.T) {
	t.Parallel()
	ist, _ := time.LoadLocation("Europe/Istanbul")
	day := time.Date(2026, 10, 28, 12, 0, 0, 0, ist)
	night := time.Date(2026, 10, 28, 23, 0, 0, 0, ist)
	p := defaultPreferences()

	if pl := planChannel(p, events.MessageReceived, model.ChannelEmail, model.PriorityNormal, day); !pl.skip {
		t.Fatal("message.received e-mail is off by default")
	}
	if pl := planChannel(p, events.MessageReceived, model.ChannelInapp, model.PriorityNormal, day); pl.skip || pl.noPush {
		t.Fatalf("message.received in-app with push by default: %+v", pl)
	}
	p.EventChannels[events.MessageReceived] = map[string]bool{"inapp": false, "push": true}
	if pl := planChannel(p, events.MessageReceived, model.ChannelInapp, model.PriorityNormal, day); pl.skip || !pl.asPush {
		t.Fatalf("in-app off + push on must become a push row: %+v", pl)
	}
	p.EventChannels[events.MessageReceived] = map[string]bool{"inapp": false, "push": false}
	if pl := planChannel(p, events.MessageReceived, model.ChannelInapp, model.PriorityNormal, day); !pl.skip {
		t.Fatal("all off must skip")
	}
	// Locked in-app stays on even when stored off.
	p.EventChannels[events.ConversationAssigned] = map[string]bool{"inapp": false}
	if pl := planChannel(p, events.ConversationAssigned, model.ChannelInapp, model.PriorityHigh, day); pl.skip {
		t.Fatal("locked in-app must stay on")
	}

	p.QuietHours = model.QuietHours{Enabled: true, Start: "22:00", End: "08:00", Timezone: "Europe/Istanbul"}
	if pl := planChannel(p, events.ConversationAssigned, model.ChannelInapp, model.PriorityHigh, night); !pl.noPush {
		t.Fatal("quiet hours mute push")
	}
	if pl := planChannel(p, events.ConversationAssigned, model.ChannelEmail, model.PriorityHigh, night); pl.scheduledAt == nil {
		t.Fatal("quiet hours hold e-mail")
	}
	if pl := planChannel(p, events.SLABreached, model.ChannelInapp, model.PriorityCritical, night); pl.noPush {
		t.Fatal("critical ignores quiet hours")
	}
	p.EmailDigest = model.DigestDaily
	if pl := planChannel(p, events.ConversationAssigned, model.ChannelEmail, model.PriorityHigh, day); !pl.digest {
		t.Fatal("digest holds non-critical e-mail")
	}
	if pl := planChannel(p, events.SLABreached, model.ChannelEmail, model.PriorityCritical, day); pl.digest {
		t.Fatal("critical e-mail is never digested")
	}
	// Sources outside the catalog honor quiet hours too (CF-157).
	if pl := planChannel(p, "status.changed", model.ChannelEmail, model.PriorityNormal, night); pl.scheduledAt == nil || pl.skip {
		t.Fatalf("non-catalog e-mail is held in quiet hours: %+v", pl)
	}
	if pl := planChannel(p, "status.changed", model.ChannelInapp, model.PriorityNormal, night); !pl.noPush || pl.skip {
		t.Fatalf("non-catalog in-app keeps the row but mutes push: %+v", pl)
	}
	if pl := planChannel(p, "status.changed", model.ChannelPush, model.PriorityNormal, night); !pl.skip {
		t.Fatal("non-catalog push is dropped in quiet hours")
	}
	if pl := planChannel(p, "status.changed", model.ChannelPush, model.PriorityCritical, night); pl != (channelPlan{}) {
		t.Fatal("critical non-catalog push is not held")
	}
	if pl := planChannel(p, "status.changed", model.ChannelEmail, model.PriorityNormal, day); pl != (channelPlan{}) {
		t.Fatal("outside quiet hours nothing changes")
	}
}

func TestDigestDue(t *testing.T) {
	t.Parallel()
	ist, _ := time.LoadLocation("Europe/Istanbul")
	p := defaultPreferences()
	p.EmailDigest = model.DigestDaily
	p.DigestHour = 9
	at := func(h int) time.Time { return time.Date(2026, 10, 28, h, 5, 0, 0, ist) }
	if digestDue(p, nil, at(8)) {
		t.Fatal("before the digest hour")
	}
	if !digestDue(p, nil, at(9)) {
		t.Fatal("at the digest hour")
	}
	sent := at(9)
	if digestDue(p, &sent, at(15)) {
		t.Fatal("already sent today")
	}
	p.EmailDigest = model.DigestHourly
	last := at(9)
	if digestDue(p, &last, at(9).Add(30*time.Minute)) || !digestDue(p, &last, at(10)) {
		t.Fatal("hourly window")
	}
}

// TestNotifyEventEndToEnd runs preferences, the matrix and the digest
// against Postgres with inline delivery.
func TestNotifyEventEndToEnd(t *testing.T) {
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
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status, locale) VALUES ($1, 'x', 'Ayşe', 'Agent', 'active', 'tr') RETURNING id`,
		"np-"+uuid.NewString()[:8]+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}, providers.NoopProvider{Name: "email", Log: slog.Default()}}, slog.Default())

	if _, err := svc.UpdatePreferences(ctx, userID, model.PreferencesUpdate{
		EventChannels: map[string]map[string]bool{events.ConversationAssigned: {"inapp": false}},
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("locked channel must be refused: %v", err)
	}
	digest := model.DigestHourly
	prefs, err := svc.UpdatePreferences(ctx, userID, model.PreferencesUpdate{
		EventChannels: map[string]map[string]bool{events.MessageReceived: {"email": true}},
		EmailDigest:   &digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !prefs.EmailEnabled || prefs.EmailDigest != "hourly" || !prefs.EventChannels[events.MessageReceived]["email"] {
		t.Fatalf("prefs = %+v", prefs)
	}
	// Master switch update keeps the matrix (merge semantics).
	if p, err := svc.UpsertPreferences(ctx, userID, model.Preferences{EmailEnabled: true, InappEnabled: true, RealtimeEnabled: true, PushEnabled: true}); err != nil || p.EmailDigest != "hourly" {
		t.Fatalf("upsert lost settings: %+v %v", p, err)
	}

	if err := svc.NotifyEvent(ctx, NotifyInput{
		Event: events.MessageReceived, UserIDs: []int64{userID, userID},
		Vars: map[string]string{"number": "42", "preview": "Merhaba", "customer": "Deniz Kaya", "channel": "whatsapp"}, ActionURL: "/inbox?c=x",
	}); err != nil {
		t.Fatal(err)
	}
	var inapp, email int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE channel = 'inapp'), count(*) FILTER (WHERE channel = 'email') FROM notifications WHERE user_id = $1`, userID).Scan(&inapp, &email); err != nil {
		t.Fatal(err)
	}
	if inapp != 1 || email != 0 {
		t.Fatalf("inapp=%d email=%d (e-mail must be held for the digest, duplicates collapsed)", inapp, email)
	}
	var title string
	if err := pool.QueryRow(ctx, `SELECT title FROM notifications WHERE user_id = $1 AND channel = 'inapp'`, userID).Scan(&title); err != nil || title != "Deniz Kaya yeni mesaj gönderdi · #42" {
		t.Fatalf("tr content not rendered: %q %v", title, err)
	}
	sent, err := svc.ProcessDigests(ctx)
	if err != nil || sent < 1 {
		t.Fatalf("digests sent=%d err=%v", sent, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND channel = 'email' AND template_code = 'notifications.digest'`, userID).Scan(&email); err != nil || email != 1 {
		t.Fatalf("digest e-mails = %d %v", email, err)
	}
	var pending int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM notification_digest_items WHERE user_id = $1 AND digested_at IS NULL`, userID).Scan(&pending)
	if pending != 0 {
		t.Fatalf("pending digest items = %d", pending)
	}
	// Second sweep within the hour sends nothing.
	if n, _ := svc.ProcessDigests(ctx); n != 0 {
		t.Fatalf("second sweep sent %d", n)
	}
}

// TestCatalogEventsHaveContent: every event a member can receive renders
// the enriched tr/en text (CF-157). ticket.commented has no producer yet.
func TestCatalogEventsHaveContent(t *testing.T) {
	t.Parallel()
	for _, et := range EventTypes() {
		if et.Key == events.TicketCommented {
			continue
		}
		if !content.Has(et.Key) {
			t.Errorf("catalog event %s has no content builder", et.Key)
		}
	}
}

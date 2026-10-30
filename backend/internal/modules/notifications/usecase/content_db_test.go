package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEnrichedContentPerChannel checks, against Postgres, that one catalog
// event becomes an in-app row with structured content and lock-screen push
// text, and an e-mail with detail rows and an absolute link, in the user's
// language; and that a redelivered event creates nothing new (CF-157).
func TestEnrichedContentPerChannel(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status, locale) VALUES ($1, 'x', 'Jane', 'Agent', 'active', 'en') RETURNING id`,
		"nc-"+uuid.NewString()[:8]+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}, providers.NoopProvider{Name: "email", Log: slog.Default()}}, slog.Default()).
		WithAppURL("https://app.example.test/")

	conv := uuid.NewString()
	in := NotifyInput{
		Event: events.ConversationAssigned, UserIDs: []int64{userID}, ActionURL: "/inbox?c=" + conv, DedupeKey: "ev-" + conv,
		Vars: map[string]string{
			"number": "42", "customer": "Deniz Kaya", "channel": "whatsapp", "actor": "Ali Veli",
			"preview": "<p>Siparişim <b>gelmedi</b></p>", "url": "/inbox?c=" + conv,
		},
	}
	for range 2 { // the second call is a redelivery
		if err := svc.NotifyEvent(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := pool.Query(ctx, `SELECT channel, title, body, payload, action_url FROM notifications WHERE user_id = $1 ORDER BY channel`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string][3]string{}
	payloads := map[string]map[string]any{}
	for rows.Next() {
		var ch, title, body string
		var raw []byte
		var action *string
		if err := rows.Scan(&ch, &title, &body, &raw, &action); err != nil {
			t.Fatal(err)
		}
		if _, dup := got[ch]; dup {
			t.Fatalf("duplicate %s row after redelivery", ch)
		}
		a := ""
		if action != nil {
			a = *action
		}
		got[ch] = [3]string{title, body, a}
		var p map[string]any
		_ = json.Unmarshal(raw, &p)
		payloads[ch] = p
	}
	if len(got) != 2 {
		t.Fatalf("rows = %v", got)
	}
	email := got[model.ChannelEmail]
	if email[0] != "Conversation assigned to you · #42 · Deniz Kaya" {
		t.Fatalf("email subject %q", email[0])
	}
	for _, want := range []string{
		"Ali Veli assigned you the conversation #42 with Deniz Kaya.",
		"“Siparişim gelmedi”", "Customer: Deniz Kaya", "Channel: WhatsApp",
	} {
		if !strings.Contains(email[1], want) {
			t.Errorf("email body misses %q:\n%s", want, email[1])
		}
	}
	if strings.Contains(email[1], "<b>") {
		t.Fatal("HTML leaked into the e-mail text")
	}
	// The branded layout renders the primary action as its button: it is the
	// row's action_url with its own label, not a line of the body.
	if email[2] != "/inbox?c="+conv || payloads[model.ChannelEmail]["action_label"] != "Open conversation" {
		t.Fatalf("email action %q label %v", email[2], payloads[model.ChannelEmail]["action_label"])
	}
	if strings.Contains(email[1], "Open conversation:") {
		t.Fatal("primary action repeated in the body")
	}
	inapp := payloads[model.ChannelInapp]
	push, _ := inapp["push"].(map[string]any)
	if push["title"] != "Conversation assigned" || push["body"] != "WhatsApp · #42" {
		t.Fatalf("push text %v", push)
	}
	if inapp["category"] != "conversation" || inapp["content"] == nil {
		t.Fatalf("payload %v", inapp)
	}
	if got[model.ChannelInapp][2] != "/inbox?c="+conv {
		t.Fatalf("in-app action %q", got[model.ChannelInapp][2])
	}
	// The web push of the row carries the same safe text, never the name.
	var row db.Notification
	if row, err = q.GetNotificationByUUID(ctx, mustUUID(t, pool, userID)); err != nil {
		t.Fatal(err)
	}
	title, body, data := WebPushContent(row, "en")
	if title != "Conversation assigned" || strings.Contains(body, "Deniz") || data["action_url"] != "/inbox?c="+conv {
		t.Fatalf("web push %q %q %v", title, body, data)
	}
}

func mustUUID(t *testing.T, pool *pgxpool.Pool, userID int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT uuid FROM notifications WHERE user_id = $1 AND channel = 'inapp'`, userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

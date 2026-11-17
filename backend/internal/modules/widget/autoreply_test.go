package widget_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/autoreply"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

// fixedHours answers the organization's working-hours state from a flag.
type fixedHours struct{ open bool }

func (h *fixedHours) HoursState(context.Context, int64, time.Time) (businesshours.State, error) {
	st := businesshours.State{Open: h.open, Scheduled: true}
	if !h.open {
		next := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
		st.NextOpenAt = &next
	}
	return st, nil
}

// fixedETA is the SLA first-response target.
type fixedETA struct{ d *time.Duration }

func (e fixedETA) FirstResponseTarget(context.Context, int64, int64) (*time.Duration, error) {
	return e.d, nil
}

// CF-198: a widget visitor never waits in silence. The first message of a
// conversation inside working hours gets the acknowledgement with the SLA
// estimate, a message outside hours gets the out-of-hours text (once per
// closed window), and a handoff tells the visitor once.
func TestAutoRepliesReachTheVisitor(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	hours := &fixedHours{open: true}
	two := 2 * time.Hour
	auto := autoreply.New(e.q, e.ib.Conversations, hours, fixedETA{&two}, nil)
	e.svc.WithHours(hours)

	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	token := str(boot.Data, "visitor", "token")
	if online, _ := boot.Data["widget"].(map[string]any)["online"].(bool); !online {
		t.Fatal("open hours: widget must be online")
	}
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "Merhaba", "client_id": uuid.New(), "name": "Ada"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", created.Code, created.Err)
	}
	convUUID := uuid.MustParse(str(created.Data, "conversation_uuid"))
	conv, err := e.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: e.org, Uuid: convUUID})
	if err != nil {
		t.Fatal(err)
	}
	lastInbound := func() uuid.UUID {
		var id uuid.UUID
		if err := e.pool.QueryRow(ctx, "SELECT uuid FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND direction = 'in' ORDER BY id DESC LIMIT 1", e.org, conv.ID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	botMessages := func() []map[string]any {
		msgs := e.call(http.MethodGet, "/conversations/"+convUUID.String()+"/messages", token, siteOrigin, nil)
		items, _ := msgs.Data["items"].([]any)
		out := []map[string]any{}
		for _, it := range items {
			m := it.(map[string]any)
			if m["direction"] == "out" {
				out = append(out, m)
			}
		}
		return out
	}

	// 1. Inside hours: the first message is acknowledged with the estimate.
	// The consumer handles the messages.received event itself, in one call
	// and without any delay of its own; the outbox relays events every
	// 500 ms, so the whole path stays far below the 10 s the product
	// promises. Its retry budget and timeout stay small so a stuck reply
	// never piles up.
	consumer := auto.Consumer()
	if consumer.Timeout > 30*time.Second || consumer.MaxRetry > 3 {
		t.Fatalf("auto-reply consumer must stay snappy: timeout=%s retries=%d", consumer.Timeout, consumer.MaxRetry)
	}
	first := lastInbound()
	ev := events.New(events.MessagesReceived).WithTenant(e.org).WithPayload(map[string]any{"message_uuid": first.String()})
	started := time.Now()
	if err := consumer.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Fatalf("acknowledgement took %s, the product promises 10 s", took)
	}
	// A replay of the same event (at-least-once delivery) adds nothing.
	if err := consumer.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	bots := botMessages()
	if len(bots) != 1 || bots[0]["content"] != "Mesajınızı aldık, en kısa sürede dönüş yapacağız. Tahmini yanıt süresi: 2 saat." {
		t.Fatalf("ack: %v", bots)
	}
	// The acknowledgement is not an agent reply: the SLA first response
	// stays open.
	if c, _ := e.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: e.org, ID: conv.ID}); c.FirstResponseAt.Valid {
		t.Fatal("bot acknowledgement must not stamp first_response_at")
	}
	// A second message inside hours stays quiet.
	e.call(http.MethodPost, "/conversations/"+convUUID.String()+"/messages", token, siteOrigin, map[string]any{"text": "Hâlâ bekliyorum", "client_id": uuid.New()})
	if err := auto.OnMessage(ctx, e.org, lastInbound()); err != nil {
		t.Fatal(err)
	}
	if len(botMessages()) != 1 {
		t.Fatal("second message inside hours must not be acknowledged again")
	}

	// 2. Outside hours: the widget goes offline and the message gets the
	// single-source out-of-hours text, once per closed window.
	hours.open = false
	if b := e.call(http.MethodPost, "/bootstrap", token, siteOrigin, nil); b.Data["widget"].(map[string]any)["online"] != false || str(b.Data, "widget", "reply", "next_open_at") == "" {
		t.Fatalf("closed hours: widget must be offline with a next opening: %v", b.Data["widget"])
	}
	e.call(http.MethodPost, "/conversations/"+convUUID.String()+"/messages", token, siteOrigin, map[string]any{"text": "Akşam yazıyorum", "client_id": uuid.New()})
	if err := auto.OnMessage(ctx, e.org, lastInbound()); err != nil {
		t.Fatal(err)
	}
	bots = botMessages()
	if len(bots) != 2 || bots[1]["content"] != autoreply.Defaults().Offline.Text["tr"] {
		t.Fatalf("offline: %v", bots)
	}
	e.call(http.MethodPost, "/conversations/"+convUUID.String()+"/messages", token, siteOrigin, map[string]any{"text": "Hâlâ akşam", "client_id": uuid.New()})
	if err := auto.OnMessage(ctx, e.org, lastInbound()); err != nil {
		t.Fatal(err)
	}
	if len(botMessages()) != 2 {
		t.Fatal("the same closed window gets one notice")
	}

	// 3. Handoff: the visitor is told once that a person takes over, in the
	// organization's own words.
	custom := autoreply.Defaults()
	custom.Handoff.Text["tr"] = "Temsilcimiz devralıyor."
	if _, err := auto.Put(ctx, e.org, custom); err != nil {
		t.Fatal(err)
	}
	if sent, err := auto.NotifyHandoff(ctx, e.org, conv.ID); err != nil || !sent {
		t.Fatalf("first handoff notice: sent=%v err=%v", sent, err)
	}
	if sent, err := auto.NotifyHandoff(ctx, e.org, conv.ID); err != nil || sent {
		t.Fatalf("second handoff notice must not be sent: sent=%v err=%v", sent, err)
	}
	bots = botMessages()
	if len(bots) != 3 || bots[2]["content"] != "Temsilcimiz devralıyor." {
		t.Fatalf("handoff: %v", bots)
	}
	// A disabled kind sends nothing.
	custom.Ack.Enabled = false
	if _, err := auto.Put(ctx, e.org, custom); err != nil {
		t.Fatal(err)
	}
	hours.open = true
	other := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "İkinci konu", "client_id": uuid.New()})
	otherConv, _ := e.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: e.org, Uuid: uuid.MustParse(str(other.Data, "conversation_uuid"))})
	var last uuid.UUID
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM messages WHERE organization_id = $1 AND conversation_id = $2 ORDER BY id DESC LIMIT 1", e.org, otherConv.ID).Scan(&last)
	if err := auto.OnMessage(ctx, e.org, last); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND sender_type = 'bot'", e.org, otherConv.ID).Scan(&n)
	if n != 0 {
		t.Fatal("disabled acknowledgement must send nothing")
	}
}

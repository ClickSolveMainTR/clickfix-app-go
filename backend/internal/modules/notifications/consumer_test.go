package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeLookup struct{ assignee *int64 }

func (f fakeLookup) Conversation(context.Context, int64, int64) (ConversationInfo, error) {
	return ConversationInfo{
		UUID: uuid.MustParse("6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11"), Number: 42, Subject: "Fatura", AssigneeUserID: f.assignee,
		Customer: "Deniz Kaya", ChannelType: "whatsapp", LastMessage: "Son mesaj", Timezone: "Europe/Istanbul",
	}, nil
}

func (fakeLookup) UserName(context.Context, int64) string { return "Ali Veli" }

func TestNotifyInputMapsEvents(t *testing.T) {
	assignee := int64(7)
	lookup := fakeLookup{assignee: &assignee}
	ctx := context.Background()

	ev, _ := events.NewDomain(events.MessageReceived, 1, events.MessagePayload{
		ConversationRef: events.ConversationRef{ConversationID: 9}, Direction: "inbound", SenderType: "contact", Preview: "Selam",
	})
	in, err := notifyInput(ctx, lookup, ev)
	if err != nil || in == nil || len(in.UserIDs) != 1 || in.UserIDs[0] != 7 || in.Vars["number"] != "42" || in.Vars["preview"] != "Selam" {
		t.Fatalf("message.received -> %+v %v", in, err)
	}
	if in.ActionURL != "/inbox?c=6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11" {
		t.Fatalf("action url %q", in.ActionURL)
	}

	private, _ := events.NewDomain(events.MessageReceived, 1, events.MessagePayload{ConversationRef: events.ConversationRef{ConversationID: 9}, Private: true})
	if in, _ := notifyInput(ctx, lookup, private); in != nil {
		t.Fatal("private notes do not notify as messages")
	}

	mention, _ := events.NewDomain(events.ConversationMentioned, 1, events.MentionPayload{
		ConversationRef: events.ConversationRef{ConversationID: 9}, AuthorUserID: 3, MentionedUserIDs: []int64{3, 4, 5},
	})
	in, err = notifyInput(ctx, lookup, mention)
	if err != nil || len(in.UserIDs) != 2 || in.Vars["actor"] != "Ali Veli" {
		t.Fatalf("mention -> %+v %v", in, err)
	}

	unassigned := fakeLookup{}
	ev2, _ := events.NewDomain(events.SLABreached, 1, events.SLAPayload{TargetType: "conversation", TargetID: 9, Metric: "first_response"})
	in, err = notifyInput(ctx, unassigned, ev2)
	if err != nil || in == nil || len(in.UserIDs) != 0 || in.Vars["target"] != "#42" {
		t.Fatalf("sla without assignee -> %+v %v", in, err)
	}

	noOrg := events.New(events.MessageReceived)
	if in, _ := notifyInput(ctx, lookup, noOrg); in != nil {
		t.Fatal("platform events without organization are ignored")
	}
}

// TestInboxEventsNotify: the inbox publishes plural event names; the
// consumer turns them into the catalog events with the conversation facts
// (CF-157).
func TestInboxEventsNotify(t *testing.T) {
	assignee := int64(7)
	lookup := fakeLookup{assignee: &assignee}
	ctx := context.Background()
	convID := int64(9)
	at := time.Date(2026, 11, 2, 10, 3, 0, 0, time.UTC)

	msg := events.New(events.MessagesReceived).WithTenant(1).WithEntity("conversation", &convID, nil).WithPayload(map[string]any{"channel_type": "whatsapp"})
	msg.OccurredAt = at
	in, err := notifyInput(ctx, lookup, msg)
	if err != nil || in == nil || in.Event != events.MessageReceived || len(in.UserIDs) != 1 || in.UserIDs[0] != 7 {
		t.Fatalf("messages.received -> %+v %v", in, err)
	}
	if in.Vars["customer"] != "Deniz Kaya" || in.Vars["channel"] != "whatsapp" || in.Vars["preview"] != "Son mesaj" || in.Vars["url"] == "" {
		t.Fatalf("vars %v", in.Vars)
	}
	// A burst of messages in the same window shares one dedupe key.
	later := msg
	later.OccurredAt = at.Add(time.Minute)
	in2, _ := notifyInput(ctx, lookup, later)
	if in.DedupeKey == "" || in.DedupeKey != in2.DedupeKey {
		t.Fatalf("dedupe keys %q %q", in.DedupeKey, in2.DedupeKey)
	}
	next := msg
	next.OccurredAt = at.Add(10 * time.Minute)
	if in3, _ := notifyInput(ctx, lookup, next); in3.DedupeKey == in.DedupeKey {
		t.Fatal("a later window notifies again")
	}

	survey := events.New(events.SurveyResponded).WithTenant(1).WithEntity("conversation", &convID, nil).WithPayload(map[string]any{"kind": "csat", "score": float64(2)})
	in, err = notifyInput(ctx, lookup, survey)
	if err != nil || in == nil || in.Event != events.CSATSubmitted || in.Vars["rating"] != "2" || in.UserIDs[0] != 7 {
		t.Fatalf("surveys.responded -> %+v %v", in, err)
	}
	nps := events.New(events.SurveyResponded).WithTenant(1).WithEntity("conversation", &convID, nil).WithPayload(map[string]any{"kind": "nps", "score": float64(9)})
	if in, _ := notifyInput(ctx, lookup, nps); in != nil {
		t.Fatal("only CSAT notifies")
	}

	due := at.Add(30 * time.Minute)
	sla, _ := events.NewDomain(events.SLAWarning, 1, events.SLAPayload{TargetType: "conversation", TargetID: 9, Metric: "first_response", DueAt: due})
	in, _ = notifyInput(ctx, lookup, sla)
	if in.Vars["due"] != due.Format(time.RFC3339) || in.Vars["customer"] != "Deniz Kaya" || in.Vars["metric"] != "first_response" {
		t.Fatalf("sla vars %v", in.Vars)
	}
}

func TestConsumerListensToInboxNames(t *testing.T) {
	c := EventConsumer(nil, fakeLookup{})
	for _, name := range []string{events.MessagesReceived, events.SurveyResponded, events.SLAWarning} {
		found := false
		for _, e := range c.Events {
			found = found || e == name
		}
		if !found {
			t.Fatalf("consumer does not listen to %s", name)
		}
	}
	for _, e := range c.Events {
		if e == "exports.ready" || e == "appointment.booked" {
			t.Fatalf("consumer must not subscribe to %s (enqueued directly)", e)
		}
	}
}

func TestNearestDue(t *testing.T) {
	a := pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true}
	b := pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true}
	if got := nearestDue(false, a, pgtype.Timestamptz{}, b); got == nil || !got.Equal(a.Time) {
		t.Fatalf("first response pending: %v", got)
	}
	if got := nearestDue(true, a, pgtype.Timestamptz{}, b); got == nil || !got.Equal(b.Time) {
		t.Fatalf("after the first response the first-response deadline no longer counts: %v", got)
	}
	if got := nearestDue(true, pgtype.Timestamptz{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}); got != nil {
		t.Fatal("no deadline")
	}
}

func TestWithoutDropsActor(t *testing.T) {
	if got := without([]int64{1, 2, 1, 3}, 1); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("without = %v", got)
	}
}

package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	org      = int64(3)
	agent    = int64(10)
	convID   = int64(77)
	startTok = "aa00aa00aa00aa00aa00aa00aa00aa00"
	updTok   = "bb00bb00bb00bb00bb00bb00bb00bb00"
)

var convUUID = uuid.MustParse("6f1c1b8e-6d1a-4c55-9d7e-0f5b2c3d4e5f")

type laFixture struct {
	store  *fakeStore
	sender *fakeSender
	svc    *Service
	now    time.Time
	due    time.Time
}

func newFixture(t *testing.T) *laFixture {
	t.Helper()
	f := &laFixture{store: newFakeStore(), sender: &fakeSender{results: map[string]apns.Result{}, errs: map[string]error{}}}
	f.svc = New(f.store, f.sender, nil)
	f.now = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	f.due = f.now.Add(10 * time.Minute)
	f.svc.now = func() time.Time { return f.now }
	f.store.recipient[agent] = db.GetLiveActivityRecipientRow{ID: agent, Locale: "en", Status: "active", PushEnabled: true, LiveActivitiesEnabled: true}
	f.store.conv[convID] = db.GetLiveActivityConversationRow{
		ID: convID, Uuid: convUUID, OrganizationID: org, Number: 42, ChannelType: "whatsapp", Status: "open",
		AssigneeUserID: pgtype.Int8{Int64: agent, Valid: true}, LastMessagePreview: "Where is my order?", ContactName: "Mehmet Kaya",
	}
	f.store.tokens = []db.LiveActivityToken{
		{ID: 1, UserID: agent, Kind: KindPushToStart, ActivityType: ActivityUrgentConversation, Token: startTok, Environment: "sandbox"},
		{ID: 2, UserID: agent, Kind: KindUpdate, ActivityType: ActivityUrgentConversation, Ref: convUUID.String(), Token: updTok, Environment: "production"},
	}
	return f
}

func event(t *testing.T, name string, payload any) events.Event {
	t.Helper()
	ev, err := events.NewDomain(name, org, payload)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func (f *laFixture) slaEvent(t *testing.T, name string) events.Event {
	return event(t, name, events.SLAPayload{PolicyID: 1, TargetType: "conversation", TargetID: convID, Metric: "first_response", DueAt: f.due})
}

func decodeAPS(t *testing.T, body []byte) (map[string]any, UrgentConversationState) {
	t.Helper()
	var doc struct {
		APS map[string]any `json:"aps"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	cs, ok := doc.APS["content-state"].(map[string]any)
	if !ok {
		t.Fatalf("content-state must be an object: %v", doc.APS["content-state"])
	}
	keys := []string{}
	for k := range cs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Exactly the fields of the app's Codable ContentState.
	if strings.Join(keys, ",") != "breached,channel,contact,dueAt,preview,subject" {
		t.Fatalf("content-state keys %v", keys)
	}
	if _, ok := cs["dueAt"].(float64); !ok {
		t.Fatalf("dueAt must be a JSON number: %v", cs["dueAt"])
	}
	raw, _ := json.Marshal(cs)
	var state UrgentConversationState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return doc.APS, state
}

func TestBuildPayloadStart(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	a := UrgentConversationActivity{Conversation: convUUID, Locale: "tr", State: UrgentConversationState{
		Contact: "Ayşe", Channel: "email", Subject: "#42", Preview: "Merhaba", DueAt: float64(now.Unix() + 60),
	}}
	body, err := BuildUrgentConversationPayload(EventStart, a, now)
	if err != nil {
		t.Fatal(err)
	}
	aps, got := decodeAPS(t, body)
	if aps["event"] != "start" || aps["attributes-type"] != "UrgentConversationAttributes" || int64(aps["timestamp"].(float64)) != now.Unix() {
		t.Fatalf("aps %v", aps)
	}
	attrs := aps["attributes"].(map[string]any)
	if len(attrs) != 1 || attrs["conversation"] != convUUID.String() {
		t.Fatalf("attributes %v", attrs)
	}
	alert := aps["alert"].(map[string]any)
	if alert["title"] != "SLA süresi yaklaşıyor" {
		t.Fatalf("alert %v", alert)
	}
	if int64(aps["stale-date"].(float64)) != now.Unix()+60+int64(staleAfterDue.Seconds()) {
		t.Fatalf("stale-date %v", aps["stale-date"])
	}
	if _, ok := aps["dismissal-date"]; ok {
		t.Fatal("dismissal-date only on end")
	}
	if got != a.State {
		t.Fatalf("state %+v", got)
	}
}

func TestBuildPayloadUpdateAndEnd(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body, _ := BuildUrgentConversationPayload(EventUpdate, UrgentConversationActivity{State: UrgentConversationState{Breached: true}}, now)
	aps, state := decodeAPS(t, body)
	if aps["event"] != "update" || aps["alert"] != nil || aps["attributes"] != nil || aps["attributes-type"] != nil || !state.Breached {
		t.Fatalf("update %v", aps)
	}
	if aps["stale-date"] == nil {
		t.Fatal("update keeps stale-date")
	}
	body, _ = BuildUrgentConversationPayload(EventEnd, UrgentConversationActivity{}, now)
	aps, _ = decodeAPS(t, body)
	if aps["event"] != "end" || int64(aps["dismissal-date"].(float64)) != now.Unix() || aps["alert"] != nil || aps["attributes"] != nil {
		t.Fatalf("end %v", aps)
	}
	if _, err := BuildUrgentConversationPayload("bogus", UrgentConversationActivity{}, now); err == nil {
		t.Fatal("unknown event must fail")
	}
}

func TestConversationActivityState(t *testing.T) {
	due := time.UnixMilli(1_800_000_000_500)
	c := db.GetLiveActivityConversationRow{
		Uuid: convUUID, Number: 7, Subject: "  ", ChannelType: "sms", ContactName: "Ali\nVeli",
		LastMessagePreview: "line one\n\tline two " + strings.Repeat("x", 200),
	}
	a := conversationActivity(c, due, true, "en")
	if a.Conversation != convUUID || a.State.Subject != "#7" || a.State.Contact != "Ali Veli" || !a.State.Breached {
		t.Fatalf("activity %+v", a)
	}
	if a.State.DueAt != 1_800_000_000.5 {
		t.Fatalf("dueAt %v", a.State.DueAt)
	}
	if strings.ContainsAny(a.State.Preview, "\n\t") || !strings.HasPrefix(a.State.Preview, "line one line two x") ||
		utf8.RuneCountInString(a.State.Preview) != 140 {
		t.Fatalf("preview %q", a.State.Preview)
	}
}

func TestWarningStartsActivityForAssignee(t *testing.T) {
	f := newFixture(t)
	f.store.tokens = f.store.tokens[:1] // no activity running yet
	if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLAWarning)); err != nil {
		t.Fatal(err)
	}
	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %d", len(f.sender.sent))
	}
	n := f.sender.sent[0]
	if n.Token != startTok || n.Environment != apns.Sandbox || n.Priority != apns.PriorityHigh {
		t.Fatalf("notification %+v", n)
	}
	aps, state := decodeAPS(t, n.Payload)
	if aps["event"] != "start" || aps["alert"].(map[string]any)["title"] != "SLA due soon" {
		t.Fatalf("aps %v", aps)
	}
	if aps["attributes"].(map[string]any)["conversation"] != convUUID.String() {
		t.Fatalf("attributes %v", aps["attributes"])
	}
	want := UrgentConversationState{
		Contact: "Mehmet Kaya", Channel: "whatsapp", Subject: "#42", Preview: "Where is my order?",
		DueAt: float64(f.due.Unix()),
	}
	if state != want {
		t.Fatalf("state %+v", state)
	}
}

func TestWarningGates(t *testing.T) {
	cases := map[string]func(f *laFixture){
		"push off": func(f *laFixture) {
			r := f.store.recipient[agent]
			r.PushEnabled = false
			f.store.recipient[agent] = r
		},
		"live activities off": func(f *laFixture) {
			r := f.store.recipient[agent]
			r.LiveActivitiesEnabled = false
			f.store.recipient[agent] = r
		},
		"no tokens": func(f *laFixture) { f.store.tokens = nil },
		// The fixture holds an update token for the conversation.
		"already running": func(*laFixture) {},
		"unassigned": func(f *laFixture) {
			c := f.store.conv[convID]
			c.AssigneeUserID = pgtype.Int8{}
			f.store.conv[convID] = c
		},
		"resolved": func(f *laFixture) { c := f.store.conv[convID]; c.Status = "resolved"; f.store.conv[convID] = c },
		"gone":     func(f *laFixture) { delete(f.store.conv, convID) },
		"apns off": func(f *laFixture) { f.svc.sender = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			mutate(f)
			if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLAWarning)); err != nil {
				t.Fatal(err)
			}
			if len(f.sender.sent) != 0 {
				t.Fatalf("nothing must be sent, got %d", len(f.sender.sent))
			}
		})
	}
	t.Run("ticket target", func(t *testing.T) {
		f := newFixture(t)
		ev := event(t, events.SLAWarning, events.SLAPayload{TargetType: "ticket", TargetID: convID, DueAt: f.due})
		if err := f.svc.HandleEvent(context.Background(), ev); err != nil || len(f.sender.sent) != 0 {
			t.Fatalf("ticket: %v %d", err, len(f.sender.sent))
		}
	})
}

func TestBreachedUpdatesRunningActivity(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLABreached)); err != nil {
		t.Fatal(err)
	}
	if len(f.sender.sent) != 1 || f.sender.sent[0].Token != updTok {
		t.Fatalf("sent %+v", f.sender.sent)
	}
	aps, state := decodeAPS(t, f.sender.sent[0].Payload)
	if aps["event"] != "update" || !state.Breached {
		t.Fatalf("aps %v state %+v", aps, state)
	}
	if _, off := f.store.disabled[2]; off {
		t.Fatal("update token stays active after an update")
	}
}

func TestResolvedAndAgentReplyEndActivity(t *testing.T) {
	ref := events.ConversationRef{ConversationID: convID, ConversationUUID: convUUID}
	cases := map[string]struct {
		ev   func(t *testing.T) events.Event
		ends bool
	}{
		"resolved": {func(t *testing.T) events.Event {
			return event(t, events.ConversationResolved, events.ConversationPayload{ConversationRef: ref, Status: "resolved"})
		}, true},
		"agent reply": {func(t *testing.T) events.Event {
			return event(t, events.MessageSent, events.MessagePayload{ConversationRef: ref, Direction: "outbound", SenderType: "agent"})
		}, true},
		"private note": {func(t *testing.T) events.Event {
			return event(t, events.MessageSent, events.MessagePayload{ConversationRef: ref, Direction: "outbound", SenderType: "agent", Private: true})
		}, false},
		"bot reply": {func(t *testing.T) events.Event {
			return event(t, events.MessageSent, events.MessagePayload{ConversationRef: ref, Direction: "outbound", SenderType: "bot"})
		}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ev := tc.ev(t)
			if err := f.svc.HandleEvent(context.Background(), ev); err != nil {
				t.Fatal(err)
			}
			if !tc.ends {
				if len(f.sender.sent) != 0 {
					t.Fatalf("sent %d", len(f.sender.sent))
				}
				return
			}
			if len(f.sender.sent) != 1 || f.sender.sent[0].Token != updTok || f.sender.sent[0].Priority != apns.PriorityLow {
				t.Fatalf("sent %+v", f.sender.sent)
			}
			aps, _ := decodeAPS(t, f.sender.sent[0].Payload)
			if aps["event"] != "end" {
				t.Fatalf("aps %v", aps)
			}
			if f.store.disabled[2] != "ended" {
				t.Fatalf("ended token must be retired: %v", f.store.disabled)
			}
			// A repeated event finds nothing left to end.
			if err := f.svc.HandleEvent(context.Background(), ev); err != nil || len(f.sender.sent) != 1 {
				t.Fatalf("repeat: %v %d", err, len(f.sender.sent))
			}
		})
	}
}

func TestInboxAliasedEventsEndActivity(t *testing.T) {
	entity := func(name string, payload map[string]any, actor int64) events.Event {
		id, ref := convID, convUUID
		ev := events.New(name).WithTenant(org).WithEntity("conversation", &id, &ref).WithPayload(payload)
		if actor > 0 {
			ev = ev.WithActor(actor)
		}
		return ev
	}
	cases := map[string]struct {
		ev   events.Event
		ends bool
	}{
		"agent reply":  {entity(events.MessageSent, map[string]any{"message_uuid": uuid.NewString(), "private": false, "kind": "reply"}, agent), true},
		"ai reply":     {entity(events.MessageSent, map[string]any{"message_uuid": uuid.NewString(), "ai": true}, 0), false},
		"system reply": {entity(events.MessageSent, map[string]any{"message_uuid": uuid.NewString()}, 0), false},
		"resolved":     {entity(events.ConversationResolved, map[string]any{"change": "status_changed", "from": "open", "to": "resolved"}, agent), true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if err := f.svc.HandleEvent(context.Background(), tc.ev); err != nil {
				t.Fatal(err)
			}
			if got := len(f.sender.sent) == 1; got != tc.ends {
				t.Fatalf("sent %d, want end=%v", len(f.sender.sent), tc.ends)
			}
		})
	}
}

func TestAPNsFailures(t *testing.T) {
	t.Run("bad token disabled, no error", func(t *testing.T) {
		f := newFixture(t)
		f.store.tokens = f.store.tokens[:1]
		f.sender.results[startTok] = apns.Result{Status: 410, Reason: "Unregistered"}
		if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLAWarning)); err != nil {
			t.Fatal(err)
		}
		if f.store.disabled[1] != "Unregistered" {
			t.Fatalf("disabled %v", f.store.disabled)
		}
	})
	t.Run("start never retried", func(t *testing.T) {
		f := newFixture(t)
		f.store.tokens = f.store.tokens[:1]
		f.sender.errs[startTok] = errors.New("network down")
		if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLAWarning)); err != nil {
			t.Fatalf("start failures must not retry: %v", err)
		}
	})
	t.Run("update retried on transient error", func(t *testing.T) {
		f := newFixture(t)
		f.sender.results[updTok] = apns.Result{Status: 503, Reason: "ServiceUnavailable"}
		if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLABreached)); err == nil {
			t.Fatal("transient update failure must be retried")
		}
	})
	t.Run("permanent payload error logged only", func(t *testing.T) {
		f := newFixture(t)
		f.sender.results[updTok] = apns.Result{Status: 400, Reason: "BadPriority"}
		if err := f.svc.HandleEvent(context.Background(), f.slaEvent(t, events.SLABreached)); err != nil {
			t.Fatal(err)
		}
		if _, off := f.store.disabled[2]; off {
			t.Fatal("a payload error must not disable the token")
		}
	})
}

func TestConsumerSubscriptions(t *testing.T) {
	c := newFixture(t).svc.Consumer()
	if c.Name != "mobile-live-activities" || len(c.Events) != 4 {
		t.Fatalf("consumer %+v", c)
	}
}

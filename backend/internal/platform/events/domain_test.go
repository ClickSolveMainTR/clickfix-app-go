package events

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDomainDefinitionsAreValidAndKnown(t *testing.T) {
	t.Parallel()
	defs := DomainDefinitions()
	if len(defs) == 0 {
		t.Fatal("empty domain catalog")
	}
	for _, d := range defs {
		if err := ValidateEventName(d.Name); err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		if !IsKnownEvent(d.Name) {
			t.Fatalf("%s missing from AllKnownEvents", d.Name)
		}
		if d.Version < 1 || d.payload == nil {
			t.Fatalf("%s: bad definition %+v", d.Name, d)
		}
	}
	for _, name := range []string{ConversationCreated, ConversationAssigned, MessageReceived, MessageSent, TicketCreated, SLABreached} {
		if _, ok := LookupDefinition(name); !ok {
			t.Fatalf("%s not registered", name)
		}
	}
}

func TestWebhookEventsAreWebhookDefinitions(t *testing.T) {
	t.Parallel()
	names := WebhookEvents()
	if len(names) == 0 {
		t.Fatal("no webhook events")
	}
	for _, name := range names {
		if d, _ := LookupDefinition(name); !d.Webhook {
			t.Fatalf("%s listed but not webhook", name)
		}
	}
}

func TestNewDomainDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	inbox := int64(4)
	in := MessagePayload{
		ConversationRef: ConversationRef{ConversationID: 10, ConversationUUID: uuid.New(), InboxID: &inbox, ChannelType: "whatsapp"},
		MessageID:       99,
		MessageUUID:     uuid.New(),
		Direction:       "inbound",
		SenderType:      "contact",
		ExternalID:      "wamid.1",
	}
	ev, err := NewDomain(MessageReceived, 7, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev.TenantID == nil || *ev.TenantID != 7 || ev.Version != 1 {
		t.Fatalf("envelope = %+v", ev)
	}
	if ev.Payload["conversation_id"] != int64(10) {
		t.Fatalf("payload numbers not normalized: %#v", ev.Payload["conversation_id"])
	}
	out, err := DecodePayload[MessagePayload](ev)
	if err != nil {
		t.Fatal(err)
	}
	if out.MessageID != 99 || out.ConversationUUID != in.ConversationUUID || *out.InboxID != 4 || out.ExternalID != "wamid.1" {
		t.Fatalf("decoded = %+v", out)
	}
}

func TestNewDomainRejectsWrongPayload(t *testing.T) {
	t.Parallel()
	if _, err := NewDomain(MessageReceived, 1, TicketPayload{}); err == nil {
		t.Fatal("expected type mismatch")
	}
	if _, err := NewDomain(MessageReceived, 1, &MessagePayload{}); err == nil {
		t.Fatal("expected pointer rejection")
	}
	if _, err := NewDomain("unknown.event", 1, MessagePayload{}); err == nil {
		t.Fatal("expected unknown event")
	}
}

func TestDecodePayloadRejectsNewerVersionAndWrongType(t *testing.T) {
	t.Parallel()
	ev, err := NewDomain(SLABreached, 1, SLAPayload{PolicyID: 1, TargetType: "ticket", TargetID: 2, Metric: "resolution", DueAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePayload[TicketPayload](ev); err == nil {
		t.Fatal("expected wrong type error")
	}
	ev.Version = 2
	if _, err := DecodePayload[SLAPayload](ev); err == nil {
		t.Fatal("expected newer version error")
	}
}

func TestWithIdempotencyKeyIsDeterministic(t *testing.T) {
	t.Parallel()
	a := New(MessageReceived).WithOrganization(1).WithIdempotencyKey("wamid.1")
	b := New(MessageReceived).WithOrganization(1).WithIdempotencyKey("wamid.1")
	c := New(MessageReceived).WithOrganization(2).WithIdempotencyKey("wamid.1")
	if a.EventID != b.EventID {
		t.Fatal("same key must give same event id")
	}
	if a.EventID == c.EventID {
		t.Fatal("different organizations must not collide")
	}
}

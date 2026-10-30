package engine

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

var (
	topicID = uuid.New()
	orderID = uuid.New()
	planID  = uuid.New()
)

func customs() []Custom {
	return []Custom{
		{UUID: topicID, Entity: SourceConversation, Def: customfields.Definition{
			Key: "topic", Label: "Konu", Type: customfields.TypeSelect,
			Options: []customfields.Option{{Value: "billing", Label: "Fatura"}, {Value: "tech", Label: "Teknik"}},
		}},
		{UUID: orderID, Entity: SourceConversation, Def: customfields.Definition{Key: "order_no", Label: "Sipariş no", Type: customfields.TypeText}},
		{UUID: planID, Entity: SourceContact, Def: customfields.Definition{Key: "plan", Label: "Paket", Type: customfields.TypeText}},
	}
}

func form(t *testing.T) []Resolved {
	t.Helper()
	fields := []Field{
		{Key: KeyName, Source: SourceSystem, Required: true},
		{Key: KeyEmail, Source: SourceSystem, Required: true},
		{Key: KeyMessage, Source: SourceSystem, Required: true},
		{Source: SourceConversation, CustomFieldUUID: &topicID, Required: true},
		{Source: SourceConversation, CustomFieldUUID: &orderID, Required: true, RequiredToResolve: true,
			Condition: &customfields.Condition{Field: "conversation.topic", Operator: customfields.CondEquals, Value: "billing"}},
		{Source: SourceContact, CustomFieldUUID: &planID, Label: "Hangi paket?"},
	}
	r, err := Resolve(fields, customs(), true)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCheckConditionalRequired(t *testing.T) {
	f := form(t)
	base := map[string]any{"name": "Ayşe", "email": "ayse@example.com", "conversation.topic": "billing"}
	_, err := Check(f, base, KeyMessage)
	var ve *customfields.ValidationError
	if !errors.As(err, &ve) || len(ve.Errors) != 1 || ve.Errors[0].Field != "conversation.order_no" {
		t.Fatalf("expected order_no required, got %v", err)
	}
	base["conversation.order_no"] = "A-1001"
	base["contact.plan"] = "Pro"
	sub, err := Check(f, base, KeyMessage)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Name != "Ayşe" || sub.Conversation["topic"] != "billing" || sub.Conversation["order_no"] != "A-1001" || sub.Contact["plan"] != "Pro" {
		t.Fatalf("submission %+v", sub)
	}
	// Another topic: the order number is neither asked nor stored.
	base["conversation.topic"] = "tech"
	sub, err = Check(f, base, KeyMessage)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.Conversation["order_no"]; ok {
		t.Fatal("hidden field must not be stored")
	}
	// The message is required outside the widget.
	if _, err := Check(f, base); err == nil {
		t.Fatal("message must be required")
	}
	// Unknown keys and bad options are rejected.
	if _, err := Check(f, map[string]any{"name": "x", "email": "x@example.com", "conversation.topic": "nope"}, KeyMessage); err == nil {
		t.Fatal("bad option accepted")
	}
	if _, err := Check(f, map[string]any{"hack": 1}, KeyMessage); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestMissingToResolve(t *testing.T) {
	f := form(t)
	if m := MissingToResolve(f, map[string]any{"topic": "billing"}); len(m) != 1 || m[0].StoredKey != "order_no" {
		t.Fatalf("missing %+v", m)
	}
	if m := MissingToResolve(f, map[string]any{"topic": "billing", "order_no": "A-1"}); len(m) != 0 {
		t.Fatalf("missing %+v", m)
	}
	if m := MissingToResolve(f, map[string]any{"topic": "tech"}); len(m) != 0 {
		t.Fatal("condition not honored")
	}
}

func TestResolveRejectsBadForms(t *testing.T) {
	bad := [][]Field{
		{{Key: "nope", Source: SourceSystem}},
		{{Key: KeyName, Source: SourceSystem}, {Key: KeyName, Source: SourceSystem}},
		{{Source: SourceContact, CustomFieldUUID: &topicID}},                                                            // conversation field as contact
		{{Source: SourceConversation, CustomFieldUUID: ptr(uuid.New())}},                                                // unknown field
		{{Key: KeyName, Source: SourceSystem, Condition: &customfields.Condition{Field: KeyEmail, Operator: "is_set"}}}, // later field
	}
	for i, fields := range bad {
		if _, err := Resolve(fields, customs(), true); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d: err=%v", i, err)
		}
	}
	// Lenient resolve (stored forms) drops fields whose custom field is gone.
	r, err := Resolve([]Field{{Key: KeyName, Source: SourceSystem}, {Source: SourceConversation, CustomFieldUUID: ptr(uuid.New())}}, customs(), false)
	if err != nil || len(r) != 1 {
		t.Fatalf("lenient %v %v", r, err)
	}
}

func ptr(u uuid.UUID) *uuid.UUID { return &u }

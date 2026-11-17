package usecase

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

// CF-245: conversation and message events carry conversation_uuid in data;
// existing keys stay and the shared payload is not modified.
func TestWebhookDataAddsConversationUUID(t *testing.T) {
	conv := uuid.New()
	id := int64(7)
	payload := map[string]any{"change": "status_changed", "from": "open", "to": "resolved"}
	ev := events.New(events.ConversationResolved).WithOrganization(1).WithEntity("conversation", &id, &conv).WithPayload(payload)
	data := WebhookData(ev)
	if data["conversation_uuid"] != conv.String() || data["to"] != "resolved" || data["change"] != "status_changed" {
		t.Fatalf("data %v", data)
	}
	if _, ok := payload["conversation_uuid"]; ok {
		t.Fatal("shared payload modified")
	}
	msg := events.New(events.MessageReceived).WithEntity("conversation", &id, &conv).WithPayload(map[string]any{"message_uuid": uuid.NewString()})
	if WebhookData(msg)["conversation_uuid"] != conv.String() {
		t.Fatal("message event without conversation_uuid")
	}
	// A producer's own value wins.
	own := uuid.NewString()
	set := events.New(events.MessageSent).WithEntity("conversation", &id, &conv).WithPayload(map[string]any{"conversation_uuid": own})
	if WebhookData(set)["conversation_uuid"] != own {
		t.Fatal("producer value replaced")
	}
	// Other entities get nothing.
	contact := uuid.New()
	other := events.New(events.ContactCreated).WithEntity("contact", &id, &contact).WithPayload(map[string]any{"name": "A"})
	if _, ok := WebhookData(other)["conversation_uuid"]; ok || ConversationUUID(other) != "" {
		t.Fatal("contact event got conversation_uuid")
	}
}

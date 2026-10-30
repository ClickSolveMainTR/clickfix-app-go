package outbox

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"testing"
)

func TestEnvelopePreservesRequestID(t *testing.T) {
	t.Parallel()
	ev := events.New("contact.updated")
	ev.RequestID = "request-42"
	b, e := MarshalEnvelope(ev)
	if e != nil {
		t.Fatal(e)
	}
	got, e := UnmarshalEnvelope(ev.Name, b)
	if e != nil || got.RequestID != ev.RequestID {
		t.Fatalf("correlation lost: %q %v", got.RequestID, e)
	}
}

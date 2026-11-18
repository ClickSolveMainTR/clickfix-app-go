package apichannel_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/apichannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/google/uuid"
)

// CF-248: an edited reply is a signed, idempotent message_edited event.
func TestEditMessageSignedAndIdempotent(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("e", 32)
	inst := channels.Instance{Credentials: channels.Credentials{"signing_secret": secret, "webhook_url": "https://customer.example/hook"}}
	eventID, msg, conv := uuid.New(), uuid.New(), uuid.New()
	var got map[string]any
	var keys []string
	status := 200
	a := apichannel.New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if err := channelhttp.Verify(r, b, secret, time.Now()); err != nil {
			t.Error(err)
		}
		if r.Header.Get("X-ClickFix-Event") != "message_edited" {
			t.Errorf("event header %q", r.Header.Get("X-ClickFix-Event"))
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})})
	edit := channels.MessageEdit{
		EventID: eventID, MessageUUID: msg, ConversationUUID: conv, ThreadID: "order-7",
		To: channels.Recipient{ExternalID: "cust-9"}, Text: "Kargonuz bugün teslim edilir.",
		EditedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}
	for range 2 {
		if err := a.EditMessage(context.Background(), inst, edit); err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) != 2 || keys[0] != eventID.String() || keys[1] != keys[0] {
		t.Fatalf("idempotency keys %v", keys)
	}
	want := map[string]any{
		"id": eventID.String(), "type": "message_edited", "message_id": msg.String(), "conversation_id": conv.String(),
		"thread_id": "order-7", "recipient_id": "cust-9", "text": "Kargonuz bugün teslim edilir.", "edited_at": "2026-11-18T10:00:00Z",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	status = 422
	if err := a.EditMessage(context.Background(), inst, edit); !channels.IsPermanent(err) {
		t.Fatalf("422: %v", err)
	}
	status = 502
	if err := a.EditMessage(context.Background(), inst, edit); err == nil || channels.IsPermanent(err) {
		t.Fatalf("502: %v", err)
	}
	empty := edit
	empty.Text = ""
	if err := a.EditMessage(context.Background(), inst, empty); !channels.IsPermanent(err) {
		t.Fatalf("empty text: %v", err)
	}
	if !channels.NewRegistry(a).CanEdit(channels.TypeAPI) {
		t.Fatal("registry does not see the editor")
	}
}

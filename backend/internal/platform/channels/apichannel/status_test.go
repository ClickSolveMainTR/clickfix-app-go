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

// CF-245: a status change is a signed, idempotent status_changed event.
func TestNotifyStatusSignedAndIdempotent(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("s", 32)
	inst := channels.Instance{Credentials: channels.Credentials{"signing_secret": secret, "webhook_url": "https://customer.example/hook"}}
	eventID, conv := uuid.New(), uuid.New()
	until := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	var got map[string]any
	var keys []string
	status := 204
	a := apichannel.New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if err := channelhttp.Verify(r, b, secret, time.Now()); err != nil {
			t.Error(err)
		}
		if r.Header.Get("X-ClickFix-Event") != "status_changed" {
			t.Errorf("event header %q", r.Header.Get("X-ClickFix-Event"))
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})})
	ch := channels.StatusChange{EventID: eventID, ConversationUUID: conv, ThreadID: "t1", From: "open", To: "snoozed", SnoozedUntil: &until}
	for range 2 {
		if err := a.NotifyStatus(context.Background(), inst, ch); err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) != 2 || keys[0] != eventID.String() || keys[1] != keys[0] {
		t.Fatalf("idempotency keys %v", keys)
	}
	want := map[string]any{"id": eventID.String(), "type": "status_changed", "conversation_id": conv.String(), "thread_id": "t1", "from": "open", "to": "snoozed", "snoozed_until": "2026-11-20T09:00:00Z"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if _, ok := got["changed_at"]; !ok {
		t.Error("changed_at missing")
	}
	if v, ok := got["custom_status"]; !ok || v != nil {
		t.Errorf("custom_status = %v (present %v), want null", v, ok)
	}

	// CF-249: the organization's own status travels with the base status.
	custom := uuid.New()
	withCustom := channels.StatusChange{EventID: uuid.New(), ConversationUUID: conv, From: "pending", To: "open",
		CustomStatus: &channels.CustomStatus{UUID: custom, Name: "İşleniyor", Color: "#3b82f6"}}
	if err := a.NotifyStatus(context.Background(), inst, withCustom); err != nil {
		t.Fatal(err)
	}
	cs, _ := got["custom_status"].(map[string]any)
	if got["to"] != "open" || cs["uuid"] != custom.String() || cs["name"] != "İşleniyor" || cs["color"] != "#3b82f6" || got["from_custom_status"] != nil {
		t.Fatalf("custom status body %v", got)
	}
	// A rejected event is not retried; a server error is.
	status = 410
	if err := a.NotifyStatus(context.Background(), inst, ch); !channels.IsPermanent(err) {
		t.Fatalf("410: %v", err)
	}
	status = 503
	if err := a.NotifyStatus(context.Background(), inst, ch); err == nil || channels.IsPermanent(err) {
		t.Fatalf("503: %v", err)
	}
	if err := a.NotifyStatus(context.Background(), inst, channels.StatusChange{ConversationUUID: conv, To: "resolved"}); !channels.IsPermanent(err) {
		t.Fatalf("missing event id: %v", err)
	}
}

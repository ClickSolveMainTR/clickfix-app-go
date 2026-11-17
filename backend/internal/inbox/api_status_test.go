package inbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/apichannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
)

type hookRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
	keys   []string
}

func (h *hookRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	h.mu.Lock()
	h.bodies = append(h.bodies, m)
	h.keys = append(h.keys, r.Header.Get("Idempotency-Key"))
	h.mu.Unlock()
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

// CF-245: the REST API channel answers an inbound message with its
// conversation, lists conversations by thread and is told about status
// changes (signed, one event id per change).
func TestAPIChannelConversationRefsAndStatus(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	hooks := &hookRecorder{}
	e.ib.Registry.Register(apichannel.New(&http.Client{Transport: hooks}))
	signing := strings.Repeat("a", 40)
	inst, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeAPI, Name: "EasyTrade",
		Credentials: channels.Credentials{"signing_secret": signing, "webhook_url": "https://example.com/clickfix"},
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/webhooks/channels/{type}/{instance}", channelshandler.New(e.ib.Channels, e.ib.Receive, nil, nil).Webhook)
	post := func(body string) map[string]any {
		req := httptest.NewRequest(http.MethodPost, inst.WebhookPath, bytes.NewBufferString(body))
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-ClickFix-Timestamp", ts)
		req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(signing, ts, []byte(body)))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("webhook = %d %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		out := map[string]any{}
		if json.Unmarshal(rec.Body.Bytes(), &env) == nil && env.Data != nil {
			return env.Data
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	first := post(`{"id":"et-1","sender_id":"cust-7","text":"Siparişim nerede?","thread_id":"order-991"}`)
	var convUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1 AND channel_type = 'api'", e.org).Scan(&convUUID); err != nil {
		t.Fatal(err)
	}
	if first["received"] != true || first["conversation_uuid"] != convUUID.String() || first["message_uuid"] == nil {
		t.Fatalf("inbound answer %v", first)
	}
	// A redelivery answers with the same conversation.
	if again := post(`{"id":"et-1","sender_id":"cust-7","text":"Siparişim nerede?","thread_id":"order-991"}`); again["conversation_uuid"] != convUUID.String() {
		t.Fatalf("redelivery answer %v", again)
	}

	// The DTO carries the thread and the list filters by it.
	list, total, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{ExternalThreadID: "order-991"})
	if err != nil || total != 1 || len(list) != 1 || list[0].UUID != convUUID || list[0].ExternalThreadID == nil || *list[0].ExternalThreadID != "order-991" {
		t.Fatalf("thread filter %+v %d %v", list, total, err)
	}
	if _, total, err = e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{ExternalThreadID: "order-000"}); err != nil || total != 0 {
		t.Fatalf("other thread %d %v", total, err)
	}

	// An agent resolves: the outbox event drives the status notification.
	resolved := convmodel.StatusResolved
	if _, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: convUUID, Status: &resolved}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := e.pool.QueryRow(ctx, "SELECT payload FROM outbox_events WHERE organization_id = $1 AND event_name = 'conversations.resolved'", e.org).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ev, err := outbox.UnmarshalEnvelope("conversations.resolved", raw)
	if err != nil {
		t.Fatal(err)
	}
	n := &channelsmodule.StatusNotifier{Channels: e.ib.Channels}
	for range 2 { // a consumer retry sends the same event id
		if err := n.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	if len(hooks.bodies) != 2 || hooks.keys[0] != ev.EventID.String() || hooks.keys[1] != hooks.keys[0] {
		t.Fatalf("status deliveries %v %v", hooks.bodies, hooks.keys)
	}
	got := hooks.bodies[0]
	if got["type"] != "status_changed" || got["conversation_id"] != convUUID.String() || got["thread_id"] != "order-991" || got["from"] != "open" || got["to"] != "resolved" {
		t.Fatalf("status body %v", got)
	}
}

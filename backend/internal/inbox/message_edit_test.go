package inbox_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
)

// CF-248: an agent corrects a delivered reply on the REST API channel. The
// message keeps its edit history and edited_at, the inbox gets a live
// update and the customer's endpoint a signed message_edited event.
func TestAPIChannelMessageEdit(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	hooks := &hookRecorder{}
	e.ib.Registry.Register(apichannel.New(&http.Client{Transport: hooks}))
	signing := strings.Repeat("b", 40)
	inst, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeAPI, Name: "EasyTrade",
		Credentials: channels.Credentials{"signing_secret": signing, "webhook_url": "https://example.com/clickfix"},
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/webhooks/channels/{type}/{instance}", channelshandler.New(e.ib.Channels, e.ib.Receive, nil, nil).Webhook)
	body := `{"id":"et-9","sender_id":"cust-9","text":"Kargom nerede?","thread_id":"order-7"}`
	req := httptest.NewRequest(http.MethodPost, inst.WebhookPath, bytes.NewBufferString(body))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-ClickFix-Timestamp", ts)
	req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(signing, ts, []byte(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook = %d %s", rec.Code, rec.Body.String())
	}
	var convUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id = $1 AND channel_type = 'api'", e.org).Scan(&convUUID); err != nil {
		t.Fatal(err)
	}
	agentUUID := uuid.New()
	_ = e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", e.agent).Scan(&agentUUID)
	posted, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{
		OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, AuthorUUID: agentUUID, AuthorName: "Elif Şahin",
		Content: "Kargonuz yarın teslim edilir.",
	})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := e.pool.QueryRow(ctx, "SELECT status FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status); err != nil || status != convmodel.MessageSent {
		t.Fatalf("reply status %q %v", status, err)
	}
	hooks.mu.Lock()
	hooks.bodies, hooks.keys = nil, nil
	hooks.mu.Unlock()

	in := convusecase.EditInput{
		OrganizationID: e.org, ConversationUUID: convUUID, MessageUUID: posted.Message.UUID,
		EditorID: e.agent, EditorUUID: agentUUID, EditorName: "Elif Şahin", Content: "Kargonuz bugün teslim edilir.",
	}
	// Someone else's reply and an unchanged text are refused.
	other := in
	other.EditorID = e.agent + 1_000_000
	if _, err := e.ib.Conversations.EditMessage(ctx, other); !errors.Is(err, convusecase.ErrNotAuthor) {
		t.Fatalf("other author: %v", err)
	}
	same := in
	same.Content = "Kargonuz yarın teslim edilir."
	if _, err := e.ib.Conversations.EditMessage(ctx, same); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("unchanged: %v", err)
	}

	out, err := e.ib.Conversations.EditMessage(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.ChannelEdit != convusecase.ChannelEditNotified || out.Message.Content != in.Content || out.Message.EditedAt == nil {
		t.Fatalf("edit result %+v", out)
	}
	var preview string
	if err := e.pool.QueryRow(ctx, "SELECT last_message_preview FROM conversations WHERE uuid = $1", convUUID).Scan(&preview); err != nil || preview != in.Content {
		t.Fatalf("preview %q %v", preview, err)
	}
	history, err := e.ib.Conversations.MessageEdits(ctx, e.org, convUUID, posted.Message.UUID)
	if err != nil || len(history) != 1 || history[0].PreviousContent != "Kargonuz yarın teslim edilir." || history[0].Editor == nil || history[0].Editor.UUID != agentUUID {
		t.Fatalf("history %+v %v", history, err)
	}
	e.live.mu.Lock()
	last := e.live.events[len(e.live.events)-1]
	e.live.mu.Unlock()
	if last.Type != convmodel.EventMessageUpdated || last.Message == nil || last.Message.EditedAt == nil || last.Message.Content != in.Content {
		t.Fatalf("live event %+v", last)
	}

	// The outbox event drives the channel notification; a retry repeats the
	// same event id.
	var raw []byte
	if err := e.pool.QueryRow(ctx, "SELECT payload FROM outbox_events WHERE organization_id = $1 AND event_name = $2 AND payload::text LIKE '%message_edited%'", e.org, events.ConversationsUpdated).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ev, err := outbox.UnmarshalEnvelope(events.ConversationsUpdated, raw)
	if err != nil {
		t.Fatal(err)
	}
	n := &channelsmodule.StatusNotifier{Channels: e.ib.Channels}
	for range 2 {
		if err := n.HandleEdit(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	// The status consumer ignores the edit.
	if err := n.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	if len(hooks.bodies) != 2 || hooks.keys[0] != ev.EventID.String() || hooks.keys[1] != hooks.keys[0] {
		t.Fatalf("edit deliveries %v %v", hooks.bodies, hooks.keys)
	}
	got := hooks.bodies[0]
	if got["type"] != "message_edited" || got["message_id"] != posted.Message.UUID.String() || got["conversation_id"] != convUUID.String() ||
		got["thread_id"] != "order-7" || got["recipient_id"] != "cust-9" || got["text"] != in.Content || got["edited_at"] == nil {
		t.Fatalf("edit body %v", got)
	}
}

// CF-248: notes, contact messages and replies past the edit window are
// not editable; a reply that was not sent yet is edited without a channel
// notification.
func TestMessageEditRules(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"r-1","from":{"id":"u-1","name":"Ayşe"},"text":"Merhaba"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var convUUID, inbound uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT c.uuid, m.uuid FROM conversations c JOIN messages m ON m.conversation_id = c.id WHERE c.organization_id = $1 AND m.direction = 'in'", e.org).Scan(&convUUID, &inbound); err != nil {
		t.Fatal(err)
	}
	edit := func(msg uuid.UUID, content string) (convusecase.Edited, error) {
		return e.ib.Conversations.EditMessage(ctx, convusecase.EditInput{
			OrganizationID: e.org, ConversationUUID: convUUID, MessageUUID: msg, EditorID: e.agent, Content: content,
		})
	}
	if _, err := edit(inbound, "değişti"); !errors.Is(err, convusecase.ErrNotEditable) {
		t.Fatalf("inbound: %v", err)
	}
	note, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "iç not", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edit(note.Message.UUID, "iç not 2"); !errors.Is(err, convusecase.ErrNotEditable) {
		t.Fatalf("note: %v", err)
	}
	reply, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "Merhba"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edit(reply.Message.UUID, " "); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
	// The test channel cannot update a delivered message: the agent is told.
	out, err := edit(reply.Message.UUID, "Merhaba")
	if err != nil {
		t.Fatal(err)
	}
	if out.ChannelEdit != convusecase.ChannelEditUnsupported {
		t.Fatalf("channel edit %q", out.ChannelEdit)
	}
	conv, err := e.ib.Conversations.Get(ctx, e.org, e.agent, convUUID)
	if err != nil || conv.ChannelEdits {
		t.Fatalf("channel_edits %v %v", conv.ChannelEdits, err)
	}
	page, err := e.ib.Conversations.Messages(ctx, e.org, convUUID, uuid.Nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range page.Items {
		if m.UUID == reply.Message.UUID && (m.EditedAt == nil || m.Content != "Merhaba") {
			t.Fatalf("listed reply %+v", m)
		}
	}
	// A queued reply: the next delivery carries the new text.
	if _, err := e.pool.Exec(ctx, "UPDATE messages SET status = 'queued' WHERE uuid = $1", reply.Message.UUID); err != nil {
		t.Fatal(err)
	}
	if out, err = edit(reply.Message.UUID, "Merhaba, nasıl yardımcı olabilirim?"); err != nil || out.ChannelEdit != convusecase.ChannelEditNotSent {
		t.Fatalf("queued edit %+v %v", out, err)
	}
	// Being handed to the channel right now: refused.
	if _, err := e.pool.Exec(ctx, "UPDATE messages SET status = 'pending' WHERE uuid = $1", reply.Message.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := edit(reply.Message.UUID, "x"); !errors.Is(err, convusecase.ErrNotEditable) {
		t.Fatalf("pending: %v", err)
	}
	// Past the window.
	if _, err := e.pool.Exec(ctx, "UPDATE messages SET status = 'sent', created_at = now() - interval '16 minutes' WHERE uuid = $1", reply.Message.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := edit(reply.Message.UUID, "geç"); !errors.Is(err, convusecase.ErrEditWindowClosed) {
		t.Fatalf("window: %v", err)
	}
	history, err := e.ib.Conversations.MessageEdits(ctx, e.org, convUUID, reply.Message.UUID)
	if err != nil || len(history) != 2 || history[0].PreviousContent != "Merhba" {
		t.Fatalf("history %+v %v", history, err)
	}
}

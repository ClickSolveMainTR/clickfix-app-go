package channels

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	platformchannels "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

// StatusConsumerName is the durable consumer that forwards conversation
// status changes to channels implementing channels.StatusNotifier (CF-245).
const StatusConsumerName = "channel_status_notify"

// StatusNotifier builds the consumer. Channels is set once the inbox is
// composed (the worker builds the consumer registry first); events that
// arrive before that are retried.
type StatusNotifier struct {
	Channels *usecase.Service
}

var errChannelsNotReady = errors.New("channels: status notifier not wired")

// statusChanges are the inbox change kinds that move a conversation's status.
var statusChanges = map[string]bool{"status_changed": true, "merged_into": true}

// Consumer returns the event consumer. Conversation updates without a
// status change are ignored.
func (n *StatusNotifier) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name:     StatusConsumerName,
		Events:   []string{events.ConversationsUpdated, events.ConversationsResolved, events.ConversationsReopened},
		MaxRetry: 8, Timeout: 30 * time.Second,
		Handle: n.Handle,
	}
}

// EditConsumerName is the durable consumer that forwards edited replies to
// channels implementing channels.MessageEditor (CF-248).
const EditConsumerName = "channel_message_edit"

// EditConsumer returns the consumer of edited replies. Only edits of a
// reply the channel already delivered are forwarded (notify_channel).
func (n *StatusNotifier) EditConsumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name:     EditConsumerName,
		Events:   []string{events.ConversationsUpdated},
		MaxRetry: 8, Timeout: 30 * time.Second,
		Handle: n.HandleEdit,
	}
}

// HandleEdit forwards one edited reply.
func (n *StatusNotifier) HandleEdit(ctx context.Context, ev events.Event) error {
	ev2, ok := EditEventFrom(ev)
	if !ok {
		return nil
	}
	if n == nil || n.Channels == nil {
		return errChannelsNotReady
	}
	return n.Channels.NotifyEdit(ctx, ev2)
}

// EditEventFrom reads an edited reply from an inbox event.
func EditEventFrom(ev events.Event) (usecase.EditEvent, bool) {
	change, _ := ev.Payload["change"].(string)
	notify, _ := ev.Payload["notify_channel"].(bool)
	if change != "message_edited" || !notify || ev.TenantID == nil || ev.EntityID == nil || ev.EntityType != "conversation" {
		return usecase.EditEvent{}, false
	}
	raw, _ := ev.Payload["message_uuid"].(string)
	id, err := uuid.Parse(raw)
	if err != nil {
		return usecase.EditEvent{}, false
	}
	return usecase.EditEvent{
		EventID: ev.EventID, OrganizationID: *ev.TenantID, ConversationID: *ev.EntityID, MessageUUID: id, At: ev.OccurredAt,
	}, true
}

// Handle forwards one event.
func (n *StatusNotifier) Handle(ctx context.Context, ev events.Event) error {
	ev2, ok := StatusEventFrom(ev)
	if !ok {
		return nil
	}
	if n == nil || n.Channels == nil {
		return errChannelsNotReady
	}
	return n.Channels.NotifyStatus(ctx, ev2)
}

// StatusEventFrom reads a status transition from an inbox event.
func StatusEventFrom(ev events.Event) (usecase.StatusEvent, bool) {
	change, _ := ev.Payload["change"].(string)
	if !statusChanges[change] || ev.TenantID == nil || ev.EntityID == nil || ev.EntityType != "conversation" {
		return usecase.StatusEvent{}, false
	}
	from, _ := ev.Payload["from"].(string)
	to, _ := ev.Payload["to"].(string)
	if to == "" {
		return usecase.StatusEvent{}, false
	}
	return usecase.StatusEvent{
		EventID: ev.EventID, OrganizationID: *ev.TenantID, ConversationID: *ev.EntityID,
		From: from, To: to, At: ev.OccurredAt,
		FromCustom: customStatusFrom(ev.Payload["from_custom_status"]), ToCustom: customStatusFrom(ev.Payload["to_custom_status"]),
	}, true
}

// customStatusFrom reads a custom status of the payload (CF-249): the
// struct the inbox put there, or its JSON object after the outbox.
func customStatusFrom(v any) *platformchannels.CustomStatus {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out platformchannels.CustomStatus
	if err := json.Unmarshal(raw, &out); err != nil || out.UUID == uuid.Nil {
		return nil
	}
	return &out
}

package channels

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
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
	}, true
}

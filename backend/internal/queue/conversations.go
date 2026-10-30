package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Conversation (inbox) task types.
const (
	// TaskConversationSend delivers one outbound message through its channel.
	TaskConversationSend = "app:conversations:send"
	// QueueMessaging carries customer-facing deliveries; it is weighted
	// above exports and search so replies leave first.
	QueueMessaging = "messaging"
	// ConversationSendMaxRetry bounds retries of temporary provider failures
	// (exponential backoff, about an hour in total).
	ConversationSendMaxRetry = 8
)

// ConversationSendPayload identifies the message to deliver.
type ConversationSendPayload struct {
	OrganizationID int64 `json:"organization_id"`
	MessageID      int64 `json:"message_id"`
}

// NewConversationSendTask builds a delivery task.
func NewConversationSendTask(orgID, messageID int64) (*asynq.Task, error) {
	body, err := json.Marshal(ConversationSendPayload{OrganizationID: orgID, MessageID: messageID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal conversation send: %w", err)
	}
	return asynq.NewTask(TaskConversationSend, body), nil
}

// ConversationSendOptions are the enqueue options of a delivery; delay > 0
// schedules it later (rate limited instance).
func ConversationSendOptions(delay time.Duration) []asynq.Option {
	opts := []asynq.Option{
		asynq.Queue(QueueMessaging),
		asynq.MaxRetry(ConversationSendMaxRetry),
		asynq.Timeout(2 * time.Minute),
	}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	return opts
}

// EnqueueConversationSend queues the delivery of a message.
func (c *Client) EnqueueConversationSend(ctx context.Context, orgID, messageID int64, delay time.Duration) error {
	task, err := NewConversationSendTask(orgID, messageID)
	if err != nil {
		return err
	}
	_, err = c.EnqueueContext(ctx, task, ConversationSendOptions(delay)...)
	return err
}

// DeliverConversationFunc delivers one message; retried / maxRetry come from
// the task so the last attempt can mark the message failed.
type DeliverConversationFunc func(ctx context.Context, orgID, messageID int64, retried, maxRetry int) error

// WithConversationSend registers the outbound message deliverer.
func (w *Worker) WithConversationSend(fn DeliverConversationFunc) *Worker {
	w.mux.HandleFunc(TaskConversationSend, func(ctx context.Context, task *asynq.Task) error {
		var p ConversationSendPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		if fn == nil {
			return nil
		}
		retried, _ := asynq.GetRetryCount(ctx)
		maxRetry, ok := asynq.GetMaxRetry(ctx)
		if !ok {
			maxRetry = ConversationSendMaxRetry
		}
		return fn(ctx, p.OrganizationID, p.MessageID, retried, maxRetry)
	})
	return w
}

// TaskConversationTick wakes snoozed conversations and fires reminders (CF-51).
const TaskConversationTick = "app:conversations:tick"

// ConversationTickFunc runs one follow-up tick.
type ConversationTickFunc func(ctx context.Context) error

// WithConversationTick registers the follow-up tick.
func (w *Worker) WithConversationTick(fn ConversationTickFunc) *Worker {
	w.mux.HandleFunc(TaskConversationTick, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		return fn(ctx)
	})
	return w
}

// RegisterConversationTickSchedule runs the tick every minute (unique so
// slow ticks never pile up).
func RegisterConversationTickSchedule(s ScheduleRegistrar) error {
	if _, err := s.Register("@every 1m", asynq.NewTask(TaskConversationTick, []byte("{}")),
		asynq.Queue(QueueMessaging), asynq.MaxRetry(0), asynq.Timeout(50*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register conversation tick: %w", err)
	}
	return nil
}

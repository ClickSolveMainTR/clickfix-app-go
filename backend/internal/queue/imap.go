package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TaskIMAPTick = "app:channels:imap:tick"
	TaskIMAPPoll = "app:channels:imap:poll"
)

type imapPayload struct {
	OrganizationID int64 `json:"organization_id"`
	InstanceID     int64 `json:"instance_id"`
}

// EnqueueIMAPPoll queues a bounded, independent poll without mailbox secrets.
func (c *Client) EnqueueIMAPPoll(_ context.Context, orgID, instanceID int64) error {
	raw, err := json.Marshal(imapPayload{OrganizationID: orgID, InstanceID: instanceID})
	if err != nil {
		return err
	}
	_, err = c.Enqueue(asynq.NewTask(TaskIMAPPoll, raw), asynq.Queue(QueueSync), asynq.MaxRetry(0), asynq.Timeout(100*time.Second), asynq.Unique(110*time.Second))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// WithIMAPPolling registers minute fanout and per-instance poll handlers.
func (w *Worker) WithIMAPPolling(tick func(context.Context) error, poll func(context.Context, int64, int64) error) *Worker {
	w.mux.HandleFunc(TaskIMAPTick, func(ctx context.Context, _ *asynq.Task) error { return tick(ctx) })
	w.mux.HandleFunc(TaskIMAPPoll, func(ctx context.Context, t *asynq.Task) error {
		var p imapPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return asynq.SkipRetry
		}
		if p.OrganizationID <= 0 || p.InstanceID <= 0 {
			return asynq.SkipRetry
		}
		return poll(ctx, p.OrganizationID, p.InstanceID)
	})
	return w
}

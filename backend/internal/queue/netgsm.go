package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TaskNetgsmTick      = "app:channels:netgsm:tick"
	TaskNetgsmPoll      = "app:channels:netgsm:poll"
	TaskNetgsmRecording = "app:channels:netgsm:recording"
)

type netgsmPayload struct {
	OrganizationID int64  `json:"organization_id"`
	InstanceID     int64  `json:"instance_id"`
	CallID         string `json:"call_id,omitempty"`
}

func (c *Client) enqueueNetgsm(p netgsmPayload, task string) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = c.Enqueue(asynq.NewTask(task,
		b),
		asynq.Queue(QueueSync),
		asynq.MaxRetry(5),
		asynq.Timeout(3*time.Minute),
		asynq.Unique(3*time.Minute))
	if errors.Is(e, asynq.ErrDuplicateTask) {
		return nil
	}
	return e
}
func (c *Client) EnqueueNetgsmPoll(_ context.Context, org, id int64) error {
	return c.enqueueNetgsm(netgsmPayload{OrganizationID: org, InstanceID: id}, TaskNetgsmPoll)
}
func (c *Client) EnqueueNetgsmRecording(_ context.Context, org, id int64, call string) error {
	return c.enqueueNetgsm(netgsmPayload{OrganizationID: org, InstanceID: id, CallID: call}, TaskNetgsmRecording)
}
func (w *Worker) WithNetgsm(tick func(context.Context) error,
	poll func(context.Context,
		int64,
		int64) error,
	archive func(context.Context,
		int64,
		int64,
		string) error) *Worker {
	w.mux.HandleFunc(TaskNetgsmTick, func(ctx context.Context, _ *asynq.Task) error { return tick(ctx) })
	for _, task := range []string{TaskNetgsmPoll, TaskNetgsmRecording} {
		w.mux.HandleFunc(task, func(ctx context.Context, t *asynq.Task) error {
			var p netgsmPayload
			if json.Unmarshal(t.Payload(), &p) != nil || p.OrganizationID <= 0 || p.InstanceID <= 0 {
				return asynq.SkipRetry
			}
			if t.Type() == TaskNetgsmRecording {
				return archive(ctx, p.OrganizationID, p.InstanceID, p.CallID)
			}
			return poll(ctx, p.OrganizationID, p.InstanceID)
		})
	}
	return w
}

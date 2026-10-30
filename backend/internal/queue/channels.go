package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Channel maintenance task types (CF-61).
const (
	// TaskChannelHealth renews expiring tokens and checks every connected
	// channel instance with its provider.
	TaskChannelHealth = "app:channels:health"
	// TaskChannelReconcile stores Meta messages webhooks missed.
	TaskChannelReconcile = "app:channels:reconcile"
)

// ChannelHealthFunc runs the health sweep.
type ChannelHealthFunc func(ctx context.Context) (checked, failing int, err error)

// ChannelReconcileFunc runs the reconcile pass and returns how many messages
// it read (stored ones are deduplicated by provider id).
type ChannelReconcileFunc func(ctx context.Context) (int, error)

// WithChannelMaintenance registers the channel health and reconcile jobs.
func (w *Worker) WithChannelMaintenance(health ChannelHealthFunc, reconcile ChannelReconcileFunc) *Worker {
	w.mux.HandleFunc(TaskChannelHealth, func(ctx context.Context, _ *asynq.Task) error {
		if health == nil {
			return nil
		}
		checked, failing, err := health(ctx)
		w.log.Info("channel_health_sweep", "checked", checked, "failing", failing)
		return err
	})
	w.mux.HandleFunc(TaskChannelReconcile, func(ctx context.Context, _ *asynq.Task) error {
		if reconcile == nil {
			return nil
		}
		n, err := reconcile(ctx)
		if n > 0 {
			w.log.Info("channel_reconcile", "messages", n)
		}
		return err
	})
	return w
}

// RegisterChannelSchedules adds the periodic channel jobs; a zero interval
// leaves a job out.
func RegisterChannelSchedules(scheduler ScheduleRegistrar, healthEvery, reconcileEvery time.Duration) error {
	jobs := []struct {
		task  string
		every time.Duration
	}{{TaskChannelHealth, healthEvery}, {TaskChannelReconcile, reconcileEvery}, {TaskIMAPTick, time.Minute}, {TaskNetgsmTick, time.Minute}}
	for _, j := range jobs {
		if j.every <= 0 {
			continue
		}
		if _, err := scheduler.Register(
			fmt.Sprintf("@every %s", j.every), asynq.NewTask(j.task, []byte("{}")),
			asynq.Queue(QueueSync), asynq.MaxRetry(0),
			asynq.Timeout(j.every), asynq.Unique(j.every),
		); err != nil {
			return fmt.Errorf("queue: register %s: %w", j.task, err)
		}
	}
	return nil
}

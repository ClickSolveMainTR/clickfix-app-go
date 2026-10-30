package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskRoutingSweep drains routing queues and hands over unanswered
// conversations (CF-53).
const TaskRoutingSweep = "app:routing:sweep"

// RegisterRoutingSchedule adds the minutely routing sweep.
func RegisterRoutingSchedule(scheduler ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		"@every 1m", asynq.NewTask(TaskRoutingSweep, []byte("{}")),
		asynq.Queue(QueueEvents), asynq.MaxRetry(0),
		asynq.Timeout(55*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register routing sweep: %w", err)
	}
	return nil
}

// RoutingSweepHandler runs the sweep.
func RoutingSweepHandler(sweep func(ctx context.Context) (int, error)) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		_, err := sweep(ctx)
		return err
	}
}

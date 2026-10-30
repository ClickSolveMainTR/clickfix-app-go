package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskCustomerHealthSweep scores every company daily (CF-87).
const TaskCustomerHealthSweep = "app:customer_health:sweep"

// customerHealthCron runs at 04:00 UTC (07:00 Europe/Istanbul), so CSMs find
// the churn-risk alerts at the start of the day.
const customerHealthCron = "0 4 * * *"

// RegisterCustomerHealthSchedule registers the daily health sweep.
func RegisterCustomerHealthSchedule(scheduler ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		customerHealthCron, asynq.NewTask(TaskCustomerHealthSweep, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute), asynq.Unique(23*time.Hour),
	); err != nil {
		return fmt.Errorf("queue: register customer health sweep: %w", err)
	}
	return nil
}

// CustomerHealthSweepHandler adapts the sweep to an Asynq handler.
func CustomerHealthSweepHandler(sweep func(context.Context) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error { return sweep(ctx) }
}

package queue

import (
	"context"
	"fmt"
	"github.com/hibiken/asynq"
	"time"
)

const TaskBillingLifecycle = "app:billing:lifecycle_sweep"

// WithBillingLifecycle registers the non-charging deadline sweep.
func (w *Worker) WithBillingLifecycle(fn func(context.Context) error) *Worker {
	w.mux.HandleFunc(TaskBillingLifecycle, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return fmt.Errorf("billing lifecycle is not configured")
		}
		return fn(ctx)
	})
	return w
}
func RegisterBillingLifecycleSchedule(s ScheduleRegistrar) error {
	_, err := s.Register("@every 1m", asynq.NewTask(TaskBillingLifecycle, []byte("{}")), asynq.Queue(QueueMaintenance), asynq.Unique(50*time.Second), asynq.MaxRetry(3))
	return err
}

const TaskBillingProviderSync = "app:billing:provider_sync"
const TaskBillingRenewal = "app:billing:renewal_sweep"
const TaskBillingReconciliation = "app:billing:reconcile"

func (w *Worker) WithBillingPayments(renew, pending, nightly func(context.Context) error) *Worker {
	w.mux.HandleFunc(TaskBillingRenewal, func(ctx context.Context, _ *asynq.Task) error {
		// Unresolved charge claims block recharging until the sync worker settles them.
		return renew(ctx)
	})
	w.mux.HandleFunc(TaskBillingProviderSync, func(ctx context.Context, _ *asynq.Task) error { return pending(ctx) })
	w.mux.HandleFunc(TaskBillingReconciliation, func(ctx context.Context, _ *asynq.Task) error { return nightly(ctx) })
	return w
}
func RegisterBillingPaymentSchedules(s ScheduleRegistrar) error {
	if _, err := s.Register("@every 5m", asynq.NewTask(TaskBillingProviderSync, []byte("{}")), asynq.Queue(QueueSync), asynq.Unique(4*time.Minute), asynq.MaxRetry(3)); err != nil {
		return err
	}
	if _, err := s.Register("@every 5m", asynq.NewTask(TaskBillingRenewal, []byte("{}")), asynq.Queue(QueueMaintenance), asynq.Unique(4*time.Minute), asynq.MaxRetry(3)); err != nil {
		return err
	}
	_, err := s.Register("0 1 * * *", asynq.NewTask(TaskBillingReconciliation, []byte("{}")), asynq.Queue(QueueSync), asynq.Unique(23*time.Hour), asynq.MaxRetry(5))
	return err
}

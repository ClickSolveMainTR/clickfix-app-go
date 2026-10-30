package developerplatform

import (
	"context"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/hibiken/asynq"
	"time"
)

const TaskEngineeringSync = "app:engineering:sync"
const TaskEngineering = "app:engineering:sweep"
const TaskScan = "app:automation:scan"
const TaskWebhooks = "app:webhooks:deliver"
const TaskCleanup = "app:integrations:cleanup"

func (r *Runtime) RegisterWorker(w *queue.Worker) {
	if w == nil {
		return
	}
	w.Handle(TaskEngineering, func(ctx context.Context, _ *asynq.Task) error {
		return r.Engineering.Sweep(ctx)
	})
	w.Handle(TaskEngineeringSync, func(ctx context.Context, _ *asynq.Task) error { return r.Engineering.SyncSweep(ctx) })
	w.Handle(TaskScan, func(ctx context.Context, _ *asynq.Task) error { return r.Automation.Sweep(ctx) })
	w.Handle(TaskWebhooks, func(ctx context.Context, _ *asynq.Task) error { return r.Webhooks.Sweep(ctx) })
	w.Handle(TaskCleanup, func(ctx context.Context, _ *asynq.Task) error {
		_, err := r.OAuth.Cleanup(ctx)
		return errors.Join(err, r.Public.Store.PurgeIntegrationRequests(ctx), r.Engineering.Cleanup(ctx))
	})
}
func RegisterSchedules(s queue.ScheduleRegistrar) error {
	for _, spec := range []struct{ cron, task string }{{"@every 1m", TaskEngineeringSync}, {"@every 1m", TaskEngineering}, {"@every 1m", TaskScan}, {"@every 1m", TaskWebhooks}, {"@every 1h", TaskCleanup}} {
		queueName := queue.QueueMaintenance
		if spec.task == TaskEngineeringSync {
			queueName = queue.QueueSync
		}
		if _, err := s.Register(spec.cron, asynq.NewTask(spec.task, nil), asynq.Queue(queueName), asynq.Unique(50*time.Second), asynq.MaxRetry(8)); err != nil {
			return err
		}
	}
	return nil
}

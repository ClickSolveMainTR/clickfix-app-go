package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPresenceSweep turns agents without a heartbeat offline (CF-10).
const TaskPresenceSweep = "app:presence:sweep"

// RegisterPresenceSweepSchedule runs the idle sweep every minute.
func RegisterPresenceSweepSchedule(scheduler ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		"@every 1m", asynq.NewTask(TaskPresenceSweep, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(50*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register presence sweep: %w", err)
	}
	return nil
}

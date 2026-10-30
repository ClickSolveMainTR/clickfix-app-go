package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskNotificationDigest sends due hourly/daily notification e-mail digests.
const TaskNotificationDigest = "app:notifications:digest"

const digestCron = "@every 5m"

// RegisterDigestSchedule adds the digest sweep (every 5 minutes; each user's
// own hourly/daily window decides who gets one).
func RegisterDigestSchedule(scheduler ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		digestCron, asynq.NewTask(TaskNotificationDigest, []byte("{}")),
		asynq.Queue(QueueNotifications), asynq.MaxRetry(0),
		asynq.Timeout(4*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register digest: %w", err)
	}
	return nil
}

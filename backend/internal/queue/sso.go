package queue

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/hibiken/asynq"
	"time"
)

func RegisterSSOSchedule(s ScheduleRegistrar) error {
	_, err := s.Register("@every 15m", asynq.NewTask(usecase.TaskCleanup, []byte("{}")), asynq.Queue(QueueMaintenance), asynq.Timeout(time.Minute), asynq.MaxRetry(3))
	return err
}

package queue

import (
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	"github.com/hibiken/asynq"
	"time"
)

func RegisterComplianceSchedule(s ScheduleRegistrar) error {
	_, err := s.Register("0 2 * * *", asynq.NewTask(usecase.TaskSweep, []byte("{}")), asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(2*time.Hour), asynq.Unique(23*time.Hour))
	if err != nil {
		return fmt.Errorf("compliance schedule: %w", err)
	}
	_, err = s.Register("@every 1h", asynq.NewTask("app:auth:account-purge", nil), asynq.Queue(QueueMaintenance), asynq.MaxRetry(8), asynq.Timeout(30*time.Minute), asynq.Unique(50*time.Minute))
	return err
}

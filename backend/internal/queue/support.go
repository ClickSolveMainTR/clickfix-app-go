package queue

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TaskUptimeSweep             = "app:uptime:sweep"
	TaskOncallSweep             = "app:oncall:sweep"
	TaskReportsSweep            = "app:reports:sweep"
	TaskIntegrationTriggerPurge = "app:integrations:trigger-purge"
	// TaskAppointmentReminders sends due appointment reminders (CF-138).
	TaskAppointmentReminders = "app:appointments:reminders"
	// CF-139: end over-time / unfunded video rooms; delete expired recordings.
	TaskVideoSweep          = "app:video:sweep"
	TaskVideoRecordingPurge = "app:video:recording-purge"
)

func RegisterSupportSchedules(s ScheduleRegistrar) error {
	for _, job := range []struct {
		kind, queue, cron string
		timeout           time.Duration
	}{{retention.Task, QueueMaintenance, "@every 1h", 5 * time.Minute}, {TaskUptimeSweep, QueueSync, "@every 1m", 5 * time.Minute}, {TaskOncallSweep, QueueEvents, "@every 1m", time.Minute}, {TaskReportsSweep, QueueExports, "@every 1m", 5 * time.Minute}, {TaskIntegrationTriggerPurge, QueueMaintenance, "0 3 * * *", 5 * time.Minute}, {TaskAppointmentReminders, QueueEvents, "@every 1m", 2 * time.Minute}, {TaskVideoSweep, QueueEvents, "@every 1m", time.Minute}, {TaskVideoRecordingPurge, QueueMaintenance, "@every 10m", 5 * time.Minute}} {
		if _, err := s.Register(job.cron, asynq.NewTask(job.kind, []byte("{}")), asynq.Queue(job.queue), asynq.Timeout(job.timeout), asynq.Unique(50*time.Second), asynq.MaxRetry(3)); err != nil {
			return err
		}
	}
	return nil
}

// Package workerschedule owns the periodic registrations shared by both worker modes.
package workerschedule

import (
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/developerplatform"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// Start returns nil without creating or registering a scheduler when disabled.
// Deploy exactly one scheduler-enabled worker; consumer replicas disable this flag.
func Start(cfg config.Config, log *slog.Logger) (*asynq.Scheduler, error) {
	if !cfg.Queue.Enabled || !cfg.Queue.SchedulerEnabled {
		return nil, nil
	}
	scheduler, err := queue.StartLogPurgeScheduler(cfg, log)
	if err != nil {
		return nil, err
	}
	if err := Register(cfg, scheduler); err != nil {
		return nil, err
	}
	if err := scheduler.Start(); err != nil {
		return nil, fmt.Errorf("worker scheduler start: %w", err)
	}
	return scheduler, nil
}

// Register installs every periodic job; consumption queues do not affect scheduler ownership.
func Register(cfg config.Config, scheduler queue.ScheduleRegistrar) error {
	if err := developerplatform.RegisterSchedules(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterBillingLifecycleSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterBillingPaymentSchedules(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterChannelSchedules(scheduler, cfg.Channels.HealthInterval, cfg.Channels.ReconcileInterval); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterPresenceSweepSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterRoutingSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterPushReceiptsSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterAISchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterSSOSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterComplianceSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterCustomerHealthSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterConversationTickSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterSupportSchedules(scheduler); err != nil {
		return err
	}
	if err := queue.RegisterDigestSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := queue.RegisterSLASchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}
	if err := eventdispatch.RegisterSchedule(scheduler); err != nil {
		return fmt.Errorf("worker schedule registration: %w", err)
	}

	return nil
}

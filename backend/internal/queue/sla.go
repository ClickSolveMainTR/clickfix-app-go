package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
)

// SLA task types (CF-46).
const (
	// TaskSLASweep raises due SLA warnings and breaches.
	TaskSLASweep = "app:sla:sweep"
	// TaskSLAResync recomputes an organization's open SLA clocks after a
	// policy, schedule or holiday change.
	TaskSLAResync = "app:sla:resync"
)

const slaSweepCron = "@every 1m"

// SLAResyncPayload names the organization to resync.
type SLAResyncPayload struct {
	OrganizationID int64 `json:"organization_id"`
}

// RegisterSLASchedule adds the minutely SLA sweep.
func RegisterSLASchedule(scheduler ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		slaSweepCron, asynq.NewTask(TaskSLASweep, []byte("{}")),
		asynq.Queue(QueueEvents), asynq.MaxRetry(0),
		asynq.Timeout(55*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register sla sweep: %w", err)
	}
	return nil
}

// EnqueueSLAResync queues a resync; repeated changes within a minute share
// one task.
func (c *Client) EnqueueSLAResync(ctx context.Context, orgID int64) error {
	body, err := json.Marshal(SLAResyncPayload{OrganizationID: orgID})
	if err != nil {
		return err
	}
	_, err = c.EnqueueContext(ctx, asynq.NewTask(TaskSLAResync, body),
		asynq.Queue(QueueEvents), asynq.MaxRetry(3), asynq.Timeout(10*time.Minute),
		asynq.TaskID("sla-resync:"+strconv.FormatInt(orgID, 10)), asynq.ProcessIn(5*time.Second),
		asynq.Retention(time.Minute))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// SLASweepHandler runs the sweep.
func SLASweepHandler(sweep func(ctx context.Context) (int, error)) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		_, err := sweep(ctx)
		return err
	}
}

// SLAResyncHandler runs a resync.
func SLAResyncHandler(resync func(ctx context.Context, orgID int64) error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p SLAResyncPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID <= 0 {
			return fmt.Errorf("queue: sla resync payload: %w", asynq.SkipRetry)
		}
		return resync(ctx, p.OrganizationID)
	}
}

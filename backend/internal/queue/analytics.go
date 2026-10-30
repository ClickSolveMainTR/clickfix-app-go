package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
	"strconv"
	"time"
)

const TaskAnalyticsBackfill = "app:analytics:backfill"

func (c *Client) EnqueueAnalyticsBackfill(ctx context.Context, org int64) error {
	body, _ := json.Marshal(SLAResyncPayload{OrganizationID: org})
	_, err := c.EnqueueContext(ctx, asynq.NewTask(TaskAnalyticsBackfill, body), asynq.Queue(QueueEvents), asynq.MaxRetry(3), asynq.Timeout(30*time.Minute), asynq.Unique(time.Minute), asynq.TaskID("analytics-backfill:"+strconv.FormatInt(org, 10)))
	if err == asynq.ErrTaskIDConflict || err == asynq.ErrDuplicateTask {
		return nil
	}
	return err
}
func AnalyticsBackfillHandler(fn func(context.Context, int64) error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p SLAResyncPayload
		if json.Unmarshal(t.Payload(), &p) != nil || p.OrganizationID <= 0 {
			return fmt.Errorf("analytics: invalid payload: %w", asynq.SkipRetry)
		}
		return fn(ctx, p.OrganizationID)
	}
}

package queue

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

// finalReportWindow throttles tracker events per task type: a failing
// dependency produces one event per window, not one per task.
const finalReportWindow = 10 * time.Minute

// finalAttempt reports whether a failed attempt will not be retried.
func finalAttempt(ctx context.Context, err error) (final bool, retried, maxRetry int) {
	retried, _ = asynq.GetRetryCount(ctx)
	maxRetry, _ = asynq.GetMaxRetry(ctx)
	return retried >= maxRetry || errors.Is(err, asynq.SkipRetry), retried, maxRetry
}

// handleTaskFailure logs a failed attempt (WARN while it will be retried,
// ERROR on the last attempt) and reports only last attempts.
func handleTaskFailure(ctx context.Context, log *slog.Logger, task *asynq.Task, err error) {
	final, retried, maxRetry := finalAttempt(ctx, err)
	attrs := []any{"type", task.Type(), "retry", retried, "max_retry", maxRetry, "error", observability.ScrubString(err.Error())}
	if !final {
		log.WarnContext(ctx, "queue_task_retry", attrs...)
		return
	}
	log.ErrorContext(ctx, "queue_task_failed", attrs...)
	reportTaskFailure(ctx, task, err)
}

// reportTaskFailure sends a task that failed for good to the error tracker,
// grouped per task type and throttled per type. Attempts that will be
// retried are not reported (they are logged at WARN).
func reportTaskFailure(ctx context.Context, task *asynq.Task, err error) {
	if !observability.Enabled() || task == nil || err == nil {
		return
	}
	final, retried, maxRetry := finalAttempt(ctx, err)
	if !final || !observability.Allow("queue-task:"+task.Type(), finalReportWindow) {
		return
	}
	queueName, _ := asynq.GetQueueName(ctx)
	observability.CaptureError(ctx, err, observability.Options{
		Level:       sentry.LevelError,
		Transaction: "task " + task.Type(),
		Fingerprint: []string{"queue-task", task.Type()},
		Tags: map[string]string{
			"task_type":     task.Type(),
			"request_id":    reqid.From(ctx),
			"queue":         queueName,
			"retry":         strconv.Itoa(retried),
			"max_retry":     strconv.Itoa(maxRetry),
			"final_attempt": "true",
		},
	})
}

// retryDelay is exponential backoff with jitter for notification delivery
// (30s, 1m, 2m, 4m ... capped at 30m) so a blocked SMTP port is not hit in
// a tight loop; other tasks keep the asynq default.
func retryDelay(n int, err error, task *asynq.Task) time.Duration {
	if task != nil && task.Type() == TaskNotificationDeliver {
		return notificationRetryDelay(n)
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

func notificationRetryDelay(n int) time.Duration {
	n = min(max(n, 0), 6)
	d := 30 * time.Second << n
	if d > 30*time.Minute {
		d = 30 * time.Minute
	}
	// Up to 20% jitter spreads retries of a burst of failed rows.
	return d + time.Duration(rand.Int64N(int64(d/5)+1)) // #nosec G404 -- jitter, not security
}

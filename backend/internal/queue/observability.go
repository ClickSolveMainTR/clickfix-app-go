package queue

import (
	"context"
	"maps"

	"github.com/hibiken/asynq"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
)

// WithRequestID returns task with the correlation id from ctx stored in the Asynq
// task headers (X-Request-ID), so the worker can log it. Without an id in ctx the
// task is returned unchanged. Options given to the task constructor are not kept,
// so pass queue/retry options to Enqueue (as every caller does).
func WithRequestID(ctx context.Context, task *asynq.Task) *asynq.Task {
	id := reqid.From(ctx)
	if task == nil || id == "" {
		return task
	}
	headers := maps.Clone(task.Headers())
	if headers == nil {
		headers = map[string]string{}
	}
	headers[reqid.Header] = id
	return asynq.NewTaskWithHeaders(task.Type(), task.Payload(), headers)
}

// TaskRequestID returns the correlation id carried by a task, or "".
func TaskRequestID(task *asynq.Task) string {
	if task == nil {
		return ""
	}
	id := task.Headers()[reqid.Header]
	if !reqid.Valid(id) {
		return ""
	}
	return id
}

// taskContext restores the task's request id into ctx, so handlers' *Context logs and
// any task they enqueue keep the same correlation id.
func taskContext(ctx context.Context, task *asynq.Task) context.Context {
	return reqid.With(ctx, TaskRequestID(task))
}

func observeMiddleware(next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error { return next.ProcessTask(ctxWithID(ctx, t), t) })
}
func ctxWithID(ctx context.Context, t *asynq.Task) context.Context { return taskContext(ctx, t) }

package queue

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
	"github.com/hibiken/asynq"
	"testing"
)

func TestTaskRequestIDRoundtripAndValidation(t *testing.T) {
	t.Parallel()
	ctx := reqid.With(context.Background(), "request-42")
	task := WithRequestID(ctx, asynq.NewTask("test", nil))
	if got := reqid.From(taskContext(context.Background(), task)); got != "request-42" {
		t.Fatalf("lost id %q", got)
	}
	bad := asynq.NewTaskWithHeaders("test", nil, map[string]string{reqid.Header: "bad\nheader"})
	if TaskRequestID(bad) != "" {
		t.Fatal("invalid task header accepted")
	}
}

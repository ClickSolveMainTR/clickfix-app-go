package queue

import (
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestNotificationRetryDelayBacksOffAndCaps(t *testing.T) {
	t.Parallel()
	task := asynq.NewTask(TaskNotificationDeliver, nil)
	prev := time.Duration(0)
	for n := 0; n < 5; n++ {
		d := retryDelay(n, errors.New("x"), task)
		base := 30 * time.Second << n
		if d < base || d > base+base/5 {
			t.Fatalf("retry %d: delay %s outside [%s, %s]", n, d, base, base+base/5)
		}
		if d <= prev {
			t.Fatalf("retry %d: delay %s did not grow (prev %s)", n, d, prev)
		}
		prev = d
	}
	if d := retryDelay(50, errors.New("x"), task); d > 36*time.Minute {
		t.Fatalf("delay not capped: %s", d)
	}
}

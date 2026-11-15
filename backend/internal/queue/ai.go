package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const TaskKnowledgeSync = "app:knowledge:sync"

const TaskRecordingTranscribe = "app:recording:transcribe"
const TaskRecordingTranscriptSweep = "app:recording:transcript_sweep"

const TaskAIReconcile = "app:ai:reconcile"

// TaskAIEvaluationSweep reruns the evaluation set of organizations whose
// agent config, reply model or knowledge changed (CF-220, opt-in watch).
const TaskAIEvaluationSweep = "app:ai:evaluation_sweep"

func RegisterAISchedule(scheduler ScheduleRegistrar) error {
	_, err := scheduler.Register("@every 1m", asynq.NewTask(TaskAIReconcile, []byte(`{}`)), asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(50*time.Second), asynq.Unique(50*time.Second))
	if err != nil {
		return fmt.Errorf("queue: register AI reconciliation: %w", err)
	}
	if _, err = scheduler.Register("@every 1m", asynq.NewTask(TaskKnowledgeSync, []byte(`{}`)), asynq.Queue(QueueSync), asynq.MaxRetry(0), asynq.Timeout(4*time.Minute), asynq.Unique(4*time.Minute)); err != nil {
		return fmt.Errorf("queue: register knowledge sync: %w", err)
	}
	if _, err = scheduler.Register("@every 5m", asynq.NewTask(TaskAIEvaluationSweep, []byte(`{}`)), asynq.Queue(QueueMaintenance), asynq.MaxRetry(0), asynq.Timeout(9*time.Minute), asynq.Unique(9*time.Minute)); err != nil {
		return fmt.Errorf("queue: register evaluation sweep: %w", err)
	}
	if _, err = scheduler.Register("@every 1m", asynq.NewTask(TaskRecordingTranscriptSweep, []byte(`{}`)), asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(50*time.Second), asynq.Unique(50*time.Second)); err != nil {
		return fmt.Errorf("queue: register transcription sweep: %w", err)
	}
	return nil
}

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

// History pipeline (CF-217): the sweep processes due runs and deletes raw
// files; the nightly task queues a run over yesterday's closed conversations.
const (
	TaskHistorySweep   = "app:history:sweep"
	TaskHistoryNightly = "app:history:nightly"
)

// TaskVectorReembed rebuilds vectors of an older embedding model (CF-214).
const TaskVectorReembed = "app:ai:vector_reembed"

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
	if _, err = scheduler.Register("@every 1m", asynq.NewTask(TaskHistorySweep, []byte(`{}`)), asynq.Queue(QueueSync), asynq.MaxRetry(0), asynq.Timeout(25*time.Minute), asynq.Unique(25*time.Minute)); err != nil {
		return fmt.Errorf("queue: register history sweep: %w", err)
	}
	// 01:17 UTC daily, off the hour so it does not pile up with other jobs.
	if _, err = scheduler.Register("17 1 * * *", asynq.NewTask(TaskHistoryNightly, []byte(`{}`)), asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(5*time.Minute), asynq.Unique(time.Hour)); err != nil {
		return fmt.Errorf("queue: register history nightly: %w", err)
	}
	if _, err = scheduler.Register("@every 5m", asynq.NewTask(TaskVectorReembed, []byte(`{}`)), asynq.Queue(QueueMaintenance), asynq.MaxRetry(0), asynq.Timeout(4*time.Minute), asynq.Unique(4*time.Minute)); err != nil {
		return fmt.Errorf("queue: register vector re-embedding: %w", err)
	}
	return nil
}

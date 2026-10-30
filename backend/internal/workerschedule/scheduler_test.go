package workerschedule

import (
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/developerplatform"
	authuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	compliance "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	sso "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/hibiken/asynq"
)

type recorder map[string]string

func (r recorder) Register(_ string, task *asynq.Task, opts ...asynq.Option) (string, error) {
	name := "default"
	for _, o := range opts {
		if o.Type() == asynq.QueueOpt {
			name = o.Value().(string)
		}
	}
	r[task.Type()] = name
	return task.Type(), nil
}

func TestPeriodicTaskRouting(t *testing.T) {
	cfg := config.Config{}
	cfg.Channels.HealthInterval = time.Minute
	cfg.Channels.ReconcileInterval = time.Minute
	got := recorder{}
	if err := Register(cfg, got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		retention.Task:        queue.QueueMaintenance,
		queue.TaskUptimeSweep: queue.QueueSync, queue.TaskOncallSweep: queue.QueueEvents, queue.TaskReportsSweep: queue.QueueExports, queue.TaskIntegrationTriggerPurge: queue.QueueMaintenance,
		queue.TaskAppointmentReminders: queue.QueueEvents,
		queue.TaskChannelHealth:        queue.QueueSync, queue.TaskChannelReconcile: queue.QueueSync, queue.TaskIMAPTick: queue.QueueSync, queue.TaskNetgsmTick: queue.QueueSync,
		queue.TaskVideoSweep: queue.QueueEvents, queue.TaskVideoRecordingPurge: queue.QueueMaintenance,
		queue.TaskBillingReconciliation: queue.QueueSync, queue.TaskBillingProviderSync: queue.QueueSync,
		queue.TaskRecordingTranscriptSweep: queue.QueueMaintenance,
		queue.TaskKnowledgeSync:            queue.QueueSync, queue.TaskPushReceipts: queue.QueueSync, developerplatform.TaskEngineeringSync: queue.QueueSync,
		queue.TaskBillingRenewal: queue.QueueMaintenance, queue.TaskBillingLifecycle: queue.QueueMaintenance, queue.TaskAIReconcile: queue.QueueMaintenance,
		queue.TaskPresenceSweep: queue.QueueMaintenance, queue.TaskCustomerHealthSweep: queue.QueueMaintenance,
		queue.TaskRoutingSweep: queue.QueueEvents, queue.TaskSLASweep: queue.QueueEvents, queue.TaskConversationTick: queue.QueueMessaging,
		queue.TaskNotificationDigest: queue.QueueNotifications, developerplatform.TaskEngineering: queue.QueueMaintenance,
		developerplatform.TaskWebhooks: queue.QueueMaintenance, developerplatform.TaskScan: queue.QueueMaintenance, developerplatform.TaskCleanup: queue.QueueMaintenance,
		authuc.TaskAccountPurge: queue.QueueMaintenance, compliance.TaskSweep: queue.QueueMaintenance, sso.TaskCleanup: queue.QueueMaintenance, eventdispatch.TaskPurgeConsumptions: queue.QueueMaintenance,
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected schedules: %v", got)
	}
	for typ, name := range want {
		if got[typ] != name {
			t.Errorf("%s routed to %q, want %q", typ, got[typ], name)
		}
	}
}

func TestSyncOnlyWorkerDoesNotStartScheduler(t *testing.T) {
	cfg := config.Config{Queue: config.QueueConfig{Enabled: true, Queues: queue.QueueSync, SchedulerEnabled: false}, Redis: config.RedisConfig{Addr: "invalid:0"}}
	scheduler, err := Start(cfg, nil)
	if err != nil || scheduler != nil {
		t.Fatalf("disabled scheduler created: %v %v", scheduler, err)
	}
}

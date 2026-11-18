package queue

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/hibiken/asynq"
	"testing"
	"time"
)

func TestIMAPPollAndOutboundRouting(t *testing.T) {
	redis := miniredis.RunT(t)
	c := NewClient(config.RedisConfig{Addr: redis.Addr()})
	defer func() { _ = c.Close() }()
	if err := c.EnqueueIMAPPoll(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redis.Addr()})
	defer func() { _ = inspector.Close() }()
	tasks, err := inspector.ListPendingTasks(QueueSync)
	if err != nil || len(tasks) != 1 || tasks[0].Type != TaskIMAPPoll {
		t.Fatalf("poll routing: %v %v", tasks, err)
	}
	for _, tc := range []struct {
		typ   string
		task  *asynq.Task
		opts  []asynq.Option
		queue string
	}{
		{TaskConversationSend, asynq.NewTask(TaskConversationSend, nil), ConversationSendOptions(0), QueueMessaging},
		{TaskPushSend, asynq.NewTask(TaskPushSend, nil), PushSendOptions(), QueueNotifications},
		{TaskNotificationDeliver, asynq.NewTask(TaskNotificationDeliver, nil), []asynq.Option{asynq.Queue(QueueNotifications)}, QueueNotifications},
	} {
		info, err := c.Enqueue(tc.task, tc.opts...)
		if err != nil {
			t.Fatal(err)
		}
		if info.Queue != tc.queue || info.Queue == QueueSync {
			t.Errorf("%s routed to %s", tc.typ, info.Queue)
		}
	}
}
func TestWorkerQueueSelection(t *testing.T) {
	defaults := workerQueues("")
	if defaults[QueueSync] != 1 || defaults[QueueMessaging] != 3 || defaults[QueueEvents] != 3 {
		t.Fatal(defaults)
	}
	// CF-251: without a dedicated ai-reply service (local development) the
	// default worker serves the AI lane, ahead of every other queue.
	for name, weight := range defaults {
		if name != QueueAI && weight >= defaults[QueueAI] {
			t.Fatalf("the AI lane must outweigh %s: %v", name, defaults)
		}
	}
	sync := workerQueues("sync:2")
	if len(sync) != 1 || sync[QueueSync] != 2 {
		t.Fatal(sync)
	}
}

func TestSyncWorkerConsumesOnlySyncQueue(t *testing.T) {
	redis := miniredis.RunT(t)
	cfg := config.Config{Redis: config.RedisConfig{Addr: redis.Addr()}, Queue: config.QueueConfig{Queues: QueueSync, Concurrency: 1}}
	processed := make(chan struct{}, 1)
	worker := NewWorker(cfg, nil, nil).Handle(TaskIMAPPoll, func(context.Context, *asynq.Task) error { processed <- struct{}{}; return nil })
	client := NewClient(cfg.Redis)
	defer func() { _ = client.Close() }()
	send, _ := NewConversationSendTask(1, 2)
	if _, err := client.Enqueue(send, ConversationSendOptions(0)...); err != nil {
		t.Fatal(err)
	}
	if err := client.EnqueueIMAPPoll(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := worker.server.Start(worker.mux); err != nil {
		t.Fatal(err)
	}
	defer worker.Shutdown()
	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("sync job was not consumed")
	}
	inspector := asynq.NewInspector(RedisOpt(cfg.Redis))
	defer func() { _ = inspector.Close() }()
	pending, err := inspector.ListPendingTasks(QueueMessaging)
	if err != nil || len(pending) != 1 || pending[0].Type != TaskConversationSend {
		t.Fatalf("sync worker touched customer queue: %v %v", pending, err)
	}
}

// CF-251: AI and automatic messages are delivered on the AI lane; ordinary
// sends keep the messaging queue. A worker that serves only "ai" (the
// ai-reply service) delivers the first and never touches the second.
func TestAILaneCarriesPrioritySends(t *testing.T) {
	redis := miniredis.RunT(t)
	cfg := config.Config{Redis: config.RedisConfig{Addr: redis.Addr()}, Queue: config.QueueConfig{Queues: QueueAI, Concurrency: 2, TaskCheckInterval: 20 * time.Millisecond}}
	client := NewClient(cfg.Redis)
	defer func() { _ = client.Close() }()
	if err := client.EnqueueConversationSendPriority(context.Background(), 1, 7); err != nil {
		t.Fatal(err)
	}
	if err := client.EnqueueConversationSend(context.Background(), 1, 8, 0); err != nil {
		t.Fatal(err)
	}
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redis.Addr()})
	defer func() { _ = inspector.Close() }()
	for name, want := range map[string]int{QueueAI: 1, QueueMessaging: 1} {
		if tasks, err := inspector.ListPendingTasks(name); err != nil || len(tasks) != want || tasks[0].Type != TaskConversationSend {
			t.Fatalf("%s: %v %v", name, tasks, err)
		}
	}
	delivered := make(chan int64, 2)
	worker := NewWorker(cfg, nil, nil).WithConversationSend(func(_ context.Context, _, messageID int64, _, _ int) error {
		delivered <- messageID
		return nil
	})
	if err := worker.server.Start(worker.mux); err != nil {
		t.Fatal(err)
	}
	defer worker.Shutdown()
	select {
	case id := <-delivered:
		if id != 7 {
			t.Fatalf("delivered message %d", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the AI lane send was not delivered")
	}
	select {
	case id := <-delivered:
		t.Fatalf("an ai-only worker delivered messaging-queue message %d", id)
	case <-time.After(200 * time.Millisecond):
	}
}

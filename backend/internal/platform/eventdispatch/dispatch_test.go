package eventdispatch_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for event dispatch integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, db.New(pool)
}

func cleanup(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Cleanup(func() {
		ctx := context.Background()
		for _, id := range ids {
			_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE uuid = $1", id)
			_, _ = pool.Exec(ctx, "DELETE FROM domain_event_log WHERE event_id = $1", id)
			_, _ = pool.Exec(ctx, "DELETE FROM event_consumptions WHERE event_id = $1", id)
			_, _ = pool.Exec(ctx, "DELETE FROM event_dead_letters WHERE event_id = $1", id)
		}
	})
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestOutboxWriteFollowsTransaction: the outbox row exists only when the
// business transaction commits, and re-enqueueing the same event keeps one row.
func TestOutboxWriteFollowsTransaction(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	store := outbox.NewStore(pool, q)

	rolledBack := events.New(events.TicketCreated)
	committed := events.New(events.TicketCreated).WithIdempotencyKey(uuid.NewString())
	cleanup(t, pool, rolledBack.EventID, committed.EventID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(dbtx.WithTx(ctx, tx), nil, rolledBack); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if n := count(t, pool, "SELECT count(*) FROM outbox_events WHERE uuid = $1", rolledBack.EventID); n != 0 {
		t.Fatalf("rolled back event persisted: %d", n)
	}

	for range 2 {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Enqueue(ctx, tx, committed); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, "SELECT count(*) FROM outbox_events WHERE uuid = $1", committed.EventID); n != 1 {
		t.Fatalf("idempotent enqueue rows = %d", n)
	}
	if err := store.Enqueue(ctx, nil, committed); err == nil {
		t.Fatal("enqueue without a transaction must fail")
	}
}

type fakeEnqueuer struct {
	mu    sync.Mutex
	tasks map[string]*asynq.Task
}

func (f *fakeEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ""
	for _, o := range opts {
		if o.Type() == asynq.TaskIDOpt {
			id = o.Value().(string)
		}
	}
	if _, ok := f.tasks[id]; ok {
		return nil, fmt.Errorf("queue: enqueue: %w", asynq.ErrTaskIDConflict)
	}
	f.tasks[id] = task
	return &asynq.TaskInfo{ID: id}, nil
}

// TestDrainLogsAndFansOutOnce: draining writes the event log and one task per
// consumer; a second fan-out of the same event is absorbed by the TaskID.
func TestDrainLogsAndFansOutOnce(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	reg, err := eventdispatch.NewRegistry(
		eventdispatch.Consumer{Name: "test-tickets", Events: []string{"ticket.*"}, Handle: func(context.Context, events.Event) error { return nil }},
		eventdispatch.Consumer{Name: "test-sla", Events: []string{events.SLABreached}, Handle: func(context.Context, events.Event) error { return nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	enq := &fakeEnqueuer{tasks: map[string]*asynq.Task{}}
	dispatcher := eventdispatch.NewDispatcher(reg, enq, q, nil)
	store := outbox.NewStore(pool, q)
	pub := outbox.NewPublisher(store, events.NewBus(nil), nil, outbox.WithSink(dispatcher), outbox.WithBatchSize(500))

	ev, err := events.NewDomain(events.TicketCreated, 0, events.TicketPayload{TicketID: 5, TicketUUID: uuid.New(), Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	ev.TenantID = nil
	cleanup(t, pool, ev.EventID)
	tx, _ := pool.Begin(ctx)
	if err := store.Enqueue(ctx, tx, ev); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)

	for i := 0; i < 20 && count(t, pool, "SELECT count(*) FROM outbox_events WHERE uuid = $1 AND status = 'published'", ev.EventID) == 0; i++ {
		if _, err := pub.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, "SELECT count(*) FROM domain_event_log WHERE event_id = $1 AND event_name = 'ticket.created'", ev.EventID); n != 1 {
		t.Fatalf("event log rows = %d", n)
	}
	want := "test-tickets:" + ev.EventID.String()
	if _, ok := enq.tasks[want]; !ok || len(enq.tasks) != 1 {
		t.Fatalf("tasks = %v, want only %s", keys(enq.tasks), want)
	}
	if err := dispatcher.Fanout(ctx, ev); err != nil {
		t.Fatalf("duplicate fan-out must be absorbed: %v", err)
	}
}

func keys(m map[string]*asynq.Task) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestProcessorRunsConsumerOncePerEvent: a duplicate delivery is skipped and a
// failing handler leaves no marker so the retry runs it again.
func TestProcessorRunsConsumerOncePerEvent(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	var calls atomic.Int32
	fail := atomic.Bool{}
	fail.Store(true)
	name := "test-once-" + uuid.NewString()[:8]
	reg, _ := eventdispatch.NewRegistry(eventdispatch.Consumer{
		Name:   name,
		Events: []string{events.ConversationCreated},
		Handle: func(ctx context.Context, ev events.Event) error {
			calls.Add(1)
			if _, ok := dbtx.Tx(ctx); !ok {
				return errors.New("consumer transaction missing")
			}
			if fail.Load() {
				return errors.New("transient")
			}
			return nil
		},
	})
	proc := eventdispatch.NewProcessor(reg, pool, q, nil)
	ev := events.New(events.ConversationCreated)
	cleanup(t, pool, ev.EventID)

	if err := proc.Process(ctx, name, ev); err == nil {
		t.Fatal("expected handler error")
	}
	if n := count(t, pool, "SELECT count(*) FROM event_consumptions WHERE event_id = $1", ev.EventID); n != 0 {
		t.Fatalf("failed handler left marker")
	}
	fail.Store(false)
	for range 3 {
		if err := proc.Process(ctx, name, ev); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("handler calls = %d, want 2 (one failure, one success)", got)
	}
}

func redisOpt(t *testing.T) asynq.RedisClientOpt {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("TEST_REDIS_URL is required for the asynq end-to-end test")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dbn, _ := strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
	pw, _ := u.User.Password()
	return asynq.RedisClientOpt{Addr: u.Host, Password: pw, DB: dbn}
}

// TestAsynqEndToEndRetryAndDeadLetter runs real Asynq: a flaky consumer
// succeeds after retries (once), a broken one lands in event_dead_letters and
// replays.
func TestAsynqEndToEndRetryAndDeadLetter(t *testing.T) {
	pool, q := testPool(t)
	opt := redisOpt(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	var flakyCalls, flakyDone, brokenCalls atomic.Int32
	broken := atomic.Bool{}
	broken.Store(true)
	reg, err := eventdispatch.NewRegistry(
		eventdispatch.Consumer{
			Name: "flaky-" + suffix, Events: []string{events.MessageReceived}, MaxRetry: 5,
			Handle: func(context.Context, events.Event) error {
				if flakyCalls.Add(1) < 3 {
					return errors.New("try again")
				}
				flakyDone.Add(1)
				return nil
			},
		},
		eventdispatch.Consumer{
			Name: "broken-" + suffix, Events: []string{events.MessageReceived}, MaxRetry: 1,
			Handle: func(context.Context, events.Event) error {
				brokenCalls.Add(1)
				if broken.Load() {
					return errors.New("always fails")
				}
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() {
		_, _ = inspector.DeleteAllCompletedTasks(queue.QueueEvents)
		_, _ = inspector.DeleteAllArchivedTasks(queue.QueueEvents)
		_ = inspector.Close()
	})

	proc := eventdispatch.NewProcessor(reg, pool, q, nil)
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency:              2,
		Queues:                   map[string]int{queue.QueueEvents: 1},
		RetryDelayFunc:           func(int, error, *asynq.Task) time.Duration { return 50 * time.Millisecond },
		DelayedTaskCheckInterval: 100 * time.Millisecond,
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(eventdispatch.TaskConsume, proc.ProcessTask)
	if err := srv.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Shutdown)

	dispatcher := eventdispatch.NewDispatcher(reg, client, q, nil)
	ev := events.New(events.MessageReceived).WithIdempotencyKey(suffix)
	cleanup(t, pool, ev.EventID)
	if err := dispatcher.Fanout(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Fanout(ctx, ev); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		// The consumption marker is committed after the handler returns, so
		// wait for the row itself rather than the handler's counter.
		return flakyDone.Load() == 1 &&
			count(t, pool, "SELECT count(*) FROM event_dead_letters WHERE event_id = $1", ev.EventID) == 1 &&
			count(t, pool, "SELECT count(*) FROM event_consumptions WHERE event_id = $1", ev.EventID) >= 1
	})
	if got := brokenCalls.Load(); got != 2 {
		t.Fatalf("broken consumer calls = %d, want 2 (MaxRetry 1)", got)
	}
	if n := count(t, pool, "SELECT count(*) FROM event_consumptions WHERE event_id = $1", ev.EventID); n != 1 {
		t.Fatalf("consumption markers = %d, want 1 (flaky only)", n)
	}

	var deadID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM event_dead_letters WHERE event_id = $1", ev.EventID).Scan(&deadID); err != nil {
		t.Fatal(err)
	}
	broken.Store(false)
	if err := dispatcher.ReplayDeadLetter(ctx, deadID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		return count(t, pool, "SELECT count(*) FROM event_consumptions WHERE event_id = $1", ev.EventID) == 2
	})
	if n := count(t, pool, "SELECT count(*) FROM event_dead_letters WHERE event_id = $1 AND replayed_at IS NOT NULL", ev.EventID); n != 1 {
		t.Fatalf("dead letter not marked replayed")
	}
	if flakyDone.Load() != 1 {
		t.Fatalf("flaky consumer completed %d times", flakyDone.Load())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met within 20s")
}

// Package eventdispatch delivers outbox events to durable consumers through
// Asynq.
//
// The outbox publisher calls Dispatcher.Fanout for every event. Fanout
// enqueues one task per matching consumer with TaskID "<consumer>:<event id>",
// so a replayed outbox row does not enqueue twice while the task is retained.
// Processor.ProcessTask runs the consumer inside a transaction that also
// inserts (consumer, event id) into event_consumptions: a duplicate delivery
// finds the row and is skipped, a failed handler rolls both back and Asynq
// retries with backoff. When the last retry fails the event is written to
// event_dead_letters; ReplayDeadLetter enqueues it again.
//
// Consumers must keep external side effects (HTTP, e-mail) idempotent or
// tolerant of a rare repeat: the marker commits after the handler returns,
// so a crash between the side effect and the commit replays it.
package eventdispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// TaskConsume delivers one event to one consumer.
	TaskConsume = "app:events:consume"
	// TaskPurgeConsumptions trims old idempotency markers.
	TaskPurgeConsumptions = "app:events:purge_consumptions"

	DefaultMaxRetry = 10
	DefaultTimeout  = 2 * time.Minute
	// TaskRetention keeps finished tasks so a re-fanned-out event hits the
	// TaskID conflict instead of a second enqueue.
	TaskRetention = 24 * time.Hour
	// ConsumptionRetention bounds event_consumptions; it must exceed the
	// longest retry schedule plus TaskRetention.
	ConsumptionRetention = 30 * 24 * time.Hour
)

// Handler processes one event. The context carries the consumer transaction
// (dbtx.Tx); repositories that read it commit together with the idempotency marker.
type Handler func(ctx context.Context, ev events.Event) error

// Consumer is a named durable subscriber.
type Consumer struct {
	// Name is stable: it is part of the TaskID and the idempotency key.
	Name string
	// Events are exact names or prefixes ending in "*" (conversation.*).
	Events   []string
	Handle   Handler
	MaxRetry int
	Timeout  time.Duration
	// Queue is the Asynq queue of this consumer's tasks; "" is the shared
	// events queue. A latency-bound consumer names its own queue so the
	// other consumers' backlog cannot delay it (CF-251).
	Queue string
}

func (c Consumer) matches(name string) bool {
	for _, pattern := range c.Events {
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if pattern == name {
			return true
		}
	}
	return false
}

// Registry holds the consumers known to this process. The API (fan-out) and
// the worker (processing) must register the same set.
type Registry struct {
	mu        sync.RWMutex
	consumers []Consumer
	byName    map[string]Consumer
}

// NewRegistry builds a registry with the given consumers.
func NewRegistry(consumers ...Consumer) (*Registry, error) {
	r := &Registry{byName: map[string]Consumer{}}
	for _, c := range consumers {
		if err := r.Register(c); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds a consumer. Names must be unique.
func (r *Registry) Register(c Consumer) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || strings.Contains(c.Name, ":") {
		return fmt.Errorf("eventdispatch: invalid consumer name %q", c.Name)
	}
	if c.Handle == nil || len(c.Events) == 0 {
		return fmt.Errorf("eventdispatch: consumer %s needs events and a handler", c.Name)
	}
	if c.MaxRetry <= 0 {
		c.MaxRetry = DefaultMaxRetry
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Queue = strings.TrimSpace(c.Queue); c.Queue == "" {
		c.Queue = queue.QueueEvents
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[c.Name]; ok {
		return fmt.Errorf("eventdispatch: consumer %s registered twice", c.Name)
	}
	r.byName[c.Name] = c
	r.consumers = append(r.consumers, c)
	return nil
}

// Match returns the consumers subscribed to name.
func (r *Registry) Match(name string) []Consumer {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Consumer{}
	for _, c := range r.consumers {
		if c.matches(name) {
			out = append(out, c)
		}
	}
	return out
}

// Get returns a consumer by name.
func (r *Registry) Get(name string) (Consumer, bool) {
	if r == nil {
		return Consumer{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byName[name]
	return c, ok
}

// TaskPayload is the body of TaskConsume.
type TaskPayload struct {
	EnqueuedAt time.Time       `json:"enqueued_at,omitempty"`
	Consumer   string          `json:"consumer"`
	EventName  string          `json:"event_name"`
	Envelope   json.RawMessage `json:"envelope"`
}

// Enqueuer is the Asynq client surface (queue.Client satisfies it).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Dispatcher fans events out to consumer tasks. It implements outbox.Sink.
type Dispatcher struct {
	reg *Registry
	enq Enqueuer
	q   *db.Queries
	log *slog.Logger
}

// NewDispatcher builds a dispatcher. q is only needed for ReplayDeadLetter.
func NewDispatcher(reg *Registry, enq Enqueuer, q *db.Queries, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{reg: reg, enq: enq, q: q, log: log}
}

var _ outbox.Sink = (*Dispatcher)(nil)

// Fanout enqueues one task per matching consumer. A TaskID conflict means the
// task already exists and counts as success.
func (d *Dispatcher) Fanout(_ context.Context, ev events.Event) error {
	if d == nil {
		return nil
	}
	consumers := d.reg.Match(ev.Name)
	if len(consumers) == 0 {
		return nil
	}
	if d.enq == nil {
		return fmt.Errorf("eventdispatch: %d consumers for %s but no queue client", len(consumers), ev.Name)
	}
	env, err := outbox.MarshalEnvelope(ev)
	if err != nil {
		return err
	}
	for _, c := range consumers {
		if err := d.enqueue(c, ev.Name, ev.EventID, env, ""); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) enqueue(c Consumer, eventName string, eventID uuid.UUID, env []byte, suffix string) error {
	body, err := json.Marshal(TaskPayload{EnqueuedAt: time.Now().UTC(), Consumer: c.Name, EventName: eventName, Envelope: env})
	if err != nil {
		return fmt.Errorf("eventdispatch: marshal task: %w", err)
	}
	taskID := c.Name + ":" + eventID.String() + suffix
	var trace struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(env, &trace)
	task := asynq.NewTask(TaskConsume, body)
	if reqid.Valid(trace.RequestID) {
		task = asynq.NewTaskWithHeaders(TaskConsume, body, map[string]string{reqid.Header: trace.RequestID})
	}
	_, err = d.enq.Enqueue(task,
		asynq.Queue(c.Queue),
		asynq.TaskID(taskID),
		asynq.MaxRetry(c.MaxRetry),
		asynq.Timeout(c.Timeout),
		asynq.Retention(TaskRetention),
	)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return fmt.Errorf("eventdispatch: enqueue %s: %w", taskID, err)
	}
	if c.Name == "ai.conversation" {
		d.log.Info("ai_stage", "stage", "job_enqueued", "event_id", eventID, "consumer", c.Name)
	}
	return nil
}

// ReplayDeadLetter enqueues a dead-lettered event for its consumer again.
func (d *Dispatcher) ReplayDeadLetter(ctx context.Context, id int64) error {
	if d == nil || d.q == nil {
		return fmt.Errorf("eventdispatch: replay needs queries")
	}
	row, err := d.q.GetEventDeadLetter(ctx, id)
	if err != nil {
		return fmt.Errorf("eventdispatch: dead letter %d: %w", id, err)
	}
	c, ok := d.reg.Get(row.Consumer)
	if !ok {
		return fmt.Errorf("eventdispatch: consumer %s is not registered", row.Consumer)
	}
	suffix := ":replay:" + time.Now().UTC().Format("20060102T150405.000")
	if err := d.enqueue(c, row.EventName, row.EventID, row.Envelope, suffix); err != nil {
		return err
	}
	return d.q.MarkEventDeadLetterReplayed(ctx, id)
}

// Processor runs consumer tasks on the worker.
type Processor struct {
	reg  *Registry
	pool *pgxpool.Pool
	q    *db.Queries
	log  *slog.Logger
}

// NewProcessor builds the worker-side task handler.
func NewProcessor(reg *Registry, pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{reg: reg, pool: pool, q: q, log: log}
}

// Delivery is the timeline of the event a consumer is handling: when it
// happened, when its task was enqueued and when the handler started. A
// latency-bound consumer logs its end-to-end time from it (CF-251).
type Delivery struct {
	OccurredAt, EnqueuedAt, StartedAt time.Time
}

type deliveryKey struct{}

// WithDelivery attaches d to ctx.
func WithDelivery(ctx context.Context, d Delivery) context.Context {
	return context.WithValue(ctx, deliveryKey{}, d)
}

// DeliveryOf returns the timeline of the running consumer task, if any.
func DeliveryOf(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}

// ProcessTask is the asynq handler for TaskConsume.
func (p *Processor) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload TaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("eventdispatch: bad task payload: %v: %w", err, asynq.SkipRetry)
	}
	ev, err := outbox.UnmarshalEnvelope(payload.EventName, payload.Envelope)
	if err != nil {
		return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
	}
	c, ok := p.reg.Get(payload.Consumer)
	if !ok {
		// A consumer removed from code: nothing can process it, keep it visible.
		err := fmt.Errorf("eventdispatch: unknown consumer %s", payload.Consumer)
		p.deadLetter(ctx, payload, ev, err)
		return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
	}
	if c.Name == "ai.conversation" {
		receivedToStart := int64(0)
		queueWait := int64(0)
		if !ev.OccurredAt.IsZero() {
			receivedToStart = time.Since(ev.OccurredAt).Milliseconds()
		}
		if !payload.EnqueuedAt.IsZero() {
			queueWait = time.Since(payload.EnqueuedAt).Milliseconds()
		}
		p.log.InfoContext(ctx, "ai_stage", "stage", "job_started", "event_id", ev.EventID, "request_id", ev.RequestID,
			"received_at", ev.OccurredAt, "received_to_start_ms", receivedToStart, "queue_wait_ms", queueWait, "debounce_ms", 0, "queue", c.Queue)
	}
	ctx = WithDelivery(ctx, Delivery{OccurredAt: ev.OccurredAt, EnqueuedAt: payload.EnqueuedAt, StartedAt: time.Now()})
	err = p.process(ctx, c, ev)
	if err == nil {
		return nil
	}
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if errors.Is(err, asynq.SkipRetry) || retried >= maxRetry {
		p.deadLetter(ctx, payload, ev, err)
	}
	return err
}

// Process runs c for ev once per (consumer, event id). Exposed for tests and
// for synchronous replays; returns nil for a duplicate.
func (p *Processor) Process(ctx context.Context, consumer string, ev events.Event) error {
	c, ok := p.reg.Get(consumer)
	if !ok {
		return fmt.Errorf("eventdispatch: unknown consumer %s", consumer)
	}
	return p.process(ctx, c, ev)
}

func (p *Processor) process(ctx context.Context, c Consumer, ev events.Event) error {
	ctx = reqid.With(ctx, ev.RequestID)
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("eventdispatch: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claimed, err := p.q.WithTx(tx).ClaimEventConsumption(ctx, db.ClaimEventConsumptionParams{
		Consumer: c.Name,
		EventID:  ev.EventID,
	})
	if err != nil {
		return fmt.Errorf("eventdispatch: claim: %w", err)
	}
	if claimed == 0 {
		p.log.Info("event_consumer_duplicate_skipped", "consumer", c.Name, "event", ev.Name, "event_id", ev.EventID.String())
		return nil
	}
	if err := c.Handle(dbtx.WithTx(ctx, tx), ev); err != nil {
		return fmt.Errorf("eventdispatch: %s: %w", c.Name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("eventdispatch: commit: %w", err)
	}
	return nil
}

func (p *Processor) deadLetter(ctx context.Context, payload TaskPayload, ev events.Event, cause error) {
	retried, _ := asynq.GetRetryCount(ctx)
	params := db.UpsertEventDeadLetterParams{
		Consumer:  payload.Consumer,
		EventID:   ev.EventID,
		EventName: payload.EventName,
		Envelope:  payload.Envelope,
		Attempts:  safeconv.ToInt32(min(retried, math.MaxInt32-1) + 1),
		LastError: cause.Error(),
	}
	if ev.TenantID != nil {
		params.OrganizationID = pgtype.Int8{Int64: *ev.TenantID, Valid: true}
	}
	if _, err := p.q.UpsertEventDeadLetter(context.WithoutCancel(ctx), params); err != nil {
		p.log.Error("event_dead_letter_write_failed", "consumer", payload.Consumer, "event_id", ev.EventID.String(), "error", err)
		return
	}
	p.log.Error("event_dead_lettered", "consumer", payload.Consumer, "event", payload.EventName, "event_id", ev.EventID.String(), "error", cause)
}

// PurgeConsumptions deletes idempotency markers older than ConsumptionRetention.
func (p *Processor) PurgeConsumptions(ctx context.Context, _ *asynq.Task) error {
	n, err := p.q.PurgeEventConsumptionsBefore(ctx, pgtype.Timestamptz{Time: time.Now().UTC().Add(-ConsumptionRetention), Valid: true})
	if err != nil {
		return fmt.Errorf("eventdispatch: purge consumptions: %w", err)
	}
	if n > 0 {
		p.log.Info("event_consumptions_purged", "count", n)
	}
	return nil
}

// WebhookSink receives every webhook-eligible organization event; the
// outgoing webhook module (CF-95) implements it by signing and POSTing to
// the organization's subscribed endpoints.
type WebhookSink interface {
	DeliverEvent(ctx context.Context, ev events.Event) error
}

// WebhookConsumerName is the consumer name used for webhook fan-out.
const WebhookConsumerName = "webhooks"

// WebhookConsumer subscribes sink to every events.WebhookEvents name.
// Platform events without an organization are skipped.
func WebhookConsumer(sink WebhookSink) Consumer {
	return Consumer{
		Name:     WebhookConsumerName,
		Events:   events.WebhookEvents(),
		MaxRetry: 12,
		Timeout:  time.Minute,
		Handle: func(ctx context.Context, ev events.Event) error {
			if ev.TenantID == nil {
				return nil
			}
			return sink.DeliverEvent(ctx, ev)
		},
	}
}

// RegisterSchedule adds the daily consumption-marker purge.
func RegisterSchedule(scheduler queue.ScheduleRegistrar) error {
	if _, err := scheduler.Register(
		"@daily", asynq.NewTask(TaskPurgeConsumptions, []byte("{}")),
		asynq.Queue(queue.QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(10*time.Minute), asynq.Unique(time.Hour),
	); err != nil {
		return fmt.Errorf("eventdispatch: register purge: %w", err)
	}
	return nil
}

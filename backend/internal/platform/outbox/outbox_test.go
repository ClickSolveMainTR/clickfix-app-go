package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
)

func TestEnqueueAndDrain(t *testing.T) {
	mem := outbox.NewMemory()
	bus := &recordingBus{}
	pub := outbox.NewMemoryPublisher(mem, bus, nil)

	tid := int64(7)
	ev := events.New("customers.created").
		WithOrganization(tid).
		WithPayload(map[string]any{"customer_event_uuid": "ce-1"})

	if err := mem.Enqueue(context.Background(), nil, ev); err != nil {
		t.Fatal(err)
	}
	if n := len(mem.Pending()); n != 1 {
		t.Fatalf("pending=%d", n)
	}

	n, err := pub.Drain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("published=%d", n)
	}
	if len(bus.names()) != 1 || bus.names()[0] != "customers.created" {
		t.Fatalf("bus=%v", bus.names())
	}
	if len(mem.Pending()) != 0 {
		t.Fatalf("expected empty pending after drain")
	}
	rows := mem.All()
	if len(rows) != 1 || rows[0].Status != outbox.StatusPublished {
		t.Fatalf("rows=%+v", rows)
	}
	if bus.evts[0].Payload["customer_event_uuid"] != "ce-1" {
		t.Fatalf("payload=%v", bus.evts[0].Payload)
	}
}

func TestForcedFailThenRetry(t *testing.T) {
	mem := outbox.NewMemory()
	bus := &flakyBus{failTimes: 2}
	pub := outbox.NewMemoryPublisher(mem, bus, nil, outbox.WithMaxAttempts(5), outbox.WithLease(time.Millisecond))

	if err := mem.Enqueue(context.Background(), nil, events.New("messages.received").WithPayload(map[string]any{
		"message_uuid": "m-1",
	})); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		n, err := pub.Drain(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("drain %d: expected 0 published, got %d", i, n)
		}
		mem.ForceAvailable()
	}

	rows := mem.All()
	if len(rows) != 1 || rows[0].Status != outbox.StatusPending || rows[0].Attempts != 2 {
		t.Fatalf("after fails: %+v", rows[0])
	}

	n, err := pub.Drain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected success publish, got %d", n)
	}
	rows = mem.All()
	if rows[0].Status != outbox.StatusPublished {
		t.Fatalf("status=%s", rows[0].Status)
	}
	if bus.successes != 1 {
		t.Fatalf("successes=%d", bus.successes)
	}
}

func TestMaxAttemptsMarksFailed(t *testing.T) {
	mem := outbox.NewMemory()
	bus := &flakyBus{failTimes: 100}
	pub := outbox.NewMemoryPublisher(mem, bus, nil, outbox.WithMaxAttempts(3), outbox.WithLease(time.Millisecond))

	if err := mem.Enqueue(context.Background(), nil, events.New("messages.sent")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, _ = pub.Drain(context.Background())
		mem.ForceAvailable()
	}
	rows := mem.All()
	if len(rows) != 1 || rows[0].Status != outbox.StatusFailed || rows[0].Attempts != 3 {
		t.Fatalf("expected failed after max attempts: %+v", rows)
	}
}

type recordingBus struct {
	mu   sync.Mutex
	evts []events.Event
}

func (b *recordingBus) Publish(_ context.Context, ev events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evts = append(b.evts, ev)
	return nil
}

func (b *recordingBus) Subscribe(string, events.Handler) {}

func (b *recordingBus) names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.evts))
	for _, e := range b.evts {
		out = append(out, e.Name)
	}
	return out
}

type flakyBus struct {
	mu        sync.Mutex
	failTimes int
	calls     int
	successes int
}

func (b *flakyBus) Publish(_ context.Context, _ events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	if b.calls <= b.failTimes {
		return errors.New("forced publish failure")
	}
	b.successes++
	return nil
}

func (b *flakyBus) Subscribe(string, events.Handler) {}

func TestEnqueueSameEventIDKeepsOneRow(t *testing.T) {
	mem := outbox.NewMemory()
	ev := events.New("message.received").WithOrganization(3).WithIdempotencyKey("ext-1")
	for range 2 {
		if err := mem.Enqueue(context.Background(), nil, ev); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(mem.All()); n != 1 {
		t.Fatalf("rows=%d", n)
	}
}

func TestSinkReceivesEventAndErrorsRetry(t *testing.T) {
	mem := outbox.NewMemory()
	sink := &flakySink{failTimes: 1}
	pub := outbox.NewMemoryPublisher(mem, &recordingBus{}, nil, outbox.WithSink(sink), outbox.WithLease(time.Millisecond))
	org := int64(9)
	if err := mem.Enqueue(context.Background(), nil, events.New("ticket.created").WithOrganization(org)); err != nil {
		t.Fatal(err)
	}
	if n, _ := pub.Drain(context.Background()); n != 0 {
		t.Fatalf("first drain published %d, want 0 (sink failed)", n)
	}
	mem.ForceAvailable()
	if n, _ := pub.Drain(context.Background()); n != 1 {
		t.Fatalf("second drain published %d", n)
	}
	if len(sink.got) != 2 || sink.got[1].TenantID == nil || *sink.got[1].TenantID != org {
		t.Fatalf("sink got %+v", sink.got)
	}
	if logged := mem.Logged(); len(logged) != 1 || logged[0].Name != "ticket.created" {
		t.Fatalf("event log = %+v", logged)
	}
}

type flakySink struct {
	mu        sync.Mutex
	failTimes int
	got       []events.Event
}

func (s *flakySink) Fanout(_ context.Context, ev events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, ev)
	if len(s.got) <= s.failTimes {
		return errors.New("queue down")
	}
	return nil
}

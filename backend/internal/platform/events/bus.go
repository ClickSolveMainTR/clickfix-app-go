package events

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Handler processes a single event.
type Handler func(ctx context.Context, event Event) error

// Bus is the publish/subscribe surface used by domain modules and listeners.
//
// Subscribe accepts an exact event name (customers.created) or a prefix pattern
// ending with "*" (customers.*). Prefix handlers run for every matching publish.
//
// MVP semantics (MemoryBus):
//   - Synchronous: handlers run on the publisher goroutine, in registration order.
//   - Fail-soft: handler errors are logged; Publish still returns nil after
//     validation succeeds. Critical paths must assert handler effects in tests.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(nameOrPrefix string, handler Handler)
}

// ReliableBus supports retryable transactional consumers.
type ReliableBus interface {
	Bus
	SubscribeReliable(string, Handler)
}

// MemoryBus is an in-process synchronous bus.
type MemoryBus struct {
	mu       sync.RWMutex
	exact    map[string][]Handler
	critical map[string][]Handler
	prefixes []prefixSub
	log      *slog.Logger
}

type prefixSub struct {
	prefix  string
	handler Handler
}

// NewBus creates an in-memory event bus. log may be nil (uses slog.Default).
func NewBus(log *slog.Logger) *MemoryBus {
	if log == nil {
		log = slog.Default()
	}
	return &MemoryBus{
		exact:    make(map[string][]Handler),
		critical: make(map[string][]Handler),
		log:      log,
	}
}

// Subscribe registers a handler for an exact name or prefix pattern (suffix "*").
func (b *MemoryBus) Subscribe(nameOrPrefix string, handler Handler) {
	if b == nil || handler == nil {
		return
	}
	nameOrPrefix = strings.TrimSpace(nameOrPrefix)
	if nameOrPrefix == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if strings.HasSuffix(nameOrPrefix, "*") {
		prefix := strings.TrimSuffix(nameOrPrefix, "*")
		b.prefixes = append(b.prefixes, prefixSub{prefix: prefix, handler: handler})
		return
	}
	b.exact[nameOrPrefix] = append(b.exact[nameOrPrefix], handler)
}

// Publish normalizes the event and invokes matching handlers (exact + prefix).
// Returns an error only when the event fails validation / normalization.
// Handler failures are logged and do not fail Publish (fail-soft).
func (b *MemoryBus) Publish(ctx context.Context, event Event) error {
	if b == nil {
		return nil
	}
	normalized, err := normalizeEvent(event)
	if err != nil {
		return err
	}

	handlers := b.handlersFor(normalized.Name)
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, normalized); err != nil {
			b.log.Error("event_handler_failed",
				"event", normalized.Name,
				"event_id", normalized.EventID.String(),
				"error", err,
			)
		}
	}
	b.mu.RLock()
	critical := append([]Handler{}, b.critical[normalized.Name]...)
	b.mu.RUnlock()
	var failures []error
	for _, handler := range critical {
		if err := handler(ctx, normalized); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// SubscribeReliable propagates listener failures to transactional outbox retry.
// Handlers must be idempotent because delivery is at least once.
func (b *MemoryBus) SubscribeReliable(name string, handler Handler) {
	if b == nil || handler == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.critical == nil {
		b.critical = make(map[string][]Handler)
	}
	b.critical[name] = append(b.critical[name], handler)
}

// HandlerCount returns how many handlers match name (exact + prefix). Tests/diagnostics.
func (b *MemoryBus) HandlerCount(name string) int {
	if b == nil {
		return 0
	}
	n := len(b.handlersFor(name))
	b.mu.RLock()
	defer b.mu.RUnlock()
	return n + len(b.critical[name])
}

func (b *MemoryBus) handlersFor(name string) []Handler {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]Handler, 0, len(b.exact[name])+len(b.prefixes))
	out = append(out, b.exact[name]...)
	for _, sub := range b.prefixes {
		if strings.HasPrefix(name, sub.prefix) {
			out = append(out, sub.handler)
		}
	}
	return out
}

func normalizeEvent(event Event) (Event, error) {
	if err := ValidateEventName(event.Name); err != nil {
		return Event{}, err
	}
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if event.Version <= 0 {
		event.Version = 1
	}
	return event, nil
}

// Ensure MemoryBus satisfies Bus.
var _ Bus = (*MemoryBus)(nil)

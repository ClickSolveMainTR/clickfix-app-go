# Domain events (CF-6)

One event stream for every module. Rules, webhooks, analytics, AI triggers,
notifications and audit listen to it.

## Publish

1. Build a typed event from the catalog in `backend/internal/platform/events/domain.go`:

   ```go
   ev, err := events.NewDomain(events.MessageReceived, orgID, events.MessagePayload{...})
   ev = ev.WithActor(userID).WithIdempotencyKey(externalMessageID) // optional
   ```

   Each name has one payload struct and a version. Producers never hand-build
   the map for a catalog event; consumers read it with
   `events.DecodePayload[events.MessagePayload](ev)`.
2. Write it to the outbox **in the same transaction** as the business change:
   `outboxStore.Enqueue(ctx, tx, ev)` (or `outbox.EnqueueQueries(ctx, q.WithTx(tx), ev)`,
   or with the ambient `dbtx.WithTx` context). Rolled back writes never publish.
   `WithIdempotencyKey` makes the event id deterministic, so enqueueing the same
   inbound webhook twice keeps one outbox row.

## Delivery

The outbox publisher (API and worker, `FOR UPDATE SKIP LOCKED`) takes each row and:

1. appends it to `domain_event_log` (once per event id; analytics source),
2. fans it out to durable consumers: one Asynq task per consumer on the
   `events` queue, TaskID `<consumer>:<event id>`,
3. publishes it on the in-process `events.Bus` (fail-soft legacy listeners),
4. marks the row published. Any error in 1-2 retries the row with backoff;
   after 8 attempts it is `failed`.

Delivery is **at least once**. Consumers are idempotent: the worker runs a
consumer inside a transaction that also inserts `(consumer, event_id)` into
`event_consumptions`; a duplicate finds the row and is skipped, a failing
handler rolls back and Asynq retries. After the last retry the event lands in
`event_dead_letters`; `Dispatcher.ReplayDeadLetter(id)` enqueues it again.
Markers older than 30 days are purged daily.

## Consume

Register a consumer in `backend/internal/eventconsumers/registry.go` (API and
worker share the list):

```go
eventdispatch.Consumer{
    Name:   "sla-timers",                 // stable: part of the idempotency key
    Events: []string{"conversation.*", events.MessageReceived},
    Handle: func(ctx context.Context, ev events.Event) error { ... },
}
```

The handler context carries the consumer transaction (`dbtx.Tx(ctx)`):
repositories that use it commit together with the idempotency marker. External
side effects (HTTP, e-mail) must tolerate a rare repeat.

Outgoing webhooks plug in through `eventdispatch.WebhookSink`
(`WebhookConsumer` subscribes it to `events.WebhookEvents()`, the catalog
entries marked `Webhook`). Billing lifecycle events (`subscription.*`) are
named in `catalog.go` by the billing module and are not webhook-exposed.

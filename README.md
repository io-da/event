# Go event bus

An in-process event bus with identifier routing, independent subscriber queues,
context propagation, middleware, and graceful draining. Requires the Go version
in [go.mod](go.mod).

```sh
go get github.com/io-da/event
```

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/io-da/event"
)

type Price struct { Value int }
func (Price) Identifier() event.Identifier { return 1 }

func main() {
    bus := event.NewBus(
        event.WithWorkerPoolSize(2),
        event.WithQueueBuffer(256),
        event.WithErrorHandlers(event.ErrorHandlerFunc(
            func(ctx context.Context, err error, evt event.Event) {
                log.Printf("event processing failed: %v", err)
            },
        )),
    )
    handler := event.NewHandler(1, func(ctx context.Context, price Price) error {
        log.Printf("price: %d", price.Value)
        return nil
    })
    if err := bus.Initialize(handler); err != nil { log.Fatal(err) }
    if err := bus.Publish(context.Background(), Price{Value: 42}); err != nil {
        log.Print(err)
    }
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := bus.Shutdown(ctx); err != nil { log.Print(err) }
}
```

Create the bus before wiring application dependencies, then initialize it once
with all handlers. Each handler subscribes to exactly one `Identifier`; register
multiple handlers for multiple identifiers. Unmatched events succeed. A typed
handler still checks the event's concrete type and reports `InvalidEventError` if
an identifier is shared by incompatible types.

## Queues and ordering

Each registration has its own queue and workers. Defaults are one worker and a
buffer of 100. `WithWorkerPoolSize`, `WithQueueBuffer`, and `WithOverflowPolicy`
configure bus defaults. Counts must be positive; invalid options panic.

Override a subscriber's defaults with:

```go
subscriber := event.WithSubscription(handler,
    event.WithSubscriptionWorkers(1),
    event.WithSubscriptionBuffer(32),
    event.WithSubscriptionOverflow(event.OverflowDropOldest),
)
// Pass subscriber to bus.Initialize.
```

| Policy | Full queue behavior |
| --- | --- |
| `OverflowBlock` (default) | Wait for capacity, context cancellation, or shutdown |
| `OverflowDropNewest` | Return `ErrQueueFull` for that delivery |
| `OverflowDropOldest` | Evict the oldest queued item and accept the new item |

Workers are isolated, but a full blocking queue delays the publisher and later
subscribers in that call. Fanout is not atomic: returned errors may represent
partial delivery. Use `errors.Is` on returned errors. Successful publication means
admission, not successful processing. Drop-oldest eviction is silent by design.

An event may implement `Topic() event.Identifier` in addition to `Identifier()`.
Each subscriber/topic pair gets a separate queue and one worker, preserving order
for sequential publications to that topic. Concurrent publishers have no defined
relative order. Topic queues remain until shutdown, so bound topic cardinality.
Multiple workers may reorder non-topic events. Subscribers execute independently;
there is no cross-handler execution order guarantee.

## Middleware and errors

`Middleware` is `func(event.HandlerFunc) event.HandlerFunc`, where `HandlerFunc`
is `func(context.Context, event.Event) error`. Install middleware using
`WithMiddlewares`; the first is outermost. Middleware is assembled per subscription.

Processing failures go to configured error handlers. Handler and middleware
execution panics become `*event.ErrHandlerPanic` with a panic value and stack;
workers continue. Error-handler panics are contained so later error handlers run.
Error handlers and handlers must support concurrent calls when workers or topics
can execute concurrently. Configuration callbacks and event identifier methods
are outside the worker panic boundary.

Admission errors are returned directly. Nil and typed-nil events are invalid.
Share only immutable events, or provide application synchronization.

## Context and shutdown

Publication contexts reach handlers and error handlers. Canceled queued work is
skipped and reported to error handlers; running work must honor cancellation
itself. Keep the context alive for the desired processing lifetime.

`Shutdown(ctx)` permanently rejects new events, unblocks waiting publishers, and
drains admitted work. A deadline stops only the caller's wait; draining continues.
Call `Shutdown` again to wait for completion. A bus cannot restart. Do not call
blocking shutdown from a handler on that bus, or recursively fill a handler's own
blocking queue.

## Migration

This revision breaks the previous API and needs a major release:

- Replace `NewBus()` followed by setters with constructor options.
- Replace `SetConcurrentWorkerPoolSize` with `WithWorkerPoolSize` (now per subscriber).
- Replace `SetTopicBuffer` with `WithQueueBuffer`; `SetTopicsCapacity` is removed.
- Replace `SetErrorHandlers` with `WithErrorHandlers`.
- Add `Handles() Identifier` to handlers and a context argument to `Handle`, or
  use `NewHandler[T]`. Handlers no longer receive every event automatically.
- Use `Publish(ctx, evt)` or `Emit(ctx, evt)` and check the admission error.
- Error handlers now use `Handle(ctx, err, evt)`.
- Check the result of `Initialize` and use `Shutdown(ctx)`.
- Replace assumptions about sequential execution across handlers with explicit
  application coordination. Topic order is guaranteed within each subscription.

There is no `Async.Await` API. Use error handlers for processing failures and
shutdown as a bus-wide completion barrier. See [refactor.md](refactor.md) for the
review, design contracts, and limitations.

## Validation

```sh
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go vet ./...
```

CI requires at least 90% statement coverage.

# Event bus review and implementation specification

## Review of the original package

The two-phase `NewBus` / `Initialize` lifecycle and constant `BusError` values
are worth preserving. The original draft, however, described several features
that were absent: handlers had no `Handles` method, identifiers were not used
for routing, middleware did not exist, and there was no `Async` or `Await` API.
Every event was broadcast to every handler. Non-topic events shared a worker
pool, while each dynamically created topic had one worker calling every handler
sequentially.

The original implementation also had correctness problems beyond worker
starvation:

- Topic lookup and append were unsynchronized and used linear searches.
- Initialization published its flag before queues and handlers were ready.
- Shutdown raced with publishers and topic creation, using nil sentinels that
  could leave events behind. Concurrent shutdown callers did not all wait.
- Setters silently ignored late changes and could race with initialization.
- Zero workers could deadlock publishing; invalid capacities were unchecked.
- A panic escaping a worker terminates the Go process; it does not merely
  remove that worker while allowing the application to continue.
- Tests relied on sleeps, leaked workers, and called `Fatal` from timer callbacks.
- CI tested older Go versions despite the workspace's existing Go 1.27 directive.

## Implemented API and compatibility decision

This is a breaking API revision, intended for a major release. Preserve the
module path in this working tree; version/tag/module-path publication decisions
remain release work.

- `NewBus(opts ...Option) *Bus` stores immutable configuration and starts no workers.
- `Initialize(handlers ...Handler) error` registers handlers once. Registration
  is transactional for invalid handlers; no workers start until registration completes.
- `Handler` has `Handles() Identifier` and `Handle(context.Context, Event) error`.
- `Publish(context.Context, Event) error` routes by identifier using a map.
  `Emit` remains as a context-aware alias.
- `NewHandler[T Event]` removes manual assertions in application code. The
  identifier/type association is still a runtime contract; mismatches return
  `InvalidEventError`. Generics cannot prove that an integer identifier denotes T.
- Middleware wraps `HandlerFunc`; the first middleware is outermost.
- `ErrorHandler.Handle(context.Context, error, Event)` receives processing errors.
- `Shutdown(context.Context) error` starts permanent shutdown and waits for drain.
- Remove setters and unused atomic flag/counter helpers. A bus cannot restart.
- Do not introduce `Async.Await`: there is no existing async result handle to
  migrate. `Publish` reports admission; error handlers report processing failures;
  `Shutdown` is the completion barrier for the bus.

## Queue isolation, ordering, and overflow

Each registration owns a bounded non-topic queue and configurable workers.
Each subscription/topic pair lazily owns another bounded queue with one worker.
Defaults are one worker, capacity 100, and `OverflowBlock`. One worker per
subscription keeps the default goroutine count predictable as listeners grow.
`WithWorkerPoolSize`, `WithQueueBuffer`, and `WithOverflowPolicy` set defaults.
`WithSubscription` and its worker/buffer/overflow options override one registration.
Queue capacities and worker counts must be positive; invalid options panic.

- Sequential publications to the same topic retain order within each subscriber.
  Simultaneous publishers have no defined relative order. Subscribers run
  independently; handler registration order is not an execution-order guarantee.
- Multiple workers can reorder non-topic events. Different topics run independently.
- `OverflowBlock` waits for room, caller cancellation, or shutdown.
- `OverflowDropNewest` rejects the incoming delivery with `ErrQueueFull`.
- `OverflowDropOldest` evicts the oldest queued delivery, never in-flight work,
  and accepts the new one. Eviction is intentional and is not a processing error.
- Fanout visits subscribers in registration order and is not atomic. An error
  can mean partial delivery; other subscribers are still attempted. Errors are joined.
- Independent workers isolate handler execution. A full blocking queue still
  delays its publisher and subsequent subscribers in that publish call. Use a
  dropping policy for lossy subscribers or deadlines for bounded waiting.
- Topic queues remain until shutdown: applications must bound topic cardinality.
  This revision does not add eviction, metrics, persistence, priority tiers, or retries.

## Context, errors, and shutdown

The publication context reaches middleware, handlers, and error handlers without
replacement. If it expires while queued, processing is skipped and its error is
reported. Running handlers must cooperate with cancellation. The caller must keep
the context alive for the desired processing lifetime, not only until enqueue returns.

Admission failures are returned directly and are not sent to error handlers.
Unmatched identifiers succeed without delivery. Nil and typed-nil events are
rejected. Events must be immutable or synchronized by their owners.

Handler and middleware execution panics become `*ErrHandlerPanic`, containing the
panic value and stack. Workers continue. Error-handler panics are contained and
remaining error handlers still run, without recursive error reporting.
Constructors, identifier methods, and middleware assembly are trusted application
configuration; the worker recovery boundary does not cover those calls.

A mutex protects lifecycle transitions and queue creation. Accepted publishers
are counted before releasing that mutex. Shutdown stops admission, wakes blocked
publishers, waits for them to leave, then closes and drains all queues. Every
shutdown caller waits on the same completion signal. A canceled shutdown context
only abandons that caller's wait; subsequent calls may continue waiting.
Shutdown also works before initialization. Handlers must not wait synchronously
for their own bus to shut down, or recursively fill their own blocking queue.

## Acceptance and validation

The revised tests replace obsolete API tests and exercise routing, typed mismatch,
context values and cancellation, middleware order, panic recovery, error-handler
isolation, subscription overflow, topic ordering, queue wakeups, option validation,
configuration slice ownership, and concurrent publishing/shutdown.

Required checks:

```sh
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go vet ./...
```

The package must have at least 90% statement coverage; CI enforces that threshold.
Coverage measures executed statements, not exhaustive concurrency interleavings;
repeat race tests when changing synchronization. README examples and CI must match
the new API and the Go version declared by the existing go.mod.

Validation result: 25 consecutive race-enabled test runs passed with 100.0%
statement coverage. `go vet ./...` and `git diff --check` passed. The pre-existing
Go 1.27 change in go.mod was preserved.

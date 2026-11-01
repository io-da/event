package event

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOverflowLogging(t *testing.T) {
	for _, policy := range []OverflowPolicy{OverflowBlock, OverflowDropOldest, OverflowDropNewest} {
		for _, topic := range []bool{false, true} {
			t.Run(string(rune('0'+policy))+map[bool]string{false: "/regular", true: "/topic"}[topic], func(t *testing.T) {
				type contextKey struct{}
				ctx := context.WithValue(bg, contextKey{}, "trace")
				logs := make(chan LogEvent, 8)
				started, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				b := NewBus(WithLogHandler(func(c context.Context, e LogEvent) {
					if c.Value(contextKey{}) != "trace" {
						t.Error("context lost")
					}
					logs <- e
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); drain(t, b) })
				h := NewHandler[Event](1, func(_ context.Context, e Event) error {
					n := 0
					switch e := e.(type) {
					case message:
						n = e.N
					case ordered:
						n = e.N
					}
					if n == 0 {
						close(started)
						<-release
					}
					return nil
				})
				check(t, b.Initialize(NewHandler(2, func(context.Context, other) error { return nil }), WithSubscription(h, WithSubscriptionBuffer(1), WithSubscriptionOverflow(policy))))
				evt := func(n int) Event {
					if topic {
						return ordered{message{n}, 0}
					}
					return message{n}
				}
				check(t, b.Publish(ctx, evt(0)))
				receive(t, started)
				check(t, b.Publish(ctx, evt(1)))
				done := make(chan error, 1)
				go func() { done <- b.Publish(ctx, evt(2)) }()
				record := receive(t, logs)
				if record.Kind != LogQueueOverflow || record.Level != slog.LevelWarn || record.Message == "" || record.EventID != 1 || record.SubscriptionID != 1 || record.HasTopic != topic || record.TopicID != 0 || record.OverflowPolicy != policy || record.QueueCapacity != 1 || record.QueueLength != 1 || record.Event != evt(2) {
					t.Fatalf("unexpected log: %+v", record)
				}
				switch policy {
				case OverflowBlock:
					if record.Error != ErrQueueOverflow || record.DroppedEvent != nil {
						t.Fatal(record)
					}
					select {
					case err := <-done:
						t.Fatalf("blocking publish finished: %v", err)
					default:
					}
				case OverflowDropOldest:
					if record.Error != ErrQueueOverflow || record.DroppedEvent != evt(1) {
						t.Fatal(record)
					}
				case OverflowDropNewest:
					if record.Error != ErrQueueFull || record.DroppedEvent != evt(2) {
						t.Fatal(record)
					}
				}
				once.Do(func() { close(release) })
				err := receive(t, done)
				if policy == OverflowDropNewest {
					if !errors.Is(err, ErrQueueFull) {
						t.Fatal(err)
					}
				} else {
					check(t, err)
				}
				drain(t, b)
				if len(logs) != 0 {
					t.Fatal("duplicate warning", len(logs))
				}
			})
		}
	}
}

func TestProcessingLogs(t *testing.T) {
	boom := errors.New("failed")
	logs := make(chan LogEvent, 16)
	failures := make(chan error, 8)
	started, release := make(chan struct{}), make(chan struct{})
	b := NewBus(WithLogHandler(func(_ context.Context, e LogEvent) { logs <- e }), WithErrorHandlers(
		ErrorHandlerFunc(func(context.Context, error, Event) { panic("reporter") }),
		ErrorHandlerFunc(func(_ context.Context, err error, _ Event) { failures <- err }),
	))
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); drain(t, b) })
	check(t, b.Initialize(NewHandler(1, func(_ context.Context, e message) error {
		switch e.N {
		case 0:
			close(started)
			<-release
			return nil
		case 1:
			return boom
		case 2:
			panic("worker")
		default:
			t.Error("canceled event executed")
			return nil
		}
	})))
	check(t, b.Publish(bg, message{}))
	receive(t, started)
	check(t, b.Publish(bg, message{1}))
	check(t, b.Publish(bg, message{2}))
	ctx, cancel := context.WithCancel(bg)
	check(t, b.Publish(ctx, message{3}))
	cancel()
	once.Do(func() { close(release) })
	drain(t, b)
	for n := 1; n <= 3; n++ {
		entry := receive(t, logs)
		if entry.Event != (message{n}) || entry.EventID != 1 || entry.SubscriptionID != 0 || entry.HasTopic || entry.Message == "" {
			t.Fatal(entry)
		}
		if n == 3 {
			if entry.Kind != LogEventCanceled || entry.Level != slog.LevelWarn || entry.Error != context.Canceled {
				t.Fatal(entry)
			}
		} else {
			if entry.Kind != LogHandlerError || entry.Level != slog.LevelError {
				t.Fatal(entry)
			}
			if n == 1 && entry.Error != boom {
				t.Fatal(entry)
			}
			if n == 2 {
				var p *ErrHandlerPanic
				if !errors.As(entry.Error, &p) || p.Value != "worker" || len(p.Stack) == 0 {
					t.Fatal(entry)
				}
			}
		}
		if err := receive(t, failures); err != entry.Error {
			t.Fatal("error handler received different error", err)
		}
		reporter := receive(t, logs)
		var p *ErrHandlerPanic
		if reporter.Kind != LogErrorHandlerPanic || reporter.Level != slog.LevelError || reporter.Event != entry.Event || !errors.As(reporter.Error, &p) || p.Value != "reporter" || len(p.Stack) == 0 {
			t.Fatal(reporter)
		}
	}
	if len(logs) != 0 {
		t.Fatal("unexpected log")
	}
}

func TestLogHandlerPanicDoesNotInterruptDelivery(t *testing.T) {
	var calls, handled atomic.Int32
	b := NewBus(WithLogHandler(func(context.Context, LogEvent) { calls.Add(1); panic("logger") }), WithErrorHandlers(ErrorHandlerFunc(func(context.Context, error, Event) { handled.Add(1) })))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { return errors.New("failure") })))
	for range 3 {
		check(t, b.Publish(bg, message{}))
	}
	drain(t, b)
	if calls.Load() != 3 || handled.Load() != 3 {
		t.Fatal(calls.Load(), handled.Load())
	}
	// A disabled logger is valid, including when the option overrides an earlier hook.
	b = NewBus(WithLogHandler(func(context.Context, LogEvent) { t.Error("disabled logger called") }), WithLogHandler(nil))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { return errors.New("failure") })))
	check(t, b.Publish(bg, message{}))
	drain(t, b)
	var disabled LogHandler
	disabled.log(bg, LogEvent{})
}

func TestOverflowLoggerRunsOutsideQueueLock(t *testing.T) {
	for _, policy := range []OverflowPolicy{OverflowBlock, OverflowDropOldest, OverflowDropNewest} {
		q := newQueue(1, policy)
		stop := make(chan struct{})
		check(t, q.push(bg, stop, work{bg, message{1}}))
		var calls int
		q.logger = func(context.Context, LogEvent) {
			calls++
			if !q.mu.TryLock() {
				t.Error("logger called under queue lock")
				return
			}
			q.mu.Unlock()
			if policy == OverflowBlock {
				q.pop()
			} // Also verifies wakeup is not lost during callback.
			panic("contained")
		}
		err := q.push(bg, stop, work{bg, message{2}})
		if policy == OverflowDropNewest {
			if err != ErrQueueFull {
				t.Fatal(err)
			}
		} else {
			check(t, err)
		}
		if calls != 1 {
			t.Fatal(calls)
		}
	}
}

func TestLogHandlerRunsOutsideBusLock(t *testing.T) {
	var b *Bus
	done := make(chan bool, 1)
	b = NewBus(WithLogHandler(func(context.Context, LogEvent) {
		unlocked := b.mu.TryLock()
		if unlocked {
			b.mu.Unlock()
		}
		done <- unlocked
	}))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { return errors.New("failure") })))
	check(t, b.Publish(bg, message{}))
	if !receive(t, done) {
		t.Fatal("logger called under bus lock")
	}
	drain(t, b)
}

func TestConcurrentLogging(t *testing.T) {
	var count atomic.Int64
	b := NewBus(WithWorkerPoolSize(4), WithQueueBuffer(2), WithLogHandler(func(_ context.Context, e LogEvent) {
		if e.Kind == LogHandlerError {
			count.Add(1)
		}
	}))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { return errors.New("failure") })))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				if err := b.Publish(bg, message{}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	drain(t, b)
	if count.Load() != 400 {
		t.Fatal(count.Load())
	}
}

// Reports every context check, allowing repeated full-queue wakeups without sleeps.
type observedContext struct {
	context.Context
	checks chan struct{}
}

func (c observedContext) Err() error { c.checks <- struct{}{}; return c.Context.Err() }

func TestBlockingOverflowWarnsOnceAcrossWakeups(t *testing.T) {
	q := newQueue(1, OverflowBlock)
	stop := make(chan struct{})
	check(t, q.push(bg, stop, work{bg, message{1}}))
	var count atomic.Int32
	logged := make(chan struct{}, 8)
	q.logger = func(context.Context, LogEvent) { count.Add(1); logged <- struct{}{} }
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	checks := make(chan struct{}, 8)
	done := make(chan error, 1)
	go func() { done <- q.push(observedContext{ctx, checks}, stop, work{ctx, message{2}}) }()
	receive(t, checks)
	receive(t, logged)
	for range 3 {
		q.mu.Lock()
		q.signal()
		q.mu.Unlock()
		receive(t, checks)
	}
	cancel()
	if err := receive(t, done); err != context.Canceled {
		t.Fatal(err)
	}
	if count.Load() != 1 {
		t.Fatal("duplicate overflow warnings", count.Load())
	}
}

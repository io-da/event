package event

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type message struct{ N int }

func (message) Identifier() Identifier { return 1 }

type ordered struct {
	message
	Key Identifier
}

func (o ordered) Topic() Identifier { return o.Key }

type other struct{}

func (other) Identifier() Identifier { return 2 }

var bg = context.Background()

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
		var zero T
		return zero
	}
}
func drain(t *testing.T, b *Bus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(bg, 5*time.Second)
	defer cancel()
	check(t, b.Shutdown(ctx))
}

func TestLifecycle(t *testing.T) {
	b := NewBus()
	if err := b.Publish(bg, nil); !errors.Is(err, InvalidEventError) {
		t.Fatal(err)
	}
	if err := b.Emit(bg, message{}); !errors.Is(err, BusNotInitializedError) {
		t.Fatal(err)
	}
	if err := b.Initialize(nil); err != InvalidHandlerError {
		t.Fatal(err)
	}
	check(t, b.Initialize())
	if err := b.Initialize(); err != BusAlreadyInitializedError {
		t.Fatal(err)
	}
	check(t, b.Publish(bg, other{}))
	drain(t, b)
	drain(t, b)
	if err := b.Initialize(); err != BusIsShuttingDownError {
		t.Fatal(err)
	}
	if err := b.Publish(bg, message{}); err != BusIsShuttingDownError {
		t.Fatal(err)
	}
	drain(t, NewBus())
	if InvalidEventError.Error() != "event: invalid event" {
		t.Fatal("error string")
	}
}
func TestRoutingContextAndMiddleware(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(bg, key{}, "value")
	var order []int
	mw := func(n int) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, e Event) error {
				order = append(order, n)
				err := next(ctx, e)
				order = append(order, -n)
				return err
			}
		}
	}
	var count atomic.Int32
	b := NewBus(WithMiddlewares(mw(1), mw(2)))
	check(t, b.Initialize(NewHandler(1, func(c context.Context, m message) error {
		if c.Value(key{}) != "value" {
			return errors.New("context lost")
		}
		order = append(order, 3)
		count.Add(1)
		return nil
	}), NewHandler(2, func(context.Context, other) error { count.Add(100); return nil })))
	check(t, b.Emit(ctx, message{}))
	drain(t, b)
	if count.Load() != 1 || !reflect.DeepEqual(order, []int{1, 2, 3, -2, -1}) {
		t.Fatal(count.Load(), order)
	}
}
func TestPanicAndErrors(t *testing.T) {
	errs := make(chan error, 10)
	boom := errors.New("boom")
	b := NewBus(WithErrorHandlers(ErrorHandlerFunc(func(context.Context, error, Event) { panic("reporter") }), ErrorHandlerFunc(func(_ context.Context, e error, _ Event) { errs <- e })))
	check(t, b.Initialize(NewHandler(1, func(_ context.Context, m message) error {
		if m.N == 0 {
			panic("handler")
		}
		return boom
	})))
	check(t, b.Publish(bg, message{}))
	check(t, b.Publish(bg, message{1}))
	drain(t, b)
	var p *ErrHandlerPanic
	if err := receive(t, errs); !errors.As(err, &p) || len(p.Stack) == 0 || p.Error() != "event: handler panic: handler" {
		t.Fatal(err)
	}
	if err := receive(t, errs); err != boom {
		t.Fatal(err)
	}
	// Middleware execution panics use the same boundary.
	b = NewBus(WithMiddlewares(func(HandlerFunc) HandlerFunc { return func(context.Context, Event) error { panic("middleware") } }), WithErrorHandlers(ErrorHandlerFunc(func(_ context.Context, e error, _ Event) { errs <- e })))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { return nil })))
	check(t, b.Publish(bg, message{}))
	drain(t, b)
	if err := receive(t, errs); !errors.As(err, &p) {
		t.Fatal(err)
	}
	h := NewHandler(1, func(context.Context, message) error { return nil })
	if h.Handle(bg, other{}) != InvalidEventError {
		t.Fatal("type mismatch")
	}
}
func TestTopicOrderingAndIsolation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	fast := make(chan int, 200)
	var got []int
	b := NewBus(WithWorkerPoolSize(3))
	check(t, b.Initialize(NewHandler(1, func(_ context.Context, e ordered) error {
		if e.Key == 1 && e.N == 0 {
			close(started)
			<-release
		}
		if e.Key == 1 {
			got = append(got, e.N)
		}
		return nil
	}), NewHandler(1, func(_ context.Context, e ordered) error { fast <- e.N; return nil })))
	check(t, b.Publish(bg, ordered{message{0}, 1}))
	receive(t, started)
	receive(t, fast) // A blocked listener does not occupy another listener's worker.
	for i := 1; i < 50; i++ {
		check(t, b.Publish(bg, ordered{message{i}, 1}))
	}
	check(t, b.Publish(bg, ordered{message{99}, 2}))
	close(release)
	drain(t, b)
	for i, n := range got {
		if i != n {
			t.Fatalf("order %v", got)
		}
	}
	if len(got) != 50 {
		t.Fatal(len(got))
	}
}
func TestOverflowPolicies(t *testing.T) {
	for _, policy := range []OverflowPolicy{OverflowDropNewest, OverflowDropOldest} {
		t.Run(map[OverflowPolicy]string{OverflowDropNewest: "newest", OverflowDropOldest: "oldest"}[policy], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var got []int
			b := NewBus()
			h := NewHandler(1, func(_ context.Context, e message) error {
				if e.N == 0 {
					close(started)
					<-release
				}
				got = append(got, e.N)
				return nil
			})
			check(t, b.Initialize(WithSubscription(h, WithSubscriptionBuffer(1), WithSubscriptionWorkers(1), WithSubscriptionOverflow(policy))))
			check(t, b.Publish(bg, message{}))
			receive(t, started)
			check(t, b.Publish(bg, message{1}))
			err := b.Publish(bg, message{2})
			want := []int{0, 2}
			if policy == OverflowDropNewest {
				if !errors.Is(err, ErrQueueFull) {
					t.Fatal(err)
				}
				want = []int{0, 1}
			} else {
				check(t, err)
			}
			close(release)
			drain(t, b)
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got)
			}
		})
	}
}
func TestCancellationAndShutdown(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	errs := make(chan error, 1)
	b := NewBus(WithQueueBuffer(1), WithErrorHandlers(ErrorHandlerFunc(func(_ context.Context, e error, _ Event) { errs <- e })))
	check(t, b.Initialize(NewHandler(1, func(_ context.Context, e message) error {
		if e.N == 0 {
			close(started)
			<-release
		}
		return nil
	})))
	check(t, b.Publish(bg, message{}))
	receive(t, started)
	ctx, cancel := context.WithCancel(bg)
	check(t, b.Publish(ctx, message{1}))
	cancel()
	if err := b.Publish(ctx, message{2}); err != context.Canceled {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(bg, 20*time.Millisecond)
	defer stop()
	if err := b.Publish(deadline, message{2}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	publishing := make(chan error, 1)
	go func() { publishing <- b.Publish(bg, message{3}) }()
	if err := b.Shutdown(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	if err := receive(t, publishing); !errors.Is(err, BusIsShuttingDownError) {
		t.Fatal(err)
	}
	close(release)
	drain(t, b)
	if err := receive(t, errs); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestBlockedPublisherResumes(t *testing.T) {
	q := newQueue(1, OverflowBlock)
	stop := make(chan struct{})
	check(t, q.push(bg, stop, work{bg, message{1}}))
	done := make(chan error, 1)
	go func() { done <- q.push(bg, stop, work{bg, message{2}}) }()
	first, ok := q.pop()
	if !ok || first.event.(message).N != 1 {
		t.Fatal(first)
	}
	check(t, receive(t, done))
	q.close()
	second, ok := q.pop()
	if !ok || second.event.(message).N != 2 {
		t.Fatal(second)
	}
	if _, ok = q.pop(); ok {
		t.Fatal("closed queue")
	}
}
func TestConcurrentLifecycle(t *testing.T) {
	b := NewBus(WithQueueBuffer(2), WithWorkerPoolSize(4))
	var count atomic.Int64
	check(t, b.Initialize(NewHandler(1, func(context.Context, ordered) error { count.Add(1); return nil })))
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				err := b.Publish(bg, ordered{message{i}, Identifier(g % 4)})
				if err != nil && !errors.Is(err, BusIsShuttingDownError) {
					t.Error(err)
				}
			}
		}(g)
	}
	check(t, b.Publish(bg, ordered{message{}, 9}))
	var shutdowns sync.WaitGroup
	for i := 0; i < 5; i++ {
		shutdowns.Add(1)
		go func() {
			defer shutdowns.Done()
			ctx, cancel := context.WithTimeout(bg, 5*time.Second)
			defer cancel()
			if err := b.Shutdown(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	shutdowns.Wait()
	if count.Load() < 1 {
		t.Fatal("accepted work lost")
	}
}
func TestInvalidOptions(t *testing.T) {
	cases := []func(){
		func() { NewBus(WithWorkerPoolSize(0)) }, func() { NewBus(WithQueueBuffer(0)) }, func() { NewBus(WithOverflowPolicy(99)) }, func() { NewBus(WithMiddlewares(nil)) }, func() { NewBus(WithErrorHandlers(nil)) }, func() { NewHandler[message](1, nil) },
	}
	for i, fn := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("case %d did not panic", i)
				}
			}()
			fn()
		}()
	}
}
func TestOptionsCopyInputs(t *testing.T) {
	var count atomic.Int32
	hs := []ErrorHandler{ErrorHandlerFunc(func(context.Context, error, Event) { count.Add(1) })}
	ms := []Middleware{func(next HandlerFunc) HandlerFunc { return next }}
	opts := []SubscriptionOption{WithSubscriptionBuffer(2)}
	h := WithSubscription(NewHandler(1, func(context.Context, message) error { return errors.New("test") }), opts...)
	bopt, mopt := WithErrorHandlers(hs...), WithMiddlewares(ms...)
	hs[0] = nil
	ms[0] = nil
	opts[0] = nil
	b := NewBus(bopt, mopt)
	check(t, b.Initialize(h))
	check(t, b.Publish(bg, message{}))
	drain(t, b)
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
}

func TestWorkerParallelismAndIndependentTopics(t *testing.T) {
	for _, topics := range []bool{false, true} {
		t.Run(map[bool]string{false: "workers", true: "topics"}[topics], func(t *testing.T) {
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			b := NewBus(WithWorkerPoolSize(2))
			check(t, b.Initialize(NewHandler[Event](1, func(context.Context, Event) error { started <- struct{}{}; <-release; return nil })))
			// Both handlers must start before either is allowed to finish.
			if topics {
				check(t, b.Publish(bg, ordered{message{}, 1}))
				check(t, b.Publish(bg, ordered{message{}, 2}))
			} else {
				check(t, b.Publish(bg, message{}))
				check(t, b.Publish(bg, message{}))
			}
			receive(t, started)
			receive(t, started)
			close(release)
			drain(t, b)
		})
	}
}

func TestConcurrentAdmissionDrainsEveryAcceptedEvent(t *testing.T) {
	var processed atomic.Int64
	b := NewBus(WithWorkerPoolSize(4), WithQueueBuffer(2))
	check(t, b.Initialize(NewHandler(1, func(context.Context, message) error { processed.Add(1); return nil })))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := b.Publish(bg, message{j}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	drain(t, b)
	if processed.Load() != 1600 {
		t.Fatal(processed.Load())
	}
}

func TestInitializationIsTransactionalAndSerialized(t *testing.T) {
	var calls atomic.Int32
	h := NewHandler(1, func(context.Context, message) error { calls.Add(1); return nil })
	b := NewBus()
	if err := b.Initialize(h, nil); err != InvalidHandlerError {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- b.Initialize(h) }()
	}
	a, c := receive(t, results), receive(t, results)
	if !((a == nil && c == BusAlreadyInitializedError) || (c == nil && a == BusAlreadyInitializedError)) {
		t.Fatal(a, c)
	}
	check(t, b.Publish(bg, message{}))
	drain(t, b)
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

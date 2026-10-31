package event

import (
	"context"
	"errors"
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
)

// Bus routes events to independent subscriptions. Construct it with NewBus.
// A bus cannot be restarted after Shutdown. Do not copy a Bus.
type Bus struct {
	mu                    sync.Mutex
	config                config
	initialized, stopping bool
	routes                map[Identifier][]*subscription
	queues                []*queue
	publishers            sync.WaitGroup
	workers               sync.WaitGroup
	stop, done            chan struct{}
}
type subscription struct {
	config  config
	handle  HandlerFunc
	regular *queue
	topics  map[Identifier]*queue
}
type work struct {
	ctx   context.Context
	event Event
}

// NewBus configures the bus without starting workers. Invalid options panic.
func NewBus(opts ...Option) *Bus {
	c := config{workers: 1, buffer: 100}
	for _, opt := range opts {
		opt(&c)
	}
	return &Bus{config: c, stop: make(chan struct{}), done: make(chan struct{})}
}

// Initialize registers handlers and starts workers. Registration is all-or-nothing.
func (b *Bus) Initialize(handlers ...Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopping {
		return BusIsShuttingDownError
	}
	if b.initialized {
		return BusAlreadyInitializedError
	}
	routes := make(map[Identifier][]*subscription)
	for _, h := range handlers {
		c := b.config
		if configured, ok := h.(configuredHandler); ok {
			h = configured.Handler
			for _, opt := range configured.options {
				opt(&c)
			}
		}
		if isNil(h) {
			return InvalidHandlerError
		}
		fn := h.Handle
		for _, v := range slices.Backward(c.middlewares) {
			fn = v(fn)
		}
		s := &subscription{config: c, handle: fn, topics: make(map[Identifier]*queue)}
		id := h.Handles()
		routes[id] = append(routes[id], s)
	}
	b.routes = routes
	for _, subs := range routes {
		for _, s := range subs {
			s.regular = b.startQueue(s, s.config.workers)
		}
	}
	b.initialized = true
	return nil
}

// Publish enqueues an event for matching handlers. Success means admission, not
// completion. Fanout is not atomic: cancellation/overflow may cause partial delivery.
// Unmatched events succeed. Publishing a mutable event requires caller synchronization.
func (b *Bus) Publish(ctx context.Context, evt Event) error {
	if isNil(evt) {
		return InvalidEventError
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id := evt.Identifier()
	topicID, ordered := Identifier(0), false
	if t, ok := evt.(Topic); ok {
		topicID, ordered = t.Topic(), true
	}
	b.mu.Lock()
	if b.stopping {
		b.mu.Unlock()
		return BusIsShuttingDownError
	}
	if !b.initialized {
		b.mu.Unlock()
		return BusNotInitializedError
	}
	var queues []*queue
	for _, s := range b.routes[id] {
		q := s.regular
		if ordered {
			q = s.topics[topicID]
			if q == nil {
				q = b.startQueue(s, 1)
				s.topics[topicID] = q
			}
		}
		queues = append(queues, q)
	}
	b.publishers.Add(1)
	b.mu.Unlock()
	defer b.publishers.Done()
	var errs []error
	for _, q := range queues {
		if err := q.push(ctx, b.stop, work{ctx, evt}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Emit is an alias for Publish.
func (b *Bus) Emit(ctx context.Context, evt Event) error { return b.Publish(ctx, evt) }

// Shutdown rejects new work and drains admitted work. Cancellation stops only
// this caller's wait; draining continues and later calls can wait again.
// Call from outside handlers: a handler cannot synchronously wait for its own drain.
func (b *Bus) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	if !b.stopping {
		b.stopping = true
		close(b.stop)
		go func() {
			b.publishers.Wait()
			for _, q := range b.queues {
				q.close()
			}
			b.workers.Wait()
			close(b.done)
		}()
	}
	b.mu.Unlock()
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Bus) startQueue(s *subscription, workers int) *queue {
	q := newQueue(s.config.buffer, s.config.overflow)
	b.queues = append(b.queues, q)
	b.workers.Add(workers)
	for range workers {
		go func() {
			defer b.workers.Done()
			for {
				w, ok := q.pop()
				if !ok {
					return
				}
				b.process(s, w)
			}
		}()
	}
	return q
}
func (b *Bus) process(s *subscription, w work) {
	err := func() (err error) {
		defer func() {
			if v := recover(); v != nil {
				err = &ErrHandlerPanic{v, debug.Stack()}
			}
		}()
		if err := w.ctx.Err(); err != nil {
			return err
		}
		return s.handle(w.ctx, w.event)
	}()
	if err != nil {
		for _, h := range b.config.errorHandlers {
			func() {
				defer func() { _ = recover() }()
				h.Handle(w.ctx, err, w.event)
			}()
		}
	}
}
func isNil(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

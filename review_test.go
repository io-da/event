package event

import (
	"context"
	"testing"
)

// Cancellation races with admission immediately after the first successful check.
type cancelAfterCheck struct {
	context.Context
	cancel context.CancelFunc
}

func (c cancelAfterCheck) Err() error { err := c.Context.Err(); c.cancel(); return err }

func TestDropOldestCommitsReplacement(t *testing.T) {
	q := newQueue(1, OverflowDropOldest)
	stop := make(chan struct{})
	check(t, q.push(bg, stop, work{bg, message{1}}))
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	check(t, q.push(cancelAfterCheck{ctx, cancel}, stop, work{ctx, message{2}}))
	q.close()
	w, ok := q.pop()
	if !ok || w.event != (message{2}) {
		t.Fatal("eviction lost the replacement", w, ok)
	}
}

func TestNestedSubscriptionOptions(t *testing.T) {
	h := NewHandler(1, func(context.Context, message) error { return nil })
	inner := WithSubscription(h, WithSubscriptionBuffer(7), WithSubscriptionWorkers(2))
	outer := WithSubscription(inner, WithSubscriptionWorkers(3), WithSubscriptionOverflow(OverflowDropOldest))
	b := NewBus()
	check(t, b.Initialize(outer, inner))
	s := b.routes[1]
	if s[0].config.buffer != 7 || s[0].config.workers != 3 || s[0].config.overflow != OverflowDropOldest || s[1].config.workers != 2 || s[1].config.overflow != OverflowBlock {
		t.Fatal("nested options were lost or mutated")
	}
	check(t, b.Publish(bg, message{}))
	drain(t, b)
	b = NewBus()
	if err := b.Initialize(WithSubscription(WithSubscription(nil))); err != InvalidHandlerError {
		t.Fatal(err)
	}
	drain(t, b)
}

func TestCompletedShutdownIgnoresCanceledWaitContext(t *testing.T) {
	b := NewBus()
	drain(t, b)
	ctx, cancel := context.WithCancel(bg)
	cancel()
	for range 100 {
		check(t, b.Shutdown(ctx))
	}
}

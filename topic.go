package event

import (
	"context"
	"sync"
)

// queue is a bounded ring. changed broadcasts state transitions to workers and
// publishers; producers never send to a channel that shutdown could close.
type queue struct {
	mu         sync.Mutex
	items      []work
	head, size int
	policy     OverflowPolicy
	closing    bool
	changed    chan struct{}
}

func newQueue(capacity int, policy OverflowPolicy) *queue {
	return &queue{items: make([]work, capacity), policy: policy, changed: make(chan struct{})}
}
func (q *queue) signal() { close(q.changed); q.changed = make(chan struct{}) }
func (q *queue) push(ctx context.Context, stop <-chan struct{}, w work) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-stop:
			return BusIsShuttingDownError
		default:
		}
		if q.size < len(q.items) {
			break
		}
		switch q.policy {
		case OverflowDropNewest:
			return ErrQueueFull
		case OverflowDropOldest:
			q.items[q.head] = work{}
			q.head = (q.head + 1) % len(q.items)
			q.size--
		default:
			changed := q.changed
			q.mu.Unlock()
			select {
			case <-changed:
			case <-ctx.Done():
			case <-stop:
			}
			q.mu.Lock()
		}
	}
	q.items[(q.head+q.size)%len(q.items)] = w
	q.size++
	q.signal()
	return nil
}
func (q *queue) pop() (work, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.size == 0 {
		if q.closing {
			return work{}, false
		}
		changed := q.changed
		q.mu.Unlock()
		<-changed
		q.mu.Lock()
	}
	w := q.items[q.head]
	q.items[q.head] = work{}
	q.head = (q.head + 1) % len(q.items)
	q.size--
	q.signal()
	return w, true
}
func (q *queue) close() { q.mu.Lock(); defer q.mu.Unlock(); q.closing = true; q.signal() }

package event

import (
	"context"
	"log/slog"
	"sync"
)

// queue is a bounded ring. changed broadcasts state transitions to workers and
// publishers; producers never send to a channel that shutdown could close.
type queue struct {
	logger     LogHandler
	logInfo    LogEvent
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
	var pending *LogEvent
	defer func() {
		q.mu.Unlock()
		if pending != nil {
			q.logger.log(ctx, *pending)
		}
	}()
	warned := false
admit:
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
		var record LogEvent
		if q.logger != nil && !warned {
			record = q.record(LogQueueOverflow, slog.LevelWarn, "queue full; applying overflow policy", ErrQueueOverflow, w.event)
			record.QueueLength = q.size
		}
		switch q.policy {
		case OverflowDropNewest:
			if q.logger != nil {
				record.Error, record.DroppedEvent = ErrQueueFull, w.event
				pending = &record
			}
			return ErrQueueFull
		case OverflowDropOldest:
			if q.logger != nil {
				record.DroppedEvent = q.items[q.head].event
				pending = &record
			}
			q.items[q.head] = work{}
			q.head = (q.head + 1) % len(q.items)
			q.size--
			// Eviction and replacement commit together, with no second cancellation
			// check that could discard the old item without admitting its replacement.
			break admit
		default:
			changed := q.changed
			q.mu.Unlock()
			if q.logger != nil && !warned {
				q.logger.log(ctx, record)
				warned = true
			}
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

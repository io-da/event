package event

import (
	"context"
	"log/slog"
)

// LogHandler receives diagnostic records synchronously, outside bus/queue locks.
// It must be concurrency-safe and return promptly. Do not synchronously wait for
// this bus to shut down or publish recursively from the callback. Panics are
// contained without recursively logging them. A nil handler disables logging.
type LogHandler func(context.Context, LogEvent)

// LogKind identifies a diagnostic without requiring message parsing.
type LogKind string

const (
	LogQueueOverflow     LogKind = "queue_overflow"
	LogHandlerError      LogKind = "handler_error"
	LogEventCanceled     LogKind = "event_canceled"
	LogErrorHandlerPanic LogKind = "error_handler_panic"
)

// LogEvent describes one subscription's diagnostic. Event and DroppedEvent are
// references to application values; callbacks must not mutate them.
type LogEvent struct {
	Kind           LogKind
	Level          slog.Level
	Message        string
	Error          error
	Event          Event // Incoming event, including for drop-oldest warnings.
	DroppedEvent   Event // Evicted/rejected event; nil when no event was dropped.
	EventID        Identifier
	SubscriptionID int        // Zero-based position in Initialize's handler arguments.
	TopicID        Identifier // Meaningful only when HasTopic is true; zero is a valid ID.
	HasTopic       bool
	OverflowPolicy OverflowPolicy
	QueueCapacity  int
	QueueLength    int // Snapshot before applying overflow; otherwise zero.
}

func (h LogHandler) log(ctx context.Context, e LogEvent) {
	if h == nil {
		return
	}
	defer func() { _ = recover() }()
	h(ctx, e)
}

func (q *queue) record(kind LogKind, level slog.Level, message string, err error, evt Event) LogEvent {
	e := q.logInfo
	e.Kind, e.Level, e.Message, e.Error, e.Event = kind, level, message, err, evt
	return e
}

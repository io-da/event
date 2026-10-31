package event

import "context"

// Handler subscribes to one event identifier. Implementations may be called concurrently.
type Handler interface {
	Handles() Identifier
	Handle(context.Context, Event) error
}

// HandlerFunc is the processing function used by middleware.
type HandlerFunc func(context.Context, Event) error

// Middleware wraps a handler; the first configured middleware is outermost.
type Middleware func(HandlerFunc) HandlerFunc

type typedHandler[T Event] struct {
	id Identifier
	fn func(context.Context, T) error
}

// NewHandler adapts a typed function. Identifier/type mismatches return InvalidEventError.
func NewHandler[T Event](id Identifier, fn func(context.Context, T) error) Handler {
	if fn == nil {
		panic("event: nil handler function")
	}
	return typedHandler[T]{id, fn}
}
func (h typedHandler[T]) Handles() Identifier { return h.id }
func (h typedHandler[T]) Handle(ctx context.Context, evt Event) error {
	e, ok := evt.(T)
	if !ok {
		return InvalidEventError
	}
	return h.fn(ctx, e)
}

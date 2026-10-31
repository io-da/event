package event

import "context"

// ErrorHandler receives processing errors, including recovered panics.
// Implementations must support concurrent calls. Panics are contained so other
// error handlers still run; they are not recursively reported.
type ErrorHandler interface {
	Handle(context.Context, error, Event)
}

// ErrorHandlerFunc adapts a function to ErrorHandler.
type ErrorHandlerFunc func(context.Context, error, Event)

func (f ErrorHandlerFunc) Handle(ctx context.Context, err error, evt Event) { f(ctx, err, evt) }

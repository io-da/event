package event

import "context"

// ErrorHandler is the interface that must be implemented by any type to be considered an error handler.
type ErrorHandler interface {
	Handle(ctx context.Context, evt Event, err error)
}

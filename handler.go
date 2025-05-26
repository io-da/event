package event

import "context"

// Handler must be implemented for a type to qualify as an event handler.
type Handler interface {
	Handle(ctx context.Context, evt Event) error
}

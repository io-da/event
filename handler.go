package event

// Handler must be implemented for a type to qualify as an event handler.
type Handler interface {
	Handle(evt Event) error
}

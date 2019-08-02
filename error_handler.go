package event

// Handler must be implemented for a type to qualify as a query handler.
type ErrorHandler interface {
	Handle(evt Event, err error)
}

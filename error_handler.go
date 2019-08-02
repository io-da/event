package event

// ErrorHandler is the interface that must be implemented by any type to be considered an error handler.
type ErrorHandler interface {
	Handle(evt Event, err error)
}

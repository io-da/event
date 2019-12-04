package event

// ErrorInvalidEvent is used when invalid events are handled.
type ErrorInvalidEvent string

// Error returns the string message of ErrorInvalidEvent.
func (e ErrorInvalidEvent) Error() string {
	return string(e)
}

// ErrorBusNotInitialized is used when events are emitted but the bus is not initialized.
type ErrorBusNotInitialized string

// Error returns the string message of ErrorBusNotInitialized.
func (e ErrorBusNotInitialized) Error() string {
	return string(e)
}

// ErrorBusIsShuttingDown is used when events are emitted but the bus is shutting down.
type ErrorBusIsShuttingDown string

// Error returns the string message of ErrorBusIsShuttingDown.
func (e ErrorBusIsShuttingDown) Error() string {
	return string(e)
}

const (
	// InvalidEventError is a constant equivalent of the ErrorInvalidEvent error.
	InvalidEventError = ErrorInvalidEvent("event: invalid event")
	// BusNotInitializedError is a constant equivalent of the ErrorBusNotInitialized error.
	BusNotInitializedError = ErrorBusNotInitialized("event: the bus is not initialized")
	// BusIsShuttingDownError is a constant equivalent of the ErrorBusIsShuttingDown error.
	BusIsShuttingDownError = ErrorBusIsShuttingDown("event: the bus is shutting down")
)

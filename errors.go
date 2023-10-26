package event

// BusError is used to create errors originating from the event bus
type BusError string

// Error returns the string message of the error.
func (e BusError) Error() string {
	return string(e)
}

const (
	// InvalidEventError is returned when attempting to handle an invalid event type.
	InvalidEventError = BusError("event: invalid event")
	// BusNotInitializedError is returned when events are emitted but the bus is not initialized.
	BusNotInitializedError = BusError("event: the bus is not initialized")
	// BusIsShuttingDownError is returned when events are emitted but the bus is shutting down.
	BusIsShuttingDownError = BusError("event: the bus is shutting down")
)

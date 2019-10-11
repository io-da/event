package event

type ErrorInvalidEvent string

func (e ErrorInvalidEvent) Error() string {
	return string(e)
}

type ErrorEventBusNotInitialized string

func (e ErrorEventBusNotInitialized) Error() string {
	return string(e)
}

type ErrorEventBusIsShuttingDown string

func (e ErrorEventBusIsShuttingDown) Error() string {
	return string(e)
}

const (
	InvalidEventError           = ErrorInvalidEvent("event: invalid event")
	EventBusNotInitializedError = ErrorEventBusNotInitialized("event: the event bus is not initialized")
	EventBusIsShuttingDownError = ErrorEventBusIsShuttingDown("event: the event bus is shutting down")
)

package event

import "fmt"

// BusError is a constant sentinel error originating from the bus.
type BusError string

func (e BusError) Error() string { return string(e) }

const (
	InvalidEventError          BusError = "event: invalid event"
	BusNotInitializedError     BusError = "event: the bus is not initialized"
	BusIsShuttingDownError     BusError = "event: the bus is shutting down"
	BusAlreadyInitializedError BusError = "event: the bus is already initialized"
	InvalidHandlerError        BusError = "event: invalid handler"
	ErrQueueFull               BusError = "event: queue full; newest event dropped"
)

// ErrHandlerPanic describes a recovered handler or middleware panic.
type ErrHandlerPanic struct {
	Value any
	Stack []byte
}

func (e *ErrHandlerPanic) Error() string { return fmt.Sprintf("event: handler panic: %v", e.Value) }

package event

// Event is the interface that must be implemented by any type to be considered an event.
type Event interface {
	ID() []byte
}

// Topic must be implemented by events whose order of emission must be respected.
type Topic interface {
	Event
	Topic() string
}

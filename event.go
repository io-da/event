package event

// Identifier is used to create a consistent identity solution for events and topics.
type Identifier int64

// Event is the interface that must be implemented by any type to be considered an event.
type Event interface {
	Identifier() Identifier
}

// Topic must be implemented by events whose order of emission must be respected.
type Topic interface {
	Event
	Topic() Identifier
}

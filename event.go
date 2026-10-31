package event

// Identifier is used to create a consistent identity solution for events and topics.
type Identifier int64

// Event is the interface that must be implemented by any type to be considered an event.
type Event interface {
	Identifier() Identifier
}

// Topic identifies an ordering lane within each subscription. Sequential
// publications retain their order within that lane; different subscribers and
// topics run independently. Concurrent publishers have no defined relative order.
type Topic interface {
	Event
	Topic() Identifier
}

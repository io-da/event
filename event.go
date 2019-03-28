package event

// Event is an empty interface used for reference and readability purposes.
type Event interface {
}

// Topic must be implemented by events whose order of emission must be respected.
type Topic interface {
	Topic() string
}

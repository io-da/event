package event

import (
	"runtime"
	"sync/atomic"
)

// Bus is the only struct exported and required for the event bus usage.
// The Bus should be instantiated using the NewBus function.
type Bus struct {
	concurrentPoolSize int
	topicsCapacity     int
	topicBuffer        int
	controller         *controller
	shuttingDown       *uint32
}

// NewBus instantiates the Bus struct.
// The Initialization of the Bus is performed separately (Initialize function) for dependency injection purposes.
func NewBus() *Bus {
	return &Bus{
		concurrentPoolSize: runtime.GOMAXPROCS(0),
		topicsCapacity:     10,
		topicBuffer:        100,
		shuttingDown:       new(uint32),
	}
}

// ConcurrentPoolSize may optionally be provided to tweak the worker pool size for concurrent events.
// It can only be adjusted *before* the bus is initialized.
// It defaults to the value returned by runtime.GOMAXPROCS(0).
func (bus *Bus) ConcurrentPoolSize(concurrentPoolSize int) {
	bus.concurrentPoolSize = concurrentPoolSize
}

// TopicsCapacity may optionally be provided to tweak the starting capacity on the topic slice.
// Ideally this value would be equal to the total amount of available topics.
// It can only be adjusted *before* the bus is initialized.
// It defaults to 10.
func (bus *Bus) TopicsCapacity(topicsCapacity int) {
	bus.topicsCapacity = topicsCapacity
}

// TopicBuffer may optionally be provided to tweak the buffer size of topics.
// This value may have high impact on performance depending on the use case.
// It can only be adjusted *before* the bus is initialized.
// It defaults to 100.
func (bus *Bus) TopicBuffer(topicBuffer int) {
	bus.topicBuffer = topicBuffer
}

// Initialize the event bus
func (bus *Bus) Initialize(hdls ...Handler) {
	// start the controller
	bus.controller = newController(bus.concurrentPoolSize, bus.topicsCapacity, bus.topicBuffer, hdls)
}

// Emit an Event to the event bus.
func (bus *Bus) Emit(evt Event) {
	if !bus.isShuttingDown() {
		bus.controller.handle(evt)
	}
}

// Shutdown the event bus gracefully.
// *Events emitted while shutting down will be disregarded*.
func (bus *Bus) Shutdown() {
	atomic.StoreUint32(bus.shuttingDown, 1)
	bus.controller.shutdown()
}

func (bus *Bus) isShuttingDown() bool {
	return atomic.LoadUint32(bus.shuttingDown) == 1
}

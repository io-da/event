package event

import (
	"runtime"
	"sync/atomic"
)

// Bus is the only struct exported and required for the event bus usage.
// The Bus should be instantiated using the NewBus function.
type Bus struct {
	concurrentPoolSize int
	controller         *controller
	shuttingDown       *uint32
}

// NewBus instantiates the Bus struct.
// The Initialization of the Bus is performed separately (Initialize function) for dependency injection purposes.
func NewBus() *Bus {
	return &Bus{
		concurrentPoolSize: runtime.GOMAXPROCS(0),
		shuttingDown:       new(uint32),
	}
}

// ConcurrentPoolSize may optionally be used to tweak the worker pool size for concurrent events.
// It can only be adjusted *before* the bus is initialized.
func (bus *Bus) ConcurrentPoolSize(concurrentPoolSize int) {
	bus.concurrentPoolSize = concurrentPoolSize
}

// Initialize the event bus
func (bus *Bus) Initialize(hdls ...Handler) {
	// start the controller
	bus.controller = newController(bus.concurrentPoolSize, hdls)
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

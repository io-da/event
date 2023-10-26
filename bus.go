package event

import (
	"runtime"
)

// Bus is the only struct exported and required for the event bus usage.
// The Bus should be instantiated using the NewBus function.
type Bus struct {
	concurrentWorkerPoolSize int
	topicsCapacity           int
	topicBuffer              int
	initialized              *flag
	shuttingDown             *flag
	workers                  *counter
	handlers                 []Handler
	errorHandlers            []ErrorHandler
	topics                   []*topic
	concurrentQueuedEvents   chan Event
	closed                   chan bool
}

// NewBus instantiates the Bus struct.
// The Initialization of the Bus is performed separately (Initialize function) for dependency injection purposes.
func NewBus() *Bus {
	return &Bus{
		concurrentWorkerPoolSize: runtime.GOMAXPROCS(0),
		topicsCapacity:           10,
		topicBuffer:              100,
		initialized:              newFlag(),
		shuttingDown:             newFlag(),
		workers:                  newCounter(),
		errorHandlers:            make([]ErrorHandler, 0),
		closed:                   make(chan bool),
	}
}

// SetConcurrentWorkerPoolSize may optionally be provided to tweak the worker pool size for concurrent events.
// It can only be adjusted *before* the bus is initialized.
// It defaults to the value returned by runtime.GOMAXPROCS(0).
func (bus *Bus) SetConcurrentWorkerPoolSize(concurrentWorkerPoolSize int) {
	if !bus.initialized.enabled() {
		bus.concurrentWorkerPoolSize = concurrentWorkerPoolSize
	}
}

// SetErrorHandlers may optionally be used to provide a list of error handlers.
// They will receive any error thrown during the event handling process.
func (bus *Bus) SetErrorHandlers(hdls ...ErrorHandler) {
	if !bus.initialized.enabled() {
		bus.errorHandlers = hdls
	}
}

// SetTopicsCapacity may optionally be used to tweak the starting capacity on the topic slice.
// Ideally this value would be equal to the total amount of available topics.
// It can only be adjusted *before* the bus is initialized.
// It defaults to 10.
func (bus *Bus) SetTopicsCapacity(topicsCapacity int) {
	if !bus.initialized.enabled() {
		bus.topicsCapacity = topicsCapacity
	}
}

// SetTopicBuffer may optionally be used to tweak the buffer size of topics.
// This value may have high impact on performance depending on the use case.
// It can only be adjusted *before* the bus is initialized.
// It defaults to 100.
func (bus *Bus) SetTopicBuffer(topicBuffer int) {
	if !bus.initialized.enabled() {
		bus.topicBuffer = topicBuffer
	}
}

// Initialize the event bus.
func (bus *Bus) Initialize(hdls ...Handler) {
	if bus.initialized.enable() {
		bus.handlers = hdls
		bus.topics = make([]*topic, 0, bus.topicsCapacity)
		bus.concurrentQueuedEvents = make(chan Event, bus.topicBuffer)
		for i := 0; i < bus.concurrentWorkerPoolSize; i++ {
			bus.workers.increment()
			go bus.worker(bus.concurrentQueuedEvents, bus.closed)
		}
	}
}

// Emit an Event to the event bus.
func (bus *Bus) Emit(evt Event) {
	if bus.isValid(evt) {
		if evt, implements := evt.(Topic); implements {
			tpc := bus.topic(evt)
			tpc.handle(evt)
			return
		}
		bus.concurrentQueuedEvents <- evt
	}
}

// Shutdown the event bus gracefully.
// *Events emitted while shutting down will be disregarded*.
func (bus *Bus) Shutdown() {
	if bus.shuttingDown.enable() {
		bus.shutdown()
	}
}

//-----Private Functions------//

func (bus *Bus) worker(queuedEvents <-chan Event, closed chan<- bool) {
	for evt := range queuedEvents {
		if evt == nil {
			break
		}
		for _, hdl := range bus.handlers {
			if err := hdl.Handle(evt); err != nil {
				bus.error(evt, err)
			}
		}
	}
	closed <- true
}

func (bus *Bus) shutdown() {
	for !bus.workers.is(0) {
		bus.concurrentQueuedEvents <- nil
		<-bus.closed
		bus.workers.decrement()
	}
	for _, tpc := range bus.topics {
		tpc.shutdown()
	}
	bus.initialized.disable()
	bus.shuttingDown.disable()
}

func (bus *Bus) topic(evt Topic) *topic {
	for _, tpc := range bus.topics {
		if tpc.id == evt.Topic() {
			return tpc
		}
	}
	tpc := newTopic(evt.Topic(), bus.topicBuffer, &bus.handlers, &bus.errorHandlers)
	bus.addTopic(tpc)
	return tpc
}

func (bus *Bus) addTopic(tpc *topic) {
	if len(bus.topics) == cap(bus.topics) {
		bus.doubleCapacity()
	}
	bus.topics = append(bus.topics, tpc)
}

func (bus *Bus) doubleCapacity() {
	l := len(bus.topics)
	c := cap(bus.topics) * 2

	nTpc := make([]*topic, l, c)
	copy(nTpc, bus.topics)
	bus.topics = nTpc
}

func (bus *Bus) isValid(evt Event) bool {
	if evt == nil {
		bus.error(evt, InvalidEventError)
		return false
	}
	if !bus.initialized.enabled() {
		bus.error(evt, BusNotInitializedError)
		return false
	}
	if bus.shuttingDown.enabled() {
		bus.error(evt, BusIsShuttingDownError)
		return false
	}
	return true
}

func (bus *Bus) error(evt Event, err error) {
	for _, errHdl := range bus.errorHandlers {
		errHdl.Handle(evt, err)
	}
}

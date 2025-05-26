package event

import (
	"context"
	"runtime"
)

// Bus is the only struct exported and required for the event bus usage.
// The Bus should be instantiated using the NewBus function.
type Bus struct {
	concurrentWorkerPoolSize int
	topicBuffer              int
	initialized              *flag
	shuttingDown             *flag
	workers                  *counter
	handlers                 []Handler
	errorHandlers            []ErrorHandler
	topics                   []*topic
	concurrentQueue          chan wrapper
	closed                   chan bool
}

// NewBus instantiates the Bus struct.
// The Initialization of the Bus is performed separately (Initialize function) for dependency injection purposes.
func NewBus() *Bus {
	return &Bus{
		concurrentWorkerPoolSize: runtime.GOMAXPROCS(0),
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

// SetTopicBuffer may optionally be used to tweak the buffer size of topics.
// This value may have high impact on performance depending on the use case.
// It can only be adjusted *before* the bus is initialized.
// It affects the buffer size of topics added afterwards.
// It does not affect the buffer size of topics added prior to this call.
// It defaults to 100.
func (bus *Bus) SetTopicBuffer(topicBuffer int) {
	if !bus.initialized.enabled() {
		bus.topicBuffer = topicBuffer
	}
}

// AddTopics may be used to provide topics that will be used by the bus.
// It can only be adjusted *before* the bus is initialized.
// It defaults to an empty list. Meaning no topics are available.
// Topics are unique by their identifier. If an existing identifier is added, it will be ignored.
func (bus *Bus) AddTopics(ids ...Identifier) {
	if !bus.initialized.enabled() {
		if bus.topics == nil {
			bus.topics = make([]*topic, 0, len(ids))
		}
		for _, id := range ids {
			tpc := bus.topic(id)
			if tpc != nil {
				continue
			}
			ntpc := newTopic(id, bus.topicBuffer, &bus.handlers, &bus.errorHandlers)
			bus.addTopic(ntpc)
		}
	}
}

// Initialize the event bus.
func (bus *Bus) Initialize(hdls ...Handler) {
	if bus.initialized.enable() {
		bus.handlers = hdls
		bus.concurrentQueue = make(chan wrapper, bus.topicBuffer)
		for i := 0; i < bus.concurrentWorkerPoolSize; i++ {
			go bus.worker(bus.concurrentQueue, bus.closed)
		}
	}
}

// Emit an Event to the event bus.
func (bus *Bus) Emit(ctx context.Context, evt Event) error {
	if err := bus.validate(evt); err != nil {
		return err
	}
	if evt, implements := evt.(Topic); implements {
		if tpc := bus.topic(evt.Topic()); tpc != nil {
			tpc.handle(ctx, evt)
			return nil
		}
		return InvalidTopicError
	}
	bus.handleConcurrent(ctx, evt)
	return nil
}

// Shutdown the event bus gracefully.
// *Events emitted while shutting down will be disregarded*.
func (bus *Bus) Shutdown() {
	if bus.shuttingDown.enable() {
		bus.shutdown()
	}
}

//-----Private Functions------//

func (bus *Bus) worker(queuedEvents <-chan wrapper, closed chan<- bool) {
	bus.workers.increment()
	for evt := range queuedEvents {
		if evt.shutdown() {
			break
		}
		for _, hdl := range bus.handlers {
			if err := hdl.Handle(evt.ctx, evt.evt); err != nil {
				bus.error(evt, err)
			}
		}
	}
	closed <- true
}

func (bus *Bus) handleConcurrent(ctx context.Context, evt Event) {
	bus.concurrentQueue <- wrapper{ctx, evt}
}

func (bus *Bus) shutdown() {
	for !bus.workers.is(0) {
		bus.concurrentQueue <- shutdown
		<-bus.closed
		bus.workers.decrement()
	}
	for _, tpc := range bus.topics {
		tpc.shutdown()
	}
	bus.initialized.disable()
	bus.shuttingDown.disable()
}

func (bus *Bus) topic(id Identifier) *topic {
	for _, tpc := range bus.topics {
		if tpc.id == id {
			return tpc
		}
	}
	return nil
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

	topics := make([]*topic, l, c)
	copy(topics, bus.topics)
	bus.topics = topics
}

func (bus *Bus) validate(evt Event) error {
	if evt == nil {
		return InvalidEventError
	}
	if !bus.initialized.enabled() {
		return BusNotInitializedError
	}
	if bus.shuttingDown.enabled() {
		return BusIsShuttingDownError
	}
	return nil
}

func (bus *Bus) error(evt wrapper, err error) {
	for _, errHdl := range bus.errorHandlers {
		errHdl.Handle(evt.ctx, evt.evt, err)
	}
}

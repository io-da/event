package event

import (
	"runtime"
	"sync/atomic"
)

// Bus is the only struct exported and required for the event bus usage.
// The Bus should be instantiated using the NewBus function.
type Bus struct {
	concurrentPoolSize     int
	topicsCapacity         int
	topicBuffer            int
	shuttingDown           *uint32
	workers                *uint32
	handlers               []Handler
	topics                 []*topic
	concurrentQueuedEvents chan Event
	closed                 chan bool
}

// NewBus instantiates the Bus struct.
// The Initialization of the Bus is performed separately (Initialize function) for dependency injection purposes.
func NewBus() *Bus {
	return &Bus{
		concurrentPoolSize: runtime.GOMAXPROCS(0),
		topicsCapacity:     10,
		topicBuffer:        100,
		shuttingDown:       new(uint32),
		workers:            new(uint32),
		closed:             make(chan bool),
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

// Initialize the event bus.
func (bus *Bus) Initialize(hdls ...Handler) {
	bus.handlers = hdls
	bus.topics = make([]*topic, 0, bus.topicsCapacity)
	bus.concurrentQueuedEvents = make(chan Event, bus.topicBuffer)
	for i := 0; i < bus.concurrentPoolSize; i++ {
		bus.workerUp()
		go bus.worker(bus.concurrentQueuedEvents, bus.closed)
	}
}

// Emit an Event to the event bus.
func (bus *Bus) Emit(evt Event) {
	if evt != nil && !bus.isShuttingDown() {
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
	atomic.StoreUint32(bus.shuttingDown, 1)
	bus.shutdown()
}

//-----Private Functions------//

func (bus *Bus) isShuttingDown() bool {
	return atomic.LoadUint32(bus.shuttingDown) == 1
}

func (bus *Bus) worker(queuedEvents <-chan Event, closed chan<- bool) {
	for evt := range queuedEvents {
		if evt == nil {
			break
		}
		for _, hdl := range bus.handlers {
			if hdl.ListensTo(evt) {
				hdl.Handle(evt)
			}
		}
	}
	closed <- true
}

func (bus *Bus) workerUp() {
	atomic.AddUint32(bus.workers, 1)
}

func (bus *Bus) workerDown() {
	atomic.AddUint32(bus.workers, ^uint32(0))
}

func (bus *Bus) shutdown() {
	for atomic.LoadUint32(bus.workers) > 0 {
		bus.concurrentQueuedEvents <- nil
		<-bus.closed
		bus.workerDown()
	}
	for _, tpc := range bus.topics {
		tpc.shutdown()
	}
}

func (bus *Bus) topic(evt Topic) *topic {
	for _, tpc := range bus.topics {
		if tpc.name == evt.Topic() {
			return tpc
		}
	}
	tpc := newTopic(evt.Topic(), bus.topicBuffer, &bus.handlers)
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

package event

import (
	"log"
	"sync/atomic"
)

const topicsCapacity = 10

type controller struct {
	topics                 []*topic
	concurrentQueuedEvents chan Event
	workers                *uint32
	closed                 chan bool
	handlers               []Handler
}

func newController(concurrentPoolSize int, handlers []Handler) *controller {
	ctrl := &controller{
		topics:                 make([]*topic, 0, topicsCapacity),
		concurrentQueuedEvents: make(chan Event, topicBuffer),
		workers:                new(uint32),
		closed:                 make(chan bool),
		handlers:               handlers,
	}

	for i := 0; i < concurrentPoolSize; i++ {
		ctrl.workerUp()
		go ctrl.worker(ctrl.concurrentQueuedEvents, ctrl.closed)
	}

	return ctrl
}

func (ctrl *controller) handle(evt Event) {
	if evt == nil {
		return
	}
	if evt, implements := evt.(Topic); implements {
		tpc := ctrl.topic(evt)
		tpc.handle(evt)
		return
	}

	ctrl.concurrentQueuedEvents <- evt
}

func (ctrl *controller) len() int {
	return len(ctrl.topics)
}

func (ctrl *controller) cap() int {
	return cap(ctrl.topics)
}

func (ctrl *controller) empty() bool {
	return len(ctrl.topics) <= 0
}

func (ctrl *controller) worker(queuedEvents <-chan Event, closed chan<- bool) {
	for evt := range queuedEvents {
		if evt == nil {
			log.Println("eventbus concurrent worker shutting down")
			break
		}
		for _, hdl := range ctrl.handlers {
			if hdl.ListensTo(evt) {
				hdl.Handle(evt)
			}
		}
	}
	closed <- true
}

func (ctrl *controller) workerUp() {
	atomic.AddUint32(ctrl.workers, 1)
}

func (ctrl *controller) workerDown() {
	atomic.AddUint32(ctrl.workers, ^uint32(0))
}

func (ctrl *controller) shutdown() {
	for atomic.LoadUint32(ctrl.workers) > 0 {
		ctrl.concurrentQueuedEvents <- nil
		<-ctrl.closed
		ctrl.workerDown()
	}
	for _, tpc := range ctrl.topics {
		tpc.shutdown()
	}
}

func (ctrl *controller) topic(evt Topic) *topic {
	for _, tpc := range ctrl.topics {
		if tpc.name == evt.Topic() {
			return tpc
		}
	}
	tpc := newTopic(evt.Topic(), &ctrl.handlers)
	ctrl.addTopic(tpc)
	return tpc
}

func (ctrl *controller) addTopic(tpc *topic) {
	if len(ctrl.topics) == cap(ctrl.topics) {
		ctrl.doubleCapacity()
	}
	ctrl.topics = append(ctrl.topics, tpc)
}

func (ctrl *controller) doubleCapacity() {
	l := len(ctrl.topics)
	c := cap(ctrl.topics) * 2

	nTpc := make([]*topic, l, c)
	copy(nTpc, ctrl.topics)
	ctrl.topics = nTpc
}

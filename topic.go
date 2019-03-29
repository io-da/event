package event

import "log"

const topicBuffer = 100

type topic struct {
	name         string
	idx          int
	queuedEvents chan Event
	closed       chan bool

	handlers *[]Handler
}

func newTopic(name string, handlers *[]Handler) *topic {
	tpc := &topic{
		name:         name,
		queuedEvents: make(chan Event, topicBuffer),
		closed:       make(chan bool),

		handlers: handlers,
	}

	go tpc.worker(tpc.queuedEvents, tpc.closed)

	return tpc
}

func (tpc *topic) handle(evt Event) {
	tpc.queuedEvents <- evt
}

func (tpc *topic) worker(queuedEvents <-chan Event, closed chan<- bool) {
	for evt := range queuedEvents {
		if evt == nil {
			log.Printf("eventbus topic worker \"%s\" shutting down", tpc.name)
			break
		}
		for _, hdl := range *tpc.handlers {
			if hdl.ListensTo(evt) {
				hdl.Handle(evt)
			}
		}
	}
	closed <- true
}

func (tpc *topic) shutdown() {
	tpc.queuedEvents <- nil
	<-tpc.closed
}

package event

import "log"

type topic struct {
	name         string
	queuedEvents chan Event
	closed       chan bool
	handlers     *[]Handler
}

func newTopic(name string, buffer int, handlers *[]Handler) *topic {
	tpc := &topic{
		name:         name,
		queuedEvents: make(chan Event, buffer),
		closed:       make(chan bool),
		handlers:     handlers,
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

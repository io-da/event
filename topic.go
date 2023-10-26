package event

type topic struct {
	id            Identifier
	queuedEvents  chan Event
	closed        chan bool
	handlers      *[]Handler
	errorHandlers *[]ErrorHandler
}

func newTopic(id Identifier, buffer int, handlers *[]Handler, errorHandlers *[]ErrorHandler) *topic {
	tpc := &topic{
		id:            id,
		queuedEvents:  make(chan Event, buffer),
		closed:        make(chan bool),
		handlers:      handlers,
		errorHandlers: errorHandlers,
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
			break
		}
		for _, hdl := range *tpc.handlers {
			if err := hdl.Handle(evt); err != nil {
				tpc.error(evt, err)
			}
		}
	}
	closed <- true
}

func (tpc *topic) shutdown() {
	tpc.queuedEvents <- nil
	<-tpc.closed
}

func (tpc *topic) error(evt Event, err error) {
	for _, errHdl := range *tpc.errorHandlers {
		errHdl.Handle(evt, err)
	}
}

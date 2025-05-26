package event

import "context"

type topic struct {
	id            Identifier
	queuedEvents  chan wrapper
	closed        chan bool
	handlers      *[]Handler
	errorHandlers *[]ErrorHandler
}

func newTopic(id Identifier, buffer int, handlers *[]Handler, errorHandlers *[]ErrorHandler) *topic {
	tpc := &topic{
		id:            id,
		queuedEvents:  make(chan wrapper, buffer),
		closed:        make(chan bool),
		handlers:      handlers,
		errorHandlers: errorHandlers,
	}

	go tpc.worker(tpc.queuedEvents, tpc.closed)

	return tpc
}

func (tpc *topic) handle(ctx context.Context, evt Event) {
	tpc.queuedEvents <- wrapper{ctx, evt}
}

func (tpc *topic) worker(queuedEvents <-chan wrapper, closed chan<- bool) {
	for wrp := range queuedEvents {
		if wrp.shutdown() {
			break
		}
		for _, hdl := range *tpc.handlers {
			if err := hdl.Handle(wrp.ctx, wrp.evt); err != nil {
				tpc.error(wrp, err)
			}
		}
	}
	closed <- true
}

func (tpc *topic) shutdown() {
	tpc.queuedEvents <- shutdown
	<-tpc.closed
}

func (tpc *topic) error(evt wrapper, err error) {
	for _, errHdl := range *tpc.errorHandlers {
		errHdl.Handle(evt.ctx, evt.evt, err)
	}
}

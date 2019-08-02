package event

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

//------Events------//

type testEvent1 struct{}

func (*testEvent1) Topic() string { return "test:topic" }
func (*testEvent1) ID() []byte {
	return []byte("UUID")
}

type testEvent2 struct{}

func (*testEvent2) Topic() string { return "test:topic-2" }
func (*testEvent2) ID() []byte {
	return []byte("UUID")
}

type testEvent3 string

func (testEvent3) Topic() string { return "test:topic-2" }
func (testEvent3) ID() []byte {
	return []byte("UUID")
}

type testEventError struct{}

func (*testEventError) ID() []byte {
	return []byte("UUID1")
}

type testEventError2 struct{}

func (*testEventError2) Topic() string { return "test:topic" }
func (*testEventError2) ID() []byte {
	return []byte("UUID2")
}

type testEventDynamicTopic struct {
	Tpc string
}

func (evt *testEventDynamicTopic) Topic() string {
	return evt.Tpc
}
func (*testEventDynamicTopic) ID() []byte {
	return []byte("UUID")
}

type eventOrder interface {
	Position() int32
}

type testEventOrder struct {
	position int32
}

func (evt *testEventOrder) Topic() string   { return "test:Event-order" }
func (evt *testEventOrder) Position() int32 { return evt.position }
func (*testEventOrder) ID() []byte {
	return []byte("UUID")
}

type testEventConcurrent struct {
	position int32
}

func (evt *testEventConcurrent) Position() int32 { return evt.position }
func (*testEventConcurrent) ID() []byte {
	return []byte("UUID")
}

type testHandlerOrderEvent struct {
	position  *uint32
	unordered *uint32
}

func (evt *testHandlerOrderEvent) HandlerPosition(position uint32) {
	if position != atomic.LoadUint32(evt.position) {
		atomic.StoreUint32(evt.unordered, 1)
	}
	atomic.AddUint32(evt.position, 1)

}
func (evt *testHandlerOrderEvent) IsUnordered() bool {
	return atomic.LoadUint32(evt.unordered) == 1
}
func (*testHandlerOrderEvent) ID() []byte {
	return []byte("UUID")
}

type benchmarkOrderedEvent struct {
}

func (evt *benchmarkOrderedEvent) Topic() string { return "benchmark:ordered" }
func (*benchmarkOrderedEvent) ID() []byte {
	return []byte("UUID")
}

type benchmarkConcurrentEvent struct {
}

func (*benchmarkConcurrentEvent) ID() []byte {
	return []byte("UUID")
}

//------Handlers------//

type testHandler1 struct {
	wg *sync.WaitGroup
}

func (hdl *testHandler1) Handle(evt Event) error {
	if _, listens := evt.(*testEvent1); listens {
		hdl.wg.Done()
	}
	return nil
}

type testHandler2 struct {
	wg *sync.WaitGroup
}

func (hdl *testHandler2) Handle(evt Event) error {
	switch evt.(type) {
	case *testEvent2, testEvent3, *testEventDynamicTopic:
		hdl.wg.Done()
	}
	return nil
}

type emptyHandler struct {
}

func (hdl *emptyHandler) Handle(evt Event) error {
	// handles everything
	return nil
}

type testHandlerWithError struct {
	wg *sync.WaitGroup
}

func (hdl *testHandlerWithError) Handle(evt Event) error {
	switch evt.(type) {
	case *testEventError, *testEventError2:
		hdl.wg.Done()
		return errors.New("event failed")
	}
	return nil
}

type testHandlerOrder struct {
	wg       *sync.WaitGroup
	position uint32
}

func (hdl *testHandlerOrder) Handle(evt Event) error {
	if evt, listens := evt.(*testHandlerOrderEvent); listens {
		evt.HandlerPosition(hdl.position)
		hdl.wg.Done()
	}
	return nil
}

type testEventOrderHandler struct {
	wg        *sync.WaitGroup
	position  *int32
	unordered *int32
}

func (hdl *testEventOrderHandler) Handle(evt Event) error {
	if evt, listens := evt.(eventOrder); listens {
		// delay the handling of the Event to simulate some sort of behavior.
		time.Sleep(time.Nanosecond * 200)

		if evt.Position() != hdl.currentPosition() {
			atomic.StoreInt32(hdl.unordered, 1)
		}
		hdl.incrementPosition()
		hdl.wg.Done()
	}
	return nil
}
func (hdl *testEventOrderHandler) IsUnordered() bool {
	return atomic.LoadInt32(hdl.unordered) == 1
}
func (hdl *testEventOrderHandler) currentPosition() int32 {
	return atomic.LoadInt32(hdl.position)
}
func (hdl *testEventOrderHandler) incrementPosition() {
	atomic.AddInt32(hdl.position, 1)
}

type benchmarkOrderedEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkOrderedEventHandler) Handle(evt Event) error {
	if _, listens := evt.(*benchmarkOrderedEvent); listens {
		time.Sleep(time.Nanosecond * 200)
		hdl.wg.Done()
	}
	return nil
}

type benchmarkConcurrentEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkConcurrentEventHandler) Handle(evt Event) error {
	if _, listens := evt.(*benchmarkConcurrentEvent); listens {
		time.Sleep(time.Nanosecond * 200)
		hdl.wg.Done()
	}
	return nil
}

//------Error Handlers------//

type storeErrorsHandler struct {
	sync.Mutex
	errs map[string]error
}

func (hdl *storeErrorsHandler) Handle(evt Event, err error) {
	hdl.Lock()
	hdl.errs[hdl.key(evt)] = err
	hdl.Unlock()
}

func (hdl *storeErrorsHandler) Error(evt Event) error {
	hdl.Lock()
	defer hdl.Unlock()
	if err, hasError := hdl.errs[hdl.key(evt)]; hasError {
		return err
	}
	return nil
}

func (hdl *storeErrorsHandler) key(evt Event) string {
	if evt == nil {
		return "nil"
	} else {
		return string(evt.ID())
	}
}

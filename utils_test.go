package event

import (
	"sync"
	"sync/atomic"
	"time"
)

//------test events------//

type testEvent1 struct{}

func (*testEvent1) Topic() string { return "test:topic" }

//------

type testEvent2 struct{}

func (*testEvent2) Topic() string { return "test:topic-2" }

//------

type testEvent3 string

func (testEvent3) Topic() string { return "test:topic-2" }

//------

type testEventDynamicTopic struct {
	Tpc string
}

func (evt *testEventDynamicTopic) Topic() string {
	return evt.Tpc
}

//------

type eventOrder interface {
	Position() int32
}

//------

type testEventOrder struct {
	position int32
}

func (evt *testEventOrder) Topic() string   { return "test:Event-order" }
func (evt *testEventOrder) Position() int32 { return evt.position }

//------

type testEventConcurrent struct {
	position int32
}

func (evt *testEventConcurrent) Position() int32 { return evt.position }

//------

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

//------

type benchmarkOrderedEvent struct {
}

func (evt *benchmarkOrderedEvent) Topic() string { return "benchmark:ordered" }

//------

type benchmarkConcurrentEvent struct {
}

//------test handlers------//

type testHandler1 struct {
	wg *sync.WaitGroup
}

func (hdl *testHandler1) Handle(evt Event) {
	if _, listens := evt.(*testEvent1); listens {
		hdl.wg.Done()
	}
}
func (*testHandler1) ListensTo(evt Event) bool {
	_, listens := evt.(*testEvent1)
	return listens
}

//------

type testHandler2 struct {
	wg *sync.WaitGroup
}

func (hdl *testHandler2) Handle(evt Event) {
	switch evt.(type) {
	case *testEvent2, testEvent3, *testEventDynamicTopic:
		hdl.wg.Done()
	}
}
func (*testHandler2) ListensTo(evt Event) bool {
	switch evt.(type) {
	case *testEvent2, testEvent3, *testEventDynamicTopic:
		return true
	}
	return false
}

//------

type emptyHandler struct {
}

func (hdl *emptyHandler) Handle(evt Event) {
	// handles everything
}
func (*emptyHandler) ListensTo(evt Event) bool {
	// listens to everything
	return true
}

//------

type testHandlerOrder struct {
	wg       *sync.WaitGroup
	position uint32
}

func (hdl *testHandlerOrder) Handle(evt Event) {
	if evt, listens := evt.(*testHandlerOrderEvent); listens {
		evt.HandlerPosition(hdl.position)
		hdl.wg.Done()
	}
}
func (*testHandlerOrder) ListensTo(evt Event) bool {
	_, listens := evt.(*testHandlerOrderEvent)
	return listens
}

//------

type testEventOrderHandler struct {
	wg        *sync.WaitGroup
	position  *int32
	unordered *int32
}

func (hdl *testEventOrderHandler) Handle(evt Event) {
	if evt, listens := evt.(eventOrder); listens {
		// delay the handling of the Event to simulate some sort of behavior.
		time.Sleep(time.Nanosecond * 200)

		if evt.Position() != hdl.currentPosition() {
			atomic.StoreInt32(hdl.unordered, 1)
		}
		hdl.incrementPosition()
		hdl.wg.Done()
	}
}
func (*testEventOrderHandler) ListensTo(evt Event) bool {
	_, listens := evt.(eventOrder)
	return listens
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

//------

type benchmarkOrderedEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkOrderedEventHandler) Handle(evt Event) {
	if _, listens := evt.(*benchmarkOrderedEvent); listens {
		time.Sleep(time.Nanosecond * 200)
		hdl.wg.Done()
	}
}
func (*benchmarkOrderedEventHandler) ListensTo(evt Event) bool {
	_, listens := evt.(*benchmarkOrderedEvent)
	return listens
}

//------

type benchmarkConcurrentEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkConcurrentEventHandler) Handle(evt Event) {
	if _, listens := evt.(*benchmarkConcurrentEvent); listens {
		time.Sleep(time.Nanosecond * 200)
		hdl.wg.Done()
	}
}
func (*benchmarkConcurrentEventHandler) ListensTo(evt Event) bool {
	_, listens := evt.(*benchmarkConcurrentEvent)
	return listens
}

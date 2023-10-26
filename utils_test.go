package event

import (
	"errors"
	"math/big"
	"sync"
)

// ------Enums------//
const (
	Unidentified Identifier = iota
	TestEvent1
	TestEvent2
	TestLiteralEvent
	TestErrorEvent
	TestErrorEvent2
	TestDynamicTopicEvent
	TestOrderEvent
	TestHandlerOrderEvent
	TestConcurrentEvent
	BenchmarkOrderedEvent
	BenchmarkConcurrentEvent
)

const (
	TestTopic Identifier = iota
	TestTopic2
	TestOrderTopic
	BenchmarkOrderTopic
)

//------Events------//

type testEvent1 struct{}

func (*testEvent1) Topic() Identifier { return TestTopic }
func (*testEvent1) Identifier() Identifier {
	return TestEvent1
}

type testEvent2 struct{}

func (*testEvent2) Topic() Identifier { return TestTopic2 }
func (*testEvent2) Identifier() Identifier {
	return TestEvent2
}

type testEvent3 string

func (testEvent3) Topic() Identifier { return TestTopic2 }
func (testEvent3) Identifier() Identifier {
	return TestLiteralEvent
}

type testEventError struct{}

func (*testEventError) Identifier() Identifier {
	return TestErrorEvent
}

type testEventError2 struct{}

func (*testEventError2) Topic() Identifier { return TestTopic }
func (*testEventError2) Identifier() Identifier {
	return TestErrorEvent2
}

type testEventDynamicTopic struct {
	Tpc Identifier
}

func (evt *testEventDynamicTopic) Topic() Identifier {
	return evt.Tpc
}
func (*testEventDynamicTopic) Identifier() Identifier {
	return TestDynamicTopicEvent
}

type eventOrder interface {
	Position() uint32
}

type testEventOrder struct {
	position uint32
}

func (evt *testEventOrder) Topic() Identifier { return TestOrderTopic }
func (evt *testEventOrder) Position() uint32  { return evt.position }
func (*testEventOrder) Identifier() Identifier {
	return TestOrderEvent
}

type testEventConcurrent struct {
	position uint32
}

func (evt *testEventConcurrent) Position() uint32 { return evt.position }
func (*testEventConcurrent) Identifier() Identifier {
	return TestConcurrentEvent
}

type testHandlerOrderEvent struct {
	position  *counter
	unordered *flag
}

func (evt *testHandlerOrderEvent) HandlerPosition(position uint32) {
	if !evt.position.is(position) {
		evt.unordered.enable()
		return
	}
	evt.position.increment()

}
func (*testHandlerOrderEvent) Identifier() Identifier {
	return TestHandlerOrderEvent
}

type benchmarkOrderedEvent struct {
}

func (evt *benchmarkOrderedEvent) Topic() Identifier { return BenchmarkOrderTopic }
func (*benchmarkOrderedEvent) Identifier() Identifier {
	return BenchmarkOrderedEvent
}

type benchmarkConcurrentEvent struct {
}

func (*benchmarkConcurrentEvent) Identifier() Identifier {
	return BenchmarkConcurrentEvent
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
	position  *counter
	unordered *flag
}

func (hdl *testEventOrderHandler) Handle(evt Event) error {
	if evt, listens := evt.(eventOrder); listens {
		// delay the handling of the Event to simulate some sort of behavior.
		fibonacci(1000)

		if !hdl.position.is(evt.Position()) {
			hdl.unordered.enable()
		}
		hdl.position.increment()
		hdl.wg.Done()
	}
	return nil
}

type benchmarkOrderedEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkOrderedEventHandler) Handle(evt Event) error {
	if _, listens := evt.(*benchmarkOrderedEvent); listens {
		fibonacci(1000)
		hdl.wg.Done()
	}
	return nil
}

type benchmarkConcurrentEventHandler struct {
	wg *sync.WaitGroup
}

func (hdl *benchmarkConcurrentEventHandler) Handle(evt Event) error {
	if _, listens := evt.(*benchmarkConcurrentEvent); listens {
		fibonacci(1000)
		hdl.wg.Done()
	}
	return nil
}

//------Error Handlers------//

type storeErrorsHandler struct {
	sync.Mutex
	wg   *sync.WaitGroup
	errs map[Identifier]error
}

func (hdl *storeErrorsHandler) Handle(evt Event, err error) {
	hdl.Lock()
	hdl.errs[hdl.key(evt)] = err
	if hdl.wg != nil {
		hdl.wg.Done()
	}
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

func (hdl *storeErrorsHandler) key(evt Event) Identifier {
	if evt == nil {
		return Unidentified
	} else {
		return evt.Identifier()
	}
}

//------General------//

func fibonacci(n uint) *big.Int {
	if n < 2 {
		return big.NewInt(int64(n))
	}
	a, b := big.NewInt(0), big.NewInt(1)
	for n--; n > 0; n-- {
		a.Add(a, b)
		a, b = b, a
	}

	return b
}

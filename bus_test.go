package event

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestBus_Initialize(t *testing.T) {
	bus := NewBus()
	hdl := &testHandler1{}
	hdl2 := &testHandler2{}

	bus.Initialize(hdl, hdl2)
	if len(bus.handlers) != 2 {
		t.Error("Unexpected number of handlers.")
	}
}

func TestBus_TopicsCapacity(t *testing.T) {
	bus := NewBus()
	bus.TopicsCapacity(100)
	bus.Initialize()
	if cap(bus.topics) != 100 {
		t.Error("Unexpected topic slice capacity.")
	}
}

func TestBus_TopicBuffer(t *testing.T) {
	bus := NewBus()
	bus.TopicBuffer(1000)
	bus.Initialize()
	if cap(bus.concurrentQueuedEvents) != 1000 {
		t.Error("Unexpected topic queue capacity.")
	}
}

func TestBus_Emit(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	hdl := &testHandler1{wg: wg}
	hdl2 := &testHandler2{wg: wg}
	hdlWErr := &testHandlerWithError{wg: wg}
	errHdl := &storeErrorsHandler{
		errs: make(map[string]error),
	}
	bus.ErrorHandlers(errHdl)

	bus.Emit(nil)
	if err := errHdl.Error(nil); err == nil {
		t.Error("A nil event is expected to throw an error.")
	}

	evt := &testEvent1{}
	bus.Emit(evt)
	if err := errHdl.Error(evt); err == nil {
		t.Error("This event is expected to throw an error since the bus is not initialized yet.")
	}

	wg.Add(5)
	bus.ErrorHandlers(errHdl)
	bus.Initialize(hdl, hdl2, hdlWErr)
	evtErr := &testEventError{}
	bus.Emit(evtErr)
	evtErr2 := &testEventError2{}
	bus.Emit(evtErr2)
	bus.Emit(&testEvent1{})
	bus.Emit(&testEvent2{})
	bus.Emit(testEvent3("test"))

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()

	if err := errHdl.Error(evtErr); err == nil {
		t.Error("Event was expected to throw an error.")
	}
	if err := errHdl.Error(evtErr2); err == nil {
		t.Error("Event was expected to throw an error.")
	}
}

func TestBus_Shutdown(t *testing.T) {
	bus := NewBus()
	hdl := &emptyHandler{}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	bus.Initialize(hdl)
	bus.Emit(&testEvent1{})
	time.AfterFunc(time.Nanosecond, func() {
		// graceful shutdown
		bus.Shutdown()
		wg.Done()
	})

	for i := 0; i < 1000; i++ {
		bus.Emit(&testEvent1{})
	}
	wg.Wait()

	if !bus.isShuttingDown() {
		t.Error("The bus should be shutting down.")
	}
}

func TestBus_ManyTopics(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	hdl := &testHandler2{wg: wg}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		bus.Emit(&testEventDynamicTopic{
			Tpc: fmt.Sprintf("test:dynamic-topic-%d-%d", i, rand.Intn(1000000)),
		})
	}
	wg.Wait()
}

func TestBus_HandlerOrder(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}

	hdls := make([]Handler, 0, 1000)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		hdls = append(hdls, &testHandlerOrder{wg: wg, position: uint32(i)})
	}
	bus.Initialize(hdls...)

	evt := &testHandlerOrderEvent{position: new(uint32), unordered: new(uint32)}
	bus.Emit(evt)

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if evt.IsUnordered() {
		t.Error("The Handler order MUST be respected.")
	}
}

func TestBus_OrderedEvents(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	hdl := &testEventOrderHandler{
		wg:        wg,
		position:  new(int32),
		unordered: new(int32),
	}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		bus.Emit(&testEventOrder{position: int32(i)})
	}

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if hdl.IsUnordered() {
		t.Error("The Event order MUST be respected within the same topic.")
	}
}

func TestBus_ConcurrentEvents(t *testing.T) {
	bus := NewBus()
	bus.ConcurrentWorkerPoolSize(4)
	wg := &sync.WaitGroup{}
	hdl := &testEventOrderHandler{
		wg:        wg,
		position:  new(int32),
		unordered: new(int32),
	}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		bus.Emit(&testEventConcurrent{position: int32(i)})
	}

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if !hdl.IsUnordered() {
		t.Log("WARNING: The events were handled in an ordered manner. " +
			"This is very unlikely unless the bus/environment is limited to a single routine. " +
			"If that is not the case, then something is likely broken.")
	}
}

func BenchmarkBus_Handling1MillionOrderedEvents(b *testing.B) {
	bus := NewBus()
	wg := &sync.WaitGroup{}

	bus.Initialize(&benchmarkOrderedEventHandler{wg: wg})
	for n := 0; n < b.N; n++ {
		wg.Add(1000000)
		for i := 0; i < 1000000; i++ {
			bus.Emit(&benchmarkOrderedEvent{})
		}
		wg.Wait()
	}
}

func BenchmarkBus_Handling1MillionConcurrentEvents(b *testing.B) {
	bus := NewBus()
	wg := &sync.WaitGroup{}

	bus.Initialize(&benchmarkConcurrentEventHandler{wg: wg})
	for n := 0; n < b.N; n++ {
		wg.Add(1000000)
		for i := 0; i < 1000000; i++ {
			bus.Emit(&benchmarkConcurrentEvent{})
		}
		wg.Wait()
	}
}

type ExampleEvent struct{}

func (*ExampleEvent) Topic() string { return "example-topic" }
func (*ExampleEvent) ID() []byte {
	return []byte("UUID")
}

type ExampleHandler struct {
}

func (hdl *ExampleHandler) Handle(evt Event) error {
	if _, listens := evt.(*ExampleEvent); listens {
		//code specific to this Event handler
	}
	return nil
}
func (*ExampleHandler) ListensTo(evt Event) bool {
	_, listens := evt.(*ExampleEvent)
	return listens
}

func ExampleBus_Emit() {
	// Create the bus
	bus := NewBus()

	// Add handlers and initialize the bus
	bus.Initialize(&ExampleHandler{})

	// Emit events
	bus.Emit(&ExampleEvent{})
}

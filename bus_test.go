package event

import (
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
	bus.SetTopicsCapacity(100)
	bus.Initialize()
	if cap(bus.topics) != 100 {
		t.Error("Unexpected topic slice capacity.")
	}
}

func TestBus_TopicBuffer(t *testing.T) {
	bus := NewBus()
	bus.SetTopicBuffer(1000)
	bus.Initialize()
	if cap(bus.concurrentQueuedEvents) != 1000 {
		t.Error("Unexpected topic queue capacity.")
	}
}

func TestBus_Emit(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	wg2 := &sync.WaitGroup{}
	hdl := &testHandler1{wg: wg}
	hdl2 := &testHandler2{wg: wg}
	hdlWErr := &testHandlerWithError{wg: wg}
	errHdl := &storeErrorsHandler{
		errs: make(map[Identifier]error),
		wg:   wg2,
	}
	bus.SetErrorHandlers(errHdl)

	wg2.Add(1)
	bus.Emit(nil)
	err := errHdl.Error(nil)
	if err == nil || err != InvalidEventError {
		t.Error("Expected InvalidEventError error.")
	} else if err.Error() != "event: invalid event" {
		t.Error("Unexpected InvalidEventError message.")
	}

	wg2.Add(1)
	evt := &testEvent1{}
	bus.Emit(evt)
	err = errHdl.Error(evt)
	if err == nil || err != BusNotInitializedError {
		t.Error("Expected BusNotInitializedError error.")
	} else if err.Error() != "event: the bus is not initialized" {
		t.Error("Unexpected BusNotInitializedError message.")
	}

	wg.Add(5)
	wg2.Add(2)
	bus.SetErrorHandlers(errHdl)
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
	wg2.Wait()
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
	errHdl := &storeErrorsHandler{
		errs: make(map[Identifier]error),
	}
	bus.SetErrorHandlers(errHdl)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	bus.SetConcurrentWorkerPoolSize(1337)
	bus.Initialize(hdl)
	bus.Emit(&testEvent1{})
	time.AfterFunc(time.Microsecond, func() {
		// graceful shutdown
		bus.Shutdown()
		wg.Done()
	})
	for i := 0; i < 1000; i++ {
		bus.Emit(&testEvent1{})
	}
	time.Sleep(time.Microsecond)
	if !bus.shuttingDown.enabled() {
		t.Error("The bus should be shutting down.")
	}
	err := errHdl.Error(&testEvent1{})
	if err == nil || err != BusIsShuttingDownError {
		t.Error("Expected BusIsShuttingDownError error.")
	} else if err.Error() != "event: the bus is shutting down" {
		t.Error("Unexpected BusIsShuttingDownError message.")
	}
	wg.Wait()
}

func TestBus_ManyTopics(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	hdl := &testHandler2{wg: wg}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := int64(0); i < 1000; i++ {
		bus.Emit(&testEventDynamicTopic{
			Tpc: Identifier(i),
		})
	}
	wg.Wait()
}

func TestBus_HandlerOrder(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}

	hdls := make([]Handler, 0, 1000)
	wg.Add(1000)
	for i := uint32(0); i < 1000; i++ {
		hdls = append(hdls, &testHandlerOrder{wg: wg, position: i})
	}
	bus.Initialize(hdls...)

	evt := &testHandlerOrderEvent{
		position:  newCounter(),
		unordered: newFlag(),
	}
	bus.Emit(evt)

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if evt.unordered.enabled() {
		t.Error("The Handler order MUST be respected.")
	}
}

func TestBus_OrderedEvents(t *testing.T) {
	bus := NewBus()
	wg := &sync.WaitGroup{}
	hdl := &testEventOrderHandler{
		wg:        wg,
		position:  newCounter(),
		unordered: newFlag(),
	}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := uint32(0); i < 1000; i++ {
		bus.Emit(&testEventOrder{position: i})
	}

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if hdl.unordered.enabled() {
		t.Error("The Event order MUST be respected within the same topic.")
	}
}

func TestBus_ConcurrentEvents(t *testing.T) {
	bus := NewBus()
	bus.SetConcurrentWorkerPoolSize(4)
	wg := &sync.WaitGroup{}
	hdl := &testEventOrderHandler{
		wg:        wg,
		position:  newCounter(),
		unordered: newFlag(),
	}

	bus.Initialize(hdl)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		bus.Emit(&testEventConcurrent{position: uint32(i)})
	}

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	timeout.Stop()
	if !hdl.unordered.enabled() {
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

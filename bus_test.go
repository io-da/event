package event

import (
	"context"
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

func TestBus_TopicBuffer(t *testing.T) {
	bus := NewBus()
	bus.SetTopicBuffer(1000)
	bus.Initialize()
	if cap(bus.concurrentQueue) != 1000 {
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
	mustErrorT(bus.Emit(context.TODO(), nil))(t, InvalidEventError)
	wg2.Add(1)
	mustErrorT(bus.Emit(context.TODO(), &testEvent1{}))(t, BusNotInitializedError)

	wg.Add(5)
	wg2.Add(2)
	bus.SetErrorHandlers(errHdl)
	bus.AddTopics(TestTopic, TestTopic2)
	evtErr := &testEventError{}
	evtErr2 := &testEventError2{}
	bus.Initialize(hdl, hdl2, hdlWErr)
	mustT(bus.Emit(context.TODO(), evtErr))(t)
	mustT(bus.Emit(context.TODO(), evtErr2))(t)
	mustT(bus.Emit(context.TODO(), &testEvent1{}))(t)
	mustT(bus.Emit(context.TODO(), &testEvent2{}))(t)
	mustT(bus.Emit(context.TODO(), testEvent3("test")))(t)

	timeout := time.AfterFunc(time.Second*10, func() {
		t.Fatal("The events should have been handled by now.")
	})

	wg.Wait()
	// wg2.Wait()
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
	mustT(bus.Emit(context.TODO(), &testEvent1{}))(t)
	time.AfterFunc(time.Microsecond, func() {
		// graceful shutdown
		bus.Shutdown()
		wg.Done()
	})
	for i := 0; i < 1000; i++ {
		mustT(bus.Emit(context.TODO(), &testEvent1{}))(t)
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
		mustT(bus.Emit(context.TODO(), &testEventDynamicTopic{
			Tpc: Identifier(i),
		}))(t)
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
	mustT(bus.Emit(context.TODO(), evt))(t)

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
		mustT(bus.Emit(context.TODO(), &testEventOrder{position: i}))(t)
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
		mustT(bus.Emit(context.TODO(), &testEventConcurrent{position: uint32(i)}))(t)
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
			_ = bus.Emit(context.TODO(), &benchmarkOrderedEvent{})
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
			_ = bus.Emit(context.TODO(), &benchmarkConcurrentEvent{})
		}
		wg.Wait()
	}
}

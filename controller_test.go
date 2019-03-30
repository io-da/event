package event

import (
	"fmt"
	"sync"
	"testing"
)

func TestController_Handle(t *testing.T) {
	wg := &sync.WaitGroup{}
	ctrl := newController(0, 10, 100, []Handler{&testHandler1{wg: wg}})
	evt := &testEvent1{}
	wg.Add(1)
	ctrl.handle(evt)
	ctrl.handle(nil)

	if len(ctrl.topics) != 1 {
		t.Error("Failed to addTopic topic to controller. Expected 1 item.")
	}

	if ctrl.topics[0].name != evt.Topic() {
		t.Error("topic name should be " + evt.Topic())
	}
}

func TestController_Len(t *testing.T) {
	wg := &sync.WaitGroup{}
	ctrl := newController(0, 10, 100, []Handler{&testHandler1{wg: wg}})
	if ctrl.len() != 0 {
		t.Error("Unexpected controller length.")
	}
	wg.Add(1)
	ctrl.handle(&testEvent1{})
	if ctrl.len() != 1 {
		t.Error("Unexpected controller length.")
	}
}

func TestController_Cap(t *testing.T) {
	wg := &sync.WaitGroup{}
	hdl := &testHandler1{wg: wg}
	topicsCapacity := 10
	ctrl := newController(0, topicsCapacity, 100, []Handler{hdl})
	if ctrl.cap() != topicsCapacity {
		t.Error("Unexpected controller capacity.")
	}

	wg.Add(1)
	ctrl.handle(&testEvent1{})
	if ctrl.cap() != topicsCapacity {
		t.Error("Unexpected controller capacity.")
	}

	for i := 0; i < topicsCapacity; i++ {
		wg.Add(1)
		ctrl.handle(&testEventDynamicTopic{
			Tpc: fmt.Sprintf("test:dynamic-topic-%d", i),
		})
	}
	if len(ctrl.topics) != topicsCapacity+1 {
		t.Error("Unexpected controller length.", len(ctrl.topics))
	}

	if cap(ctrl.topics) != topicsCapacity*2 {
		t.Error("Unexpected controller capacity.")
	}
}

func TestController_Empty(t *testing.T) {
	ctrl := newController(0, 10, 100, []Handler{})
	if !ctrl.empty() {
		t.Error("Should be empty.")
	}
}

# [Go](https://golang.org/) Event Bus
An event bus developed with a focus on performance and robustness.

[![Build Status](https://travis-ci.org/io-da/event.svg?branch=master)](https://travis-ci.org/io-da/event)
[![Maintainability](https://api.codeclimate.com/v1/badges/f256105248459e250292/maintainability)](https://codeclimate.com/github/io-da/event/maintainability) 
[![Test Coverage](https://api.codeclimate.com/v1/badges/f256105248459e250292/test_coverage)](https://codeclimate.com/github/io-da/event/test_coverage) 
[![GoDoc](https://godoc.org/github.com/io-da/event?status.svg)](https://godoc.org/github.com/io-da/event)  

## Installation
``` go get github.com/io-da/event ```

## Overview

1. [Overview](#Overview)
1. [Events](#Events)
2. [Handlers](#Handlers)
3. [The Bus](#The-Bus)  
   1. [Tweaking Performance](#Tweaking-Performance)  
   1. [Shutting Down](#Shutting-Down)  
4. [Examples](#Examples)

### Diagram
![Diagram](event-bus-diagram.png?raw=true "Diagram")

## Usage

### Events
Events can be of any type. Ideally they should contain immutable data.
```go
type ExampleEvent struct{
    someValue string
}
```
An event may optionally implement the _Topic_ interface. If it does, then it will be handled within that topic.   
```go
type Topic interface {
    Topic() string
}
```
Any event _emitted_ within the same topic is **guaranteed to be _handled_ respecting their order of emission.**  
However, this order is **not guaranteed across different topics**.  
A topic is just a string (the name), the event bus will take care of the rest.
```go
func (*ExampleEvent) Topic() string { 
    return "example-topic" 
}
```
Events that **do not** implement the _Topic_ interface will be considered concurrent.  
Concurrent events will take advantage of [goroutines](https://gobyexample.com/goroutines) to be handled faster, but their **emission order will not be respected**.

### Handlers
Handlers are any type that implements the _Handler_ interface. Handlers must be instantiated and provided to the bus on initialization.    
```go
type Handler interface {
    ListensTo(evt Event) bool
    Handle(evt Event)
}
```

```go
// An event handler
type ExampleHandler struct {
}

// Handle performs the actual handler specific logic
func (hdl *ExampleHandler) Handle(evt Event) {
	// type assert to the expected event type
    if evt, listens := evt.(*ExampleEvent); listens {
        // handler logic
    }
}

// ListensTo verifies if the event is expected by this handler before any logic
func (*ExampleHandler) ListensTo(evt Event) bool {
	// type assert to the expected event type
    _, listens := evt.(*ExampleEvent)
    return listens
}
```

### The Bus
_Bus_ is the _struct_ that will be used to emit all the application's events.  
The _Bus_ should be instantiated and initialized on application startup. The initialization is separated from the instantiation for dependency injection purposes.  
The application should instantiate the _Bus_ once and then use it's reference in all the places that will be emitting events.  
**The order the handlers are provided to the _Bus_ is always respected.**
```go
import (
    "github.com/io-da/event"
)

func main() {
    // instantiate the bus (returns *event.Bus)
    bus := event.NewBus()
    
    // optional
    bus.ConcurrentPoolSize(4)
    
    // initialize the bus with all of the application's event handlers
    bus.Initialize(
    	// this handler will always be executed first
    	&ExampleHandler{},
    	// this one second
    	&ExampleHandler2{},
    	// and this one third
    	&ExampleHandler3{},
    )
    
    // emit events
    bus.Emit(&ExampleEvent{})
}
```

#### Tweaking Performance
For applications that take advantage of concurrent events, the number of concurrent workers can be adjusted.
```go
bus.ConcurrentPoolSize(10)
```
If used this function **must** be called **before** the _Bus_ is initialized. And it specifies the number of [goroutines](https://gobyexample.com/goroutines) used to handle concurrent events.  
In some scenarios increasing the value can drastically improve performance.  
It defaults to the value returned by ```runtime.GOMAXPROCS(0)```.  
  
When aware of the total amount of different topics available in the application. Then that value should be provided with this function.
```go
bus.TopicsCapacity(10)
```
If used this function **must** be called **before** the _Bus_ is initialized.  
It defaults to 10.  
  
The buffer size of topics can also be adjusted.  
Depending on the use case this value may greatly impact performance.
```go
bus.TopicBuffer(100)
```
If used this function **must** be called **before** the _Bus_ is initialized.  
It defaults to 100.  

#### Shutting Down
The _Bus_ also provides a shutdown function that attempts to gracefully stop the event bus and all its routines.
```go
bus.Shutdown()
```  
**This function will block until the bus is fully stopped.**

### Examples
An event handler that listens to multiple event types.
```go
type ExampleHandler2 struct {
}

func (hdl *ExampleHandler2) Handle(evt Event) {
	// an other way to assert the type of the event. More convenient for handlers that expect different event types.
    switch evt := evt.(type) {
    case *ExampleEvent, *ExampleEvent2:
        // handler logic
    }
}

func (*ExampleHandler2) ListensTo(evt Event) bool {
	// an other way to assert the type of the event. More convenient for handlers that expect different event types.
    switch evt := evt.(type) {
    case *ExampleEvent, *ExampleEvent2:
        return true
    }
    return false
}
```

An event handler that logs every event emitted.
```go
type ExampleEventLogger struct {
}

func (hdl *ExampleEventLogger) Handle(evt Event) {
    log.Printf("event %T emitted", evt)
}

func (*ExampleEventLogger) ListensTo(evt Event) bool {
    // listen to all event types
    return true
}
```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License
[MIT](https://choosealicense.com/licenses/mit/)
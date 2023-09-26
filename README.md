# [Go](https://golang.org/) Event Bus
An event bus to react to all the things.

[![Maintainability](https://api.codeclimate.com/v1/badges/f256105248459e250292/maintainability)](https://codeclimate.com/github/io-da/event/maintainability) 
[![Test Coverage](https://api.codeclimate.com/v1/badges/f256105248459e250292/test_coverage)](https://codeclimate.com/github/io-da/event/test_coverage) 
[![GoDoc](https://godoc.org/github.com/io-da/event?status.svg)](https://godoc.org/github.com/io-da/event)  

## Installation
``` go get github.com/io-da/event ```

## Overview
1. [Events](#Events)
2. [Handlers](#Handlers)
3. [Error Handlers](#Error-Handlers)
4. [The Bus](#The-Bus)  
   1. [Tweaking Performance](#Tweaking-Performance)  
   2. [Shutting Down](#Shutting-Down)  
   3. [Available Errors](#Available-Errors)  
5. [Benchmarks](#Benchmarks)
6. [Examples](#Examples)

## Introduction
This library is intended for anyone looking to emit events in their application. And it achieves this objective using an event bus architecture.  
The _Bus_ will use _workers_ ([goroutines](https://gobyexample.com/goroutines)) to attempt handling the events in **non-blocking** manner.  
Clean and simple codebase. **No reflection, no closures.**

![Flowchart](flowchart.png?raw=true "Flowchart")

## Getting Started

### Events
Events are any type that implements the _Event_ interface. Ideally they should contain immutable data.  
```go
type Event interface {
    ID() []byte
}
```

An event may optionally implement the _Topic_ interface. If it does, then it will be handled within that topic.   
```go
type Topic interface {
    Event
    Topic() string
}
```
Any event _emitted_ within the same topic is **guaranteed to be _handled_ respecting their order of emission.**  
However, this order is **not guaranteed across different topics**.  
A topic is just a string (the name), the event bus will take care of the rest.  
Events that **do not** implement the _Topic_ interface will be considered concurrent.  
The _Bus_ takes advantage of **additional** _workers_ ([goroutines](https://gobyexample.com/goroutines)) to handle concurrent events faster, but their **emission order will not be respected**.

### Handlers
Handlers are any type that implements the _Handler_ interface. Handlers must be instantiated and provided to the bus on initialization.    
```go
type Handler interface {
    Handle(evt Event) error
}
```

### Error Handlers
Error handlers are any type that implements the _ErrorHandler_ interface. Error handlers are optional (but advised) and provided to the bus using the ```bus.ErrorHandlers``` function.  
```go
type ErrorHandler interface {
    Handle(evt Event, err error)
}
```
Any time an error occurs within the bus, it will be passed on to the error handlers. This strategy can be used for decoupled error handling.

### The Bus
_Bus_ is the _struct_ that will be used to emit all the application's events.  
The _Bus_ should be instantiated and initialized on application startup. The initialization is separated from the instantiation for dependency injection purposes.  
The application should instantiate the _Bus_ once and then use it's reference in all the places that will be emitting events.  
**The order in which the handlers are provided to the _Bus_ is always respected.**

#### Tweaking Performance
For applications that take advantage of concurrent events, the number of concurrent workers can be adjusted.
```go
bus.ConcurrentWorkerPoolSize(10)
```
If used, this function **must** be called **before** the _Bus_ is initialized. And it specifies the number of [goroutines](https://gobyexample.com/goroutines) used to handle concurrent events.  
In some scenarios increasing the value can drastically improve performance.  
It defaults to the value returned by ```runtime.GOMAXPROCS(0)```.  
  
When aware of the total amount of different topics available in the application. Then that value should be provided with this function.
```go
bus.TopicsCapacity(10)
```
If used, this function **must** be called **before** the _Bus_ is initialized.  
It defaults to 10.  
  
The buffer size of topics can also be adjusted.  
Depending on the use case, this value may greatly impact performance.
```go
bus.TopicBuffer(100)
```
If used, this function **must** be called **before** the _Bus_ is initialized.  
It defaults to 100.  

#### Shutting Down
The _Bus_ also provides a shutdown function that attempts to gracefully stop the event bus and all its routines.
```go
bus.Shutdown()
```  
**This function will block until the bus is fully stopped.**

#### Available Errors 
Below is a list of errors that can occur when calling bus.Emit.  

```go
// event.ErrorInvalidEvent  
// event.ErrorEventBusNotInitialized
// event.ErrorEventBusIsShuttingDown

if err := bus.Emit(&Event{}); err != nil {
    switch(err.(type)) {
        case event.ErrorInvalidEvent:
            // do something
        case event.ErrorEventBusNotInitialized:
            // do something
        case event.ErrorEventBusIsShuttingDown:
            // do something
        default:
            // do something
    }
}
```

## Benchmarks
All the benchmarks are performed against batches of 1 million events.  
All the ordered events belong to the same topic.  
All the benchmarks contain some overhead due to the usage of _sync.WaitGroup_.

#### Benchmarks without handler behavior
The event handlers are empty to test solely the bus overhead.  

| Benchmark Type | Time |
| :--- | :---: |
| Ordered Events | 116 ns/op |
| Concurrent Events | 114 ns/op |

#### Benchmarks with simulated handler behavior
The event handlers use ```time.Sleep(time.Nanosecond * 200)``` for simulation purposes.  

| Benchmark Type | Time |
| :--- | :---: |
| Ordered Events | 796 ns/op |
| Concurrent Events | 501 ns/op |

## Examples

#### Example Events
A simple ```struct``` event.
```go
type Foo struct {
    bar string
}
func (*Foo) ID() []byte {
    return []byte("FOO-UUID")
}
```

A ```string``` event that implements the _Topic_ interface.
```go
type Bar string
func (Bar) Topic() string { return "bar-topic" }
func (Bar) ID() []byte {
    return []byte("BAR-UUID")
}
```

#### Example Handlers
An event handler that logs every event emitted.
```go
type LoggerHandler struct {
}

func (hdl *LoggerHandler) Handle(evt Event) error {
    log.Printf("event %T emitted", evt)
    return nil
}
```

An event handler that listens to multiple event types.
```go
type FooBarHandler struct {
}

func (hdl *FooBarHandler) Handle(evt Event) error {
    // a convenient way to assert multiple event types.
    switch evt := evt.(type) {
    case *Foo, Bar:
        // handler logic
    }
    return nil
}
```

#### Putting it together
Initialization and usage of the exemplified events and handlers
```go
import (
    "github.com/io-da/event"
)

func main() {
    // instantiate the bus (returns *event.Bus)
    bus := event.NewBus()
    
    // initialize the bus with all of the application's event handlers
    bus.Initialize(
    	// this handler will always be executed first
        &LoggerHandler{},
        // this one second
        &FooBarHandler{},
    )
    
    // emit events
    bus.Emit(&Foo{})
    bus.Emit(Bar("bar"))
}
```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License
[MIT](https://choosealicense.com/licenses/mit/)
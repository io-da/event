package event

import "sync/atomic"

type counter struct {
	atomic.Uint32
}

func newCounter() *counter {
	return &counter{
		Uint32: atomic.Uint32{},
	}
}

func (c *counter) increment() {
	c.Add(1)
}

func (c *counter) decrement() {
	c.Add(^uint32(0))
}

func (c *counter) is(v uint32) bool {
	return c.Load() == v
}

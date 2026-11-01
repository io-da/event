package event

// OverflowPolicy controls admission when a subscription queue is full.
type OverflowPolicy uint8

const (
	OverflowBlock OverflowPolicy = iota
	OverflowDropOldest
	OverflowDropNewest
)

type config struct {
	workers, buffer int
	overflow        OverflowPolicy
	middlewares     []Middleware
	errorHandlers   []ErrorHandler
	logger          LogHandler
}

// Option configures a bus at construction time.
type Option func(*config)

// WithWorkerPoolSize sets workers per subscriber for events without a Topic (default 1).
func WithWorkerPoolSize(n int) Option {
	return func(c *config) {
		if n < 1 {
			panic("event: worker count must be positive")
		}
		c.workers = n
	}
}

// WithQueueBuffer sets each queue's capacity (default 100, minimum 1).
func WithQueueBuffer(n int) Option {
	return func(c *config) {
		if n < 1 {
			panic("event: queue capacity must be positive")
		}
		c.buffer = n
	}
}

// WithOverflowPolicy sets the default overflow policy (default OverflowBlock).
func WithOverflowPolicy(p OverflowPolicy) Option {
	return func(c *config) {
		if p > OverflowDropNewest {
			panic("event: invalid overflow policy")
		}
		c.overflow = p
	}
}

// WithMiddlewares appends middleware in outermost-first order.
func WithMiddlewares(m ...Middleware) Option {
	copied := append([]Middleware(nil), m...)
	return func(c *config) {
		for _, mw := range copied {
			if mw == nil {
				panic("event: nil middleware")
			}
		}
		c.middlewares = append(c.middlewares, copied...)
	}
}

// WithErrorHandlers appends error handlers.
func WithErrorHandlers(h ...ErrorHandler) Option {
	copied := append([]ErrorHandler(nil), h...)
	return func(c *config) {
		for _, eh := range copied {
			if eh == nil {
				panic("event: nil error handler")
			}
		}
		c.errorHandlers = append(c.errorHandlers, copied...)
	}
}

// WithLogHandler installs optional diagnostic logging. The last option wins;
// passing nil disables logging. Existing error handlers still receive failures.
func WithLogHandler(handler LogHandler) Option {
	return func(c *config) { c.logger = handler }
}

// SubscriptionOption overrides queue settings for one handler registration.
type SubscriptionOption func(*config)

func WithSubscriptionWorkers(n int) SubscriptionOption {
	return SubscriptionOption(WithWorkerPoolSize(n))
}
func WithSubscriptionBuffer(n int) SubscriptionOption { return SubscriptionOption(WithQueueBuffer(n)) }
func WithSubscriptionOverflow(p OverflowPolicy) SubscriptionOption {
	return SubscriptionOption(WithOverflowPolicy(p))
}

type configuredHandler struct {
	Handler
	options []SubscriptionOption
}

// WithSubscription gives a handler independent queue settings. Nested calls
// preserve inner options; outer options take precedence.
func WithSubscription(h Handler, opts ...SubscriptionOption) Handler {
	var combined []SubscriptionOption
	if inner, ok := h.(configuredHandler); ok {
		h = inner.Handler
		combined = append(combined, inner.options...)
	}
	combined = append(combined, opts...)
	return configuredHandler{h, combined}
}

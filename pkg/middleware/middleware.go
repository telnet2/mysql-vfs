package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

// Middleware represents a Hertz middleware function
type Middleware func(ctx context.Context, c *app.RequestContext)

// Chain represents a chain of middleware functions
type Chain struct {
	middlewares []Middleware
}

// NewChain creates a new middleware chain
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Then adds additional middleware to the chain
func (ch *Chain) Then(m Middleware) *Chain {
	ch.middlewares = append(ch.middlewares, m)
	return ch
}

// Handler returns a Hertz handler that executes the middleware chain
func (ch *Chain) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		for _, middleware := range ch.middlewares {
			// Check if request was aborted by previous middleware
			if c.IsAborted() {
				return
			}
			middleware(ctx, c)
		}
	}
}

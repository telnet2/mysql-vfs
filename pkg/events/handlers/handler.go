package handlers

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

// Handler is the interface that all event handlers must implement
type Handler interface {
	// Handle processes an event
	// payload can be FileEventPayload, DirectoryEventPayload, or MoveEventPayload
	Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) error

	// Type returns the handler type (webhook, log, metrics)
	Type() events.HandlerType
}

// Registry holds all registered handler implementations
type Registry struct {
	handlers map[events.HandlerType]Handler
}

// NewRegistry creates a new handler registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[events.HandlerType]Handler),
	}
}

// Register registers a handler implementation
func (r *Registry) Register(handler Handler) {
	r.handlers[handler.Type()] = handler
}

// Get returns a handler by type
func (r *Registry) Get(handlerType events.HandlerType) (Handler, bool) {
	handler, exists := r.handlers[handlerType]
	return handler, exists
}

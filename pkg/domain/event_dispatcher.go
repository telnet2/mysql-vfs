package domain

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/events/handlers"
)

// EventDispatcher dispatches events to registered handlers
type EventDispatcher struct {
	eventsLoader    *EventsLoader
	handlerRegistry *handlers.Registry
	workerPool      chan struct{} // Semaphore for limiting concurrent handlers
	wg              sync.WaitGroup
}

// EventDispatcherConfig configures the event dispatcher
type EventDispatcherConfig struct {
	MaxConcurrentHandlers int // Maximum number of handlers running concurrently
}

// NewEventDispatcher creates a new event dispatcher
func NewEventDispatcher(eventsLoader *EventsLoader, handlerRegistry *handlers.Registry, config EventDispatcherConfig) *EventDispatcher {
	maxConcurrent := config.MaxConcurrentHandlers
	if maxConcurrent <= 0 {
		maxConcurrent = 10 // Default: 10 concurrent handlers
	}

	return &EventDispatcher{
		eventsLoader:    eventsLoader,
		handlerRegistry: handlerRegistry,
		workerPool:      make(chan struct{}, maxConcurrent),
	}
}

// DispatchFileEvent dispatches a file event to all matching handlers
func (d *EventDispatcher) DispatchFileEvent(ctx context.Context, dirID string, payload *events.FileEventPayload) {
	// Load handlers for this event type
	eventHandlers, err := d.eventsLoader.GetHandlersForEvent(ctx, dirID, payload.Event.Type)
	if err != nil {
		log.Printf("failed to load handlers for event %s: %v", payload.Event.Type, err)
		return
	}

	// Filter handlers based on file properties
	var matchingHandlers []events.EventHandler
	for _, handler := range eventHandlers {
		if d.eventsLoader.ShouldHandleEvent(
			&handler,
			payload.Resource.Name,
			payload.Resource.SizeBytes,
			payload.Resource.ContentType,
		) {
			matchingHandlers = append(matchingHandlers, handler)
		}
	}

	// Dispatch to matching handlers (async)
	for _, handler := range matchingHandlers {
		d.dispatchAsync(ctx, handler, payload)
	}
}

// DispatchDirectoryEvent dispatches a directory event to all matching handlers
func (d *EventDispatcher) DispatchDirectoryEvent(ctx context.Context, dirID string, payload *events.DirectoryEventPayload) {
	// Load handlers for this event type
	eventHandlers, err := d.eventsLoader.GetHandlersForEvent(ctx, dirID, payload.Event.Type)
	if err != nil {
		log.Printf("failed to load handlers for event %s: %v", payload.Event.Type, err)
		return
	}

	// Directory events don't have filters, so dispatch to all handlers
	for _, handler := range eventHandlers {
		d.dispatchAsync(ctx, handler, payload)
	}
}

// DispatchMoveEvent dispatches a file move event to all matching handlers
func (d *EventDispatcher) DispatchMoveEvent(ctx context.Context, dirID string, payload *events.MoveEventPayload) {
	// Load handlers for this event type
	eventHandlers, err := d.eventsLoader.GetHandlersForEvent(ctx, dirID, payload.Event.Type)
	if err != nil {
		log.Printf("failed to load handlers for event %s: %v", payload.Event.Type, err)
		return
	}

	// Filter handlers based on file properties
	var matchingHandlers []events.EventHandler
	for _, handler := range eventHandlers {
		if d.eventsLoader.ShouldHandleEvent(
			&handler,
			payload.Resource.Name,
			payload.Resource.SizeBytes,
			payload.Resource.ContentType,
		) {
			matchingHandlers = append(matchingHandlers, handler)
		}
	}

	// Dispatch to matching handlers (async)
	for _, handler := range matchingHandlers {
		d.dispatchAsync(ctx, handler, payload)
	}
}

// dispatchAsync dispatches an event to a handler asynchronously
func (d *EventDispatcher) dispatchAsync(ctx context.Context, handler events.EventHandler, payload interface{}) {
	// Acquire worker slot (blocks if pool is full)
	d.workerPool <- struct{}{}
	d.wg.Add(1)

	go func() {
		defer func() {
			<-d.workerPool // Release worker slot
			d.wg.Done()
		}()

		// Get handler implementation
		handlerImpl, exists := d.handlerRegistry.Get(handler.Type)
		if !exists {
			log.Printf("handler type %s not registered", handler.Type)
			return
		}

		// Execute handler
		response := handlerImpl.Handle(ctx, &handler, payload)
		if !response.Success {
			log.Printf("handler %s (type: %s) failed: %s", handler.Name, handler.Type, response.Message)
		}
	}()
}

// Wait waits for all pending handlers to complete
// This is useful for testing or graceful shutdown
func (d *EventDispatcher) Wait() {
	d.wg.Wait()
}

// Shutdown gracefully shuts down the dispatcher
func (d *EventDispatcher) Shutdown(ctx context.Context) error {
	done := make(chan struct{})

	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

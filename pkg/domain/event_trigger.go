package domain

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/events/handlers"
)

// EventTrigger is the interface for emitting lifecycle events
// Aliased from events package for domain layer convenience
type EventTrigger = events.EventTrigger

// LifecycleEventTrigger is the concrete implementation of EventTrigger
type LifecycleEventTrigger struct {
	eventsLoader    *EventsLoader
	handlerRegistry *handlers.Registry
	patternMatcher  events.PatternMatcher
	workerPool      chan struct{} // Semaphore for limiting concurrent handlers
	wg              sync.WaitGroup
}

// EventTriggerConfig configures the event trigger
type EventTriggerConfig struct {
	MaxConcurrentHandlers int // Maximum number of async handlers running concurrently
}

// NewLifecycleEventTrigger creates a new lifecycle event trigger
func NewLifecycleEventTrigger(
	eventsLoader *EventsLoader,
	handlerRegistry *handlers.Registry,
	config EventTriggerConfig,
) *LifecycleEventTrigger {
	maxConcurrent := config.MaxConcurrentHandlers
	if maxConcurrent <= 0 {
		maxConcurrent = 10 // Default: 10 concurrent handlers
	}

	return &LifecycleEventTrigger{
		eventsLoader:    eventsLoader,
		handlerRegistry: handlerRegistry,
		patternMatcher:  events.NewWildcardPatternMatcher(),
		workerPool:      make(chan struct{}, maxConcurrent),
	}
}

// Emit emits an event asynchronously (fire and forget)
func (t *LifecycleEventTrigger) Emit(ctx context.Context, eventType string, payload interface{}) {
	t.EmitWithOperation(ctx, nil, eventType, payload)
}

// EmitSync emits an event synchronously and waits for handler responses
func (t *LifecycleEventTrigger) EmitSync(ctx context.Context, eventType string, payload interface{}) error {
	return t.EmitSyncWithOperation(ctx, nil, eventType, payload)
}

// EmitWithOperation emits an event with operation context tracking
func (t *LifecycleEventTrigger) EmitWithOperation(ctx context.Context, opCtx *events.OperationContext, eventType string, payload interface{}) {
	// Get directory ID from payload
	dirID := t.extractDirectoryID(payload)
	if dirID == "" {
		log.Printf("failed to extract directory ID from payload for event %s", eventType)
		return
	}

	// Get matching handlers
	matchingHandlers := t.getMatchingHandlers(ctx, dirID, eventType, payload)

	// Dispatch to async handlers only
	for _, handler := range matchingHandlers {
		if !handler.IsSynchronous() {
			t.dispatchAsync(ctx, handler, payload)
		}
	}
}

// EmitSyncWithOperation emits an event synchronously with operation context
func (t *LifecycleEventTrigger) EmitSyncWithOperation(ctx context.Context, opCtx *events.OperationContext, eventType string, payload interface{}) error {
	// Get directory ID from payload
	dirID := t.extractDirectoryID(payload)
	if dirID == "" {
		return fmt.Errorf("failed to extract directory ID from payload for event %s", eventType)
	}

	// Get matching handlers
	matchingHandlers := t.getMatchingHandlers(ctx, dirID, eventType, payload)

	// Execute synchronous handlers first
	for _, handler := range matchingHandlers {
		if handler.IsSynchronous() {
			response := t.executeHandler(ctx, handler, payload)

			// Check for veto
			if response.Veto && handler.IsVetoEnabled() {
				return &events.VetoError{
					HandlerName: handler.Name,
					EventType:   eventType,
					Message:     response.Message,
					Code:        response.Code,
				}
			}

			// Log errors but don't abort
			if !response.Success && !response.Veto {
				log.Printf("synchronous handler %s failed (non-veto): %s", handler.Name, response.Message)
			}
		}
	}

	// Dispatch async handlers (fire and forget)
	for _, handler := range matchingHandlers {
		if !handler.IsSynchronous() {
			t.dispatchAsync(ctx, handler, payload)
		}
	}

	return nil
}

// getMatchingHandlers returns all handlers that match the event type
func (t *LifecycleEventTrigger) getMatchingHandlers(ctx context.Context, dirID string, eventType string, payload interface{}) []events.EventHandler {
	// Get all handlers for this directory (including inherited)
	allHandlers, err := t.eventsLoader.GetAllHandlers(ctx, dirID)
	if err != nil {
		log.Printf("failed to load handlers for directory %s: %v", dirID, err)
		return nil
	}

	var matchingHandlers []events.EventHandler

	for _, handler := range allHandlers {
		// Check if handler is enabled
		if !handler.IsEnabled() {
			continue
		}

		// Check if event type matches any of the handler's patterns
		matched := false
		for _, pattern := range handler.Events {
			if t.patternMatcher.Match(string(pattern), eventType) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		// Apply filters (for file events)
		if !t.shouldHandleEvent(&handler, payload) {
			continue
		}

		matchingHandlers = append(matchingHandlers, handler)
	}

	return matchingHandlers
}

// shouldHandleEvent checks if a handler should handle this specific event based on filters
func (t *LifecycleEventTrigger) shouldHandleEvent(handler *events.EventHandler, payload interface{}) bool {
	// If no filter, always handle
	if handler.Filter == nil {
		return true
	}

	// Try to extract file properties from payload
	fileName, fileSize, contentType := t.extractFileProperties(payload)
	if fileName == "" {
		// Not a file event or can't extract properties, skip filter
		return true
	}

	// Use EventsLoader's filter logic
	return t.eventsLoader.ShouldHandleEvent(handler, fileName, fileSize, contentType)
}

// extractFileProperties extracts file properties from payload for filtering
func (t *LifecycleEventTrigger) extractFileProperties(payload interface{}) (name string, size int64, contentType string) {
	// Try FileEventPayload
	if fep, ok := payload.(*events.FileEventPayload); ok {
		return fep.Resource.Name, fep.Resource.SizeBytes, fep.Resource.ContentType
	}

	// Try AuthorizationEventPayload (can be FileResource or DirectoryResource)
	if aep, ok := payload.(*events.AuthorizationEventPayload); ok {
		if fileRes, ok := aep.Resource.(events.FileResource); ok {
			return fileRes.Name, fileRes.SizeBytes, fileRes.ContentType
		}
		if dirRes, ok := aep.Resource.(events.DirectoryResource); ok {
			return dirRes.Name, 0, ""
		}
	}

	// Try ValidationEventPayload (can be FileResource or DirectoryResource)
	if vep, ok := payload.(*events.ValidationEventPayload); ok {
		if fileRes, ok := vep.Resource.(events.FileResource); ok {
			return fileRes.Name, fileRes.SizeBytes, fileRes.ContentType
		}
		if dirRes, ok := vep.Resource.(events.DirectoryResource); ok {
			return dirRes.Name, 0, ""
		}
	}

	// Try ExecutionEventPayload
	if eep, ok := payload.(*events.ExecutionEventPayload); ok {
		return eep.Resource.Name, eep.Resource.SizeBytes, eep.Resource.ContentType
	}

	// Try CompletionEventPayload (can be FileResource or DirectoryResource)
	if cep, ok := payload.(*events.CompletionEventPayload); ok {
		if fileRes, ok := cep.Resource.(events.FileResource); ok {
			return fileRes.Name, fileRes.SizeBytes, fileRes.ContentType
		}
		if dirRes, ok := cep.Resource.(events.DirectoryResource); ok {
			return dirRes.Name, 0, ""
		}
	}

	return "", 0, ""
}

// extractDirectoryID extracts directory ID from payload
func (t *LifecycleEventTrigger) extractDirectoryID(payload interface{}) string {
	// Try different payload types
	if fep, ok := payload.(*events.FileEventPayload); ok {
		return fep.Event.DirectoryPath
	}

	if dep, ok := payload.(*events.DirectoryEventPayload); ok {
		return dep.Event.DirectoryPath
	}

	if mep, ok := payload.(*events.MoveEventPayload); ok {
		return mep.Event.DirectoryPath
	}

	if aep, ok := payload.(*events.AuthorizationEventPayload); ok {
		// Extract from lifecycle event
		// For now, we need to pass this in metadata or extend the payload
		// Temporary: use metadata.RequestID as directory path (should be fixed)
		return aep.Metadata.RequestID
	}

	if vep, ok := payload.(*events.ValidationEventPayload); ok {
		return vep.Metadata.RequestID
	}

	if eep, ok := payload.(*events.ExecutionEventPayload); ok {
		return eep.Metadata.RequestID
	}

	if cep, ok := payload.(*events.CompletionEventPayload); ok {
		return cep.Metadata.RequestID
	}

	return ""
}

// executeHandler executes a handler synchronously and returns the response
func (t *LifecycleEventTrigger) executeHandler(ctx context.Context, handler events.EventHandler, payload interface{}) events.HandlerResponse {
	// Get handler implementation
	handlerImpl, exists := t.handlerRegistry.Get(handler.Type)
	if !exists {
		log.Printf("handler type %s not registered", handler.Type)
		return events.ErrorResponse(fmt.Sprintf("handler type %s not registered", handler.Type))
	}

	// Execute handler
	return handlerImpl.Handle(ctx, &handler, payload)
}

// dispatchAsync dispatches an event to a handler asynchronously
func (t *LifecycleEventTrigger) dispatchAsync(ctx context.Context, handler events.EventHandler, payload interface{}) {
	// Acquire worker slot (blocks if pool is full)
	t.workerPool <- struct{}{}
	t.wg.Add(1)

	go func() {
		defer func() {
			<-t.workerPool // Release worker slot
			t.wg.Done()
		}()

		response := t.executeHandler(ctx, handler, payload)

		// Log errors or vetos for async handlers
		if response.Veto {
			log.Printf("async handler %s vetoed (ignored in async mode): %s", handler.Name, response.Message)
		} else if !response.Success {
			log.Printf("async handler %s failed: %s", handler.Name, response.Message)
		}
	}()
}

// Wait waits for all pending async handlers to complete
func (t *LifecycleEventTrigger) Wait() {
	t.wg.Wait()
}

// Shutdown gracefully shuts down the event trigger
func (t *LifecycleEventTrigger) Shutdown(ctx context.Context) error {
	done := make(chan struct{})

	go func() {
		t.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

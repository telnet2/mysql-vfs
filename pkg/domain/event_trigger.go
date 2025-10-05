package domain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/events/handlers"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
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
	asyncTimeout    time.Duration
}

// EventTriggerConfig configures the event trigger
type EventTriggerConfig struct {
	MaxConcurrentHandlers int // Maximum number of async handlers running concurrently
	AsyncHandlerTimeout   time.Duration
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

	asyncTimeout := config.AsyncHandlerTimeout
	if asyncTimeout <= 0 {
		asyncTimeout = 30 * time.Second
	}

	return &LifecycleEventTrigger{
		eventsLoader:    eventsLoader,
		handlerRegistry: handlerRegistry,
		patternMatcher:  events.NewWildcardPatternMatcher(),
		workerPool:      make(chan struct{}, maxConcurrent),
		asyncTimeout:    asyncTimeout,
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
	// Resolve directory context before loading handlers
	dirID, err := t.resolveDirectoryID(ctx, payload)
	if err != nil {
		log.Printf("failed to resolve directory context for event %s: %v", eventType, err)
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
	// Resolve directory context before loading handlers
	dirID, err := t.resolveDirectoryID(ctx, payload)
	if err != nil {
		return fmt.Errorf("failed to resolve directory context for event %s: %w", eventType, err)
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

// resolveDirectoryID resolves the directory identifier associated with a payload.
func (t *LifecycleEventTrigger) resolveDirectoryID(ctx context.Context, payload interface{}) (string, error) {
	var candidateIDs []string
	var candidatePaths []string

	switch p := payload.(type) {
	case *events.FileEventPayload:
		t.addDirectoryPathCandidate(&candidatePaths, p.Event.DirectoryPath)
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.DirectoryEventPayload:
		t.addDirectoryContextCandidate(&candidatePaths, p.Event.DirectoryPath)
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.MoveEventPayload:
		t.addDirectoryContextCandidate(&candidatePaths, p.Event.DirectoryPath)
		t.addDirectoryPathCandidate(&candidatePaths, p.NewDirectory)
		t.addDirectoryPathCandidate(&candidatePaths, p.OldDirectory)
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.AuthorizationEventPayload:
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.ValidationEventPayload:
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.ExecutionEventPayload:
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
	case *events.CompletionEventPayload:
		t.collectPathsFromResource(&candidatePaths, &candidateIDs, p.Resource)
		if p.OperationContext != nil {
			if p.OperationContext.Category == events.CategoryDirectory {
				cleaned := t.addDirectoryPathCandidate(&candidatePaths, p.OperationContext.ResourcePath)
				if cleaned != "" {
					parent := path.Dir(cleaned)
					if parent != cleaned {
						t.addDirectoryPathCandidate(&candidatePaths, parent)
					}
				}
			} else {
				t.addFileDirectoryCandidate(&candidatePaths, p.OperationContext.ResourcePath)
			}
		}
	default:
		// No additional context available
	}

	// Prefer explicit identifiers if present
	for _, id := range candidateIDs {
		if id != "" {
			return id, nil
		}
	}

	if len(candidatePaths) == 0 {
		return "", fmt.Errorf("no directory path hints available")
	}

	var notFound []string
	var lastErr error
	for _, dirPath := range candidatePaths {
		resolved, err := t.eventsLoader.ResolveDirectoryID(ctx, dirPath)
		if err == nil {
			return resolved, nil
		}
		if errors.Is(err, db.ErrNotFound) {
			notFound = append(notFound, dirPath)
			lastErr = err
			continue
		}
		lastErr = err
	}

	if len(notFound) == len(candidatePaths) && len(notFound) > 0 {
		return "", fmt.Errorf("directories not found for paths: %s", strings.Join(notFound, ", "))
	}
	if lastErr != nil {
		return "", lastErr
	}

	return "", fmt.Errorf("unable to resolve directory context")
}

func (t *LifecycleEventTrigger) handlerContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		if requestID := parent.Value("requestID"); requestID != nil {
			base = context.WithValue(base, "requestID", requestID)
		}
	}

	if t.asyncTimeout > 0 {
		return context.WithTimeout(base, t.asyncTimeout)
	}

	return context.WithCancel(base)
}

func (t *LifecycleEventTrigger) collectPathsFromResource(paths *[]string, ids *[]string, resource interface{}) {
	switch res := resource.(type) {
	case events.FileResource:
		t.addFileDirectoryCandidate(paths, res.Path)
	case *events.FileResource:
		if res != nil {
			t.addFileDirectoryCandidate(paths, res.Path)
		}
	case events.DirectoryResource:
		if res.ID != "" {
			t.addIDCandidate(ids, res.ID)
		}
		t.addDirectoryContextCandidate(paths, res.Path)
	case *events.DirectoryResource:
		if res != nil {
			if res.ID != "" {
				t.addIDCandidate(ids, res.ID)
			}
			t.addDirectoryContextCandidate(paths, res.Path)
		}
	}
}

func (t *LifecycleEventTrigger) addIDCandidate(ids *[]string, candidate string) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return
	}
	for _, existing := range *ids {
		if existing == candidate {
			return
		}
	}
	*ids = append(*ids, candidate)
}

func (t *LifecycleEventTrigger) addFileDirectoryCandidate(paths *[]string, filePath string) {
	cleaned := t.cleanAbsolutePath(filePath)
	if cleaned == "" {
		return
	}
	dirPath := path.Dir(cleaned)
	t.addCandidatePath(paths, dirPath)
}

func (t *LifecycleEventTrigger) addDirectoryPathCandidate(paths *[]string, dirPath string) string {
	return t.addCandidatePath(paths, dirPath)
}

func (t *LifecycleEventTrigger) addDirectoryContextCandidate(paths *[]string, dirPath string) {
	cleaned := t.addDirectoryPathCandidate(paths, dirPath)
	if cleaned == "" {
		return
	}
	parent := path.Dir(cleaned)
	if parent != cleaned {
		t.addCandidatePath(paths, parent)
	}
}

func (t *LifecycleEventTrigger) addCandidatePath(paths *[]string, candidate string) string {
	cleaned := t.cleanAbsolutePath(candidate)
	if cleaned == "" {
		return ""
	}
	for _, existing := range *paths {
		if existing == cleaned {
			return cleaned
		}
	}
	*paths = append(*paths, cleaned)
	return cleaned
}

func (t *LifecycleEventTrigger) cleanAbsolutePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return ""
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return ""
	}
	return cleaned
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
		handlerCtx, cancel := t.handlerContext(ctx)
		defer func() {
			cancel()
			<-t.workerPool // Release worker slot
			t.wg.Done()
		}()

		response := t.executeHandler(handlerCtx, handler, payload)

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

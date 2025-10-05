package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// EventsLoader loads and caches .events configurations
type EventsLoader struct {
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    sync.Map // map[directoryID]*eventsCacheEntry
	ttl      time.Duration
}

type eventsCacheEntry struct {
	config    *events.EventsFile
	expiresAt time.Time
}

// NewEventsLoader creates a new events loader
func NewEventsLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, ttl time.Duration) *EventsLoader {
	return &EventsLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		ttl:      ttl,
	}
}

// Load loads .events config for a directory (with inheritance and merging)
func (l *EventsLoader) Load(ctx context.Context, dirID string) (*events.EventsFile, error) {
	// Check cache
	if entry, ok := l.cache.Load(dirID); ok {
		cached := entry.(*eventsCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.config, nil
		}
		// Expired, remove from cache
		l.cache.Delete(dirID)
	}

	// Load from this directory and all parents (for inheritance)
	config, err := l.loadWithInheritance(ctx, dirID)
	if err != nil {
		return nil, err
	}

	// Cache it
	l.cache.Store(dirID, &eventsCacheEntry{
		config:    config,
		expiresAt: time.Now().Add(l.ttl),
	})

	return config, nil
}

// loadWithInheritance loads .events from directory and merges with parents
func (l *EventsLoader) loadWithInheritance(ctx context.Context, dirID string) (*events.EventsFile, error) {
	// Try to load from this directory
	var currentConfig *events.EventsFile
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dirID, ".events")
	if err == nil {
		// Found .events in this directory
		var content []byte
		if file.JSONContent != nil {
			content = []byte(*file.JSONContent)
		} else if file.TextContent != nil {
			content = []byte(*file.TextContent)
		} else {
			return nil, fmt.Errorf(".events has no content")
		}

		var config events.EventsFile
		if err := json.Unmarshal(content, &config); err != nil {
			return nil, fmt.Errorf("invalid .events: %w", err)
		}
		currentConfig = &config
	}

	// Load parent config (if exists)
	dir, err := l.dirRepo.FindByID(ctx, dirID)
	if err != nil {
		// Directory not found, return current config or error
		if currentConfig != nil {
			return currentConfig, nil
		}
		return nil, fmt.Errorf(".events not found")
	}

	// If no parent, return current config
	if dir.ParentID == nil {
		if currentConfig != nil {
			return currentConfig, nil
		}
		// No .events found anywhere
		return nil, fmt.Errorf(".events not found")
	}

	// Recursively load parent config
	parentConfig, err := l.loadWithInheritance(ctx, *dir.ParentID)
	if err != nil {
		// Parent has no .events, return current config
		if currentConfig != nil {
			return currentConfig, nil
		}
		return nil, err
	}

	// Merge current with parent (current overrides parent)
	if currentConfig != nil {
		return l.mergeConfigs(parentConfig, currentConfig), nil
	}

	// No config in current dir, return parent
	return parentConfig, nil
}

// mergeConfigs merges child config with parent config
// Rules:
// 1. All parent handlers are included
// 2. Child handlers with same name override parent handlers
// 3. Child handlers with enabled=false remove parent handlers
func (l *EventsLoader) mergeConfigs(parent, child *events.EventsFile) *events.EventsFile {
	merged := &events.EventsFile{
		Handlers: []events.EventHandler{},
	}

	// Create map of child handler names for quick lookup
	childHandlerMap := make(map[string]*events.EventHandler)
	for i := range child.Handlers {
		childHandlerMap[child.Handlers[i].Name] = &child.Handlers[i]
	}

	// Add parent handlers (unless overridden or disabled by child)
	for _, parentHandler := range parent.Handlers {
		if childHandler, exists := childHandlerMap[parentHandler.Name]; exists {
			// Child has handler with same name
			if !childHandler.IsEnabled() {
				// Child disabled this handler - skip it
				continue
			}
			// Child overrides parent - we'll add child version later
			continue
		}
		// No override from child - add parent handler
		merged.Handlers = append(merged.Handlers, parentHandler)
	}

	// Add all child handlers (except disabled ones)
	for _, childHandler := range child.Handlers {
		if childHandler.IsEnabled() {
			merged.Handlers = append(merged.Handlers, childHandler)
		}
	}

	return merged
}

// GetAllHandlers returns all enabled handlers (regardless of event type)
func (l *EventsLoader) GetAllHandlers(ctx context.Context, dirID string) ([]events.EventHandler, error) {
	config, err := l.Load(ctx, dirID)
	if err != nil {
		// No .events config - return empty list
		return []events.EventHandler{}, nil
	}

	var handlers []events.EventHandler
	for _, handler := range config.Handlers {
		// Only return enabled handlers
		if handler.IsEnabled() {
			handlers = append(handlers, handler)
		}
	}

	return handlers, nil
}

// GetHandlersForEvent returns all enabled handlers for a specific event type
func (l *EventsLoader) GetHandlersForEvent(ctx context.Context, dirID string, eventType events.EventType) ([]events.EventHandler, error) {
	config, err := l.Load(ctx, dirID)
	if err != nil {
		// No .events config - return empty list
		return []events.EventHandler{}, nil
	}

	var handlers []events.EventHandler
	for _, handler := range config.Handlers {
		// Check if handler is enabled
		if !handler.IsEnabled() {
			continue
		}

		// Check if handler handles this event type
		for _, et := range handler.Events {
			if et == eventType {
				handlers = append(handlers, handler)
				break
			}
		}
	}

	return handlers, nil
}

// ShouldHandleEvent checks if an event should be handled by a handler
// Returns true if the event matches the handler's filter criteria
func (l *EventsLoader) ShouldHandleEvent(handler *events.EventHandler, fileName string, sizeBytes int64, contentType string) bool {
	// No filter - handle all events
	if handler.Filter == nil {
		return true
	}

	filter := handler.Filter

	// Check pattern match
	if filter.Pattern != "" {
		matched, err := l.matchPattern(fileName, filter.Pattern, filter.Type)
		if err != nil || !matched {
			return false
		}
	}

	// Check size constraints
	if filter.MinSizeBytes != nil && sizeBytes < *filter.MinSizeBytes {
		return false
	}
	if filter.MaxSizeBytes != nil && sizeBytes > *filter.MaxSizeBytes {
		return false
	}

	// Check content type
	if len(filter.ContentTypes) > 0 {
		matched := false
		for _, ct := range filter.ContentTypes {
			if ct == contentType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// matchPattern checks if filename matches pattern
func (l *EventsLoader) matchPattern(fileName, pattern, patternType string) (bool, error) {
	switch patternType {
	case "glob", "":
		// Default to glob if not specified
		return filepath.Match(pattern, fileName)
	case "regex":
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(fileName), nil
	default:
		return false, fmt.Errorf("unknown pattern type: %s", patternType)
	}
}

// InvalidateCache invalidates the cache for a directory
func (l *EventsLoader) InvalidateCache(dirID string) {
	l.cache.Delete(dirID)
}

// InvalidateCacheRecursive invalidates cache for directory and all children
// This is useful when a parent .events file is updated
func (l *EventsLoader) InvalidateCacheRecursive(ctx context.Context, dirID string) error {
	// Invalidate this directory
	l.InvalidateCache(dirID)

	// TODO: In a full implementation, we would query all child directories
	// and invalidate their caches as well. For now, we rely on TTL expiration.
	// This could be improved with a more sophisticated cache invalidation strategy.

	return nil
}

// ResolveDirectoryID resolves a directory path to its persistent identifier.
func (l *EventsLoader) ResolveDirectoryID(ctx context.Context, dirPath string) (string, error) {
	if dirPath == "" {
		return "", fmt.Errorf("empty directory path")
	}

	dir, err := l.dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		return "", err
	}

	return dir.ID, nil
}

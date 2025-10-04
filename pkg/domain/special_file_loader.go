package domain

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/repository"
)

// CacheEntry holds cached special file content with expiration
type CacheEntry struct {
	Content   []byte
	ExpiresAt time.Time
}

// GenericLoader loads and caches special files with inheritance support
type GenericLoader struct {
	fileRepo repository.FileRepository
	dirRepo  repository.DirectoryRepository
	fileType SpecialFileType
	cache    *sync.Map
	cacheTTL time.Duration
}

// NewGenericLoader creates a loader for any special file type
func NewGenericLoader(
	fileRepo repository.FileRepository,
	dirRepo repository.DirectoryRepository,
	fileType SpecialFileType,
	cacheTTL time.Duration,
) *GenericLoader {
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute // Default to 5 minutes
	}

	return &GenericLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		fileType: fileType,
		cache:    &sync.Map{},
		cacheTTL: cacheTTL,
	}
}

// Load loads the special file for a directory (with inheritance if supported)
func (l *GenericLoader) Load(ctx context.Context, directoryPath string) ([]byte, error) {
	// Normalize path
	directoryPath = path.Clean(directoryPath)

	// Check cache first
	if cached, ok := l.getFromCache(directoryPath); ok {
		return cached, nil
	}

	// Get the definition to check if inheritance is supported
	def, exists := GetDefinition(l.fileType)
	if !exists {
		return nil, fmt.Errorf("special file type not registered: %s", l.fileType)
	}

	// Try to load from current directory
	content, err := l.loadFromDirectory(ctx, directoryPath)
	if err == nil {
		// Found it - cache and return
		l.putInCache(directoryPath, content)
		return content, nil
	}

	// If not found and supports inheritance, try parent directory
	if def.InheritFromParent && directoryPath != "/" {
		parentPath := path.Dir(directoryPath)
		if parentPath == "." {
			parentPath = "/"
		}
		return l.Load(ctx, parentPath)
	}

	// Not found and no inheritance
	return nil, ErrNotFound
}

// LoadParsed loads and parses a special file into the provided struct
func (l *GenericLoader) LoadParsed(ctx context.Context, directoryPath string, target interface{}) error {
	content, err := l.Load(ctx, directoryPath)
	if err != nil {
		return err
	}

	// For JSON-based special files, unmarshal into target
	// This is a generic helper - specific loaders can override
	return parseJSON(content, target)
}

// loadFromDirectory loads the special file from a specific directory (no inheritance)
func (l *GenericLoader) loadFromDirectory(ctx context.Context, directoryPath string) ([]byte, error) {
	// Find the directory
	dir, err := l.dirRepo.FindByPath(ctx, directoryPath)
	if err != nil {
		return nil, err
	}

	// Look for the special file in this directory
	fileName := string(l.fileType)
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dir.ID, fileName)
	if err != nil {
		return nil, err
	}

	// Get the latest version
	version, err := l.fileRepo.GetLatestVersion(ctx, file.ID)
	if err != nil {
		return nil, err
	}

	content := ""
	if version.JSONContent != nil {
		content = *version.JSONContent
	}

	return []byte(content), nil
}

// getFromCache retrieves content from cache if not expired
func (l *GenericLoader) getFromCache(directoryPath string) ([]byte, bool) {
	value, ok := l.cache.Load(directoryPath)
	if !ok {
		return nil, false
	}

	entry, ok := value.(*CacheEntry)
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		l.cache.Delete(directoryPath)
		return nil, false
	}

	return entry.Content, true
}

// putInCache stores content in cache with TTL
func (l *GenericLoader) putInCache(directoryPath string, content []byte) {
	entry := &CacheEntry{
		Content:   content,
		ExpiresAt: time.Now().Add(l.cacheTTL),
	}
	l.cache.Store(directoryPath, entry)
}

// Invalidate clears cache for a directory and all its children
func (l *GenericLoader) Invalidate(directoryPath string) {
	directoryPath = path.Clean(directoryPath)

	// Delete the specific path
	l.cache.Delete(directoryPath)

	// Also invalidate all child paths (they might inherit from this one)
	// We need to iterate through all cache entries and remove those that are children
	l.cache.Range(func(key, value interface{}) bool {
		cachedPath, ok := key.(string)
		if !ok {
			return true
		}

		// Check if cachedPath is a child of directoryPath
		if isChildPath(cachedPath, directoryPath) {
			l.cache.Delete(cachedPath)
		}

		return true
	})
}

// InvalidateAll clears the entire cache
func (l *GenericLoader) InvalidateAll() {
	l.cache = &sync.Map{}
}

// Exists checks if a special file exists at the given path (with inheritance)
func (l *GenericLoader) Exists(ctx context.Context, directoryPath string) bool {
	_, err := l.Load(ctx, directoryPath)
	return err == nil
}

// isChildPath checks if childPath is a descendant of parentPath
func isChildPath(childPath, parentPath string) bool {
	// Normalize paths
	childPath = path.Clean(childPath)
	parentPath = path.Clean(parentPath)

	// Root is parent of everything
	if parentPath == "/" {
		return childPath != "/"
	}

	// Check if child starts with parent + /
	return len(childPath) > len(parentPath) &&
		childPath[:len(parentPath)] == parentPath &&
		childPath[len(parentPath)] == '/'
}

// parseJSON is a helper to parse JSON content
func parseJSON(content []byte, target interface{}) error {
	// Import encoding/json at the top if needed
	// For now, this is a placeholder
	return fmt.Errorf("parseJSON not implemented - use specific loader methods")
}

package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// GroupLoader loads and caches .group files for resolving user groups
type GroupLoader struct {
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    sync.Map // map[directoryID]*groupCacheEntry
	ttl      time.Duration
}

type groupCacheEntry struct {
	config    *GroupConfig
	expiresAt time.Time
}

// NewGroupLoader creates a new group loader
func NewGroupLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, ttl time.Duration) *GroupLoader {
	return &GroupLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		ttl:      ttl,
	}
}

// Load loads .group config for a directory (with inheritance)
func (l *GroupLoader) Load(ctx context.Context, dirID string) (*GroupConfig, error) {
	// Check cache
	if entry, ok := l.cache.Load(dirID); ok {
		cached := entry.(*groupCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.config, nil
		}
		// Expired, remove from cache
		l.cache.Delete(dirID)
	}

	// Try to load from this directory
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dirID, string(SpecialFileTypeGroup))
	if err == nil {
		// Found .group in this directory
		var content []byte
		if file.JSONContent != nil {
			content = []byte(*file.JSONContent)
		} else if file.TextContent != nil {
			content = []byte(*file.TextContent)
		} else {
			return nil, fmt.Errorf(".group has no content")
		}

		var config GroupConfig
		if err := json.Unmarshal(content, &config); err != nil {
			return nil, fmt.Errorf("invalid .group: %w", err)
		}

		// Cache it
		l.cache.Store(dirID, &groupCacheEntry{
			config:    &config,
			expiresAt: time.Now().Add(l.ttl),
		})

		return &config, nil
	}

	// Not found in this directory - try parent (inheritance)
	dir, err := l.dirRepo.FindByID(ctx, dirID)
	if err != nil {
		return nil, fmt.Errorf(".group not found")
	}

	if dir.ParentID == nil {
		// Reached root, no .group found
		return nil, fmt.Errorf(".group not found")
	}

	// Recursively check parent
	return l.Load(ctx, *dir.ParentID)
}

// LoadFromRoot loads .group config from root directory
func (l *GroupLoader) LoadFromRoot(ctx context.Context) (*GroupConfig, error) {
	// Find root directory
	rootDir, err := l.dirRepo.FindByPath(ctx, "/")
	if err != nil {
		return nil, fmt.Errorf("failed to find root directory: %w", err)
	}

	return l.Load(ctx, rootDir.ID)
}

// GetUserGroups resolves which groups a user belongs to
func (l *GroupLoader) GetUserGroups(ctx context.Context, userID string) ([]string, error) {
	// Load .group from root (since .group can only exist at root)
	config, err := l.LoadFromRoot(ctx)
	if err != nil {
		// No .group file found - return empty groups
		return []string{}, nil
	}

	// Find all groups the user belongs to
	var userGroups []string
	for _, group := range config.Groups {
		for _, member := range group.Members {
			if member == userID {
				userGroups = append(userGroups, group.GroupID)
				break
			}
		}
	}

	return userGroups, nil
}

// GroupExists checks if a group ID exists in the .group file
func (l *GroupLoader) GroupExists(ctx context.Context, groupID string) (bool, error) {
	// Load .group from root
	config, err := l.LoadFromRoot(ctx)
	if err != nil {
		// No .group file found
		return false, nil
	}

	// Check if group exists
	for _, group := range config.Groups {
		if group.GroupID == groupID {
			return true, nil
		}
	}

	return false, nil
}

// InvalidateCache invalidates the cache for a directory
func (l *GroupLoader) InvalidateCache(dirID string) {
	l.cache.Delete(dirID)
}

// InvalidateAll invalidates the entire cache
func (l *GroupLoader) InvalidateAll() {
	l.cache = sync.Map{}
}

package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// OwnerLoader loads and caches .owner files for directory ownership
type OwnerLoader struct {
	fileRepo    db.FileRepository
	dirRepo     db.DirectoryRepository
	groupLoader *GroupLoader
	cache       sync.Map // map[directoryID]*ownerCacheEntry
	ttl         time.Duration
}

type ownerCacheEntry struct {
	config    *OwnerConfig
	expiresAt time.Time
}

// NewOwnerLoader creates a new owner loader
func NewOwnerLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, groupLoader *GroupLoader, ttl time.Duration) *OwnerLoader {
	return &OwnerLoader{
		fileRepo:    fileRepo,
		dirRepo:     dirRepo,
		groupLoader: groupLoader,
		ttl:         ttl,
	}
}

// Load loads .owner config for a directory (with inheritance)
func (l *OwnerLoader) Load(ctx context.Context, dirID string) (*OwnerConfig, error) {
	// Check cache
	if entry, ok := l.cache.Load(dirID); ok {
		cached := entry.(*ownerCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.config, nil
		}
		// Expired, remove from cache
		l.cache.Delete(dirID)
	}

	// Try to load from this directory
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dirID, string(SpecialFileTypeOwner))
	if err == nil {
		// Found .owner in this directory
		var content []byte
		if file.JSONContent != nil {
			content = []byte(*file.JSONContent)
		} else if file.TextContent != nil {
			content = []byte(*file.TextContent)
		} else {
			return nil, fmt.Errorf(".owner has no content")
		}

		var config OwnerConfig
		if err := json.Unmarshal(content, &config); err != nil {
			return nil, fmt.Errorf("invalid .owner: %w", err)
		}

		// Cache it
		l.cache.Store(dirID, &ownerCacheEntry{
			config:    &config,
			expiresAt: time.Now().Add(l.ttl),
		})

		return &config, nil
	}

	// Not found in this directory - try parent (inheritance)
	dir, err := l.dirRepo.FindByID(ctx, dirID)
	if err != nil {
		return nil, fmt.Errorf(".owner not found")
	}

	if dir.ParentID == nil {
		// Reached root, no .owner found
		return nil, fmt.Errorf(".owner not found")
	}

	// Recursively check parent
	return l.Load(ctx, *dir.ParentID)
}

// LoadByPath loads .owner config for a directory by path
func (l *OwnerLoader) LoadByPath(ctx context.Context, dirPath string) (*OwnerConfig, error) {
	// Find directory by path
	dir, err := l.dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %w", err)
	}

	return l.Load(ctx, dir.ID)
}

// GetOwnerGroups returns the owner groups for a directory
func (l *OwnerLoader) GetOwnerGroups(ctx context.Context, dirID string) ([]string, error) {
	config, err := l.Load(ctx, dirID)
	if err != nil {
		// No .owner found - no ownership constraint
		return []string{}, nil
	}

	return config.Owners, nil
}

// IsUserOwner checks if a user is part of any owner group for a directory
func (l *OwnerLoader) IsUserOwner(ctx context.Context, dirID string, userGroups []string) (bool, error) {
	ownerGroups, err := l.GetOwnerGroups(ctx, dirID)
	if err != nil {
		return false, err
	}

	// No owner groups means no restriction
	if len(ownerGroups) == 0 {
		return true, nil
	}

	// Check if user belongs to any of the owner groups
	ownerGroupSet := make(map[string]bool)
	for _, ownerGroup := range ownerGroups {
		ownerGroupSet[ownerGroup] = true
	}

	for _, userGroup := range userGroups {
		if ownerGroupSet[userGroup] {
			return true, nil
		}
	}

	return false, nil
}

// CanUserAccessDirectory checks if a user can access a directory based on ownership
func (l *OwnerLoader) CanUserAccessDirectory(ctx context.Context, dirID string, userID string) (bool, error) {
	// Get user's groups
	var userGroups []string
	if l.groupLoader != nil {
		groups, err := l.groupLoader.GetUserGroups(ctx, userID)
		if err != nil {
			return false, fmt.Errorf("failed to resolve user groups: %w", err)
		}
		userGroups = groups
	}

	// Check if user is owner
	return l.IsUserOwner(ctx, dirID, userGroups)
}

// FilterVisibleDirectories filters a list of directories to only those the user can access
func (l *OwnerLoader) FilterVisibleDirectories(ctx context.Context, dirIDs []string, userGroups []string) ([]string, error) {
	var visibleDirs []string

	for _, dirID := range dirIDs {
		isOwner, err := l.IsUserOwner(ctx, dirID, userGroups)
		if err != nil {
			// On error, skip this directory
			continue
		}

		if isOwner {
			visibleDirs = append(visibleDirs, dirID)
		}
	}

	return visibleDirs, nil
}

// InvalidateCache invalidates the cache for a directory
func (l *OwnerLoader) InvalidateCache(dirID string) {
	l.cache.Delete(dirID)
}

// InvalidateAll invalidates the entire cache
func (l *OwnerLoader) InvalidateAll() {
	l.cache = sync.Map{}
}

package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"golang.org/x/crypto/bcrypt"
)

// UserLoader loads and caches .user files
type UserLoader struct {
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    sync.Map // map[directoryID]*userCacheEntry
	ttl      time.Duration
}

type userCacheEntry struct {
	config    *UserConfig
	expiresAt time.Time
}

// NewUserLoader creates a new user loader
func NewUserLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, ttl time.Duration) *UserLoader {
	return &UserLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		ttl:      ttl,
	}
}

// LoadUser loads a user credential by directory path and user ID
func (l *UserLoader) LoadUser(ctx context.Context, dirPath, userID string) (*UserCredential, error) {
	// Find directory
	dir, err := l.dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %w", err)
	}

	// Load user config for this directory
	userConfig, err := l.loadUserConfig(ctx, dir.ID)
	if err != nil {
		return nil, err
	}

	// Find user in config
	for _, user := range userConfig.Users {
		if user.UserID == userID {
			return &user, nil
		}
	}

	return nil, fmt.Errorf("user not found: %s", userID)
}

// LoadUserByToken finds a user by their static token
func (l *UserLoader) LoadUserByToken(ctx context.Context, dirPath, token string) (*UserCredential, error) {
	// Find directory
	dir, err := l.dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %w", err)
	}

	// Load user config
	userConfig, err := l.loadUserConfig(ctx, dir.ID)
	if err != nil {
		return nil, err
	}

	// Find user by token
	for _, user := range userConfig.Users {
		if user.Token != "" && user.Token == token {
			return &user, nil
		}
	}

	return nil, fmt.Errorf("invalid token")
}

// ValidatePassword checks if a password matches the user's password hash
func (l *UserLoader) ValidatePassword(user *UserCredential, password string) error {
	if user.PasswordHash == "" {
		return fmt.Errorf("user has no password")
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// loadUserConfig loads .user file from a directory (with caching)
func (l *UserLoader) loadUserConfig(ctx context.Context, dirID string) (*UserConfig, error) {
	// Check cache
	if entry, ok := l.cache.Load(dirID); ok {
		cached := entry.(*userCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.config, nil
		}
		// Expired, remove from cache
		l.cache.Delete(dirID)
	}

	// Load .user file
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dirID, string(SpecialFileTypeUser))
	if err != nil {
		return nil, fmt.Errorf(".user file not found in directory %s", dirID)
	}

	// Parse content
	var userConfig UserConfig
	var content []byte
	if file.JSONContent != nil {
		content = []byte(*file.JSONContent)
	} else if file.TextContent != nil {
		content = []byte(*file.TextContent)
	} else {
		return nil, fmt.Errorf(".user file has no content")
	}

	if err := json.Unmarshal(content, &userConfig); err != nil {
		return nil, fmt.Errorf("invalid .user file: %w", err)
	}

	// Cache it
	l.cache.Store(dirID, &userCacheEntry{
		config:    &userConfig,
		expiresAt: time.Now().Add(l.ttl),
	})

	return &userConfig, nil
}

// InvalidateCache invalidates the cache for a directory
func (l *UserLoader) InvalidateCache(dirID string) {
	l.cache.Delete(dirID)
}

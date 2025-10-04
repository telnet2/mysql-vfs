package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/repository"
	"github.com/xeipuuv/gojsonschema"
)

// FilesLoader loads and caches .files rules
type FilesLoader struct {
	fileRepo repository.FileRepository
	dirRepo  repository.DirectoryRepository
	cache    sync.Map // map[directoryID]*filesCacheEntry
	ttl      time.Duration
}

type filesCacheEntry struct {
	config    *FilesConfig
	expiresAt time.Time
}

// NewFilesLoader creates a new files loader
func NewFilesLoader(fileRepo repository.FileRepository, dirRepo repository.DirectoryRepository, ttl time.Duration) *FilesLoader {
	return &FilesLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		ttl:      ttl,
	}
}

// ValidateFile validates a file against .files rules
func (l *FilesLoader) ValidateFile(ctx context.Context, dirID, fileName string, content []byte) error {
	// Load .files config for this directory (with inheritance)
	config, err := l.Load(ctx, dirID)
	if err != nil {
		// No .files config found - allow by default
		return nil
	}

	// Find matching rule
	matchedRule := l.findMatchingRule(config, fileName)

	// No match - check default action
	if matchedRule == nil {
		defaultAction := config.DefaultAction
		if defaultAction == "" {
			defaultAction = "allow" // Default is allow
		}

		if defaultAction == "deny" {
			return fmt.Errorf("file %s does not match any allowed pattern", fileName)
		}

		// Allow without validation
		return nil
	}

	// Matched rule - validate if schema is provided
	if matchedRule.Schema != nil {
		return l.validateAgainstSchema(fileName, content, matchedRule.Schema)
	}

	// Matched but no schema - allow
	return nil
}

// findMatchingRule finds the first matching rule (order matters!)
func (l *FilesLoader) findMatchingRule(config *FilesConfig, fileName string) *FileRule {
	for _, rule := range config.Rules {
		matched, err := l.matchPattern(fileName, rule.Pattern, rule.Type)
		if err != nil {
			// Invalid pattern - skip
			continue
		}

		if matched {
			return &rule
		}
	}

	return nil
}

// matchPattern checks if filename matches pattern
func (l *FilesLoader) matchPattern(fileName, pattern, patternType string) (bool, error) {
	switch patternType {
	case "glob":
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

// validateAgainstSchema validates content against JSON schema
func (l *FilesLoader) validateAgainstSchema(fileName string, content []byte, schema map[string]interface{}) error {
	// Convert schema to JSON bytes
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	documentLoader := gojsonschema.NewBytesLoader(content)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if !result.Valid() {
		errMsg := fmt.Sprintf("content validation failed for %s:", fileName)
		for _, desc := range result.Errors() {
			errMsg += fmt.Sprintf("\n  - %s", desc)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// Load loads .files config for a directory (with inheritance)
func (l *FilesLoader) Load(ctx context.Context, dirID string) (*FilesConfig, error) {
	// Check cache
	if entry, ok := l.cache.Load(dirID); ok {
		cached := entry.(*filesCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.config, nil
		}
		// Expired, remove from cache
		l.cache.Delete(dirID)
	}

	// Try to load from this directory
	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dirID, string(SpecialFileTypeFiles))
	if err == nil {
		// Found .files in this directory
		var content []byte
		if file.JSONContent != nil {
			content = []byte(*file.JSONContent)
		} else {
			return nil, fmt.Errorf(".files has no content")
		}

		var config FilesConfig
		if err := json.Unmarshal(content, &config); err != nil {
			return nil, fmt.Errorf("invalid .files: %w", err)
		}

		// Cache it
		l.cache.Store(dirID, &filesCacheEntry{
			config:    &config,
			expiresAt: time.Now().Add(l.ttl),
		})

		return &config, nil
	}

	// Not found in this directory - try parent (inheritance)
	dir, err := l.dirRepo.FindByID(ctx, dirID)
	if err != nil {
		return nil, fmt.Errorf(".files not found")
	}

	if dir.ParentID == nil {
		// Reached root, no .files found
		return nil, fmt.Errorf(".files not found")
	}

	// Recursively check parent
	return l.Load(ctx, *dir.ParentID)
}

// InvalidateCache invalidates the cache for a directory
func (l *FilesLoader) InvalidateCache(dirID string) {
	l.cache.Delete(dirID)
}

package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"github.com/xeipuuv/gojsonschema"
)

// FilesLoader loads and caches .files rules
type FilesLoader struct {
	fileRepo    db.FileRepository
	dirRepo     db.DirectoryRepository
	cache       sync.Map // map[directoryID]*filesCacheEntry
	schemaCache sync.Map // map[schemaPath]interface{} - cache for loaded schemas
	ttl         time.Duration
}

type filesCacheEntry struct {
	config    *FilesConfig
	expiresAt time.Time
}

// NewFilesLoader creates a new files loader
func NewFilesLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, ttl time.Duration) *FilesLoader {
	return &FilesLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		ttl:      ttl,
	}
}

// ValidateFile validates a file against .files rules
func (l *FilesLoader) ValidateFile(ctx context.Context, dirID, fileName string, content []byte) error {
	// Built-in rules: .user and .group can only be created at root
	if err := l.validateBuiltInRules(ctx, dirID, fileName); err != nil {
		return err
	}

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
		return l.validateAgainstSchema(ctx, fileName, content, matchedRule.Schema)
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

// validateAgainstSchema validates content against JSON schema with $ref support
func (l *FilesLoader) validateAgainstSchema(ctx context.Context, fileName string, content []byte, schema map[string]interface{}) error {
	// Convert schema to JSON bytes
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	// Create a custom schema loader that supports schema:// protocol for VFS
	factory := NewVFSSchemaLoaderFactory(ctx, l.fileRepo, l.dirRepo, &l.schemaCache)

	// Create schema loader with custom reference loader
	sl := gojsonschema.NewSchemaLoader()
	sl.Validate = true

	// First, load the base schema
	baseLoader := gojsonschema.NewBytesLoader(schemaBytes)

	// If the schema contains schema:// references, preload them
	if needsVFSResolution(schema) {
		// Extract and preload schema:// references
		if err := l.preloadSchemaRefs(ctx, sl, schema, factory); err != nil {
			return fmt.Errorf("failed to preload schema references: %w", err)
		}
	}

	// Compile the schema
	compiledSchema, err := sl.Compile(baseLoader)
	if err != nil {
		return fmt.Errorf("schema compilation error: %w", err)
	}

	// Validate the document
	documentLoader := gojsonschema.NewBytesLoader(content)
	result, err := compiledSchema.Validate(documentLoader)
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

// needsVFSResolution checks if schema contains schema:// references
func needsVFSResolution(schema map[string]interface{}) bool {
	return containsSchemaProtocol(schema)
}

// containsSchemaProtocol recursively checks for schema:// in $ref
func containsSchemaProtocol(obj interface{}) bool {
	switch v := obj.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"].(string); ok {
			if strings.HasPrefix(ref, "schema://") {
				return true
			}
		}
		for _, val := range v {
			if containsSchemaProtocol(val) {
				return true
			}
		}
	case []interface{}:
		for _, val := range v {
			if containsSchemaProtocol(val) {
				return true
			}
		}
	}
	return false
}

// preloadSchemaRefs extracts and preloads all schema:// references
func (l *FilesLoader) preloadSchemaRefs(ctx context.Context, sl *gojsonschema.SchemaLoader, schema map[string]interface{}, factory *VFSSchemaLoaderFactory) error {
	refs := extractSchemaRefs(schema)
	for _, ref := range refs {
		loader := NewVFSSchemaLoader(ctx, l.fileRepo, l.dirRepo, &l.schemaCache, ref, factory)
		if err := sl.AddSchema(ref, loader); err != nil {
			return fmt.Errorf("failed to add schema %s: %w", ref, err)
		}
	}
	return nil
}

// extractSchemaRefs recursively extracts all schema:// $ref values
func extractSchemaRefs(obj interface{}) []string {
	var refs []string
	extractRefsRecursive(obj, &refs)
	return refs
}

func extractRefsRecursive(obj interface{}, refs *[]string) {
	switch v := obj.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"].(string); ok {
			if strings.HasPrefix(ref, "schema://") {
				*refs = append(*refs, ref)
			}
		}
		for _, val := range v {
			extractRefsRecursive(val, refs)
		}
	case []interface{}:
		for _, val := range v {
			extractRefsRecursive(val, refs)
		}
	}
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
		} else if file.TextContent != nil {
			content = []byte(*file.TextContent)
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

// validateBuiltInRules validates built-in rules that cannot be overridden
func (l *FilesLoader) validateBuiltInRules(ctx context.Context, dirID, fileName string) error {
	// Rule: .user and .group files can only be created at root directory
	if fileName == ".user" || fileName == ".group" {
		// Get directory path to check if it's root
		dir, err := l.dirRepo.FindByID(ctx, dirID)
		if err != nil {
			return fmt.Errorf("failed to check directory path: %w", err)
		}
		
		if dir.Path != "/" {
			return fmt.Errorf("%s files can only be created at root directory (/)", fileName)
		}
	}
	
	return nil
}

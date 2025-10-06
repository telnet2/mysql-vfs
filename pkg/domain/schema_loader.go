package domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// VFSSchemaLoader loads schemas from VFS using custom schema:// protocol
type VFSSchemaLoader struct {
	ctx      context.Context
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    *sync.Map // map[schemaPath][]byte - cached schema content
}

// NewVFSSchemaLoader creates a new VFS schema loader
func NewVFSSchemaLoader(
	ctx context.Context,
	fileRepo db.FileRepository,
	dirRepo db.DirectoryRepository,
	cache *sync.Map,
) *VFSSchemaLoader {
	return &VFSSchemaLoader{
		ctx:      ctx,
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		cache:    cache,
	}
}

// Load loads a schema from VFS by URL (implements custom loader for jsonschema.Compiler)
// Supports schema:///absolute/path.json format
func (l *VFSSchemaLoader) Load(url string) (io.ReadCloser, error) {
	if !strings.HasPrefix(url, "schema://") {
		return nil, fmt.Errorf("unsupported scheme: %s (expected schema://)", url)
	}

	// Parse schema:///path/to/file.json → /path/to/file.json
	path := strings.TrimPrefix(url, "schema://")
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("schema:// URLs must use absolute paths: %s", url)
	}

	// Check cache first
	if cached, ok := l.cache.Load(path); ok {
		return io.NopCloser(bytes.NewReader(cached.([]byte))), nil
	}

	// Load from VFS
	content, err := l.loadFromVFS(path)
	if err != nil {
		return nil, err
	}

	// Cache it
	l.cache.Store(path, content)

	return io.NopCloser(bytes.NewReader(content)), nil
}

// loadFromVFS loads a schema file from VFS
func (l *VFSSchemaLoader) loadFromVFS(path string) ([]byte, error) {
	// Parse the path to get directory and filename
	dirPath := filepath.Dir(path)
	fileName := filepath.Base(path)

	// Find the directory
	dir, err := l.dirRepo.FindByPath(l.ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s (for schema: %s)", dirPath, path)
	}

	// Find the file
	file, err := l.fileRepo.FindByDirectoryAndName(l.ctx, dir.ID, fileName)
	if err != nil {
		return nil, fmt.Errorf("schema file not found: %s", path)
	}

	// Get file content
	var content []byte
	if file.JSONContent != nil {
		content = []byte(*file.JSONContent)
	} else if file.TextContent != nil {
		content = []byte(*file.TextContent)
	} else {
		return nil, fmt.Errorf("schema file has no content: %s", path)
	}

	// Validate it's valid JSON
	var temp interface{}
	if err := json.Unmarshal(content, &temp); err != nil {
		return nil, fmt.Errorf("invalid JSON in schema file %s: %w", path, err)
	}

	return content, nil
}

// LazySchema wraps a JSON schema and resolves $ref references lazily before validation
type LazySchema struct {
	raw    interface{}
	loader *VFSSchemaLoader
	cache  map[string]*jsonschema.Schema // compiled schema cache by hash
}

// NewLazySchema creates a new lazy schema
func NewLazySchema(raw interface{}, loader *VFSSchemaLoader) *LazySchema {
	return &LazySchema{
		raw:    raw,
		loader: loader,
		cache:  make(map[string]*jsonschema.Schema),
	}
}

// ValidationError represents a structured validation error with field-specific messages
type ValidationError struct {
	Message string
	Errors  []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("%s: %s", e.Message, strings.Join(e.Errors, "; "))
	}
	return e.Message
}

// Validate validates data against the schema, resolving $ref dynamically
func (s *LazySchema) Validate(data interface{}) error {
	// Resolve all $ref references into inline schemas
	resolved, err := s.resolveRefs(s.copyData(s.raw))
	if err != nil {
		return err
	}

	// Cache key based on resolved schema hash
	key := s.hashData(resolved)
	if schema, ok := s.cache[key]; ok {
		return s.validateWithSchema(schema, data)
	}

	// Compile the resolved schema
	jsonBytes, err := json.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("failed to marshal resolved schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020 // Use latest draft

	// Set up custom loader for any remaining refs
	compiler.LoadURL = func(url string) (io.ReadCloser, error) {
		if url == "lazy://resolved" {
			return io.NopCloser(bytes.NewReader(jsonBytes)), nil
		}
		return s.loader.Load(url)
	}

	schema, err := compiler.Compile("lazy://resolved")
	if err != nil {
		return fmt.Errorf("schema compilation error: %w", err)
	}

	// Cache compiled schema
	s.cache[key] = schema

	// Validate the data
	return s.validateWithSchema(schema, data)
}

// validateWithSchema performs validation and returns structured errors
func (s *LazySchema) validateWithSchema(schema *jsonschema.Schema, data interface{}) error {
	if err := schema.Validate(data); err != nil {
		// Try to extract detailed validation errors
		if validationErr, ok := err.(*jsonschema.ValidationError); ok {
			var errors []string
			// The jsonschema library has Causes field for detailed errors
			for _, cause := range validationErr.Causes {
				field := cause.InstanceLocation
				if field == "" {
					field = "root"
				} else {
					// Clean up the field path - remove leading slash and #/
					field = strings.TrimPrefix(field, "/")
					field = strings.TrimPrefix(field, "#/")
				}
				message := cause.Message
				errors = append(errors, fmt.Sprintf("%s: %s", field, message))
			}

			if len(errors) > 0 {
				return &ValidationError{
					Message: "content validation failed",
					Errors:  errors,
				}
			}
		}

		// Fallback to generic error
		return &ValidationError{
			Message: "validation failed",
			Errors:  []string{err.Error()},
		}
	}

	return nil
}

// resolveRefs recursively resolves all $ref references into inline schemas
func (s *LazySchema) resolveRefs(data interface{}) (interface{}, error) {
	switch v := data.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"]; ok {
			url := ref.(string)
			// Load the referenced schema
			loaded, err := s.loader.Load(url)
			if err != nil {
				return nil, err
			}

			// Read the loaded schema
			buf := &bytes.Buffer{}
			io.Copy(buf, loaded)
			var loadedData interface{}
			if err := json.Unmarshal(buf.Bytes(), &loadedData); err != nil {
				return nil, fmt.Errorf("failed to parse $ref %s: %w", url, err)
			}

			// Recursively resolve refs in the loaded schema
			return s.resolveRefs(loadedData)
		}

		// Recursively resolve refs in all properties
		for k, val := range v {
			resolved, err := s.resolveRefs(val)
			if err != nil {
				return nil, err
			}
			v[k] = resolved
		}

	case []interface{}:
		// Recursively resolve refs in arrays
		for i, val := range v {
			resolved, err := s.resolveRefs(val)
			if err != nil {
				return nil, err
			}
			v[i] = resolved
		}
	}

	return data, nil
}

// copyData creates a deep copy of the data
func (s *LazySchema) copyData(data interface{}) interface{} {
	bytes, _ := json.Marshal(data)
	var copy interface{}
	json.Unmarshal(bytes, &copy)
	return copy
}

// hashData generates a hash of the data for cache keys
func (s *LazySchema) hashData(data interface{}) string {
	bytes, _ := json.Marshal(data)
	return fmt.Sprintf("%x", sha256.Sum256(bytes))
}

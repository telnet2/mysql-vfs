package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/xeipuuv/gojsonschema"
)

// ValidationMiddleware validates incoming requests against JSON schemas
type ValidationMiddleware struct {
	schemas map[string]*gojsonschema.Schema
	mu      sync.RWMutex
}

// NewValidationMiddleware creates a new validation middleware
func NewValidationMiddleware() *ValidationMiddleware {
	return &ValidationMiddleware{
		schemas: make(map[string]*gojsonschema.Schema),
	}
}

// LoadSchemaFromFile loads a JSON schema from a file
func (m *ValidationMiddleware) LoadSchemaFromFile(route, schemaPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Read schema file
	file, err := os.Open(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to open schema file: %w", err)
	}
	defer file.Close()

	schemaBytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Load schema
	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	m.schemas[route] = schema
	return nil
}

// LoadSchemasFromDirectory loads all JSON schemas from a directory
func (m *ValidationMiddleware) LoadSchemasFromDirectory(schemasDir string) error {
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		return fmt.Errorf("failed to read schemas directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Derive route from filename (e.g., "create_file_request.json" -> "/api/v1/files")
		route := deriveRouteFromFilename(entry.Name())
		schemaPath := filepath.Join(schemasDir, entry.Name())

		if err := m.LoadSchemaFromFile(route, schemaPath); err != nil {
			return fmt.Errorf("failed to load schema %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// RegisterSchema registers a schema for a specific route
func (m *ValidationMiddleware) RegisterSchema(route string, schema *gojsonschema.Schema) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schemas[route] = schema
}

// Handler returns a Hertz middleware handler
func (m *ValidationMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		route := string(c.FullPath())

		m.mu.RLock()
		schema, exists := m.schemas[route]
		m.mu.RUnlock()

		if !exists {
			// No schema registered for this route, skip validation
			c.Next(ctx)
			return
		}

		// Read request body
		bodyBytes := c.Request.Body()
		if len(bodyBytes) == 0 {
			c.JSON(400, map[string]string{
				"error": "request body is empty",
			})
			c.Abort()
			return
		}

		// Parse JSON
		var body interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			c.JSON(400, map[string]string{
				"error":   "invalid JSON",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		// Validate against schema
		documentLoader := gojsonschema.NewGoLoader(body)
		result, err := schema.Validate(documentLoader)
		if err != nil {
			c.JSON(500, map[string]string{
				"error":   "validation error",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		if !result.Valid() {
			// Collect validation errors
			errors := make([]string, 0, len(result.Errors()))
			for _, desc := range result.Errors() {
				errors = append(errors, desc.String())
			}

			c.JSON(400, map[string]interface{}{
				"error":   "validation failed",
				"details": errors,
			})
			c.Abort()
			return
		}

		// Store validated body in context for handlers to use
		ctx = context.WithValue(ctx, "validated_body", body)
		c.Next(ctx)
	}
}

// deriveRouteFromFilename converts schema filename to route
// Example: "create_file_request.json" -> "/api/v1/files"
func deriveRouteFromFilename(filename string) string {
	// Remove extension
	name := filename[:len(filename)-len(filepath.Ext(filename))]

	// Simple mapping (can be extended)
	routeMap := map[string]string{
		"create_directory_request": "/api/v1/directories",
		"create_file_request":      "/api/v1/files",
		"update_file_request":      "/api/v1/files/:id",
		"move_file_request":        "/api/v1/files/move",
	}

	if route, ok := routeMap[name]; ok {
		return route
	}

	return ""
}

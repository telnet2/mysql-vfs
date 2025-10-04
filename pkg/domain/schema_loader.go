package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/repository"
	"github.com/xeipuuv/gojsonschema"
)

// SchemaLoader loads and caches JSON schemas from .jsonschema files
type SchemaLoader struct {
	*GenericLoader
}

// NewSchemaLoader creates a new schema loader with caching and inheritance
func NewSchemaLoader(
	fileRepo repository.FileRepository,
	dirRepo repository.DirectoryRepository,
	cacheTTL time.Duration,
) *SchemaLoader {
	return &SchemaLoader{
		GenericLoader: NewGenericLoader(
			fileRepo,
			dirRepo,
			SpecialFileTypeSchema,
			cacheTTL,
		),
	}
}

// LoadSchema loads and compiles a JSON schema for a directory
func (l *SchemaLoader) LoadSchema(ctx context.Context, directoryPath string) (*gojsonschema.Schema, error) {
	// Load the raw schema content
	content, err := l.Load(ctx, directoryPath)
	if err != nil {
		return nil, err
	}

	// Compile the schema
	schemaLoader := gojsonschema.NewBytesLoader(content)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return schema, nil
}

// ValidateContent validates content against the schema for a directory
func (l *SchemaLoader) ValidateContent(ctx context.Context, directoryPath string, content []byte) error {
	// Load the schema
	schema, err := l.LoadSchema(ctx, directoryPath)
	if err != nil {
		if err == ErrNotFound {
			// No schema = no validation required
			return nil
		}
		return fmt.Errorf("failed to load schema: %w", err)
	}

	// Validate the content
	documentLoader := gojsonschema.NewBytesLoader(content)
	result, err := schema.Validate(documentLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		// Collect all validation errors
		errors := make([]string, 0, len(result.Errors()))
		for _, desc := range result.Errors() {
			errors = append(errors, desc.String())
		}

		return &ValidationError{
			Message: "content validation failed",
			Errors:  errors,
		}
	}

	return nil
}

// ValidationError represents content validation errors
type ValidationError struct {
	Message string
	Errors  []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 0 {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Errors)
}

// GetValidationErrors returns the detailed validation errors
func (e *ValidationError) GetValidationErrors() []string {
	return e.Errors
}

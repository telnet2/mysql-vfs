package domain

import (
	"encoding/json"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
)

// ContentValidator validates file content against JSON schemas
type ContentValidator struct {
}

// NewContentValidator creates a new content validator
func NewContentValidator() *ContentValidator {
	return &ContentValidator{}
}

// ValidateAgainstSchema validates content against a given JSON schema string
func (v *ContentValidator) ValidateAgainstSchema(schemaContent, content, contentType string) error {
	// Only validate JSON content
	if contentType != "application/json" {
		return nil // Skip validation for non-JSON files
	}

	// Parse schema
	schemaLoader := gojsonschema.NewStringLoader(schemaContent)
	jsonSchema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	// Parse content
	var contentData interface{}
	if err := json.Unmarshal([]byte(content), &contentData); err != nil {
		return fmt.Errorf("invalid JSON content: %w", err)
	}

	// Validate
	documentLoader := gojsonschema.NewGoLoader(contentData)
	result, err := jsonSchema.Validate(documentLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		errors := make([]string, 0, len(result.Errors()))
		for _, desc := range result.Errors() {
			errors = append(errors, desc.String())
		}
		return &ContentValidationError{
			Errors: errors,
		}
	}

	return nil
}

// ContentValidationError represents content validation failure
type ContentValidationError struct {
	Errors []string
}

func (e *ContentValidationError) Error() string {
	return "content validation failed"
}

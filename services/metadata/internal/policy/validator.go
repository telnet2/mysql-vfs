package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

// ValidationInput captures the data evaluated against JSON Schema manifests.
type ValidationInput struct {
	FileName string
	Document any
}

// SchemaFailure describes a single manifest that failed validation.
type SchemaFailure struct {
	Manifest Manifest
	Errors   []string
}

// SchemaValidationError aggregates schema failures for a validation attempt.
type SchemaValidationError struct {
	Failures []SchemaFailure
}

func (e *SchemaValidationError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Failures) == 0 {
		return "schema validation failed"
	}
	first := e.Failures[0]
	if len(first.Errors) == 0 {
		return fmt.Sprintf("schema validation failed for %s", first.Manifest.Name)
	}
	return fmt.Sprintf("schema validation failed for %s: %s", first.Manifest.Name, first.Errors[0])
}

type compiledSchema struct {
	manifest Manifest
	schema   *gojsonschema.Schema
}

func (c compiledSchema) applies(name string) bool {
	if len(c.manifest.AppliesTo) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(name)
	for _, pattern := range c.manifest.AppliesTo {
		patt := strings.TrimSpace(pattern)
		if patt == "" {
			continue
		}
		match, err := path.Match(patt, trimmed)
		if err != nil {
			continue
		}
		if match {
			return true
		}
	}
	return false
}

// Validator compiles JSON Schema manifests and validates documents against them.
type Validator struct {
	resolver Resolver

	mu    sync.RWMutex
	cache map[string][]compiledSchema
}

// NewValidator constructs a Validator backed by the provided manifest resolver.
func NewValidator(resolver Resolver) *Validator {
	return &Validator{resolver: resolver, cache: make(map[string][]compiledSchema)}
}

// Validate evaluates the provided input against all applicable schemas.
func (v *Validator) Validate(ctx context.Context, directoryID string, manifests []Manifest, input ValidationInput) error {
	if v == nil {
		return nil
	}
	dir := strings.TrimSpace(directoryID)
	if dir == "" {
		return ErrDirectoryNotFound
	}

	var err error
	if manifests == nil {
		manifests, err = v.resolver.Resolve(ctx, dir)
		if err != nil {
			return err
		}
	}

	compiled, err := v.prepare(ctx, dir, manifests)
	if err != nil {
		return err
	}

	document, err := normalizeDocument(input.Document)
	if err != nil {
		return err
	}

	loader := gojsonschema.NewGoLoader(document)
	failures := make([]SchemaFailure, 0)
	for _, schema := range compiled {
		if !schema.applies(input.FileName) {
			continue
		}
		result, err := schema.schema.Validate(loader)
		if err != nil {
			return fmt.Errorf("policy: execute json schema: %w", err)
		}
		if result.Valid() {
			continue
		}
		failure := SchemaFailure{Manifest: schema.manifest}
		for _, desc := range result.Errors() {
			failure.Errors = append(failure.Errors, desc.String())
		}
		failures = append(failures, failure)
	}

	if len(failures) > 0 {
		return &SchemaValidationError{Failures: failures}
	}
	return nil
}

// Invalidate removes cached schemas for the provided directories.
func (v *Validator) Invalidate(directoryIDs ...string) {
	if v == nil || len(directoryIDs) == 0 {
		return
	}
	v.mu.Lock()
	for _, id := range directoryIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			delete(v.cache, trimmed)
		}
	}
	v.mu.Unlock()
}

func (v *Validator) prepare(ctx context.Context, directoryID string, manifests []Manifest) ([]compiledSchema, error) {
	v.mu.RLock()
	entry, ok := v.cache[directoryID]
	v.mu.RUnlock()
	if ok {
		return entry, nil
	}

	compiled := make([]compiledSchema, 0)
	for idx, manifest := range manifests {
		if manifest.Type != TypeJSONSchema {
			continue
		}
		if len(strings.TrimSpace(string(manifest.Schema))) == 0 {
			continue
		}
		loader := gojsonschema.NewStringLoader(string(manifest.Schema))
		schema, err := gojsonschema.NewSchema(loader)
		if err != nil {
			return nil, fmt.Errorf("policy: compile json schema for %s (manifest %d): %w", manifest.SourcePath, idx, err)
		}
		compiled = append(compiled, compiledSchema{manifest: manifest, schema: schema})
	}

	v.mu.Lock()
	v.cache[directoryID] = compiled
	v.mu.Unlock()
	return compiled, nil
}

func normalizeDocument(doc any) (any, error) {
	switch value := doc.(type) {
	case nil:
		return map[string]any{}, nil
	case json.RawMessage:
		if len(value) == 0 {
			return map[string]any{}, nil
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("policy: decode json payload: %w", err)
		}
		return decoded, nil
	case []byte:
		if len(value) == 0 {
			return map[string]any{}, nil
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("policy: decode json payload: %w", err)
		}
		return decoded, nil
	default:
		return value, nil
	}
}

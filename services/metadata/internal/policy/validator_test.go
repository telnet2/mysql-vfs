package policy

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeSchemaResolver struct {
	manifests map[string][]Manifest
	err       error
}

func (f *fakeSchemaResolver) Resolve(ctx context.Context, directoryID string) ([]Manifest, error) {
	if f.err != nil {
		return nil, f.err
	}
	items := f.manifests[directoryID]
	copied := make([]Manifest, len(items))
	copy(copied, items)
	return copied, nil
}

func TestValidatorAllowsWhenSchemaSatisfied(t *testing.T) {
	resolver := &fakeSchemaResolver{manifests: map[string][]Manifest{
		"dir": {
			{
				Type:   TypeJSONSchema,
				Name:   ".jsonschema",
				Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
			},
		},
	}}
	validator := NewValidator(resolver)
	input := ValidationInput{FileName: "record.json", Document: map[string]any{"name": "alice"}}
	if err := validator.Validate(context.Background(), "dir", nil, input); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

func TestValidatorDeniesWhenSchemaFails(t *testing.T) {
	resolver := &fakeSchemaResolver{manifests: map[string][]Manifest{
		"dir": {
			{
				Type:   TypeJSONSchema,
				Name:   ".jsonschema",
				Schema: json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","minimum":1}},"required":["count"]}`),
			},
		},
	}}
	validator := NewValidator(resolver)
	input := ValidationInput{FileName: "record.json", Document: map[string]any{"count": 0}}
	err := validator.Validate(context.Background(), "dir", nil, input)
	if err == nil {
		t.Fatalf("expected validation failure")
	}
	schemaErr, ok := err.(*SchemaValidationError)
	if !ok {
		t.Fatalf("expected SchemaValidationError, got %T", err)
	}
	if len(schemaErr.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(schemaErr.Failures))
	}
	if len(schemaErr.Failures[0].Errors) == 0 {
		t.Fatalf("expected failure details")
	}
}

func TestValidatorRespectsAppliesTo(t *testing.T) {
	resolver := &fakeSchemaResolver{manifests: map[string][]Manifest{
		"dir": {
			{
				Type:      TypeJSONSchema,
				Name:      ".jsonschema",
				AppliesTo: []string{"*.json"},
				Schema:    json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
			},
		},
	}}
	validator := NewValidator(resolver)
	// Non-matching file should bypass schema.
	input := ValidationInput{FileName: "image.png", Document: map[string]any{"name": 123}}
	if err := validator.Validate(context.Background(), "dir", nil, input); err != nil {
		t.Fatalf("expected validation bypass, got %v", err)
	}
}

func TestValidatorCachesAndInvalidates(t *testing.T) {
	resolver := &fakeSchemaResolver{manifests: map[string][]Manifest{
		"dir": {
			{
				Type:   TypeJSONSchema,
				Name:   ".jsonschema",
				Schema: json.RawMessage(`{"type":"object","properties":{"flag":{"type":"boolean"}},"required":["flag"]}`),
			},
		},
	}}
	validator := NewValidator(resolver)
	input := ValidationInput{FileName: "record.json", Document: map[string]any{"flag": true}}
	if err := validator.Validate(context.Background(), "dir", nil, input); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}

	// Update resolver to require integer, but without invalidation cached schema should still allow boolean.
	resolver.manifests["dir"] = []Manifest{
		{
			Type:   TypeJSONSchema,
			Name:   ".jsonschema",
			Schema: json.RawMessage(`{"type":"object","properties":{"flag":{"type":"integer"}},"required":["flag"]}`),
		},
	}
	if err := validator.Validate(context.Background(), "dir", nil, input); err != nil {
		t.Fatalf("expected cached schema to pass, got %v", err)
	}

	validator.Invalidate("dir")
	if err := validator.Validate(context.Background(), "dir", nil, input); err == nil {
		t.Fatalf("expected validation failure after invalidation")
	}
}

func TestValidatorNormalizesRawJSON(t *testing.T) {
	resolver := &fakeSchemaResolver{manifests: map[string][]Manifest{
		"dir": {
			{
				Type:   TypeJSONSchema,
				Name:   ".jsonschema",
				Schema: json.RawMessage(`{"type":"object","properties":{"active":{"type":"boolean"}}}`),
			},
		},
	}}
	validator := NewValidator(resolver)
	raw := json.RawMessage(`{"active":true}`)
	input := ValidationInput{FileName: "record.json", Document: raw}
	if err := validator.Validate(context.Background(), "dir", nil, input); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

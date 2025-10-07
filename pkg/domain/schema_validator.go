package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/telnet2/mysql-vfs/pkg/etc"
)

// SchemaValidator provides JSON schema validation with lazy loading and caching
type SchemaValidator struct {
	schemaFile string
	once       sync.Once
	schema     *jsonschema.Schema
	err        error

	// Hooks
	preHooks  []ValidationHook
	postHooks []ValidationHook

	// Metrics
	metricsEnabled bool
	metrics        ValidationMetrics
	metricsMutex   sync.RWMutex
}

// ValidationHook is a function called before or after validation
type ValidationHook func(ctx context.Context, content []byte) error

// NewSchemaValidator creates a validator for a specific schema file
func NewSchemaValidator(schemaFile string) *SchemaValidator {
	return &SchemaValidator{
		schemaFile:     schemaFile,
		preHooks:       []ValidationHook{},
		postHooks:      []ValidationHook{},
		metricsEnabled: false,
	}
}

// WithPreHook adds a pre-validation hook
func (v *SchemaValidator) WithPreHook(hook ValidationHook) *SchemaValidator {
	v.preHooks = append(v.preHooks, hook)
	return v
}

// WithPostHook adds a post-validation hook
func (v *SchemaValidator) WithPostHook(hook ValidationHook) *SchemaValidator {
	v.postHooks = append(v.postHooks, hook)
	return v
}

// WithMetrics enables metrics collection
func (v *SchemaValidator) WithMetrics() *SchemaValidator {
	v.metricsEnabled = true
	return v
}

// GetMetrics returns a copy of the validation metrics
func (v *SchemaValidator) GetMetrics() ValidationMetrics {
	v.metricsMutex.RLock()
	defer v.metricsMutex.RUnlock()
	return v.metrics
}

// ResetMetrics resets all metrics counters
func (v *SchemaValidator) ResetMetrics() {
	v.metricsMutex.Lock()
	defer v.metricsMutex.Unlock()
	v.metrics = ValidationMetrics{}
}

// Validate validates content against the schema with hooks and metrics
func (v *SchemaValidator) Validate(content []byte) error {
	return v.ValidateWithContext(context.Background(), content)
}

// ValidateWithContext validates content against the schema with context support
func (v *SchemaValidator) ValidateWithContext(ctx context.Context, content []byte) error {
	startTime := time.Now()

	// Update metrics
	if v.metricsEnabled {
		defer func() {
			v.metricsMutex.Lock()
			v.metrics.TotalValidations++
			v.metrics.TotalDuration += time.Since(startTime)
			v.metrics.LastValidation = time.Now()
			v.metricsMutex.Unlock()
		}()
	}

	// Run pre-validation hooks
	for _, hook := range v.preHooks {
		if err := hook(ctx, content); err != nil {
			if v.metricsEnabled {
				v.metricsMutex.Lock()
				v.metrics.FailedValidations++
				v.metricsMutex.Unlock()
			}
			return fmt.Errorf("pre-validation hook failed: %w", err)
		}
	}

	// Load and compile schema on first use
	v.once.Do(func() {
		schemaContent, err := etc.GetSchemaContent(v.schemaFile)
		if err != nil {
			v.err = fmt.Errorf("failed to load schema %s: %w", v.schemaFile, err)
			return
		}

		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		v.err = compiler.AddResource(v.schemaFile, bytes.NewReader(schemaContent))
		if v.err != nil {
			return
		}

		v.schema, v.err = compiler.Compile(v.schemaFile)
	})

	if v.err != nil {
		if v.metricsEnabled {
			v.metricsMutex.Lock()
			v.metrics.FailedValidations++
			v.metricsMutex.Unlock()
		}
		return v.err
	}

	// Parse JSON
	var jsonObj map[string]interface{}
	if err := json.Unmarshal(content, &jsonObj); err != nil {
		if v.metricsEnabled {
			v.metricsMutex.Lock()
			v.metrics.FailedValidations++
			v.metricsMutex.Unlock()
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate against schema
	if err := v.schema.Validate(jsonObj); err != nil {
		if v.metricsEnabled {
			v.metricsMutex.Lock()
			v.metrics.FailedValidations++
			v.metricsMutex.Unlock()
		}
		return fmt.Errorf("schema validation failed: %w", err)
	}

	// Run post-validation hooks
	for _, hook := range v.postHooks {
		if err := hook(ctx, content); err != nil {
			if v.metricsEnabled {
				v.metricsMutex.Lock()
				v.metrics.FailedValidations++
				v.metricsMutex.Unlock()
			}
			return fmt.Errorf("post-validation hook failed: %w", err)
		}
	}

	return nil
}

// ValidateAndUnmarshal validates content against schema and unmarshals into target
func (v *SchemaValidator) ValidateAndUnmarshal(content []byte, target interface{}) error {
	if err := v.Validate(content); err != nil {
		return err
	}
	return json.Unmarshal(content, target)
}

// ValidateMap validates a map against the schema (useful for YAML that's already parsed)
func (v *SchemaValidator) ValidateMap(data map[string]interface{}) error {
	// Load and compile schema on first use
	v.once.Do(func() {
		schemaContent, err := etc.GetSchemaContent(v.schemaFile)
		if err != nil {
			v.err = fmt.Errorf("failed to load schema %s: %w", v.schemaFile, err)
			return
		}

		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		v.err = compiler.AddResource(v.schemaFile, bytes.NewReader(schemaContent))
		if v.err != nil {
			return
		}

		v.schema, v.err = compiler.Compile(v.schemaFile)
	})

	if v.err != nil {
		return v.err
	}

	// Validate the map directly
	if err := v.schema.Validate(data); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

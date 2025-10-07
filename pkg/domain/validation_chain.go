package domain

import (
	"context"
	"fmt"
	"time"
)

// Validator is a function that validates content
type Validator func(ctx context.Context, content []byte) error

// ValidationChain allows composing multiple validators
type ValidationChain struct {
	validators []Validator
	metrics    *ValidationMetrics
}

// ValidationMetrics tracks validation performance and failures
type ValidationMetrics struct {
	Enabled           bool
	TotalValidations  int64
	FailedValidations int64
	TotalDuration     time.Duration
	LastValidation    time.Time
}

// NewValidationChain creates a new validation chain
func NewValidationChain(validators ...Validator) *ValidationChain {
	return &ValidationChain{
		validators: validators,
		metrics:    &ValidationMetrics{Enabled: false},
	}
}

// WithMetrics enables metrics collection
func (c *ValidationChain) WithMetrics() *ValidationChain {
	c.metrics.Enabled = true
	return c
}

// Add adds a validator to the chain
func (c *ValidationChain) Add(validator Validator) *ValidationChain {
	c.validators = append(c.validators, validator)
	return c
}

// AddBefore adds a validator at the beginning of the chain
func (c *ValidationChain) AddBefore(validator Validator) *ValidationChain {
	c.validators = append([]Validator{validator}, c.validators...)
	return c
}

// Validate runs all validators in sequence
func (c *ValidationChain) Validate(ctx context.Context, content []byte) error {
	startTime := time.Now()

	if c.metrics.Enabled {
		defer func() {
			c.metrics.TotalValidations++
			c.metrics.TotalDuration += time.Since(startTime)
			c.metrics.LastValidation = time.Now()
		}()
	}

	for i, validator := range c.validators {
		if err := validator(ctx, content); err != nil {
			if c.metrics.Enabled {
				c.metrics.FailedValidations++
			}
			return fmt.Errorf("validator %d failed: %w", i, err)
		}
	}

	return nil
}

// GetMetrics returns a copy of the validation metrics
func (c *ValidationChain) GetMetrics() ValidationMetrics {
	if c.metrics == nil {
		return ValidationMetrics{}
	}
	return *c.metrics
}

// ResetMetrics resets all metrics counters
func (c *ValidationChain) ResetMetrics() {
	if c.metrics != nil {
		c.metrics.TotalValidations = 0
		c.metrics.FailedValidations = 0
		c.metrics.TotalDuration = 0
		c.metrics.LastValidation = time.Time{}
	}
}

// ValidationContext provides additional context for validators
type ValidationContext struct {
	Loaders       *SpecialFileLoaders
	DirectoryPath string
	FileName      string
	Operation     string // "create", "update", "delete"
}

// ContextAwareValidator is a validator that can access context
type ContextAwareValidator func(ctx context.Context, valCtx *ValidationContext, content []byte) error

// WrapContextAware wraps a context-aware validator into a regular Validator
func WrapContextAware(valCtx *ValidationContext, validator ContextAwareValidator) Validator {
	return func(ctx context.Context, content []byte) error {
		return validator(ctx, valCtx, content)
	}
}

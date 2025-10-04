package policy

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/open-policy-agent/opa/rego"
)

// TriggerContext carries runtime data used when evaluating event triggers.
type TriggerContext struct {
	DirectoryID string
	EventType   string
	Scope       Scope
	Actor       string
	RequestID   string
	FileID      string
	FileName    string
	FilePath    string
	StorageMode string
	Metadata    map[string]any
	Attributes  map[string]any
	Payload     any
}

// TriggerMatch ties a manifest trigger to its originating manifest.
type TriggerMatch struct {
	Manifest Manifest
	Trigger  *EventTrigger
}

// TriggerEngine evaluates `.events` manifests for matching triggers.
type TriggerEngine struct {
	resolver Resolver
}

// NewTriggerEngine constructs a TriggerEngine backed by the provided manifest resolver.
func NewTriggerEngine(resolver Resolver) *TriggerEngine {
	return &TriggerEngine{resolver: resolver}
}

// Evaluate returns triggers that match the supplied context. When manifests is nil the registry
// resolver is consulted for the directory.
func (e *TriggerEngine) Evaluate(ctx context.Context, directoryID string, manifests []Manifest, input TriggerContext) ([]TriggerMatch, error) {
	if e == nil {
		return nil, nil
	}
	dir := strings.TrimSpace(directoryID)
	if dir == "" {
		return nil, ErrDirectoryNotFound
	}
	input.DirectoryID = dir

	var err error
	if manifests == nil {
		manifests, err = e.resolver.Resolve(ctx, dir)
		if err != nil {
			return nil, err
		}
	}

	matches := make([]TriggerMatch, 0)
	for _, manifest := range manifests {
		if manifest.Type != TypeEvents || manifest.Events == nil {
			continue
		}
		if !scopeApplies(manifest.Scope, input.Scope) {
			continue
		}
		for idx := range manifest.Events.Triggers {
			trigger := &manifest.Events.Triggers[idx]
			if !strings.EqualFold(strings.TrimSpace(trigger.On), strings.TrimSpace(input.EventType)) {
				continue
			}
			if trigger.Scope != nil && (input.Scope == "" || *trigger.Scope != input.Scope) {
				continue
			}
			if !matchesFilters(trigger.Match, input) {
				continue
			}
			ok, err := evaluateConditions(ctx, trigger.Conditions, input)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			matches = append(matches, TriggerMatch{Manifest: manifest, Trigger: trigger})
		}
	}
	return matches, nil
}

func scopeApplies(manifestScope, inputScope Scope) bool {
	if manifestScope == "" {
		return true
	}
	if inputScope == "" {
		return manifestScope == ScopeTree
	}
	if manifestScope == ScopeTree {
		return true
	}
	return manifestScope == inputScope
}

func matchesFilters(filters EventTriggerMatch, input TriggerContext) bool {
	if len(filters) == 0 {
		return true
	}
	for key, patterns := range filters {
		value, ok := input.matchValue(key)
		if !ok {
			return false
		}
		if !patternListMatches(patterns, value) {
			return false
		}
	}
	return true
}

func (c TriggerContext) matchValue(rawKey string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(rawKey))
	switch key {
	case "event_type":
		if c.EventType == "" {
			return "", false
		}
		return c.EventType, true
	case "scope":
		if c.Scope == "" {
			return "", false
		}
		return string(c.Scope), true
	case "directory_id":
		if c.DirectoryID == "" {
			return "", false
		}
		return c.DirectoryID, true
	case "file_id":
		if c.FileID == "" {
			return "", false
		}
		return c.FileID, true
	case "file_name", "filename":
		if c.FileName == "" {
			return "", false
		}
		return c.FileName, true
	case "file_path", "path":
		if c.FilePath == "" {
			return "", false
		}
		return c.FilePath, true
	case "storage_mode":
		if c.StorageMode == "" {
			return "", false
		}
		return c.StorageMode, true
	case "actor":
		if c.Actor == "" {
			return "", false
		}
		return c.Actor, true
	case "request_id":
		if c.RequestID == "" {
			return "", false
		}
		return c.RequestID, true
	}
	if c.Attributes != nil {
		if val, ok := c.Attributes[key]; ok {
			return fmt.Sprint(val), true
		}
	}
	if c.Metadata != nil {
		if val, ok := c.Metadata[key]; ok {
			return fmt.Sprint(val), true
		}
	}
	return "", false
}

func patternListMatches(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return false
	}
	trimmedValue := strings.TrimSpace(value)
	for _, pattern := range patterns {
		patt := strings.TrimSpace(pattern)
		if patt == "" {
			continue
		}
		if matchesPattern(patt, trimmedValue) {
			return true
		}
	}
	return false
}

func matchesPattern(pattern, value string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := path.Match(pattern, value)
		return err == nil && matched
	}
	return pattern == value
}

func evaluateConditions(ctx context.Context, condition EventTriggerCondition, input TriggerContext) (bool, error) {
	reg := strings.TrimSpace(condition.Rego)
	if reg == "" {
		return true, nil
	}
	module := fmt.Sprintf("package trigger_condition\n\ndefault allow = false\n\nallow {\n%s\n}\n", reg)
	prepared, err := rego.New(
		rego.Query("data.trigger_condition.allow"),
		rego.Module("trigger_condition.rego", module),
	).PrepareForEval(ctx)
	if err != nil {
		return false, fmt.Errorf("policy: compile events condition: %w", err)
	}
	results, err := prepared.Eval(ctx, rego.EvalInput(input.toRegoInput()))
	if err != nil {
		return false, fmt.Errorf("policy: evaluate events condition: %w", err)
	}
	for _, result := range results {
		for _, expr := range result.Expressions {
			if allowed, ok := expr.Value.(bool); ok && allowed {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c TriggerContext) toRegoInput() map[string]any {
	event := map[string]any{
		"type":         c.EventType,
		"scope":        string(c.Scope),
		"directory_id": c.DirectoryID,
	}
	file := map[string]any{
		"id":           c.FileID,
		"name":         c.FileName,
		"path":         c.FilePath,
		"storage_mode": c.StorageMode,
	}
	input := map[string]any{
		"actor":        c.Actor,
		"event":        event,
		"file":         file,
		"metadata":     c.Metadata,
		"attributes":   c.Attributes,
		"payload":      c.Payload,
		"request_id":   c.RequestID,
		"directory_id": c.DirectoryID,
	}
	return input
}

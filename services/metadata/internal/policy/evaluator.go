package policy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/rego"
)

// Action represents a high-level mutation being authorized.
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// AuthorizationInput is provided to Rego policies when evaluating an operation.
type AuthorizationInput struct {
	Actor       string         `json:"actor"`
	Action      Action         `json:"action"`
	DirectoryID string         `json:"directory_id"`
	FileID      string         `json:"file_id,omitempty"`
	FileName    string         `json:"file_name,omitempty"`
	Path        string         `json:"path,omitempty"`
	Principals  PrincipalSet   `json:"principals"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type compiledPolicy struct {
	allowAll bool
	query    *rego.PreparedEvalQuery
}

// Evaluator compiles and executes Rego policy modules for directories.
type Evaluator struct {
	resolver Resolver

	mu    sync.RWMutex
	cache map[string]compiledPolicy
}

// NewEvaluator constructs an Evaluator backed by the provided manifest resolver.
func NewEvaluator(resolver Resolver) *Evaluator {
	return &Evaluator{resolver: resolver, cache: make(map[string]compiledPolicy)}
}

// Evaluate returns true when the configured policies allow the provided input.
func (e *Evaluator) Evaluate(ctx context.Context, directoryID string, manifests []Manifest, input AuthorizationInput) (bool, error) {
	if e == nil {
		return true, nil
	}
	dir := strings.TrimSpace(directoryID)
	if dir == "" {
		return false, ErrDirectoryNotFound
	}

	var err error
	if manifests == nil {
		manifests, err = e.resolver.Resolve(ctx, dir)
		if err != nil {
			return false, err
		}
	}

	entry, err := e.prepare(ctx, dir, manifests)
	if err != nil {
		return false, err
	}
	if entry.allowAll {
		return true, nil
	}

	results, err := entry.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, err
	}
	for _, result := range results {
		for _, expression := range result.Expressions {
			if allowed, ok := expression.Value.(bool); ok && allowed {
				return true, nil
			}
		}
	}
	return false, nil
}

// Invalidate removes cached compiled policies for the provided directories.
func (e *Evaluator) Invalidate(directoryIDs ...string) {
	if e == nil || len(directoryIDs) == 0 {
		return
	}
	e.mu.Lock()
	for _, id := range directoryIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			delete(e.cache, trimmed)
		}
	}
	e.mu.Unlock()
}

func (e *Evaluator) prepare(ctx context.Context, directoryID string, manifests []Manifest) (compiledPolicy, error) {
	e.mu.RLock()
	entry, ok := e.cache[directoryID]
	e.mu.RUnlock()
	if ok {
		return entry, nil
	}

	modules := gatherModules(manifests)
	if len(modules) == 0 {
		entry = compiledPolicy{allowAll: true}
		e.mu.Lock()
		e.cache[directoryID] = entry
		e.mu.Unlock()
		return entry, nil
	}

	opts := make([]func(*rego.Rego), 0, len(modules)+1)
	opts = append(opts, rego.Query("data.policy.allow"))
	for _, mod := range modules {
		opts = append(opts, rego.Module(mod.path, mod.source))
	}

	prepared, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return compiledPolicy{}, fmt.Errorf("policy: compile rego module: %w", err)
	}

	entry = compiledPolicy{query: &prepared}
	e.mu.Lock()
	e.cache[directoryID] = entry
	e.mu.Unlock()
	return entry, nil
}

type regoModule struct {
	path   string
	source string
}

func gatherModules(manifests []Manifest) []regoModule {
	modules := make([]regoModule, 0, len(manifests))
	for idx, manifest := range manifests {
		if manifest.Type != TypeRego {
			continue
		}
		source := strings.TrimSpace(manifest.Module)
		if source == "" {
			continue
		}
		path := manifest.SourcePath
		if strings.TrimSpace(path) == "" {
			path = fmt.Sprintf("manifest_%d.rego", idx)
		}
		modules = append(modules, regoModule{path: path, source: source})
	}
	return modules
}

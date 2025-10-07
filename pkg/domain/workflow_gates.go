package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/rego"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// WorkflowGateEvaluator evaluates workflow gate policies using OPA Rego
type WorkflowGateEvaluator struct {
	fileRepo db.FileRepository
	cache    *sync.Map
	cacheTTL time.Duration
}

// GateEvaluationResult represents the outcome of a gate evaluation
type GateEvaluationResult struct {
	Allowed    bool
	PolicyType string
}

type cachedRegoQuery struct {
	query     *rego.PreparedEvalQuery
	expiresAt time.Time
}

// WorkflowGateInput represents the data passed to gate policies
type WorkflowGateInput struct {
	User       WorkflowGateUser       `json:"user"`
	Transition WorkflowGateTransition `json:"transition"`
	File       WorkflowGateFile       `json:"file"`
	Workflow   WorkflowGateWorkflow   `json:"workflow"`
}

// WorkflowGateUser contains actor information
type WorkflowGateUser struct {
	ID     string   `json:"id"`
	Groups []string `json:"groups"`
}

// WorkflowGateTransition describes the attempted transition
type WorkflowGateTransition struct {
	Operation       string `json:"operation"`
	From            string `json:"from"`
	To              string `json:"to"`
	SourcePath      string `json:"source_path,omitempty"`
	DestinationPath string `json:"destination_path,omitempty"`
}

// WorkflowGateFile provides target file information
type WorkflowGateFile struct {
	Path     string                 `json:"path"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// WorkflowGateWorkflow includes workflow context
type WorkflowGateWorkflow struct {
	Path             string                     `json:"path"`
	Home             string                     `json:"home"`
	InitialState     string                     `json:"initial_state"`
	States           map[string]StateDefinition `json:"states"`
	StateDirectories map[string]string          `json:"state_directories"`
	RelativeDirs     map[string]string          `json:"relative_directories"`
}

// NewWorkflowGateEvaluator creates a new gate evaluator
func NewWorkflowGateEvaluator(fileRepo db.FileRepository, cacheTTL time.Duration) *WorkflowGateEvaluator {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	return &WorkflowGateEvaluator{
		fileRepo: fileRepo,
		cache:    &sync.Map{},
		cacheTTL: cacheTTL,
	}
}

// Evaluate runs the gate policy for the given workflow and input
func (e *WorkflowGateEvaluator) Evaluate(ctx context.Context, workflow *WorkflowDefinition, input *WorkflowGateInput) (*GateEvaluationResult, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow definition is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("gate input is nil")
	}

	policy, policyType, err := e.loadPolicy(ctx, workflow)
	if err != nil {
		return nil, err
	}
	if policy == "" {
		return &GateEvaluationResult{Allowed: true, PolicyType: policyType}, nil
	}

	cacheKey := buildPolicyCacheKey(workflow.WorkflowPath, policy)
	query, err := e.getPreparedQuery(ctx, cacheKey, policy)
	if err != nil {
		return nil, err
	}

	allowed, err := evaluateWithQuery(ctx, query, input)
	if err != nil {
		return nil, err
	}

	return &GateEvaluationResult{Allowed: allowed, PolicyType: policyType}, nil
}

// Invalidate clears cached queries for a workflow path
func (e *WorkflowGateEvaluator) Invalidate(workflowPath string) {
	if e == nil {
		return
	}
	prefix := workflowPath + ":"
	e.cache.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok {
			if strings.HasPrefix(k, prefix) {
				e.cache.Delete(k)
			}
		}
		return true
	})
}

func (e *WorkflowGateEvaluator) getPreparedQuery(ctx context.Context, cacheKey, policy string) (*rego.PreparedEvalQuery, error) {
	if value, ok := e.cache.Load(cacheKey); ok {
		if entry, ok := value.(*cachedRegoQuery); ok {
			if time.Now().Before(entry.expiresAt) {
				return entry.query, nil
			}
			e.cache.Delete(cacheKey)
		}
	}

	r := rego.New(
		rego.Query("data.vfs.workflow.gates.allow"),
		rego.Module("workflow_policy.rego", policy),
	)
	prepared, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, newWorkflowValidationError(ErrInvalidGatePolicy, fmt.Sprintf("failed to prepare gate policy: %v", err), map[string]interface{}{"reason": err.Error()})
	}

	ent := &cachedRegoQuery{query: &prepared, expiresAt: time.Now().Add(e.cacheTTL)}
	e.cache.Store(cacheKey, ent)
	return ent.query, nil
}

func (e *WorkflowGateEvaluator) loadPolicy(ctx context.Context, workflow *WorkflowDefinition) (string, string, error) {
	inline := strings.TrimSpace(workflow.GatePolicy)
	if inline != "" {
		return inline, "inline", nil
	}

	ref := strings.TrimSpace(workflow.GatePolicyRef)
	if ref == "" {
		return "", "none", nil
	}

	if e.fileRepo == nil {
		return "", "", fmt.Errorf("file repository not configured for external policy")
	}
	if workflow.WorkflowDirectoryID == "" {
		return "", "", fmt.Errorf("workflow directory id missing")
	}

	file, err := e.fileRepo.FindByDirectoryAndName(ctx, workflow.WorkflowDirectoryID, ref)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", "", newWorkflowValidationError(ErrGatePolicyNotFound, fmt.Sprintf("gate_policy_ref '%s' not found", ref), map[string]interface{}{"workflow_path": workflow.WorkflowPath})
		}
		return "", "", err
	}

	version, err := e.fileRepo.GetLatestVersion(ctx, file.ID)
	if err != nil {
		return "", "", err
	}

	var policy string
	if version.TextContent != nil {
		policy = *version.TextContent
	} else if version.JSONContent != nil {
		policy = *version.JSONContent
	} else {
		return "", "", newWorkflowValidationError(ErrInvalidGatePolicy, "gate_policy_ref content unavailable", map[string]interface{}{"workflow_path": workflow.WorkflowPath})
	}

	return strings.TrimSpace(policy), "external", nil
}

func evaluateWithQuery(ctx context.Context, query *rego.PreparedEvalQuery, input *WorkflowGateInput) (bool, error) {
	if query == nil {
		return false, fmt.Errorf("rego query is nil")
	}
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, newWorkflowValidationError(ErrInvalidGatePolicy, fmt.Sprintf("gate evaluation failed: %v", err), map[string]interface{}{"reason": err.Error()})
	}
	if len(results) == 0 {
		return false, nil
	}
	for _, result := range results {
		for _, expression := range result.Expressions {
			if allowed, ok := expression.Value.(bool); ok {
				return allowed, nil
			}
		}
	}
	return false, nil
}

func buildPolicyCacheKey(workflowPath, policy string) string {
	sum := sha256.Sum256([]byte(policy))
	return fmt.Sprintf("%s:%x", workflowPath, sum)
}

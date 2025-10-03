package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/open-policy-agent/opa/rego"
)

// Policy represents a Rego policy script with its pre-computed hash.
type Policy struct {
	Script string
	Hash   string
}

// EnsureHash calculates the hash of the policy script when not provided.
func (p *Policy) EnsureHash() {
	if p.Hash != "" || p.Script == "" {
		return
	}

	sum := sha256.Sum256([]byte(p.Script))
	p.Hash = hex.EncodeToString(sum[:])
}

// PolicyEngine caches compiled Rego policies keyed by policy hash.
type PolicyEngine struct {
	cache sync.Map
}

// NewPolicyEngine instantiates a policy engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// Evaluate executes the policy with the provided input. Policies must define
// data.authz.allow as the root allow decision. Evaluation returns true when the
// policy authorizes the request.
func (e *PolicyEngine) Evaluate(ctx context.Context, policy Policy, input map[string]any) (bool, error) {
	policy.EnsureHash()

	prepared, err := e.loadOrCompile(ctx, policy)
	if err != nil {
		return false, err
	}

	rs, err := prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, err
	}

	if len(rs) == 0 {
		return false, fmt.Errorf("policy returned no results")
	}

	allow, ok := rs[0].Expressions[0].Value.(bool)
	if !ok {
		return false, fmt.Errorf("policy returned non-boolean value")
	}

	return allow, nil
}

func (e *PolicyEngine) loadOrCompile(ctx context.Context, policy Policy) (*rego.PreparedEvalQuery, error) {
	if v, ok := e.cache.Load(policy.Hash); ok {
		if prepared, ok := v.(rego.PreparedEvalQuery); ok {
			return &prepared, nil
		}
	}

	moduleName := fmt.Sprintf("policy_%s.rego", policy.Hash)

	compiler := rego.New(
		rego.Query("data.authz.allow"),
		rego.Module(moduleName, policy.Script),
	)

	prepared, err := compiler.PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}

	e.cache.Store(policy.Hash, prepared)

	return &prepared, nil
}

package policy

import (
	"context"
	"testing"
)

func TestEvaluatorAllowsWhenRegoPermits(t *testing.T) {
	evaluator := NewEvaluator(nil)
	manifests := []Manifest{
		{
			Type:       TypeRego,
			SourcePath: "/policies/test.rego",
			Module: `package policy

default allow = false

allow {
    input.actor == "admin"
}`,
		},
	}

	input := AuthorizationInput{Actor: "admin", Action: ActionCreate, DirectoryID: "dir1"}
	allowed, err := evaluator.Evaluate(context.Background(), "dir1", manifests, input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !allowed {
		t.Fatalf("expected policy to allow admin actor")
	}
}

func TestEvaluatorDeniesWhenRegoRejects(t *testing.T) {
	evaluator := NewEvaluator(nil)
	manifests := []Manifest{
		{
			Type:       TypeRego,
			SourcePath: "/policies/test.rego",
			Module: `package policy

default allow = false

allow {
    input.actor == "admin"
}`,
		},
	}

	input := AuthorizationInput{Actor: "user", Action: ActionCreate, DirectoryID: "dir1"}
	allowed, err := evaluator.Evaluate(context.Background(), "dir1", manifests, input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if allowed {
		t.Fatalf("expected policy to deny non-admin actor")
	}
}

func TestEvaluatorErrorsOnInvalidModule(t *testing.T) {
	evaluator := NewEvaluator(nil)
	manifests := []Manifest{{Type: TypeRego, SourcePath: "invalid.rego", Module: "package policy\nallow { syntax error"}}

	_, err := evaluator.Evaluate(context.Background(), "dir1", manifests, AuthorizationInput{Actor: "admin", Action: ActionCreate, DirectoryID: "dir1"})
	if err == nil {
		t.Fatalf("expected compilation error for invalid module")
	}
}

func TestEvaluatorCachesAndInvalidates(t *testing.T) {
	evaluator := NewEvaluator(nil)
	manifests := []Manifest{
		{Type: TypeRego, SourcePath: "allow.rego", Module: "package policy\nallow = true"},
	}

	ctx := context.Background()
	if allowed, err := evaluator.Evaluate(ctx, "dir1", manifests, AuthorizationInput{Actor: "any", Action: ActionCreate, DirectoryID: "dir1"}); err != nil || !allowed {
		t.Fatalf("expected allow, got %v err %v", allowed, err)
	}

	evaluator.Invalidate("dir1")

	manifests = []Manifest{
		{Type: TypeRego, SourcePath: "deny.rego", Module: "package policy\nallow = false"},
	}
	if allowed, err := evaluator.Evaluate(ctx, "dir1", manifests, AuthorizationInput{Actor: "any", Action: ActionCreate, DirectoryID: "dir1"}); err != nil || allowed {
		t.Fatalf("expected deny after invalidation, got %v err %v", allowed, err)
	}
}

package middleware

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/rego"
	"github.com/stretchr/testify/assert"
)

func TestDefaultRegoPolicy(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected bool
	}{
		{
			name: "admin can read",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "alice",
					"role":    "admin",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "read",
			},
			expected: true,
		},
		{
			name: "admin can write",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "alice",
					"role":    "admin",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "write",
			},
			expected: true,
		},
		{
			name: "admin can delete",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "alice",
					"role":    "admin",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "delete",
			},
			expected: true,
		},
		{
			name: "user can read",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "bob",
					"role":    "user",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "read",
			},
			expected: true,
		},
		{
			name: "user cannot write",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "bob",
					"role":    "user",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "write",
			},
			expected: false,
		},
		{
			name: "user cannot delete",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "bob",
					"role":    "user",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "delete",
			},
			expected: false,
		},
		{
			name: "unknown role cannot read",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "guest",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "read",
			},
			expected: false,
		},
		{
			name: "unknown role cannot write",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "guest",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/file.json",
					"type": "file",
				},
				"action": "write",
			},
			expected: false,
		},
		{
			name: "user with admin group still needs admin role (not owner)",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "bob",
					"role":    "user",
					"groups":  []string{"admin"},
				},
				"resource": map[string]interface{}{
					"path":   "/data/file.json",
					"type":   "file",
					"owners": []string{"project-team"},
				},
				"action": "write",
			},
			expected: false,
		},
		{
			name: "user as owner can write",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "user",
					"groups":  []string{"project-alpha"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "write",
			},
			expected: true,
		},
		{
			name: "user as owner can delete",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "user",
					"groups":  []string{"project-alpha"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "delete",
			},
			expected: true,
		},
		{
			name: "user in multiple groups, one is owner",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "david",
					"role":    "user",
					"groups":  []string{"user-group", "project-beta", "other-group"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/beta/file.json",
					"type":   "file",
					"owners": []string{"admin", "project-beta"},
				},
				"action": "write",
			},
			expected: true,
		},
		{
			name: "user not in owner groups cannot write",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "eve",
					"role":    "user",
					"groups":  []string{"user-group"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "write",
			},
			expected: false,
		},
		{
			name: "user not in owner groups cannot delete",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "eve",
					"role":    "user",
					"groups":  []string{"user-group"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "delete",
			},
			expected: false,
		},
		{
			name: "user as owner but reading (should allow via read rule)",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "user",
					"groups":  []string{"project-alpha"},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "read",
			},
			expected: true,
		},
		{
			name: "no owners means no ownership restriction (user can only read)",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "charlie",
					"role":    "user",
					"groups":  []string{"project-alpha"},
				},
				"resource": map[string]interface{}{
					"path":   "/public/file.json",
					"type":   "file",
					"owners": []string{},
				},
				"action": "write",
			},
			expected: false,
		},
		{
			name: "admin ignores ownership",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"user_id": "admin-user",
					"role":    "admin",
					"groups":  []string{},
				},
				"resource": map[string]interface{}{
					"path":   "/projects/alpha/file.json",
					"type":   "file",
					"owners": []string{"project-alpha"},
				},
				"action": "write",
			},
			expected: true,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the policy
			query, err := rego.New(
				rego.Query("data.vfs.authz.allow"),
				rego.Module("default_policy.rego", DefaultRegoPolicy),
			).PrepareForEval(ctx)
			assert.NoError(t, err)

			// Evaluate the policy
			results, err := query.Eval(ctx, rego.EvalInput(tt.input))
			assert.NoError(t, err)

			// Check the result
			allowed := false
			if len(results) > 0 && len(results[0].Expressions) > 0 {
				allowed, _ = results[0].Expressions[0].Value.(bool)
			}

			assert.Equal(t, tt.expected, allowed, "Policy evaluation mismatch")
		})
	}
}

func TestDefaultPolicyFormat(t *testing.T) {
	// Verify the policy is valid Rego
	ctx := context.Background()

	_, err := rego.New(
		rego.Query("data.vfs.authz.allow"),
		rego.Module("default_policy.rego", DefaultRegoPolicy),
	).PrepareForEval(ctx)

	assert.NoError(t, err, "Default policy should be valid Rego")
}

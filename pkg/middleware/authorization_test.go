package middleware

import (
	"testing"
)

func TestEvaluateRegoPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		input    map[string]interface{}
		expected bool
	}{
		{
			name: "allow admin users",
			policy: `package vfs.authz

allow {
	input.user.role == "admin"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-1",
					"username": "alice",
					"role":     "admin",
					"groups":   []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/users",
					"type": "file",
				},
				"action": "create",
			},
			expected: true,
		},
		{
			name: "deny regular users",
			policy: `package vfs.authz

allow {
	input.user.role == "admin"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-2",
					"username": "bob",
					"role":     "user",
					"groups":   []string{},
				},
				"resource": map[string]interface{}{
					"path": "/data/users",
					"type": "file",
				},
				"action": "create",
			},
			expected: false,
		},
		{
			name: "allow read for everyone",
			policy: `package vfs.authz

allow {
	input.action == "read"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-3",
					"username": "charlie",
					"role":     "readonly",
					"groups":   []string{},
				},
				"resource": map[string]interface{}{
					"path": "/public/data",
					"type": "file",
				},
				"action": "read",
			},
			expected: true,
		},
		{
			name: "deny write for readonly users",
			policy: `package vfs.authz

allow {
	input.action == "read"
}

allow {
	input.user.role == "admin"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-4",
					"username": "dave",
					"role":     "readonly",
					"groups":   []string{},
				},
				"resource": map[string]interface{}{
					"path": "/public/data",
					"type": "file",
				},
				"action": "create",
			},
			expected: false,
		},
		{
			name: "allow group members",
			policy: `package vfs.authz

allow {
	input.user.groups[_] == "developers"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-5",
					"username": "eve",
					"role":     "user",
					"groups":   []string{"developers", "users"},
				},
				"resource": map[string]interface{}{
					"path": "/projects/dev",
					"type": "file",
				},
				"action": "create",
			},
			expected: true,
		},
		{
			name: "deny non-group members",
			policy: `package vfs.authz

allow {
	input.user.groups[_] == "developers"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-6",
					"username": "frank",
					"role":     "user",
					"groups":   []string{"users"},
				},
				"resource": map[string]interface{}{
					"path": "/projects/dev",
					"type": "file",
				},
				"action": "create",
			},
			expected: false,
		},
		{
			name: "complex policy with path matching",
			policy: `package vfs.authz

allow {
	startswith(input.resource.path, "/public")
}

allow {
	input.user.role == "admin"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       "user-7",
					"username": "grace",
					"role":     "user",
					"groups":   []string{},
				},
				"resource": map[string]interface{}{
					"path": "/public/announcements",
					"type": "file",
				},
				"action": "create",
			},
			expected: true,
		},
		{
			name: "invalid policy syntax",
			policy: `package vfs.authz

allow {
	invalid syntax here
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"role": "admin",
				},
			},
			expected: false, // Should fail closed on compilation error
		},
		{
			name: "policy without package",
			policy: `allow {
	input.user.role == "admin"
}`,
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"role": "admin",
				},
			},
			expected: false, // Wrong package, should deny
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateRegoPolicy(tt.policy, tt.input)

			if result != tt.expected {
				t.Errorf("evaluateRegoPolicy() = %v, want %v", result, tt.expected)
				t.Logf("Policy: %s", tt.policy)
				t.Logf("Input: %+v", tt.input)
			}
		})
	}
}

func TestEvaluateRegoPolicy_EdgeCases(t *testing.T) {
	t.Run("empty policy", func(t *testing.T) {
		result := evaluateRegoPolicy("", map[string]interface{}{})
		if result {
			t.Error("Empty policy should deny (fail closed)")
		}
	})

	t.Run("nil input", func(t *testing.T) {
		policy := `package vfs.authz
allow {
	true
}`
		result := evaluateRegoPolicy(policy, nil)
		if !result {
			t.Error("Policy 'allow { true }' with nil input should allow")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		policy := `package vfs.authz
allow {
	true
}`
		result := evaluateRegoPolicy(policy, map[string]interface{}{})
		if !result {
			t.Error("Policy 'allow { true }' with empty input should allow")
		}
	})
}

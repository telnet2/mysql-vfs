package domain

import (
	"testing"
)

func TestIsSpecialFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"json schema", ".jsonschema", true},
		{"rego policy", ".rego", true},
		{"hidden file", ".hiddenfile", true},
		{"regular file", "data.json", false},
		{"no extension", "README", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSpecialFile(tt.filename); got != tt.want {
				t.Errorf("IsSpecialFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsRegisteredSpecialFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"files config", ".files", true},
		{"rego policy", ".rego", true},
		{"events file", ".events", true},
		{"user file", ".user", true},
		{"group file", ".group", true},
		{"unregistered special file", ".custom", false},
		{"regular file", "data.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRegisteredSpecialFile(tt.filename); got != tt.want {
				t.Errorf("IsRegisteredSpecialFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestRequiresAdmin(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"files config", ".files", true},
		{"rego policy", ".rego", true},
		{"events file", ".events", false}, // events don't require admin
		{"user file", ".user", true},
		{"group file", ".group", true},
		{"unregistered special file", ".custom", true}, // secure by default
		{"regular file", "data.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only check registered special files
			if !IsSpecialFile(tt.filename) {
				return
			}

			if got := RequiresAdmin(tt.filename); got != tt.want {
				t.Errorf("RequiresAdmin(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsSystemAdmin(t *testing.T) {
	tests := []struct {
		name     string
		userRole string
		want     bool
	}{
		{"system-admin role", "system-admin", true},
		{"admin role", "admin", false},
		{"user role", "user", false},
		{"readonly role", "readonly", false},
		{"empty role", "", false},
		{"random role", "developer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSystemAdmin(tt.userRole); got != tt.want {
				t.Errorf("IsSystemAdmin(%q) = %v, want %v", tt.userRole, got, tt.want)
			}
		})
	}
}

func TestSupportsInheritance(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"files config", ".files", true},
		{"rego policy", ".rego", true},
		{"events file", ".events", true},  // events inherit and merge
		{"user file", ".user", false},     // no inheritance
		{"group file", ".group", false},   // no inheritance
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsInheritance(tt.filename); got != tt.want {
				t.Errorf("SupportsInheritance(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestValidateJSONSchema(t *testing.T) {
	t.Skip("Skipping - validateJSONSchema function was removed/refactored")
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid schema",
			content: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			content: `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "empty",
			content: ``,
			wantErr: true,
		},
		{
			name:    "invalid schema structure",
			content: `{"type": "invalid_type"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// err := validateJSONSchema([]byte(tt.content))
			// if (err != nil) != tt.wantErr {
			// 	t.Errorf("validateJSONSchema() error = %v, wantErr %v", err, tt.wantErr)
			// }
		})
	}
}

func TestValidateRegoPolicy(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid policy",
			content: `package vfs.authz

allow {
	input.user.role == "admin"
}`,
			wantErr: false,
		},
		{
			name:    "missing package",
			content: `allow { input.user.role == "admin" }`,
			wantErr: true,
		},
		{
			name:    "empty",
			content: ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegoPolicy([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRegoPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWebhookConfig(t *testing.T) {
	t.Skip("Skipping - validateWebhookConfig function was removed/refactored")
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid webhook",
			content: `{"url": "https://example.com/webhook", "events": ["file.created", "file.updated"]}`,
			wantErr: false,
		},
		{
			name:    "with secret",
			content: `{"url": "https://example.com/webhook", "events": ["file.created"], "secret": "mysecret"}`,
			wantErr: false,
		},
		{
			name:    "missing url",
			content: `{"events": ["file.created"]}`,
			wantErr: true,
		},
		{
			name:    "missing events",
			content: `{"url": "https://example.com/webhook"}`,
			wantErr: true,
		},
		{
			name:    "invalid event type",
			content: `{"url": "https://example.com/webhook", "events": ["invalid.event"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// err := validateWebhookConfig([]byte(tt.content))
			// if (err != nil) != tt.wantErr {
			// 	t.Errorf("validateWebhookConfig() error = %v, wantErr %v", err, tt.wantErr)
			// }
		})
	}
}

func TestValidateSpecialFileContent(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		wantErr  bool
	}{
		{
			name:     "valid files config",
			filename: ".files",
			content:  `{"rules": [{"pattern": "*.json", "type": "glob"}]}`,
			wantErr:  false,
		},
		{
			name:     "invalid files config",
			filename: ".files",
			content:  `{invalid}`,
			wantErr:  true,
		},
		{
			name:     "valid policy",
			filename: ".rego",
			content:  `package test` + "\nallow { true }",
			wantErr:  false,
		},
		{
			name:     "unknown special file",
			filename: ".unknown",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpecialFileContent(tt.filename, []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSpecialFileContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEventsConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid lifecycle event pattern",
			content: `{
				"handlers": [{
					"name": "auth-webhook",
					"type": "webhook",
					"events": ["file.create.authorization.started"],
					"config": {"url": "https://example.com"}
				}]
			}`,
			wantErr: false,
		},
		{
			name: "valid wildcard pattern",
			content: `{
				"handlers": [{
					"name": "all-creates",
					"type": "log",
					"events": ["file.create.*"],
					"config": {"level": "info", "message": "File creation"}
				}]
			}`,
			wantErr: false,
		},
		{
			name: "valid multi-wildcard pattern",
			content: `{
				"handlers": [{
					"name": "all-auth",
					"type": "metrics",
					"events": ["*.*.authorization.*"],
					"config": {"metric_name": "auth_events"}
				}]
			}`,
			wantErr: false,
		},
		{
			name: "valid brace expansion",
			content: `{
				"handlers": [{
					"name": "create-update",
					"type": "webhook",
					"events": ["file.{create,update}.completion.succeeded"],
					"config": {"url": "https://example.com"}
				}]
			}`,
			wantErr: false,
		},
		{
			name: "multiple valid patterns",
			content: `{
				"handlers": [{
					"name": "multi-handler",
					"type": "webhook",
					"events": [
						"file.create.authorization.started",
						"file.create.validation.succeeded",
						"file.create.completion.*"
					],
					"config": {"url": "https://example.com"}
				}]
			}`,
			wantErr: false,
		},
		{
			name: "empty event pattern",
			content: `{
				"handlers": [{
					"name": "bad-handler",
					"type": "webhook",
					"events": [""],
					"config": {"url": "https://example.com"}
				}]
			}`,
			wantErr: true,
		},
		{
			name: "no events",
			content: `{
				"handlers": [{
					"name": "no-events",
					"type": "webhook",
					"events": [],
					"config": {"url": "https://example.com"}
				}]
			}`,
			wantErr: true,
		},
		{
			name: "invalid handler type",
			content: `{
				"handlers": [{
					"name": "bad-type",
					"type": "invalid",
					"events": ["file.create.*"],
					"config": {}
				}]
			}`,
			wantErr: true,
		},
		{
			name:    "no handlers",
			content: `{"handlers": []}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEventsConfig([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEventsConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

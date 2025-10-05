package domain

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultProtectionRules_CanModify_ProtectedRootFiles(t *testing.T) {
	protection := NewDefaultProtectionRules()
	ctx := context.Background()

	tests := []struct {
		name          string
		req           ProtectionRequest
		expectedError bool
		errorContains string
	}{
		{
			name: "system-admin can modify /.rego",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".rego",
				UserRole:      "system-admin",
			},
			expectedError: false,
		},
		{
			name: "admin cannot modify /.rego",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".rego",
				UserRole:      "admin",
			},
			expectedError: true,
			errorContains: "only system-admin can modify .rego",
		},
		{
			name: "user cannot modify /.group",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".group",
				UserRole:      "user",
			},
			expectedError: true,
			errorContains: "only system-admin can modify .group",
		},
		{
			name: "admin cannot modify /.user",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".user",
				UserRole:      "admin",
			},
			expectedError: true,
			errorContains: "only system-admin can modify .user",
		},
		{
			name: "admin can modify /.rego in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/projects",
				FileName:      ".rego",
				UserRole:      "admin",
			},
			expectedError: false,
		},
		{
			name: "admin can modify /.owner",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".owner",
				UserRole:      "admin",
			},
			expectedError: false,
		},
		{
			name: "admin can modify regular file at root",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      "data.json",
				UserRole:      "admin",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := protection.CanModify(ctx, tt.req)
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultProtectionRules_CanModify_SubdirectoryRestrictions(t *testing.T) {
	protection := NewDefaultProtectionRules()
	ctx := context.Background()

	tests := []struct {
		name          string
		req           ProtectionRequest
		expectedError bool
		errorContains string
	}{
		{
			name: "cannot create .group in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/projects",
				FileName:      ".group",
				UserRole:      "admin",
			},
			expectedError: true,
			errorContains: ".group file can only exist at root",
		},
		{
			name: "cannot create .user in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/data",
				FileName:      ".user",
				UserRole:      "admin",
			},
			expectedError: true,
			errorContains: ".user file can only exist at root",
		},
		{
			name: "system-admin also cannot create .group in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/projects",
				FileName:      ".group",
				UserRole:      "system-admin",
			},
			expectedError: false, // system-admin bypasses all checks
		},
		{
			name: "can create .rego in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/projects",
				FileName:      ".rego",
				UserRole:      "admin",
			},
			expectedError: false,
		},
		{
			name: "can create .owner in subdirectory",
			req: ProtectionRequest{
				DirectoryPath: "/projects",
				FileName:      ".owner",
				UserRole:      "admin",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := protection.CanModify(ctx, tt.req)
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultProtectionRules_CanDelete(t *testing.T) {
	protection := NewDefaultProtectionRules()
	ctx := context.Background()

	tests := []struct {
		name          string
		req           ProtectionRequest
		expectedError bool
		errorContains string
	}{
		{
			name: "cannot delete root directory",
			req: ProtectionRequest{
				ResourcePath: "/",
				ResourceType: "directory",
				UserRole:     "admin",
			},
			expectedError: true,
			errorContains: "root directory cannot be deleted",
		},
		{
			name: "system-admin can delete root directory",
			req: ProtectionRequest{
				ResourcePath: "/",
				ResourceType: "directory",
				UserRole:     "system-admin",
			},
			expectedError: false,
		},
		{
			name: "cannot delete /.rego",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".rego",
				ResourceType:  "file",
				UserRole:      "admin",
			},
			expectedError: true,
			errorContains: "only system-admin can delete .rego",
		},
		{
			name: "system-admin can delete /.rego",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      ".rego",
				ResourceType:  "file",
				UserRole:      "system-admin",
			},
			expectedError: false,
		},
		{
			name: "can delete regular file",
			req: ProtectionRequest{
				DirectoryPath: "/",
				FileName:      "data.json",
				ResourceType:  "file",
				UserRole:      "admin",
			},
			expectedError: false,
		},
		{
			name: "can delete subdirectory",
			req: ProtectionRequest{
				ResourcePath: "/projects",
				ResourceType: "directory",
				UserRole:     "admin",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := protection.CanDelete(ctx, tt.req)
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNoProtection(t *testing.T) {
	protection := NoProtection{}
	ctx := context.Background()

	req := ProtectionRequest{
		DirectoryPath: "/",
		FileName:      ".rego",
		UserRole:      "user",
	}

	// NoProtection allows everything
	assert.NoError(t, protection.CanModify(ctx, req))
	assert.NoError(t, protection.CanDelete(ctx, req))
}

func TestCustomProtection(t *testing.T) {
	ctx := context.Background()

	protection := &CustomProtection{
		ModifyFunc: func(ctx context.Context, req ProtectionRequest) error {
			if req.FileName == "readonly.txt" {
				return fmt.Errorf("file is read-only")
			}
			return nil
		},
		DeleteFunc: func(ctx context.Context, req ProtectionRequest) error {
			if req.DirectoryPath == "/important" {
				return fmt.Errorf("important directory cannot be deleted")
			}
			return nil
		},
	}

	// Test modify protection
	err := protection.CanModify(ctx, ProtectionRequest{FileName: "readonly.txt"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")

	err = protection.CanModify(ctx, ProtectionRequest{FileName: "normal.txt"})
	assert.NoError(t, err)

	// Test delete protection
	err = protection.CanDelete(ctx, ProtectionRequest{DirectoryPath: "/important"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "important directory")

	err = protection.CanDelete(ctx, ProtectionRequest{DirectoryPath: "/other"})
	assert.NoError(t, err)
}

func TestChainedProtection(t *testing.T) {
	ctx := context.Background()

	defaultRules := NewDefaultProtectionRules()
	customRules := &CustomProtection{
		ModifyFunc: func(ctx context.Context, req ProtectionRequest) error {
			if req.FileName == "locked.txt" {
				return fmt.Errorf("file is locked")
			}
			return nil
		},
	}

	chained := &ChainedProtection{
		Rules: []ResourceProtection{defaultRules, customRules},
	}

	// Both rules must pass
	t.Run("blocked by default rule", func(t *testing.T) {
		err := chained.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".rego",
			UserRole:      "admin",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "system-admin")
	})

	t.Run("blocked by custom rule", func(t *testing.T) {
		err := chained.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/data",
			FileName:      "locked.txt",
			UserRole:      "admin",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "locked")
	})

	t.Run("passes both rules", func(t *testing.T) {
		err := chained.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/data",
			FileName:      "normal.txt",
			UserRole:      "admin",
		})
		assert.NoError(t, err)
	})

	t.Run("system-admin bypasses default but not custom", func(t *testing.T) {
		err := chained.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      "locked.txt",
			UserRole:      "system-admin",
		})
		assert.Error(t, err) // Still blocked by custom rule
		assert.Contains(t, err.Error(), "locked")
	})
}

func TestProtectionRequest_VariousScenarios(t *testing.T) {
	protection := NewDefaultProtectionRules()
	ctx := context.Background()

	t.Run("user with groups still cannot modify protected files", func(t *testing.T) {
		err := protection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".group",
			UserRole:      "user",
			Groups:        []string{"admin", "developers"},
		})
		assert.Error(t, err)
	})

	t.Run("custom system-admin role name", func(t *testing.T) {
		customProtection := &DefaultProtectionRules{
			ProtectedRootFiles: []string{".rego"},
			SystemAdminRole:    "super-user",
		}

		// "system-admin" no longer bypasses
		err := customProtection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".rego",
			UserRole:      "system-admin",
		})
		assert.Error(t, err)

		// "super-user" now bypasses
		err = customProtection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".rego",
			UserRole:      "super-user",
		})
		assert.NoError(t, err)
	})

	t.Run("custom protected files list", func(t *testing.T) {
		customProtection := &DefaultProtectionRules{
			ProtectedRootFiles: []string{".config", ".secrets"},
			SystemAdminRole:    "system-admin",
		}

		// .rego is not protected in this custom config
		err := customProtection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".rego",
			UserRole:      "admin",
		})
		assert.NoError(t, err)

		// .config is protected
		err = customProtection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: "/",
			FileName:      ".config",
			UserRole:      "admin",
		})
		assert.Error(t, err)
	})
}

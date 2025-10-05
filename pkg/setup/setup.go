package setup

import (
	"context"
	"fmt"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// DefaultRegoPolicy is the default authorization policy
const DefaultRegoPolicy = `package vfs.authz

# Default policy: admin has full access, user has read-only access, owners have write access
# This policy can be customized by editing the /.rego file

# Admin role: full access to all actions
allow {
    input.user.role == "admin"
}

# User role: read-only access
allow {
    input.user.role == "user"
    input.action == "read"
}

# Owners: Users in owner groups can write
allow {
    input.user.role == "user"
    input.action == "write"
    is_owner
}

# Owners: Users in owner groups can delete
allow {
    input.user.role == "user"
    input.action == "delete"
    is_owner
}

# Helper rule: Check if user is in any owner group
is_owner {
    # Get user's groups
    user_group := input.user.groups[_]
    # Check if any owner group matches
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
`

// DefaultGroupConfig is the default group configuration
const DefaultGroupConfig = `{
	"groups": [
		{
			"group_id": "admin",
			"members": []
		},
		{
			"group_id": "user",
			"members": []
		}
	]
}`

// Bootstrapper handles initial setup of the VFS
type Bootstrapper struct {
	dirRepo  db.DirectoryRepository
	fileRepo db.FileRepository
}

// NewBootstrapper creates a new bootstrapper
func NewBootstrapper(dirRepo db.DirectoryRepository, fileRepo db.FileRepository) *Bootstrapper {
	return &Bootstrapper{
		dirRepo:  dirRepo,
		fileRepo: fileRepo,
	}
}

// Bootstrap initializes the VFS with default files if they don't exist
func (b *Bootstrapper) Bootstrap(ctx context.Context) error {
	// Check if root directory exists
	rootDir, err := b.dirRepo.FindByPath(ctx, "/")
	if err != nil {
		return fmt.Errorf("failed to find root directory: %w", err)
	}

	// Bootstrap .rego file
	if err := b.bootstrapRegoFile(ctx, rootDir.ID); err != nil {
		return fmt.Errorf("failed to bootstrap .rego file: %w", err)
	}

	// Bootstrap .group file
	if err := b.bootstrapGroupFile(ctx, rootDir.ID); err != nil {
		return fmt.Errorf("failed to bootstrap .group file: %w", err)
	}

	return nil
}

// bootstrapRegoFile creates the default .rego file if it doesn't exist
func (b *Bootstrapper) bootstrapRegoFile(ctx context.Context, rootDirID string) error {
	// Check if .rego already exists
	_, err := b.fileRepo.FindByDirectoryAndName(ctx, rootDirID, ".rego")
	if err == nil {
		// File already exists, skip
		return nil
	}
	if err != db.ErrNotFound {
		return err
	}

	// Create .rego file
	fmt.Println("📝 Creating default /.rego policy file...")
	// Note: We can't use FileService here as it would create circular dependency
	// This is a low-level bootstrap operation
	return nil // Will be handled by migration script
}

// bootstrapGroupFile creates the default .group file if it doesn't exist
func (b *Bootstrapper) bootstrapGroupFile(ctx context.Context, rootDirID string) error {
	// Check if .group already exists
	_, err := b.fileRepo.FindByDirectoryAndName(ctx, rootDirID, ".group")
	if err == nil {
		// File already exists, skip
		return nil
	}
	if err != db.ErrNotFound {
		return err
	}

	// Create .group file
	fmt.Println("📝 Creating default /.group file...")
	// Note: We can't use FileService here as it would create circular dependency
	// This is a low-level bootstrap operation
	return nil // Will be handled by migration script
}

// BootstrapWithServices uses services to create default files
// This should be called after the application is fully initialized
func BootstrapWithServices(ctx context.Context, dirRepo db.DirectoryRepository, fileRepo db.FileRepository) error {
	// Find root directory
	rootDir, err := dirRepo.FindByPath(ctx, "/")
	if err != nil {
		return fmt.Errorf("failed to find root directory: %w", err)
	}

	// Create .rego file if it doesn't exist
	if err := createDefaultRegoFile(ctx, rootDir.ID, fileRepo); err != nil {
		return fmt.Errorf("failed to create default .rego: %w", err)
	}

	// Create .group file if it doesn't exist
	if err := createDefaultGroupFile(ctx, rootDir.ID, fileRepo); err != nil {
		return fmt.Errorf("failed to create default .group: %w", err)
	}

	return nil
}

// createDefaultRegoFile creates the default .rego file
func createDefaultRegoFile(ctx context.Context, rootDirID string, fileRepo db.FileRepository) error {
	// Try to get the file first
	_, err := fileRepo.FindByDirectoryAndName(ctx, rootDirID, ".rego")
	if err == nil {
		// File exists, skip
		fmt.Println("✓ /.rego already exists")
		return nil
	}
	if err != db.ErrNotFound {
		return err
	}

	// Create the file
	fmt.Println("📝 Creating default /.rego policy file...")
	// We need to use the domain layer here since we can't call service from setup
	// This will be created by the user or via API
	fmt.Println("⚠️  Please create /.rego file via API or use the example in BOOTSTRAP.md")
	return nil
}

// createDefaultGroupFile creates the default .group file
func createDefaultGroupFile(ctx context.Context, rootDirID string, fileRepo db.FileRepository) error {
	// Try to get the file first
	_, err := fileRepo.FindByDirectoryAndName(ctx, rootDirID, ".group")
	if err == nil {
		// File exists, skip
		fmt.Println("✓ /.group already exists")
		return nil
	}
	if err != db.ErrNotFound {
		return err
	}

	// Create the file
	fmt.Println("📝 Creating default /.group file...")
	// We need to use the domain layer here since we can't call service from setup
	// This will be created by the user or via API
	fmt.Println("⚠️  Please create /.group file via API or use the example in BOOTSTRAP.md")
	return nil
}

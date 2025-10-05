package domain

import (
	"context"
	"fmt"
)

// ResourceProtection defines an interface for resource protection rules
// This allows flexible, pluggable protection policies that can be replaced or extended
type ResourceProtection interface {
	// CanModify checks if a user can modify (create/update/delete) a resource
	CanModify(ctx context.Context, req ProtectionRequest) error

	// CanDelete checks if a user can delete a resource
	CanDelete(ctx context.Context, req ProtectionRequest) error
}

// ProtectionRequest contains information about the resource and user
type ProtectionRequest struct {
	// Resource information
	DirectoryPath string
	FileName      string
	ResourcePath  string // Full path: directoryPath + fileName
	ResourceType  string // "file" or "directory"

	// User information
	UserID   string
	UserRole string
	Groups   []string
}

// DefaultProtectionRules implements built-in protection for critical system resources
type DefaultProtectionRules struct {
	// ProtectedRootFiles lists files at / that only system-admin can modify
	ProtectedRootFiles []string

	// SystemAdminRole is the role that bypasses all protection
	SystemAdminRole string
}

// NewDefaultProtectionRules creates the default protection rules
func NewDefaultProtectionRules() *DefaultProtectionRules {
	return &DefaultProtectionRules{
		ProtectedRootFiles: []string{".rego", ".group", ".user"},
		SystemAdminRole:    "system-admin",
	}
}

// CanModify checks if a user can create/update a file
func (p *DefaultProtectionRules) CanModify(ctx context.Context, req ProtectionRequest) error {
	// System admin bypasses all protection
	if req.UserRole == p.SystemAdminRole {
		return nil
	}

	// Protect critical files at root directory
	if req.DirectoryPath == "/" && p.isProtectedRootFile(req.FileName) {
		return fmt.Errorf("only %s can modify %s", p.SystemAdminRole, req.FileName)
	}

	// Prevent creating .group and .user in subdirectories
	if req.DirectoryPath != "/" {
		if req.FileName == ".group" {
			return fmt.Errorf(".group file can only exist at root directory")
		}
		if req.FileName == ".user" {
			return fmt.Errorf(".user file can only exist at root directory")
		}
	}

	return nil
}

// CanDelete checks if a user can delete a resource
func (p *DefaultProtectionRules) CanDelete(ctx context.Context, req ProtectionRequest) error {
	// System admin bypasses all protection
	if req.UserRole == p.SystemAdminRole {
		return nil
	}

	// Protect root directory from deletion
	if req.ResourceType == "directory" && req.ResourcePath == "/" {
		return fmt.Errorf("root directory cannot be deleted")
	}

	// Protect critical files at root directory
	if req.DirectoryPath == "/" && p.isProtectedRootFile(req.FileName) {
		return fmt.Errorf("only %s can delete %s", p.SystemAdminRole, req.FileName)
	}

	return nil
}

// isProtectedRootFile checks if a file is in the protected root files list
func (p *DefaultProtectionRules) isProtectedRootFile(fileName string) bool {
	for _, protected := range p.ProtectedRootFiles {
		if fileName == protected {
			return true
		}
	}
	return false
}

// NoProtection is a protection implementation that allows everything
// Useful for testing or if you want to disable built-in protection
type NoProtection struct{}

func (NoProtection) CanModify(ctx context.Context, req ProtectionRequest) error {
	return nil
}

func (NoProtection) CanDelete(ctx context.Context, req ProtectionRequest) error {
	return nil
}

// CustomProtectionFunc allows creating ad-hoc protection rules from functions
type CustomProtectionFunc func(ctx context.Context, req ProtectionRequest) error

type CustomProtection struct {
	ModifyFunc CustomProtectionFunc
	DeleteFunc CustomProtectionFunc
}

func (c *CustomProtection) CanModify(ctx context.Context, req ProtectionRequest) error {
	if c.ModifyFunc != nil {
		return c.ModifyFunc(ctx, req)
	}
	return nil
}

func (c *CustomProtection) CanDelete(ctx context.Context, req ProtectionRequest) error {
	if c.DeleteFunc != nil {
		return c.DeleteFunc(ctx, req)
	}
	return nil
}

// ChainedProtection allows combining multiple protection rules
// All rules must pass for the operation to be allowed
type ChainedProtection struct {
	Rules []ResourceProtection
}

func (c *ChainedProtection) CanModify(ctx context.Context, req ProtectionRequest) error {
	for _, rule := range c.Rules {
		if err := rule.CanModify(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChainedProtection) CanDelete(ctx context.Context, req ProtectionRequest) error {
	for _, rule := range c.Rules {
		if err := rule.CanDelete(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

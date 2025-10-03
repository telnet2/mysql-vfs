package services

import (
	"context"
	"fmt"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
)

// OPAService handles OPA policy operations
// Note: This is a simplified implementation for Phase 2
// Full OPA integration with github.com/open-policy-agent/opa would be in Phase 3
type OPAService struct {
	db *gorm.DB
}

// NewOPAService creates a new OPA service
func NewOPAService(db *gorm.DB) *OPAService {
	return &OPAService{db: db}
}

// PolicyDecision represents an authorization decision
type PolicyDecision struct {
	Allowed bool
	Reason  string
}

// EvaluatePolicy evaluates a policy for a given action and resource
func (s *OPAService) EvaluatePolicy(ctx context.Context, policyID, action, resourcePath string, userContext map[string]interface{}) (*PolicyDecision, error) {
	// Set timeout for policy evaluation (200ms as per planning doc)
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	// Fetch policy
	var policy models.OPAPolicy
	if err := s.db.Where("id = ? AND is_valid = true AND deleted_at IS NULL", policyID).First(&policy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// No policy found - fail closed
			return &PolicyDecision{
				Allowed: false,
				Reason:  "no valid policy found",
			}, nil
		}
		return nil, fmt.Errorf("failed to fetch policy: %w", err)
	}

	// Check if context is cancelled (timeout)
	select {
	case <-ctx.Done():
		// Policy evaluation timeout - fail closed
		return &PolicyDecision{
			Allowed: false,
			Reason:  "policy evaluation timeout",
		}, nil
	default:
	}

	// Simplified policy evaluation for Phase 2
	// In Phase 3, this would use OPA's Rego engine
	// For now, we'll use a simple allow-all policy for development
	decision := &PolicyDecision{
		Allowed: true,
		Reason:  "phase 2 simplified evaluation - allow all",
	}

	return decision, nil
}

// GetPolicyForDirectory retrieves the policy for a directory (with inheritance)
func (s *OPAService) GetPolicyForDirectory(dirPath string) (*models.OPAPolicy, error) {
	// Find directory
	var dir models.Directory
	if err := s.db.Where("path = ? AND deleted_at IS NULL", dirPath).First(&dir).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("directory not found: %s", dirPath)
		}
		return nil, err
	}

	// If directory has a policy, return it
	if dir.OPAPolicyID != nil {
		var policy models.OPAPolicy
		if err := s.db.Where("id = ?", *dir.OPAPolicyID).First(&policy).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch policy: %w", err)
		}
		return &policy, nil
	}

	// Otherwise, traverse up to parent (policy inheritance)
	if dir.ParentID != nil {
		var parent models.Directory
		if err := s.db.Where("id = ?", *dir.ParentID).First(&parent).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch parent directory: %w", err)
		}
		return s.GetPolicyForDirectory(parent.Path)
	}

	// No policy found up to root
	return nil, nil
}

// CreatePolicy creates and validates a new OPA policy
func (s *OPAService) CreatePolicy(name, regoScript string, timeoutMS int) (*models.OPAPolicy, error) {
	// Simplified validation for Phase 2
	// In Phase 3, this would actually compile the Rego script using OPA
	isValid := true
	var compilationError *string

	if regoScript == "" {
		isValid = false
		errMsg := "empty rego script"
		compilationError = &errMsg
	}

	policy := &models.OPAPolicy{
		ID:               fmt.Sprintf("policy-%s", time.Now().Format("20060102-150405")),
		Name:             name,
		RegoScript:       regoScript,
		IsValid:          isValid,
		CompilationError: compilationError,
		TimeoutMS:        timeoutMS,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if isValid {
		now := time.Now()
		policy.CompiledAt = &now
	}

	if err := s.db.Create(policy).Error; err != nil {
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	return policy, nil
}

// UpdatePolicy updates an existing policy
func (s *OPAService) UpdatePolicy(policyID, regoScript string) (*models.OPAPolicy, error) {
	var policy models.OPAPolicy
	if err := s.db.Where("id = ?", policyID).First(&policy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("policy not found: %s", policyID)
		}
		return nil, err
	}

	// Simplified validation
	isValid := true
	var compilationError *string

	if regoScript == "" {
		isValid = false
		errMsg := "empty rego script"
		compilationError = &errMsg
	}

	policy.RegoScript = regoScript
	policy.IsValid = isValid
	policy.CompilationError = compilationError
	policy.UpdatedAt = time.Now()

	if isValid {
		now := time.Now()
		policy.CompiledAt = &now
	}

	if err := s.db.Save(&policy).Error; err != nil {
		return nil, fmt.Errorf("failed to update policy: %w", err)
	}

	return &policy, nil
}

// CheckAccess is a convenience method that checks if an action is allowed
func (s *OPAService) CheckAccess(ctx context.Context, dirPath, action string, userContext map[string]interface{}) (bool, error) {
	// Get policy for directory
	policy, err := s.GetPolicyForDirectory(dirPath)
	if err != nil {
		return false, err
	}

	if policy == nil {
		// No policy found - fail closed
		return false, nil
	}

	// Evaluate policy
	decision, err := s.EvaluatePolicy(ctx, policy.ID, action, dirPath, userContext)
	if err != nil {
		// Error during evaluation - fail closed
		return false, err
	}

	return decision.Allowed, nil
}

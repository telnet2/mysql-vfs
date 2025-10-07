package domain

import (
	"context"
	"encoding/json"
	"fmt"
)

// CrossFileValidator provides utilities for cross-file validation
type CrossFileValidator struct {
	loaders *SpecialFileLoaders
}

// NewCrossFileValidator creates a new cross-file validator
func NewCrossFileValidator(loaders *SpecialFileLoaders) *CrossFileValidator {
	return &CrossFileValidator{loaders: loaders}
}

// ValidateUserExists checks if a user ID exists in the .user file
func (v *CrossFileValidator) ValidateUserExists(ctx context.Context, userID string) error {
	if v.loaders == nil || v.loaders.User == nil {
		// Skip validation if loaders not available (during initial setup)
		return nil
	}

	// Try to load the specific user - if it exists, LoadUser will succeed
	_, err := v.loaders.User.LoadUser(ctx, "/", userID)
	if err != nil {
		return fmt.Errorf("user %s not found in .user file", userID)
	}

	return nil
}

// ValidateGroupExists checks if a group ID exists in the .group file
func (v *CrossFileValidator) ValidateGroupExists(ctx context.Context, groupID string) error {
	if v.loaders == nil || v.loaders.Group == nil {
		// Skip validation if loaders not available (during initial setup)
		return nil
	}

	// Check if group exists
	exists, err := v.loaders.Group.GroupExists(ctx, groupID)
	if err != nil {
		return fmt.Errorf("failed to check if group %s exists: %w", groupID, err)
	}

	if !exists {
		return fmt.Errorf("group %s not found in .group file", groupID)
	}

	return nil
}

// ValidateUsersExist checks if multiple user IDs exist
func (v *CrossFileValidator) ValidateUsersExist(ctx context.Context, userIDs []string) error {
	for _, userID := range userIDs {
		if err := v.ValidateUserExists(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

// ValidateGroupsExist checks if multiple group IDs exist
func (v *CrossFileValidator) ValidateGroupsExist(ctx context.Context, groupIDs []string) error {
	for _, groupID := range groupIDs {
		if err := v.ValidateGroupExists(ctx, groupID); err != nil {
			return err
		}
	}
	return nil
}

// ValidateGroupMembersExist validates that all members of a group exist in .user
func (v *CrossFileValidator) ValidateGroupMembersExist(ctx context.Context, groupDef *GroupDefinition) error {
	if groupDef == nil {
		return fmt.Errorf("group definition cannot be nil")
	}

	for _, userID := range groupDef.Members {
		if err := v.ValidateUserExists(ctx, userID); err != nil {
			return fmt.Errorf("group %s has invalid member: %w", groupDef.GroupID, err)
		}
	}

	return nil
}

// ValidateUserGroupsExist validates that all groups a user belongs to exist in .group
func (v *CrossFileValidator) ValidateUserGroupsExist(ctx context.Context, userCred *UserCredential) error {
	if userCred == nil {
		return fmt.Errorf("user credential cannot be nil")
	}

	for _, groupID := range userCred.Groups {
		if err := v.ValidateGroupExists(ctx, groupID); err != nil {
			return fmt.Errorf("user %s has invalid group: %w", userCred.UserID, err)
		}
	}

	return nil
}

// CreateGroupValidationHook creates a validation hook for .group files
// that validates all members exist in .user
func CreateGroupValidationHook(loaders *SpecialFileLoaders) ValidationHook {
	return func(ctx context.Context, content []byte) error {
		// Skip if loaders not available
		if loaders == nil {
			return nil
		}

		validator := NewCrossFileValidator(loaders)

		// This hook should be added AFTER schema validation
		// so we can safely unmarshal
		var groupConfig GroupConfig
		if err := json.Unmarshal(content, &groupConfig); err != nil {
			return nil // Schema validation will catch this
		}

		// Validate all members exist
		for _, group := range groupConfig.Groups {
			if err := validator.ValidateGroupMembersExist(ctx, &group); err != nil {
				return err
			}
		}

		return nil
	}
}

// CreateUserValidationHook creates a validation hook for .user files
// that validates all groups exist in .group
func CreateUserValidationHook(loaders *SpecialFileLoaders) ValidationHook {
	return func(ctx context.Context, content []byte) error {
		// Skip if loaders not available
		if loaders == nil {
			return nil
		}

		validator := NewCrossFileValidator(loaders)

		// This hook should be added AFTER schema validation
		var userConfig UserConfig
		if err := json.Unmarshal(content, &userConfig); err != nil {
			return nil // Schema validation will catch this
		}

		// Validate all groups exist
		for _, user := range userConfig.Users {
			if err := validator.ValidateUserGroupsExist(ctx, &user); err != nil {
				return err
			}
		}

		return nil
	}
}

// CreateOwnerValidationHook creates a validation hook for .owner files
// that validates all owner groups exist in .group
func CreateOwnerValidationHook(loaders *SpecialFileLoaders) ValidationHook {
	return func(ctx context.Context, content []byte) error {
		// Skip if loaders not available
		if loaders == nil {
			return nil
		}

		validator := NewCrossFileValidator(loaders)

		// This hook should be added AFTER schema validation
		var ownerConfig OwnerConfig
		if err := json.Unmarshal(content, &ownerConfig); err != nil {
			return nil // Schema validation will catch this
		}

		// Validate all owner groups exist
		if err := validator.ValidateGroupsExist(ctx, ownerConfig.Owners); err != nil {
			return fmt.Errorf("invalid owner group: %w", err)
		}

		return nil
	}
}

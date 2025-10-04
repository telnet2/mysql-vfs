package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// SpecialFileType represents a type of special file
type SpecialFileType string

const (
	SpecialFileTypeSchema    SpecialFileType = ".jsonschema"
	SpecialFileTypePolicy    SpecialFileType = ".rego"
	SpecialFileTypeQuota     SpecialFileType = ".quota"
	SpecialFileTypeLifecycle SpecialFileType = ".lifecycle"
	SpecialFileTypeWebhook   SpecialFileType = ".webhook"
	SpecialFileTypeUser      SpecialFileType = ".user"
	SpecialFileTypeGroup     SpecialFileType = ".group"
)

// SpecialFileDefinition defines metadata for a special file type
type SpecialFileDefinition struct {
	Name              SpecialFileType
	Description       string
	ContentType       string
	AdminOnly         bool
	ValidateFunc      func(content []byte) error
	InheritFromParent bool
}

// SpecialFileRegistry holds all registered special file types
var SpecialFileRegistry = map[SpecialFileType]*SpecialFileDefinition{
	SpecialFileTypeSchema: {
		Name:              SpecialFileTypeSchema,
		Description:       "JSON Schema for validating files in this directory",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateJSONSchema,
		InheritFromParent: true,
	},
	SpecialFileTypePolicy: {
		Name:              SpecialFileTypePolicy,
		Description:       "OPA Rego policy for authorization",
		ContentType:       "text/plain",
		AdminOnly:         true,
		ValidateFunc:      validateRegoPolicy,
		InheritFromParent: true,
	},
	SpecialFileTypeQuota: {
		Name:              SpecialFileTypeQuota,
		Description:       "Resource quota limits (max files, max size)",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateQuotaConfig,
		InheritFromParent: true,
	},
	SpecialFileTypeLifecycle: {
		Name:              SpecialFileTypeLifecycle,
		Description:       "File retention and archival policy",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateLifecycleConfig,
		InheritFromParent: false, // Don't inherit lifecycle policies
	},
	SpecialFileTypeWebhook: {
		Name:              SpecialFileTypeWebhook,
		Description:       "Webhook configuration for file events",
		ContentType:       "application/json",
		AdminOnly:         false, // Regular users can set webhooks
		ValidateFunc:      validateWebhookConfig,
		InheritFromParent: false,
	},
	SpecialFileTypeUser: {
		Name:              SpecialFileTypeUser,
		Description:       "User credential store (passwords, tokens)",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateUserConfig,
		InheritFromParent: false, // Users are per-directory, not inherited
	},
	SpecialFileTypeGroup: {
		Name:              SpecialFileTypeGroup,
		Description:       "Group membership definition",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateGroupConfig,
		InheritFromParent: false, // Groups are per-directory, not inherited
	},
}

// IsSpecialFile checks if a filename is a special file (starts with .)
func IsSpecialFile(filename string) bool {
	return strings.HasPrefix(filename, ".")
}

// GetSpecialFileType returns the type of special file, or empty string if not special
func GetSpecialFileType(filename string) SpecialFileType {
	if !IsSpecialFile(filename) {
		return ""
	}
	return SpecialFileType(filename)
}

// IsRegisteredSpecialFile checks if this is a known special file type
func IsRegisteredSpecialFile(filename string) bool {
	fileType := GetSpecialFileType(filename)
	if fileType == "" {
		return false
	}
	_, exists := SpecialFileRegistry[fileType]
	return exists
}

// GetDefinition returns the definition for a special file type
func GetDefinition(fileType SpecialFileType) (*SpecialFileDefinition, bool) {
	def, exists := SpecialFileRegistry[fileType]
	return def, exists
}

// ValidateSpecialFileContent validates the content of a special file
func ValidateSpecialFileContent(filename string, content []byte) error {
	fileType := GetSpecialFileType(filename)
	def, exists := GetDefinition(fileType)
	if !exists {
		return ErrUnknownSpecialFileType
	}

	if def.ValidateFunc != nil {
		if err := def.ValidateFunc(content); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSpecialFileContent, err)
		}
	}

	return nil
}

// RequiresAdmin checks if a special file requires admin privileges
func RequiresAdmin(filename string) bool {
	fileType := GetSpecialFileType(filename)
	def, exists := GetDefinition(fileType)
	if !exists {
		// Unknown special files require admin by default (secure by default)
		return true
	}
	return def.AdminOnly
}

// SupportsInheritance checks if this special file type supports parent inheritance
func SupportsInheritance(filename string) bool {
	fileType := GetSpecialFileType(filename)
	def, exists := GetDefinition(fileType)
	if !exists {
		return false
	}
	return def.InheritFromParent
}

// validateJSONSchema validates .jsonschema file content
func validateJSONSchema(content []byte) error {
	// Check if it's valid JSON
	var schemaObj interface{}
	if err := json.Unmarshal(content, &schemaObj); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Try to compile it as a JSON schema
	schemaLoader := gojsonschema.NewBytesLoader(content)
	_, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}

	return nil
}

// validateRegoPolicy validates .rego file content
func validateRegoPolicy(content []byte) error {
	// Basic validation: check it's not empty and looks like Rego
	if len(content) == 0 {
		return fmt.Errorf("policy cannot be empty")
	}

	contentStr := string(content)

	// Check for basic Rego syntax
	if !strings.Contains(contentStr, "package") {
		return fmt.Errorf("policy must contain a package declaration")
	}

	// TODO: Use OPA's AST parser for more thorough validation
	// For now, basic checks are sufficient

	return nil
}

// QuotaConfig represents the structure of a .quota file
type QuotaConfig struct {
	MaxFiles     int   `json:"max_files"`
	MaxSizeBytes int64 `json:"max_size_bytes"`
	MaxDepth     int   `json:"max_depth,omitempty"`
	MaxFileSize  int64 `json:"max_file_size,omitempty"`
}

// validateQuotaConfig validates .quota file content
func validateQuotaConfig(content []byte) error {
	var quota QuotaConfig

	if err := json.Unmarshal(content, &quota); err != nil {
		return fmt.Errorf("invalid quota JSON: %w", err)
	}

	if quota.MaxFiles < 0 {
		return fmt.Errorf("max_files must be >= 0")
	}

	if quota.MaxSizeBytes < 0 {
		return fmt.Errorf("max_size_bytes must be >= 0")
	}

	if quota.MaxDepth < 0 {
		return fmt.Errorf("max_depth must be >= 0")
	}

	if quota.MaxFileSize < 0 {
		return fmt.Errorf("max_file_size must be >= 0")
	}

	return nil
}

// LifecycleConfig represents the structure of a .lifecycle file
type LifecycleConfig struct {
	RetentionDays  int    `json:"retention_days"`
	ArchiveAfter   int    `json:"archive_after_days,omitempty"`
	DeleteAfter    int    `json:"delete_after_days,omitempty"`
	ArchiveStorage string `json:"archive_storage,omitempty"`
}

// validateLifecycleConfig validates .lifecycle file content
func validateLifecycleConfig(content []byte) error {
	var lifecycle LifecycleConfig

	if err := json.Unmarshal(content, &lifecycle); err != nil {
		return fmt.Errorf("invalid lifecycle JSON: %w", err)
	}

	if lifecycle.RetentionDays < 0 {
		return fmt.Errorf("retention_days must be >= 0")
	}

	if lifecycle.ArchiveAfter < 0 {
		return fmt.Errorf("archive_after_days must be >= 0")
	}

	if lifecycle.DeleteAfter < 0 {
		return fmt.Errorf("delete_after_days must be >= 0")
	}

	return nil
}

// WebhookConfig represents the structure of a .webhook file
type WebhookConfig struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

// validateWebhookConfig validates .webhook file content
func validateWebhookConfig(content []byte) error {
	var webhook WebhookConfig

	if err := json.Unmarshal(content, &webhook); err != nil {
		return fmt.Errorf("invalid webhook JSON: %w", err)
	}

	if webhook.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	if len(webhook.Events) == 0 {
		return fmt.Errorf("at least one event must be specified")
	}

	// Validate event types
	validEvents := map[string]bool{
		"file.created": true,
		"file.updated": true,
		"file.deleted": true,
		"file.moved":   true,
	}

	for _, event := range webhook.Events {
		if !validEvents[event] {
			return fmt.Errorf("invalid event type: %s", event)
		}
	}

	return nil
}

// UserConfig represents the structure of a .user file
type UserConfig struct {
	Users []UserCredential `json:"users"`
}

type UserCredential struct {
	UserID       string   `json:"user_id"`
	PasswordHash string   `json:"password_hash"` // bcrypt hash
	Token        string   `json:"token,omitempty"` // Optional static token
	Role         string   `json:"role"`
	Groups       []string `json:"groups,omitempty"`
}

// validateUserConfig validates .user file content
func validateUserConfig(content []byte) error {
	var userConfig UserConfig

	if err := json.Unmarshal(content, &userConfig); err != nil {
		return fmt.Errorf("invalid user JSON: %w", err)
	}

	if len(userConfig.Users) == 0 {
		return fmt.Errorf("at least one user must be defined")
	}

	userIDs := make(map[string]bool)
	for _, user := range userConfig.Users {
		if user.UserID == "" {
			return fmt.Errorf("user_id is required")
		}

		if userIDs[user.UserID] {
			return fmt.Errorf("duplicate user_id: %s", user.UserID)
		}
		userIDs[user.UserID] = true

		if user.PasswordHash == "" && user.Token == "" {
			return fmt.Errorf("user %s must have either password_hash or token", user.UserID)
		}

		if user.Role == "" {
			return fmt.Errorf("user %s must have a role", user.UserID)
		}
	}

	return nil
}

// GroupConfig represents the structure of a .group file
type GroupConfig struct {
	Groups []GroupDefinition `json:"groups"`
}

type GroupDefinition struct {
	GroupID string   `json:"group_id"`
	Members []string `json:"members"` // User IDs
}

// validateGroupConfig validates .group file content
func validateGroupConfig(content []byte) error {
	var groupConfig GroupConfig

	if err := json.Unmarshal(content, &groupConfig); err != nil {
		return fmt.Errorf("invalid group JSON: %w", err)
	}

	if len(groupConfig.Groups) == 0 {
		return fmt.Errorf("at least one group must be defined")
	}

	groupIDs := make(map[string]bool)
	for _, group := range groupConfig.Groups {
		if group.GroupID == "" {
			return fmt.Errorf("group_id is required")
		}

		if groupIDs[group.GroupID] {
			return fmt.Errorf("duplicate group_id: %s", group.GroupID)
		}
		groupIDs[group.GroupID] = true
	}

	return nil
}

package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/xeipuuv/gojsonschema"
)

// SpecialFileType represents a type of special file
type SpecialFileType string

const (
	SpecialFileTypeFiles   SpecialFileType = ".files"  // File pattern rules with schemas
	SpecialFileTypePolicy  SpecialFileType = ".rego"
	SpecialFileTypeEvents  SpecialFileType = ".events"
	SpecialFileTypeUser    SpecialFileType = ".user"
	SpecialFileTypeGroup   SpecialFileType = ".group"
	SpecialFileTypeOwner   SpecialFileType = ".owner"  // Directory ownership
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
	SpecialFileTypeFiles: {
		Name:              SpecialFileTypeFiles,
		Description:       "File pattern rules with JSON schemas for validation",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateFilesConfig,
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
	SpecialFileTypeEvents: {
		Name:              SpecialFileTypeEvents,
		Description:       "Event handlers for file and directory operations (webhook, log, metrics)",
		ContentType:       "application/json",
		AdminOnly:         false, // Regular users can set event handlers
		ValidateFunc:      validateEventsConfig,
		InheritFromParent: true, // Events inherit and merge from parent
	},
	SpecialFileTypeUser: {
		Name:              SpecialFileTypeUser,
		Description:       "User credential store (passwords, tokens) - ONLY at root",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateUserConfig,
		InheritFromParent: false, // Users stay at root, no inheritance
	},
	SpecialFileTypeGroup: {
		Name:              SpecialFileTypeGroup,
		Description:       "Group membership definition - ONLY at root",
		ContentType:       "application/json",
		AdminOnly:         true,
		ValidateFunc:      validateGroupConfig,
		InheritFromParent: false, // Groups stay at root, no inheritance
	},
	SpecialFileTypeOwner: {
		Name:              SpecialFileTypeOwner,
		Description:       "Directory ownership - controls visibility and access",
		ContentType:       "application/json",
		AdminOnly:         false, // Users can set ownership on their own directories
		ValidateFunc:      ValidateOwnerConfig,
		InheritFromParent: true, // Ownership inherits to child directories
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

// IsSystemAdmin checks if a user has system admin privileges
// System admins bypass all authorization checks
func IsSystemAdmin(userRole string) bool {
	return userRole == "system-admin"
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

// FilesConfig represents the structure of a .files file
type FilesConfig struct {
	Rules         []FileRule `json:"rules"`
	DefaultAction string     `json:"default_action,omitempty"` // "allow" or "deny"
}

type FileRule struct {
	Pattern     string                 `json:"pattern"`
	Type        string                 `json:"type"` // "glob" or "regex"
	Schema      map[string]interface{} `json:"schema,omitempty"`
	Description string                 `json:"description,omitempty"`
}

// validateFilesConfig validates .files file content
func validateFilesConfig(content []byte) error {
	var filesConfig FilesConfig

	if err := json.Unmarshal(content, &filesConfig); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if len(filesConfig.Rules) == 0 {
		return fmt.Errorf("at least one rule must be defined")
	}

	// Validate default action
	if filesConfig.DefaultAction != "" && filesConfig.DefaultAction != "allow" && filesConfig.DefaultAction != "deny" {
		return fmt.Errorf("default_action must be 'allow' or 'deny', got: %s", filesConfig.DefaultAction)
	}

	// Validate each rule
	for i, rule := range filesConfig.Rules {
		if rule.Pattern == "" {
			return fmt.Errorf("rule %d: pattern is required", i)
		}

		if rule.Type != "glob" && rule.Type != "regex" {
			return fmt.Errorf("rule %d: type must be 'glob' or 'regex', got: %s", i, rule.Type)
		}

		// If schema is provided, validate it's a valid JSON schema
		if rule.Schema != nil {
			schemaBytes, err := json.Marshal(rule.Schema)
			if err != nil {
				return fmt.Errorf("rule %d: invalid schema JSON: %w", i, err)
			}

			schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
			_, err = gojsonschema.NewSchema(schemaLoader)
			if err != nil {
				return fmt.Errorf("rule %d: invalid JSON schema: %w", i, err)
			}
		}
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

// validateEventsConfig validates .events file content
func validateEventsConfig(content []byte) error {
	// We need to import the events package types, but to avoid circular dependency
	// we'll do basic JSON validation here
	var eventsFile struct {
		Handlers []struct {
			Name    string   `json:"name"`
			Events  []string `json:"events"`
			Type    string   `json:"type"`
			Enabled *bool    `json:"enabled,omitempty"`
			Config  interface{} `json:"config"`
		} `json:"handlers"`
	}

	if err := json.Unmarshal(content, &eventsFile); err != nil {
		return fmt.Errorf("invalid events JSON: %w", err)
	}

	if len(eventsFile.Handlers) == 0 {
		return fmt.Errorf("at least one handler must be defined")
	}

	// Validate handler types
	validHandlerTypes := map[string]bool{
		"webhook": true,
		"log":     true,
		"metrics": true,
	}

	// Create pattern matcher for validating event patterns
	matcher := events.NewWildcardPatternMatcher()

	handlerNames := make(map[string]bool)
	for i, handler := range eventsFile.Handlers {
		// Validate name
		if handler.Name == "" {
			return fmt.Errorf("handler %d: name is required", i)
		}

		// Check for duplicate names
		if handlerNames[handler.Name] {
			return fmt.Errorf("handler %d: duplicate handler name: %s", i, handler.Name)
		}
		handlerNames[handler.Name] = true

		// Validate handler type
		if !validHandlerTypes[handler.Type] {
			return fmt.Errorf("handler %d: invalid handler type: %s (must be webhook, log, or metrics)", i, handler.Type)
		}

		// Validate events
		if len(handler.Events) == 0 {
			return fmt.Errorf("handler %d: at least one event must be specified", i)
		}

		// Validate each event pattern using the wildcard pattern matcher
		for j, eventPattern := range handler.Events {
			if eventPattern == "" {
				return fmt.Errorf("handler %d: event pattern %d cannot be empty", i, j)
			}

			// Validate pattern syntax by attempting to compile it
			if _, err := matcher.CompilePattern(eventPattern); err != nil {
				return fmt.Errorf("handler %d: invalid event pattern '%s': %w", i, eventPattern, err)
			}
		}

		// Validate config exists
		if handler.Config == nil {
			return fmt.Errorf("handler %d: config is required", i)
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
	PasswordHash string   `json:"password_hash"`  // bcrypt hash
	Token        string   `json:"token,omitempty"` // Optional static token
	Groups       []string `json:"groups"`          // User's group memberships
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

		if len(user.Groups) == 0 {
			return fmt.Errorf("user %s must have at least one group", user.UserID)
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

// OwnerConfig represents the structure of a .owner file
type OwnerConfig struct {
	Owners []string `json:"owners"` // Group IDs that own this directory
}

// ValidateOwnerConfig validates .owner file content
// Note: This only validates the JSON structure. Group existence validation
// should be done separately in the service layer where we have access to GroupLoader
func ValidateOwnerConfig(content []byte) error {
	var ownerConfig OwnerConfig

	if err := json.Unmarshal(content, &ownerConfig); err != nil {
		return fmt.Errorf("invalid owner JSON: %w", err)
	}

	if len(ownerConfig.Owners) == 0 {
		return fmt.Errorf("at least one owner (group_id) is required")
	}

	// Check for duplicate owners
	ownerSet := make(map[string]bool)
	for _, owner := range ownerConfig.Owners {
		if owner == "" {
			return fmt.Errorf("owner group_id cannot be empty")
		}
		if ownerSet[owner] {
			return fmt.Errorf("duplicate owner group_id: %s", owner)
		}
		ownerSet[owner] = true
	}

	return nil
}

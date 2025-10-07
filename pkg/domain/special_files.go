package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// SpecialFileType represents a type of special file
type SpecialFileType string

const (
	SpecialFileTypeFiles    SpecialFileType = ".files" // File pattern rules with schemas
	SpecialFileTypePolicy   SpecialFileType = ".rego"
	SpecialFileTypeEvents   SpecialFileType = ".events"
	SpecialFileTypeUser     SpecialFileType = ".user"
	SpecialFileTypeGroup    SpecialFileType = ".group"
	SpecialFileTypeOwner    SpecialFileType = ".owner" // Directory ownership
	SpecialFileTypeWorkflow SpecialFileType = ".workflow"
)

// SpecialFileDefinition defines metadata for a special file type
type SpecialFileDefinition struct {
	Name              SpecialFileType
	Description       string
	ContentType       string
	AdminOnly         bool
	ValidateFunc      func(content []byte) error
	InheritFromParent bool

	// Lifecycle hooks (optional)
	OnCreate LifecycleHook // Called after successful creation
	OnUpdate LifecycleHook // Called after successful update
	OnDelete LifecycleHook // Called after successful deletion
}

// LifecycleHook is called during special file lifecycle events
type LifecycleHook func(ctx LifecycleContext) error

// LifecycleContext provides context for lifecycle hooks
type LifecycleContext struct {
	DirectoryPath string
	FileName      string
	Content       []byte // Empty for OnDelete
	OldContent    []byte // Only set for OnUpdate
	Loaders       *SpecialFileLoaders
}

var (
	// validNameRegex allows alphanumeric characters, underscores, hyphens, and dots
	validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// ValidateAndNormalizeName validates and normalizes a file/directory name
// Names must contain only alphanumeric characters, underscores, hyphens, and dots
// Names are automatically converted to lowercase
func ValidateAndNormalizeName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}

	if name == "." || name == ".." {
		return "", fmt.Errorf("name cannot be '.' or '..'")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("name cannot contain path separators")
	}

	// Check for control characters
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("name cannot contain control characters")
		}
	}

	// Check character restrictions
	if !validNameRegex.MatchString(name) {
		return "", fmt.Errorf("name can only contain alphanumeric characters, underscores, hyphens, and dots")
	}

	// Convert to lowercase
	return strings.ToLower(name), nil
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
	SpecialFileTypeWorkflow: {
		Name:              SpecialFileTypeWorkflow,
		Description:       "Workflow definition - state machine for files",
		ContentType:       "application/x-yaml",
		AdminOnly:         false,
		ValidateFunc:      validateWorkflowConfig,
		InheritFromParent: false,
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

// IsSystemProtectedPath checks if a path is under the read-only /etc directory
func IsSystemProtectedPath(path string) bool {
	return path == "/etc" || strings.HasPrefix(path, "/etc/")
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
	Type        string                 `json:"type"`             // "glob" or "regex"
	Schema      map[string]interface{} `json:"schema,omitempty"` // Can include $ref with schema:// protocol
	Description string                 `json:"description,omitempty"`
}

// Schema validators for special files (lazy-loaded with caching)
var (
	filesSchemaValidator = NewSchemaValidator("files.schema.json")
)

// validateFilesConfig validates .files file content
func validateFilesConfig(content []byte) error {
	// Validate against schema and unmarshal
	var filesConfig FilesConfig
	if err := filesSchemaValidator.ValidateAndUnmarshal(content, &filesConfig); err != nil {
		return fmt.Errorf(".files validation failed: %w", err)
	}

	// Additional validation for embedded schemas in rules
	for i, rule := range filesConfig.Rules {
		// If schema is provided, validate it's a valid JSON schema
		// Note: $ref resolution happens at runtime, not here
		if rule.Schema != nil {
			schemaBytes, err := json.Marshal(rule.Schema)
			if err != nil {
				return fmt.Errorf("rule %d: invalid schema JSON: %w", i, err)
			}

			// Skip validation if schema contains $ref to schema://
			// These will be validated at runtime when files are uploaded
			if isSchemaProtocolRef(rule.Schema) {
				// Just check it's valid JSON, don't validate the $ref yet
				continue
			}

			// Basic validation - just check it parses as a schema
			compiler := jsonschema.NewCompiler()
			compiler.Draft = jsonschema.Draft2020

			// Add the schema with a temporary URL
			err = compiler.AddResource("temp://schema.json", bytes.NewReader(schemaBytes))
			if err != nil {
				return fmt.Errorf("rule %d: invalid JSON schema: %w", i, err)
			}

			_, err = compiler.Compile("temp://schema.json")
			if err != nil {
				return fmt.Errorf("rule %d: invalid JSON schema: %w", i, err)
			}
		}
	}

	return nil
}

// isSchemaProtocolRef checks if a schema contains any schema:// references
func isSchemaProtocolRef(schema map[string]interface{}) bool {
	return hasSchemaProtocolRef(schema)
}

// hasSchemaProtocolRef recursively checks for schema:// in $ref
func hasSchemaProtocolRef(obj interface{}) bool {
	switch v := obj.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"].(string); ok {
			if len(ref) >= 9 && ref[:9] == "schema://" {
				return true
			}
		}
		for _, val := range v {
			if hasSchemaProtocolRef(val) {
				return true
			}
		}
	case []interface{}:
		for _, val := range v {
			if hasSchemaProtocolRef(val) {
				return true
			}
		}
	}
	return false
}

// validateRegoPolicy validates .rego file content using OPA AST parser
func validateRegoPolicy(content []byte) error {
	if len(content) == 0 {
		return fmt.Errorf("policy cannot be empty")
	}

	// Parse Rego module using OPA AST parser
	module, err := ast.ParseModule("policy.rego", string(content))
	if err != nil {
		return fmt.Errorf("rego syntax error: %w", err)
	}

	// Verify package declaration exists
	if module.Package == nil {
		return fmt.Errorf("policy must contain a package declaration")
	}

	// Compile to check for semantic errors (undefined references, type errors, etc.)
	compiler := ast.NewCompiler()
	compiler.Compile(map[string]*ast.Module{
		"policy.rego": module,
	})

	if compiler.Failed() {
		// Format compilation errors
		var errMsgs []string
		for _, compileErr := range compiler.Errors {
			errMsgs = append(errMsgs, compileErr.Error())
		}
		return fmt.Errorf("rego compilation failed:\n%s", strings.Join(errMsgs, "\n"))
	}

	return nil
}

// validateEventsConfig validates .events file content
func validateEventsConfig(content []byte) error {
	// Basic structural validation - most detailed validation is handled by JSON schema
	// We keep minimal Go validation for immediate feedback during file operations
	var eventsFile struct {
		Handlers []struct {
			Name   string   `json:"name"`
			Events []string `json:"events"`
			Type   string   `json:"type"`
		} `json:"handlers"`
	}

	if err := json.Unmarshal(content, &eventsFile); err != nil {
		return fmt.Errorf("invalid events JSON: %w", err)
	}

	if len(eventsFile.Handlers) == 0 {
		return fmt.Errorf("at least one handler must be defined")
	}

	// Validate handler types (keep for immediate feedback)
	validHandlerTypes := map[string]bool{
		"webhook":   true,
		"log":       true,
		"metrics":   true,
		"move_file": true,
	}

	// Check for duplicate handler names (cannot be done in JSON schema)
	handlerNames := make(map[string]bool)
	for i, handler := range eventsFile.Handlers {
		// Basic validation for immediate feedback
		if handler.Name == "" {
			return fmt.Errorf("handler %d: name is required", i)
		}

		if handlerNames[handler.Name] {
			return fmt.Errorf("handler %d: duplicate handler name: %s", i, handler.Name)
		}
		handlerNames[handler.Name] = true

		if !validHandlerTypes[handler.Type] {
			return fmt.Errorf("handler %d: invalid handler type: %s (must be webhook, log, metrics, or move_file)", i, handler.Type)
		}

		if len(handler.Events) == 0 {
			return fmt.Errorf("handler %d: at least one event must be specified", i)
		}

		// Check for empty event patterns
		for j, event := range handler.Events {
			if event == "" {
				return fmt.Errorf("handler %d: event pattern %d cannot be empty", i, j)
			}
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
	PasswordHash string   `json:"password_hash"`   // bcrypt hash
	Token        string   `json:"token,omitempty"` // Optional static token
	Groups       []string `json:"groups"`          // User's group memberships
}

var (
	userSchemaValidator = NewSchemaValidator("user.schema.json")
)

// validateUserConfig validates .user file content
func validateUserConfig(content []byte) error {
	// Validate against schema and unmarshal
	var userConfig UserConfig
	if err := userSchemaValidator.ValidateAndUnmarshal(content, &userConfig); err != nil {
		return fmt.Errorf(".user validation failed: %w", err)
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

var (
	groupSchemaValidator = NewSchemaValidator("group.schema.json")
)

// validateGroupConfig validates .group file content
func validateGroupConfig(content []byte) error {
	// Validate against schema and unmarshal
	var groupConfig GroupConfig
	if err := groupSchemaValidator.ValidateAndUnmarshal(content, &groupConfig); err != nil {
		return fmt.Errorf(".group validation failed: %w", err)
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

package policy

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

type Type string

const (
	TypeRego       Type = "rego"
	TypeJSONSchema Type = "jsonschema"
	TypeWorkflow   Type = "workflow"
	TypeWebhook    Type = "webhook"
	TypeUser       Type = "user"
	TypeGroup      Type = "group"
)

type Scope string

const (
	ScopeDirectory Scope = "directory"
	ScopeTree      Scope = "tree"
	ScopeFile      Scope = "file"
)

type InheritanceMode string

const (
	InheritanceCascade  InheritanceMode = "cascade"
	InheritanceOverride InheritanceMode = "override"
	InheritanceBreak    InheritanceMode = "break"
)

type Manifest struct {
	Type        Type            `json:"type"`
	Name        string          `json:"name"`
	SourcePath  string          `json:"source_path"`
	DirectoryID string          `json:"directory_id"`
	Scope       Scope           `json:"scope"`
	Inheritance InheritanceMode `json:"inheritance"`
	AppliesTo   []string        `json:"applies_to,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	Principals  *PrincipalSet   `json:"principals,omitempty"`
	Module      string          `json:"module,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

var specialFiles = map[string]Type{
	".rego":       TypeRego,
	".jsonschema": TypeJSONSchema,
	".workflow":   TypeWorkflow,
	".webhook":    TypeWebhook,
	".user":       TypeUser,
	".group":      TypeGroup,
}

func TypeFromFilename(name string) (Type, bool) {
	base := strings.ToLower(filepath.Base(name))
	t, ok := specialFiles[base]
	return t, ok
}

func ParseType(name string) (Type, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case string(TypeRego):
		return TypeRego, true
	case string(TypeJSONSchema):
		return TypeJSONSchema, true
	case string(TypeWorkflow):
		return TypeWorkflow, true
	case string(TypeWebhook):
		return TypeWebhook, true
	case string(TypeUser):
		return TypeUser, true
	case string(TypeGroup):
		return TypeGroup, true
	default:
		return "", false
	}
}

func ParseScope(value string) (Scope, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ScopeDirectory):
		return ScopeDirectory, true
	case string(ScopeTree):
		return ScopeTree, true
	case string(ScopeFile):
		return ScopeFile, true
	default:
		return "", false
	}
}

func ParseInheritanceMode(value string) (InheritanceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(InheritanceCascade):
		return InheritanceCascade, true
	case string(InheritanceOverride):
		return InheritanceOverride, true
	case string(InheritanceBreak):
		return InheritanceBreak, true
	default:
		return "", false
	}
}

func IsSpecialFile(name string) bool {
	_, ok := TypeFromFilename(name)
	return ok
}

func DefaultManifest(name, sourcePath, directoryID string, t Type) Manifest {
	return Manifest{
		Type:        t,
		Name:        name,
		SourcePath:  sourcePath,
		DirectoryID: directoryID,
		Scope:       ScopeTree,
		Inheritance: InheritanceCascade,
	}
}

func (m Manifest) AdminOnly() bool {
	return true
}

func (m Manifest) WithInheritance(mode InheritanceMode) Manifest {
	clone := m
	if mode != "" {
		clone.Inheritance = mode
	}
	return clone
}

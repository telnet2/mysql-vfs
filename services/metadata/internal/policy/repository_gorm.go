package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type GormDirectoryRepository struct {
	db *gorm.DB
}

func NewGormDirectoryRepository(db *gorm.DB) *GormDirectoryRepository {
	return &GormDirectoryRepository{db: db}
}

func (r *GormDirectoryRepository) GetDirectory(ctx context.Context, id string) (DirectoryNode, error) {
	var dir db.Directory
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&dir).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DirectoryNode{}, ErrDirectoryNotFound
		}
		return DirectoryNode{}, err
	}
	return DirectoryNode{ID: dir.ID, ParentID: dir.ParentID, Path: dir.Path}, nil
}

type GormManifestRepository struct {
	db *gorm.DB
}

func NewGormManifestRepository(db *gorm.DB) *GormManifestRepository {
	return &GormManifestRepository{db: db}
}

var policyFilenames = []string{".rego", ".jsonschema", ".workflow", ".webhook", ".user", ".group", ".events"}

func (r *GormManifestRepository) ListManifests(ctx context.Context, directoryID string) ([]Manifest, error) {
	var files []db.File
	if err := r.db.WithContext(ctx).
		Where("directory_id = ? AND deleted_at IS NULL", directoryID).
		Where("name IN ?", policyFilenames).
		Find(&files).Error; err != nil {
		return nil, err
	}
	versionIDs := make([]string, 0, len(files))
	for _, f := range files {
		if strings.TrimSpace(f.CurrentVersionID) != "" {
			versionIDs = append(versionIDs, f.CurrentVersionID)
		}
	}
	versionMap := make(map[string]db.FileVersion, len(versionIDs))
	if len(versionIDs) > 0 {
		var versions []db.FileVersion
		if err := r.db.WithContext(ctx).Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
			return nil, err
		}
		for _, v := range versions {
			versionMap[v.ID] = v
		}
	}
	manifests := make([]Manifest, 0, len(files))
	for _, f := range files {
		t, ok := TypeFromFilename(strings.ToLower(f.Name))
		if !ok {
			continue
		}
		m := DefaultManifest(f.Name, f.Path, f.DirectoryID, t)
		if m.Metadata == nil {
			m.Metadata = make(map[string]any)
		}
		m.Metadata["file_id"] = f.ID
		if strings.TrimSpace(f.CurrentVersionID) != "" {
			m.Metadata["version_id"] = f.CurrentVersionID
		}

		if t == TypeUser || t == TypeGroup {
			version, ok := versionMap[f.CurrentVersionID]
			if !ok {
				return nil, fmt.Errorf("policy: missing current version for file %s", f.ID)
			}
			if version.StorageMode != "inline_json" {
				return nil, fmt.Errorf("policy: principal manifests must use inline_json storage for file %s", f.ID)
			}
			principals, overrides, err := ParsePrincipalManifest([]byte(version.JSONPayload), t)
			if err != nil {
				return nil, err
			}
			m.Principals = &principals
			if overrides.Scope != nil {
				m.Scope = *overrides.Scope
			}
			if overrides.Inheritance != nil {
				m.Inheritance = *overrides.Inheritance
			}
		} else if t == TypeRego {
			version, ok := versionMap[f.CurrentVersionID]
			if !ok {
				return nil, fmt.Errorf("policy: missing current version for file %s", f.ID)
			}
			if version.StorageMode != "inline_json" {
				return nil, fmt.Errorf("policy: rego manifests must use inline_json storage for file %s", f.ID)
			}
			if len(version.JSONPayload) == 0 {
				return nil, fmt.Errorf("policy: rego manifest payload required for file %s", f.ID)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(version.JSONPayload), &payload); err != nil {
				return nil, fmt.Errorf("policy: invalid rego manifest for file %s: %w", f.ID, err)
			}
			module, ok := payload["module"].(string)
			if !ok || strings.TrimSpace(module) == "" {
				return nil, fmt.Errorf("policy: rego manifest for file %s missing module", f.ID)
			}
			m.Module = module
		} else if t == TypeJSONSchema {
			version, ok := versionMap[f.CurrentVersionID]
			if !ok {
				return nil, fmt.Errorf("policy: missing current version for file %s", f.ID)
			}
			if version.StorageMode != "inline_json" {
				return nil, fmt.Errorf("policy: jsonschema manifests must use inline_json storage for file %s", f.ID)
			}
			if len(version.JSONPayload) == 0 {
				return nil, fmt.Errorf("policy: jsonschema manifest payload required for file %s", f.ID)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal([]byte(version.JSONPayload), &payload); err != nil {
				return nil, fmt.Errorf("policy: invalid jsonschema manifest for file %s: %w", f.ID, err)
			}
			schemaRaw, hasWrapper := payload["schema"]
			if !hasWrapper {
				schemaRaw = json.RawMessage(version.JSONPayload)
			}
			if len(bytes.TrimSpace(schemaRaw)) == 0 {
				return nil, fmt.Errorf("policy: jsonschema manifest for file %s missing schema", f.ID)
			}
			m.Schema = schemaRaw
			if hasWrapper {
				if scopeRaw, ok := payload["scope"]; ok {
					var scopeValue string
					if err := json.Unmarshal(scopeRaw, &scopeValue); err != nil {
						return nil, fmt.Errorf("policy: invalid scope for jsonschema manifest %s: %w", f.ID, err)
					}
					if scopeValue != "" {
						if parsed, ok := ParseScope(scopeValue); ok {
							m.Scope = parsed
						} else {
							return nil, fmt.Errorf("policy: unsupported scope %q for jsonschema manifest %s", scopeValue, f.ID)
						}
					}
				}
				if inheritRaw, ok := payload["inheritance"]; ok {
					var modeValue string
					if err := json.Unmarshal(inheritRaw, &modeValue); err != nil {
						return nil, fmt.Errorf("policy: invalid inheritance for jsonschema manifest %s: %w", f.ID, err)
					}
					if modeValue != "" {
						if parsed, ok := ParseInheritanceMode(modeValue); ok {
							m.Inheritance = parsed
						} else {
							return nil, fmt.Errorf("policy: unsupported inheritance %q for jsonschema manifest %s", modeValue, f.ID)
						}
					}
				}
				if appliesRaw, ok := payload["applies_to"]; ok {
					var applies []string
					if err := json.Unmarshal(appliesRaw, &applies); err != nil {
						return nil, fmt.Errorf("policy: invalid applies_to for jsonschema manifest %s: %w", f.ID, err)
					}
					m.AppliesTo = applies
				}
			}
		} else if t == TypeEvents {
			version, ok := versionMap[f.CurrentVersionID]
			if !ok {
				return nil, fmt.Errorf("policy: missing current version for file %s", f.ID)
			}
			if version.StorageMode != "inline_json" {
				return nil, fmt.Errorf("policy: events manifests must use inline_json storage for file %s", f.ID)
			}
			config, overrides, err := ParseEventsManifest([]byte(version.JSONPayload))
			if err != nil {
				return nil, err
			}
			m.Events = &config
			if overrides.Scope != nil {
				m.Scope = *overrides.Scope
			}
			if overrides.Inheritance != nil {
				m.Inheritance = *overrides.Inheritance
			}
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

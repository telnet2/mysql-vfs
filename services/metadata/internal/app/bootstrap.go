package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/service"
)

const adminPasswordFilename = ".metadata_admin_password"

// BootstrapAdmin ensures the builtin admin group/user exist and that a master password is stored locally.
func BootstrapAdmin(ctx context.Context, deps Dependencies) error {
	_, passwordPath, err := ensureAdminPassword()
	if err != nil {
		return fmt.Errorf("ensure admin password: %w", err)
	}

	dirSvc := service.NewDirectoryService(deps.DB)
	fileSvc := service.NewFileService(deps.DB, deps.PolicyRegistry, deps.PolicyEvaluator, deps.PolicyValidator, deps.PolicyTriggerEngine)

	root, err := dirSvc.ResolvePath(ctx, "/")
	if err != nil {
		if errors.Is(err, service.ErrDirectoryNotFound) {
			created, createErr := dirSvc.Create(ctx, service.CreateDirectoryInput{
				Name:      "/",
				RequestID: "bootstrap",
			})
			if createErr != nil && !errors.Is(createErr, service.ErrNameConflict) {
				return fmt.Errorf("create root directory: %w", createErr)
			}
			if createErr == nil {
				root = created
			} else {
				root, err = dirSvc.ResolvePath(ctx, "/")
				if err != nil {
					return fmt.Errorf("resolve root directory: %w", err)
				}
			}
		} else {
			return fmt.Errorf("resolve root directory: %w", err)
		}
	}

	groupManifest := map[string]any{
		"groups": []map[string]any{
			{
				"id":           "admin",
				"display_name": "Administrators",
				"members":      []string{"_admin"},
			},
		},
	}

	userManifest := map[string]any{
		"users": []map[string]any{
			{
				"id":           "_admin",
				"display_name": "Builtin Administrator",
				"groups":       []string{"admin"},
				"attributes": map[string]any{
					"password_file": passwordPath,
				},
			},
		},
	}

	if err := upsertPolicyFile(ctx, fileSvc, root.ID, ".group", groupManifest); err != nil {
		return fmt.Errorf("ensure admin group: %w", err)
	}
	if err := upsertPolicyFile(ctx, fileSvc, root.ID, ".user", userManifest); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}

	return nil
}

func ensureAdminPassword() (string, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(wd, adminPasswordFilename)
	if data, err := os.ReadFile(path); err == nil {
		password := strings.TrimSpace(string(data))
		if password != "" {
			return password, path, nil
		}
	}
	password, err := generatePassword()
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return password, path, nil
}

func generatePassword() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func upsertPolicyFile(ctx context.Context, fs *service.FileService, directoryID, name string, manifest map[string]any) error {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	input := service.CreateFileInput{
		DirectoryID: directoryID,
		Name:        name,
		RequestID:   uuid.NewString(),
		Actor:       "system",
		VersionData: service.FileVersionData{
			StorageMode: "inline_json",
			JSONPayload: payload,
			Actor:       "system",
		},
	}
	if _, err := fs.Create(ctx, input); err != nil {
		if errors.Is(err, service.ErrPolicyForbidden) {
			return fmt.Errorf("policy permission denied for %s", name)
		}
		if errors.Is(err, service.ErrInvalidRequest) && strings.Contains(err.Error(), "already exists") {
			var existing db.File
			if lookupErr := fs.DB.WithContext(ctx).
				Where("directory_id = ? AND name = ? AND deleted_at IS NULL", directoryID, name).
				First(&existing).Error; lookupErr != nil {
				if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
					return fmt.Errorf("bootstrap lookup %s: %w", name, lookupErr)
				}
				return lookupErr
			}
			versionData := input.VersionData
			_, updateErr := fs.Update(ctx, service.UpdateFileInput{
				FileID:      existing.ID,
				VersionData: &versionData,
				RequestID:   uuid.NewString(),
				Actor:       "system",
			})
			return updateErr
		}
		return err
	}
	return nil
}

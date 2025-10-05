package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

const (
	// MaxFileSize is the maximum allowed file size (100MB)
	MaxFileSize = 100 * 1024 * 1024
)

// CreateFileRequest represents a request to create a file
type CreateFileRequest struct {
	DirectoryPath string
	Name          string
	ContentType   string
	Content       string
	UserRole      string // "admin", "user", "readonly"
}

// FileValidator handles file validation and business rules
type FileValidator struct {
	uow          db.UnitOfWork
	filesLoader  *FilesLoader
	policyLoader *PolicyLoader
	groupLoader  *GroupLoader
	ownerLoader  *OwnerLoader
	protection   ResourceProtection
}

// NewFileValidator creates a new file validator with special file loaders
func NewFileValidator(
	uow db.UnitOfWork,
	filesLoader *FilesLoader,
	policyLoader *PolicyLoader,
	groupLoader *GroupLoader,
	ownerLoader *OwnerLoader,
	protection ResourceProtection,
) *FileValidator {
	return &FileValidator{
		uow:          uow,
		filesLoader:  filesLoader,
		policyLoader: policyLoader,
		groupLoader:  groupLoader,
		ownerLoader:  ownerLoader,
		protection:   protection,
	}
}

// CreateFile creates a new file with special file handling and validation
func (s *FileValidator) CreateFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Check protection rules first
	if s.protection != nil {
		if err := s.protection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: req.DirectoryPath,
			FileName:      req.Name,
			UserRole:      req.UserRole,
		}); err != nil {
			return nil, err
		}
	}

	// Validate file size
	if len(req.Content) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Check if this is a special file
	if IsSpecialFile(req.Name) {
		return s.createSpecialFile(ctx, req)
	}

	// Regular file - check quota and validate against schema
	return s.createRegularFile(ctx, req)
}

// createSpecialFile handles creation of special files (.jsonschema, .rego, etc.)
func (s *FileValidator) createSpecialFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Check admin permissions
	if RequiresAdmin(req.Name) && !IsSystemAdmin(req.UserRole) {
		return nil, ErrPermissionDenied
	}

	// Validate special file content
	if err := ValidateSpecialFileContent(req.Name, []byte(req.Content)); err != nil {
		return nil, err
	}

	// Additional validation for .owner file - check if all groups exist
	if req.Name == string(SpecialFileTypeOwner) && s.groupLoader != nil {
		var ownerConfig OwnerConfig
		if err := json.Unmarshal([]byte(req.Content), &ownerConfig); err == nil {
			// Check if all owner groups exist
			for _, ownerGroup := range ownerConfig.Owners {
				exists, err := s.groupLoader.GroupExists(ctx, ownerGroup)
				if err != nil {
					return nil, fmt.Errorf("failed to validate owner group: %w", err)
				}
				if !exists {
					return nil, fmt.Errorf("owner group '%s' does not exist in /.group file", ownerGroup)
				}
			}
		}
	}

	// Create the file
	file, err := s.createFileInternal(ctx, req)
	if err != nil {
		return nil, err
	}

	// Invalidate relevant caches
	s.invalidateCacheForSpecialFile(req.DirectoryPath, req.Name)

	return file, nil
}

// createRegularFile handles creation of regular files with validation
func (s *FileValidator) createRegularFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Get directory to validate file against .files rules
	dirRepo := s.uow.Directories()
	dir, err := dirRepo.FindByPath(ctx, req.DirectoryPath)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrDirectoryNotFound
		}
		return nil, err
	}

	// Validate against .files rules (pattern + schema)
	if s.filesLoader != nil {
		if err := s.filesLoader.ValidateFile(ctx, dir.ID, req.Name, []byte(req.Content)); err != nil {
			return nil, err
		}
	}

	// Create the file
	return s.createFileInternal(ctx, req)
}

// createFileInternal performs the actual file creation
func (s *FileValidator) createFileInternal(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Start transaction
	tx, err := s.uow.BeginTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get repositories
	dirRepo := s.uow.Directories()
	fileRepo := s.uow.Files()

	// Find directory
	dir, err := dirRepo.FindByPath(ctx, req.DirectoryPath)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrDirectoryNotFound
		}
		return nil, err
	}

	// Check if file already exists
	exists, err := fileRepo.Exists(ctx, dir.ID, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyExists
	}

	// Create file entity
	now := time.Now()
	jsonContent := req.Content
	checksum := calculateChecksum(req.Content)

	file := &models.File{
		ID:             uuid.New().String(),
		DirectoryID:    dir.ID,
		Name:           req.Name,
		ContentType:    req.ContentType,
		SizeBytes:      int64(len(req.Content)),
		StorageType:    models.StorageTypeJSON,
		JSONContent:    &jsonContent,
		ChecksumSHA256: checksum,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Save file
	if err := fileRepo.Create(ctx, file); err != nil {
		return nil, err
	}

	// Create initial version
	version := &models.FileVersion{
		ID:             uuid.New().String(),
		FileID:         file.ID,
		VersionNumber:  1,
		ContentType:    req.ContentType,
		SizeBytes:      int64(len(req.Content)),
		StorageType:    models.StorageTypeJSON,
		JSONContent:    &jsonContent,
		ChecksumSHA256: checksum,
		CreatedAt:      now,
	}

	if err := fileRepo.CreateVersion(ctx, version); err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return file, nil
}

// UpdateFile updates an existing file
func (s *FileValidator) UpdateFile(ctx context.Context, fileID string, newContent string, userRole string) (*models.File, error) {
	// Validate file size
	if len(newContent) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Start transaction (we need to get file info before checking protection)
	tx, err := s.uow.BeginTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get repositories
	fileRepo := s.uow.Files()
	dirRepo := s.uow.Directories()

	// Find file
	file, err := fileRepo.FindByID(ctx, fileID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	// Get directory for path resolution
	dir, err := dirRepo.FindByID(ctx, file.DirectoryID)
	if err != nil {
		return nil, err
	}

	// Check protection rules
	if s.protection != nil {
		if err := s.protection.CanModify(ctx, ProtectionRequest{
			DirectoryPath: dir.Path,
			FileName:      file.Name,
			UserRole:      userRole,
		}); err != nil {
			return nil, err
		}
	}

	// Check if this is a special file
	if IsSpecialFile(file.Name) {
		// Check admin permissions
		if RequiresAdmin(file.Name) && !IsSystemAdmin(userRole) {
			return nil, ErrPermissionDenied
		}

		// Validate special file content
		if err := ValidateSpecialFileContent(file.Name, []byte(newContent)); err != nil {
			return nil, err
		}
	} else {
		// Regular file - validate against .files rules
		if s.filesLoader != nil {
			if err := s.filesLoader.ValidateFile(ctx, file.DirectoryID, file.Name, []byte(newContent)); err != nil {
				return nil, err
			}
		}
	}

	// Update file metadata
	jsonContent := newContent
	checksum := calculateChecksum(newContent)

	file.SizeBytes = int64(len(newContent))
	file.JSONContent = &jsonContent
	file.ChecksumSHA256 = checksum
	file.Version++
	file.UpdatedAt = time.Now()

	if err := fileRepo.Update(ctx, file); err != nil {
		return nil, err
	}

	// Create new version
	version := &models.FileVersion{
		ID:             uuid.New().String(),
		FileID:         file.ID,
		VersionNumber:  file.Version,
		ContentType:    file.ContentType,
		SizeBytes:      int64(len(newContent)),
		StorageType:    models.StorageTypeJSON,
		JSONContent:    &jsonContent,
		ChecksumSHA256: checksum,
		CreatedAt:      time.Now(),
	}

	if err := fileRepo.CreateVersion(ctx, version); err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// If this was a special file, invalidate cache
	if IsSpecialFile(file.Name) {
		s.invalidateCacheForSpecialFile(dir.Path, file.Name)
	}

	return file, nil
}

// DeleteFile deletes a file
func (s *FileValidator) DeleteFile(ctx context.Context, fileID string, userRole string) error {
	// Start transaction
	tx, err := s.uow.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get repositories
	fileRepo := s.uow.Files()
	dirRepo := s.uow.Directories()

	// Find file
	file, err := fileRepo.FindByID(ctx, fileID)
	if err != nil {
		if err == db.ErrNotFound {
			return ErrFileNotFound
		}
		return err
	}

	// Get directory for path and protection check
	dir, err := dirRepo.FindByID(ctx, file.DirectoryID)
	if err != nil {
		return err
	}

	// Check protection rules
	if s.protection != nil {
		if err := s.protection.CanDelete(ctx, ProtectionRequest{
			DirectoryPath: dir.Path,
			FileName:      file.Name,
			ResourceType:  "file",
			UserRole:      userRole,
		}); err != nil {
			return err
		}
	}

	// Check if this is a special file - require admin
	if IsSpecialFile(file.Name) && userRole != "admin" {
		return ErrPermissionDenied
	}

	// Soft delete the file
	if err := fileRepo.SoftDelete(ctx, fileID); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	// If this was a special file, invalidate cache
	if IsSpecialFile(file.Name) {
		s.invalidateCacheForSpecialFile(dir.Path, file.Name)
	}

	return nil
}

// invalidateCacheForSpecialFile invalidates the appropriate cache when a special file changes
func (s *FileValidator) invalidateCacheForSpecialFile(directoryPath, fileName string) {
	// Get directory ID for cache invalidation
	dirRepo := s.uow.Directories()
	dir, err := dirRepo.FindByPath(context.Background(), directoryPath)
	if err != nil {
		return // Can't invalidate if directory not found
	}

	switch GetSpecialFileType(fileName) {
	case SpecialFileTypeFiles:
		if s.filesLoader != nil {
			s.filesLoader.InvalidateCache(dir.ID)
		}
	case SpecialFileTypePolicy:
		if s.policyLoader != nil {
			s.policyLoader.Invalidate(directoryPath)
		}
	case SpecialFileTypeGroup:
		if s.groupLoader != nil {
			// Group files can only be at root, invalidate all
			s.groupLoader.InvalidateAll()
		}
	case SpecialFileTypeOwner:
		if s.ownerLoader != nil {
			s.ownerLoader.InvalidateCache(dir.ID)
		}
	}
}

// GetFile retrieves a file by ID
func (s *FileValidator) GetFile(ctx context.Context, fileID string) (*models.File, string, error) {
	fileRepo := s.uow.Files()

	file, err := fileRepo.FindByID(ctx, fileID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, "", ErrFileNotFound
		}
		return nil, "", err
	}

	// Get latest version
	version, err := fileRepo.GetLatestVersion(ctx, fileID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file version: %w", err)
	}

	content := ""
	if version.JSONContent != nil {
		content = *version.JSONContent
	}

	return file, content, nil
}

// GetFileByPath retrieves a file by its full path
func (s *FileValidator) GetFileByPath(ctx context.Context, filePath string) (*models.File, string, error) {
	dirPath := path.Dir(filePath)
	fileName := path.Base(filePath)

	dirRepo := s.uow.Directories()
	fileRepo := s.uow.Files()

	// Find directory
	dir, err := dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, "", ErrDirectoryNotFound
		}
		return nil, "", err
	}

	// Find file
	file, err := fileRepo.FindByDirectoryAndName(ctx, dir.ID, fileName)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, "", ErrFileNotFound
		}
		return nil, "", err
	}

	// Get latest version
	version, err := fileRepo.GetLatestVersion(ctx, file.ID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file version: %w", err)
	}

	content := ""
	if version.JSONContent != nil {
		content = *version.JSONContent
	}

	return file, content, nil
}

// calculateChecksum calculates SHA256 checksum of content
func calculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

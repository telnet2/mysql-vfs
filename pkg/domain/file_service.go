package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/repository"
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

// FileService contains pure business logic for file operations
type FileService struct {
	uow          repository.UnitOfWork
	filesLoader  *FilesLoader
	policyLoader *PolicyLoader
}

// NewFileService creates a new file service with special file loaders
func NewFileService(
	uow repository.UnitOfWork,
	filesLoader *FilesLoader,
	policyLoader *PolicyLoader,
) *FileService {
	return &FileService{
		uow:          uow,
		filesLoader:  filesLoader,
		policyLoader: policyLoader,
	}
}

// CreateFile creates a new file with special file handling and validation
func (s *FileService) CreateFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
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
func (s *FileService) createSpecialFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Check admin permissions
	if RequiresAdmin(req.Name) && req.UserRole != "admin" {
		return nil, ErrPermissionDenied
	}

	// Validate special file content
	if err := ValidateSpecialFileContent(req.Name, []byte(req.Content)); err != nil {
		return nil, err
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
func (s *FileService) createRegularFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
	// Get directory to validate file against .files rules
	dirRepo := s.uow.Directories()
	dir, err := dirRepo.FindByPath(ctx, req.DirectoryPath)
	if err != nil {
		if err == repository.ErrNotFound {
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
func (s *FileService) createFileInternal(ctx context.Context, req CreateFileRequest) (*models.File, error) {
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
		if err == repository.ErrNotFound {
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
func (s *FileService) UpdateFile(ctx context.Context, fileID string, newContent string, userRole string) (*models.File, error) {
	// Validate file size
	if len(newContent) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Start transaction
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
		if err == repository.ErrNotFound {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	// Get directory for path resolution
	dir, err := dirRepo.FindByID(ctx, file.DirectoryID)
	if err != nil {
		return nil, err
	}

	// Check if this is a special file
	if IsSpecialFile(file.Name) {
		// Check admin permissions
		if RequiresAdmin(file.Name) && userRole != "admin" {
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
func (s *FileService) DeleteFile(ctx context.Context, fileID string, userRole string) error {
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
		if err == repository.ErrNotFound {
			return ErrFileNotFound
		}
		return err
	}

	// Check if this is a special file - require admin
	if IsSpecialFile(file.Name) && userRole != "admin" {
		return ErrPermissionDenied
	}

	// Get directory for cache invalidation
	dir, err := dirRepo.FindByID(ctx, file.DirectoryID)
	if err != nil {
		return err
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
func (s *FileService) invalidateCacheForSpecialFile(directoryPath, fileName string) {
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
	}
}

// GetFile retrieves a file by ID
func (s *FileService) GetFile(ctx context.Context, fileID string) (*models.File, string, error) {
	fileRepo := s.uow.Files()

	file, err := fileRepo.FindByID(ctx, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
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
func (s *FileService) GetFileByPath(ctx context.Context, filePath string) (*models.File, string, error) {
	dirPath := path.Dir(filePath)
	fileName := path.Base(filePath)

	dirRepo := s.uow.Directories()
	fileRepo := s.uow.Files()

	// Find directory
	dir, err := dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, "", ErrDirectoryNotFound
		}
		return nil, "", err
	}

	// Find file
	file, err := fileRepo.FindByDirectoryAndName(ctx, dir.ID, fileName)
	if err != nil {
		if err == repository.ErrNotFound {
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

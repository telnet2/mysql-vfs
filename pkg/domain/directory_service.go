package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/repository"
)

const (
	// MaxDirectoryDepth is the maximum allowed directory depth
	MaxDirectoryDepth = 100
)

// CreateDirectoryRequest represents a request to create a directory
type CreateDirectoryRequest struct {
	ParentPath  string
	Name        string
	OPAPolicyID *string
}

// DirectoryService contains pure business logic for directory operations
type DirectoryService struct {
	uow repository.UnitOfWork
}

// NewDirectoryService creates a new directory service
func NewDirectoryService(uow repository.UnitOfWork) *DirectoryService {
	return &DirectoryService{
		uow: uow,
	}
}

// CreateDirectory creates a new directory
func (s *DirectoryService) CreateDirectory(ctx context.Context, req CreateDirectoryRequest) (*models.Directory, error) {
	// Calculate full path (pure business logic)
	fullPath := path.Join(req.ParentPath, req.Name)

	// Normalize path
	fullPath = path.Clean(fullPath)
	if fullPath != "/" && !strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}

	// Check depth limit (business rule)
	depth := strings.Count(fullPath, "/")
	if depth > MaxDirectoryDepth {
		return nil, ErrDepthLimitExceeded
	}

	// Start transaction
	tx, err := s.uow.BeginTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get parent directory (through repository)
	dirRepo := s.uow.Directories()
	var parent *models.Directory

	if req.ParentPath != "" && req.ParentPath != "/" {
		parent, err = dirRepo.FindByPath(ctx, req.ParentPath)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, ErrParentNotFound
			}
			return nil, err
		}
	}

	// Check if directory already exists
	exists, err := dirRepo.Exists(ctx, fullPath)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyExists
	}

	// Acquire tree lock (business rule - prevents race conditions)
	pathComponents := getPathComponents(fullPath)
	if err := dirRepo.LockPaths(ctx, tx, pathComponents); err != nil {
		return nil, err
	}

	// Create directory entity
	now := time.Now()
	dir := &models.Directory{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Path:        fullPath,
		PathHash:    calculatePathHash(fullPath),
		Version:     1,
		OPAPolicyID: req.OPAPolicyID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if parent != nil {
		dir.ParentID = &parent.ID
	}

	// Persist through repository
	if err := dirRepo.Create(ctx, dir); err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return dir, nil
}

// DeleteDirectory deletes a directory
func (s *DirectoryService) DeleteDirectory(ctx context.Context, dirPath string, recursive bool) error {
	// Start transaction
	tx, err := s.uow.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dirRepo := s.uow.Directories()
	fileRepo := s.uow.Files()

	// Find directory
	dir, err := dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrDirectoryNotFound
		}
		return err
	}

	// Check if directory is empty (if not recursive)
	if !recursive {
		// Check for child directories
		childDirs, _, err := dirRepo.FindByParentID(ctx, dir.ID, 1, "")
		if err != nil {
			return err
		}
		if len(childDirs) > 0 {
			return ErrDirectoryNotEmpty
		}

		// Check for files
		files, _, err := fileRepo.FindByDirectoryID(ctx, dir.ID, 1, "")
		if err != nil {
			return err
		}
		if len(files) > 0 {
			return ErrDirectoryNotEmpty
		}
	}

	// Acquire tree lock
	pathComponents := getPathComponents(dir.Path)
	if err := dirRepo.LockPaths(ctx, tx, pathComponents); err != nil {
		return err
	}

	// Delete directory (soft delete)
	if err := dirRepo.SoftDelete(ctx, dir.ID); err != nil {
		return err
	}

	// If recursive, delete children
	if recursive {
		// Delete child directories
		childDirs, _, err := dirRepo.FindByParentID(ctx, dir.ID, 0, "")
		if err != nil {
			return err
		}
		for _, child := range childDirs {
			if err := s.DeleteDirectory(ctx, child.Path, true); err != nil {
				return err
			}
		}

		// Delete files
		files, _, err := fileRepo.FindByDirectoryID(ctx, dir.ID, 0, "")
		if err != nil {
			return err
		}
		for _, file := range files {
			if err := fileRepo.SoftDelete(ctx, file.ID); err != nil {
				return err
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// GetDirectory retrieves a directory by path
func (s *DirectoryService) GetDirectory(ctx context.Context, dirPath string) (*models.Directory, error) {
	dir, err := s.uow.Directories().FindByPath(ctx, dirPath)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrDirectoryNotFound
		}
		return nil, err
	}
	return dir, nil
}

// ListDirectory lists contents of a directory
func (s *DirectoryService) ListDirectory(ctx context.Context, dirPath string, limit int, cursor string) ([]*models.Directory, string, error) {
	// Find parent directory
	dir, err := s.uow.Directories().FindByPath(ctx, dirPath)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, "", ErrDirectoryNotFound
		}
		return nil, "", err
	}

	// List child directories
	dirs, nextCursor, err := s.uow.Directories().FindByParentID(ctx, dir.ID, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	return dirs, nextCursor, nil
}

// Helper functions

// getPathComponents returns all path components for tree locking
func getPathComponents(fullPath string) []string {
	parts := strings.Split(strings.Trim(fullPath, "/"), "/")
	components := make([]string, 0, len(parts))

	currentPath := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		currentPath = path.Join(currentPath, part)
		if !strings.HasPrefix(currentPath, "/") {
			currentPath = "/" + currentPath
		}
		components = append(components, currentPath)
	}

	return components
}

// calculatePathHash calculates SHA256 hash of the path
func calculatePathHash(p string) string {
	hash := sha256.Sum256([]byte(p))
	return hex.EncodeToString(hash[:])
}

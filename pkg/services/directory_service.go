package services

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
)

// DirectoryService handles directory operations
type DirectoryService struct {
	db *gorm.DB
}

// NewDirectoryService creates a new directory service
func NewDirectoryService(db *gorm.DB) *DirectoryService {
	return &DirectoryService{db: db}
}

// CreateDirectory creates a new directory
func (s *DirectoryService) CreateDirectory(parentPath, name string, opaPolicyID *string) (*models.Directory, error) {
	// Validate name
	if name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid directory name")
	}

	// Calculate full path
	fullPath := path.Join(parentPath, name)

	// Check depth limit (100 levels)
	depth := strings.Count(fullPath, "/")
	if depth > 100 {
		return nil, fmt.Errorf("directory tree depth limit exceeded (max 100 levels)")
	}

	var dir *models.Directory
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock parent directory path (tree lock)
		pathComponents := s.getPathComponents(parentPath)
		if err := s.lockPaths(tx, pathComponents); err != nil {
			return fmt.Errorf("failed to acquire tree lock: %w", err)
		}

		// Find parent directory
		var parent models.Directory
		if parentPath != "/" {
			if err := tx.Where("path = ? AND deleted_at IS NULL", parentPath).First(&parent).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return fmt.Errorf("parent directory not found: %s", parentPath)
				}
				return err
			}
		}

		// Check if directory already exists
		var existing models.Directory
		err := tx.Where("path = ? AND deleted_at IS NULL", fullPath).First(&existing).Error
		if err == nil {
			return fmt.Errorf("directory already exists: %s", fullPath)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Create directory
		dir = &models.Directory{
			ID:          uuid.New().String(),
			Name:        name,
			Path:        fullPath,
			Version:     1,
			OPAPolicyID: opaPolicyID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if parentPath != "/" {
			dir.ParentID = &parent.ID
		}

		if err := tx.Create(dir).Error; err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return dir, nil
}

// ListDirectory lists contents of a directory
func (s *DirectoryService) ListDirectory(dirPath string, limit int, cursor string) ([]models.Directory, []models.File, string, error) {
	var directories []models.Directory
	var files []models.File
	var nextCursor string

	// Find the directory
	var dir models.Directory
	if dirPath == "/" {
		// Root directory special case
		dir.ID = ""
		dir.Path = "/"
	} else {
		if err := s.db.Where("path = ? AND deleted_at IS NULL", dirPath).First(&dir).Error; err != nil {
			return nil, nil, "", fmt.Errorf("directory not found: %s", dirPath)
		}
	}

	// Query subdirectories
	query := s.db.Where("deleted_at IS NULL")
	if dirPath == "/" {
		query = query.Where("parent_id IS NULL OR parent_id = ''")
	} else {
		query = query.Where("parent_id = ?", dir.ID)
	}

	if err := query.Order("name").Find(&directories).Error; err != nil {
		return nil, nil, "", fmt.Errorf("failed to list directories: %w", err)
	}

	// Query files
	fileQuery := s.db.Where("deleted_at IS NULL")
	if dirPath != "/" {
		fileQuery = fileQuery.Where("directory_id = ?", dir.ID)
	}

	if err := fileQuery.Order("name").Find(&files).Error; err != nil {
		return nil, nil, "", fmt.Errorf("failed to list files: %w", err)
	}

	return directories, files, nextCursor, nil
}

// DeleteDirectory deletes a directory (optionally recursive)
func (s *DirectoryService) DeleteDirectory(dirPath string, recursive bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Lock directory path (tree lock)
		pathComponents := s.getPathComponents(dirPath)
		if err := s.lockPaths(tx, pathComponents); err != nil {
			return fmt.Errorf("failed to acquire tree lock: %w", err)
		}

		// Find directory
		var dir models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", dirPath).First(&dir).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("directory not found: %s", dirPath)
			}
			return err
		}

		if !recursive {
			// Check if directory is empty
			var childCount int64
			if err := tx.Model(&models.Directory{}).Where("parent_id = ? AND deleted_at IS NULL", dir.ID).Count(&childCount).Error; err != nil {
				return err
			}
			if childCount > 0 {
				return fmt.Errorf("directory not empty (contains %d subdirectories)", childCount)
			}

			var fileCount int64
			if err := tx.Model(&models.File{}).Where("directory_id = ? AND deleted_at IS NULL", dir.ID).Count(&fileCount).Error; err != nil {
				return err
			}
			if fileCount > 0 {
				return fmt.Errorf("directory not empty (contains %d files)", fileCount)
			}
		} else {
			// Recursive delete: mark all children as deleted
			if err := s.recursiveDelete(tx, dir.ID); err != nil {
				return err
			}
		}

		// Soft delete the directory
		if err := tx.Delete(&dir).Error; err != nil {
			return fmt.Errorf("failed to delete directory: %w", err)
		}

		return nil
	})
}

// recursiveDelete recursively deletes all subdirectories and files
func (s *DirectoryService) recursiveDelete(tx *gorm.DB, dirID string) error {
	// Delete all files in this directory
	if err := tx.Where("directory_id = ?", dirID).Delete(&models.File{}).Error; err != nil {
		return err
	}

	// Find all subdirectories
	var subdirs []models.Directory
	if err := tx.Where("parent_id = ? AND deleted_at IS NULL", dirID).Find(&subdirs).Error; err != nil {
		return err
	}

	// Recursively delete subdirectories
	for _, subdir := range subdirs {
		if err := s.recursiveDelete(tx, subdir.ID); err != nil {
			return err
		}
		if err := tx.Delete(&subdir).Error; err != nil {
			return err
		}
	}

	return nil
}

// getPathComponents splits a path into its components for tree locking
func (s *DirectoryService) getPathComponents(p string) []string {
	if p == "/" {
		return []string{"/"}
	}

	parts := strings.Split(strings.Trim(p, "/"), "/")
	components := []string{"/"}
	currentPath := ""
	for _, part := range parts {
		currentPath = path.Join(currentPath, part)
		components = append(components, currentPath)
	}
	return components
}

// lockPaths acquires locks on all paths in order (tree lock protocol)
func (s *DirectoryService) lockPaths(tx *gorm.DB, paths []string) error {
	// Lock paths in order from root to leaf to prevent deadlocks
	for _, p := range paths {
		var dir models.Directory
		query := tx.Where("path = ? AND deleted_at IS NULL", p)

		// Use FOR UPDATE to lock the row
		if err := query.Clauses(gorm.Expr("FOR UPDATE")).First(&dir).Error; err != nil {
			if err == gorm.ErrRecordNotFound && p == "/" {
				// Root directory doesn't exist in DB, skip
				continue
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
		}
	}
	return nil
}

// GetDirectory retrieves a directory by path
func (s *DirectoryService) GetDirectory(dirPath string) (*models.Directory, error) {
	var dir models.Directory
	if err := s.db.Where("path = ? AND deleted_at IS NULL", dirPath).First(&dir).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("directory not found: %s", dirPath)
		}
		return nil, err
	}
	return &dir, nil
}

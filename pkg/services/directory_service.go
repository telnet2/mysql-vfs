package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// Retry configuration for optimistic locking
	maxRetries     = 5
	baseBackoffMs  = 10
	maxBackoffMs   = 500
	jitterPercent  = 0.3
)

// DirectoryService handles directory operations
type DirectoryService struct {
	db *gorm.DB
}

// NewDirectoryService creates a new directory service
func NewDirectoryService(db *gorm.DB) *DirectoryService {
	// Initialize random seed for backoff jitter (seed once per service instance)
	rand.Seed(time.Now().UnixNano())
	return &DirectoryService{db: db}
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: baseBackoff * 2^attempt
	backoff := baseBackoffMs * (1 << uint(attempt))
	if backoff > maxBackoffMs {
		backoff = maxBackoffMs
	}

	// Add jitter to prevent thundering herd
	jitter := float64(backoff) * jitterPercent * (rand.Float64()*2 - 1)
	backoffWithJitter := int(float64(backoff) + jitter)

	return time.Duration(backoffWithJitter) * time.Millisecond
}

// isDuplicateKeyError checks if error is a duplicate key violation
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// MySQL duplicate key errors
	return strings.Contains(errMsg, "Error 1062") ||
		   strings.Contains(errMsg, "Duplicate entry") ||
		   strings.Contains(errMsg, "duplicate key")
}

// calculatePathHash calculates SHA256 hash of a path for uniqueness constraint
func calculatePathHash(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

// emitEvent creates an event in the same transaction
func (s *DirectoryService) emitEvent(ctx context.Context, tx *gorm.DB, eventType, aggregateID string, payload interface{}) error {
	// Get request ID from context
	requestID := ctx.Value("requestID")
	if requestID == nil {
		// If no request ID, generate one (for non-API calls)
		requestID = uuid.New().String()
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	// Create event
	event := &models.Event{
		ID:          uuid.New().String(),
		EventType:   eventType,
		AggregateID: aggregateID,
		Payload:     string(payloadJSON),
		RequestID:   requestID.(string),
		Status:      models.EventStatusPending,
		CreatedAt:   time.Now(),
	}

	// Insert event (VisibleAt will be set by BeforeCreate hook)
	if err := tx.Create(event).Error; err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

// CreateDirectory creates a new directory using optimistic locking with retries
func (s *DirectoryService) CreateDirectory(ctx context.Context, parentPath, name string, opaPolicyID *string) (*models.Directory, error) {
	// Validate name
	if name == "" || name == "." || name == ".." {
		return nil, fmt.Errorf("invalid directory name")
	}

	// Reject path separators (both Unix and Windows style)
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("invalid directory name")
	}

	// Reject control characters (null bytes, etc.)
	for _, r := range name {
		if r < 32 || r == 127 { // Control characters
			return nil, fmt.Errorf("invalid directory name")
		}
	}

	// Calculate full path
	fullPath := path.Join(parentPath, name)

	// Check depth limit (100 levels)
	depth := strings.Count(fullPath, "/")
	if depth > 100 {
		return nil, fmt.Errorf("directory tree depth limit exceeded (max 100 levels)")
	}

	// Retry loop for optimistic locking
	var dir *models.Directory
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := calculateBackoff(attempt - 1)
			time.Sleep(backoff)
		}

		// Try to create directory (lock-free)
		dir, lastErr = s.tryCreateDirectory(ctx, parentPath, fullPath, name, opaPolicyID)

		if lastErr == nil {
			// Success!
			return dir, nil
		}

		// Check if this is a retryable error
		if isDuplicateKeyError(lastErr) {
			// Directory was created concurrently, check if it's the same one
			var existing models.Directory
			if err := s.db.Where("path = ? AND deleted_at IS NULL", fullPath).First(&existing).Error; err == nil {
				// Directory exists - return "already exists" error
				return nil, fmt.Errorf("directory already exists: %s", fullPath)
			}
			// Duplicate was from a different path collision, retry
			continue
		}

		if strings.Contains(lastErr.Error(), "parent directory not found") {
			// Parent doesn't exist yet (might be created concurrently), retry
			continue
		}

		// Non-retryable error
		return nil, lastErr
	}

	// Max retries exceeded
	return nil, fmt.Errorf("failed to create directory after %d attempts: %w", maxRetries, lastErr)
}

// tryCreateDirectory attempts to create a directory without locks (single attempt)
func (s *DirectoryService) tryCreateDirectory(ctx context.Context, parentPath, fullPath, name string, opaPolicyID *string) (*models.Directory, error) {
	var dir *models.Directory

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Verify parent exists (no lock - optimistic approach)
		var parent models.Directory
		if parentPath == "/" {
			// Look up root directory
			if err := tx.Where("path = ? AND deleted_at IS NULL", "/").First(&parent).Error; err != nil {
				if err != gorm.ErrRecordNotFound {
					return err
				}
				// Root doesn't exist in DB, that's okay for root-level creates
			}
		} else {
			if err := tx.Where("path = ? AND deleted_at IS NULL", parentPath).First(&parent).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return fmt.Errorf("parent directory not found: %s", parentPath)
				}
				return err
			}
		}

		// Create directory
		pathHash := calculatePathHash(fullPath)
		dir = &models.Directory{
			ID:          uuid.New().String(),
			Name:        name,
			Path:        fullPath,
			PathHash:    pathHash,
			Version:     1,
			OPAPolicyID: opaPolicyID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if parent.ID != "" {
			dir.ParentID = &parent.ID
		}

		// Insert - will fail if path_hash conflicts (another concurrent create)
		if err := tx.Create(dir).Error; err != nil {
			return err
		}

		// Emit directory.created event
		if err := s.emitEvent(ctx, tx, "directory.created", dir.ID, map[string]interface{}{
			"directory_id":   dir.ID,
			"name":           dir.Name,
			"path":           dir.Path,
			"parent_id":      dir.ParentID,
			"opa_policy_id":  dir.OPAPolicyID,
		}); err != nil {
			return err
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

	// Normalize path - strip trailing slashes (except for root)
	if dirPath != "/" && strings.HasSuffix(dirPath, "/") {
		dirPath = strings.TrimRight(dirPath, "/")
	}

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
		// For root directory, match directories with parent_id = root, NULL, or empty string
		query = query.Where("parent_id IS NULL OR parent_id = '' OR parent_id = 'root'")
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
func (s *DirectoryService) DeleteDirectory(ctx context.Context, dirPath string, recursive bool) error {
	// Prevent deleting root directory
	if dirPath == "/" {
		return fmt.Errorf("cannot delete root directory")
	}

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

		// Emit directory.deleted event
		if err := s.emitEvent(ctx, tx, "directory.deleted", dir.ID, map[string]interface{}{
			"directory_id": dir.ID,
			"name":         dir.Name,
			"path":         dir.Path,
			"recursive":    recursive,
		}); err != nil {
			return err
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
		if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).First(&dir).Error; err != nil {
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

package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/storage"
	"gorm.io/gorm"
)

const (
	MaxFileSize    = 104857600 // 100MB
	JSONThreshold  = 16777216  // 16MB
	MaxVersions    = 10         // Keep last 10 versions
)

// FileService handles file operations
type FileService struct {
	db      *gorm.DB
	storage storage.Storage
}

// NewFileService creates a new file service
func NewFileService(db *gorm.DB, storage storage.Storage) *FileService {
	return &FileService{
		db:      db,
		storage: storage,
	}
}

// CreateFile creates a new file
func (s *FileService) CreateFile(ctx context.Context, directoryPath, name, contentType string, size int64, content io.Reader) (*models.File, error) {
	// Validate size
	if size > MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
	}

	// Validate name
	if name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid file name")
	}

	// Read content into buffer for checksum and storage
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, content); err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	contentBytes := buf.Bytes()

	// Calculate checksum
	hash := sha256.Sum256(contentBytes)
	checksum := hex.EncodeToString(hash[:])

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find directory
		var dir models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", directoryPath).First(&dir).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("directory not found: %s", directoryPath)
			}
			return err
		}

		// Check if file already exists
		var existing models.File
		err := tx.Where("directory_id = ? AND name = ? AND deleted_at IS NULL", dir.ID, name).First(&existing).Error
		if err == nil {
			return fmt.Errorf("file already exists: %s", name)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var s3Key *string

		if size < JSONThreshold && s.isJSON(contentBytes) {
			// Store as JSON in MySQL
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr
		} else {
			// Store in S3
			storageType = models.StorageTypeS3
			key := fmt.Sprintf("files/%s/%s", dir.ID, uuid.New().String())

			if err := s.storage.Put(ctx, key, bytes.NewReader(contentBytes)); err != nil {
				return fmt.Errorf("failed to upload to S3: %w", err)
			}
			s3Key = &key
		}

		// Create file record
		file = &models.File{
			ID:             uuid.New().String(),
			DirectoryID:    dir.ID,
			Name:           name,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			Version:        1,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := tx.Create(file).Error; err != nil {
			// Rollback S3 upload if DB insert fails
			if s3Key != nil {
				s.storage.Delete(ctx, *s3Key)
			}
			return fmt.Errorf("failed to create file record: %w", err)
		}

		// Create initial version
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  1,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create file version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return file, nil
}

// GetFile retrieves file metadata and content
func (s *FileService) GetFile(ctx context.Context, filePath string) (*models.File, io.ReadCloser, error) {
	// Parse path
	dirPath, fileName := s.parsePath(filePath)

	// Find file
	var file models.File
	err := s.db.Joins("JOIN directories ON directories.id = files.directory_id").
		Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
		First(&file).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, nil, err
	}

	// Get content
	var reader io.ReadCloser
	if file.StorageType == models.StorageTypeJSON {
		reader = io.NopCloser(strings.NewReader(*file.JSONContent))
	} else {
		r, err := s.storage.Get(ctx, *file.S3Key)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to retrieve from S3: %w", err)
		}
		reader = r
	}

	return &file, reader, nil
}

// UpdateFile updates an existing file
func (s *FileService) UpdateFile(ctx context.Context, filePath, contentType string, size int64, content io.Reader, expectedVersion int64) (*models.File, error) {
	if size > MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes", size, MaxFileSize)
	}

	// Read content
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, content); err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	contentBytes := buf.Bytes()

	// Calculate checksum
	hash := sha256.Sum256(contentBytes)
	checksum := hex.EncodeToString(hash[:])

	dirPath, fileName := s.parsePath(filePath)

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find and lock file
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
			Clauses(gorm.Expr("FOR UPDATE")).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("file not found: %s", filePath)
			}
			return err
		}

		// Check version for optimistic locking
		if file.Version != expectedVersion {
			return fmt.Errorf("version conflict: expected %d, got %d", expectedVersion, file.Version)
		}

		// Determine storage type
		var storageType models.StorageType
		var jsonContent *string
		var s3Key *string
		var oldS3Key *string

		if size < JSONThreshold && s.isJSON(contentBytes) {
			storageType = models.StorageTypeJSON
			contentStr := string(contentBytes)
			jsonContent = &contentStr

			// If old version was S3, mark for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		} else {
			storageType = models.StorageTypeS3
			key := fmt.Sprintf("files/%s/%s", file.DirectoryID, uuid.New().String())

			if err := s.storage.Put(ctx, key, bytes.NewReader(contentBytes)); err != nil {
				return fmt.Errorf("failed to upload to S3: %w", err)
			}
			s3Key = &key

			// Mark old S3 key for cleanup
			if file.StorageType == models.StorageTypeS3 {
				oldS3Key = file.S3Key
			}
		}

		// Update file
		file.ContentType = contentType
		file.SizeBytes = size
		file.StorageType = storageType
		file.JSONContent = jsonContent
		file.S3Key = s3Key
		file.ChecksumSHA256 = checksum
		file.Version++
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}

		// Create new version
		version := &models.FileVersion{
			ID:             uuid.New().String(),
			FileID:         file.ID,
			VersionNumber:  file.Version,
			ContentType:    contentType,
			SizeBytes:      size,
			StorageType:    storageType,
			JSONContent:    jsonContent,
			S3Key:          s3Key,
			ChecksumSHA256: checksum,
			CreatedAt:      time.Now(),
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create file version: %w", err)
		}

		// Cleanup old versions (keep last MaxVersions)
		if err := s.cleanupOldVersions(tx, file.ID); err != nil {
			return err
		}

		// Schedule old S3 key for deletion
		if oldS3Key != nil {
			// In a real system, this would be done asynchronously
			go s.storage.Delete(ctx, *oldS3Key)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return file, nil
}

// DeleteFile deletes a file
func (s *FileService) DeleteFile(ctx context.Context, filePath string) error {
	dirPath, fileName := s.parsePath(filePath)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Find and lock file
		var file models.File
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", dirPath, fileName).
			Clauses(gorm.Expr("FOR UPDATE")).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("file not found: %s", filePath)
			}
			return err
		}

		// Soft delete
		if err := tx.Delete(&file).Error; err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}

		// Schedule S3 cleanup (async in real system)
		if file.StorageType == models.StorageTypeS3 && file.S3Key != nil {
			go s.storage.Delete(ctx, *file.S3Key)
		}

		return nil
	})
}

// MoveFile moves a file to a different directory or renames it
func (s *FileService) MoveFile(ctx context.Context, sourcePath, destPath string) (*models.File, error) {
	srcDir, srcName := s.parsePath(sourcePath)
	dstDir, dstName := s.parsePath(destPath)

	var file *models.File
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Find source file
		err := tx.Joins("JOIN directories ON directories.id = files.directory_id").
			Where("directories.path = ? AND files.name = ? AND files.deleted_at IS NULL", srcDir, srcName).
			Clauses(gorm.Expr("FOR UPDATE")).
			First(&file).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("source file not found: %s", sourcePath)
			}
			return err
		}

		// Find destination directory
		var destDir models.Directory
		if err := tx.Where("path = ? AND deleted_at IS NULL", dstDir).First(&destDir).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("destination directory not found: %s", dstDir)
			}
			return err
		}

		// Check if destination file already exists
		var existing models.File
		err = tx.Where("directory_id = ? AND name = ? AND deleted_at IS NULL", destDir.ID, dstName).First(&existing).Error
		if err == nil {
			return fmt.Errorf("destination file already exists: %s", destPath)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// Update file
		file.DirectoryID = destDir.ID
		file.Name = dstName
		file.UpdatedAt = time.Now()

		if err := tx.Save(file).Error; err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return file, nil
}

// cleanupOldVersions keeps only the last MaxVersions versions
func (s *FileService) cleanupOldVersions(tx *gorm.DB, fileID string) error {
	var versions []models.FileVersion
	if err := tx.Where("file_id = ?", fileID).Order("version_number DESC").Find(&versions).Error; err != nil {
		return err
	}

	if len(versions) <= MaxVersions {
		return nil
	}

	// Delete old versions
	for i := MaxVersions; i < len(versions); i++ {
		if err := tx.Delete(&versions[i]).Error; err != nil {
			return err
		}

		// Schedule S3 cleanup for old versions
		if versions[i].StorageType == models.StorageTypeS3 && versions[i].S3Key != nil {
			// Async cleanup
			go s.storage.Delete(context.Background(), *versions[i].S3Key)
		}
	}

	return nil
}

// isJSON checks if content is valid JSON
func (s *FileService) isJSON(content []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(content, &js) == nil
}

// parsePath splits a file path into directory and filename
func (s *FileService) parsePath(filePath string) (string, string) {
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash == -1 {
		return "/", filePath
	}
	if lastSlash == 0 {
		return "/", filePath[1:]
	}
	return filePath[:lastSlash], filePath[lastSlash+1:]
}

package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"github.com/telnet2/mysql-vfs/pkg/persistence/storage"
	"gorm.io/gorm"
)

// GormFileRepository implements FileRepository using GORM
type GormFileRepository struct {
	db      *gorm.DB
	storage storage.Storage
}

// NewGormFileRepository creates a new GORM file repository
func NewGormFileRepository(db *gorm.DB, storage storage.Storage) *GormFileRepository {
	return &GormFileRepository{
		db:      db,
		storage: storage,
	}
}

// Create creates a new file
func (r *GormFileRepository) Create(ctx context.Context, file *models.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}

// FindByID finds a file by ID
func (r *GormFileRepository) FindByID(ctx context.Context, id string) (*models.File, error) {
	var file models.File
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&file).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &file, err
}

// FindByDirectoryAndName finds a file by directory ID and name
func (r *GormFileRepository) FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error) {
	var file models.File
	err := r.db.WithContext(ctx).
		Where("directory_id = ? AND name = ? AND deleted_at IS NULL", dirID, name).
		First(&file).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &file, err
}

// FindByDirectoryID finds all files in a directory
func (r *GormFileRepository) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	query := r.db.WithContext(ctx).
		Where("directory_id = ? AND deleted_at IS NULL", dirID).
		Order("name ASC")

	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	if limit > 0 {
		query = query.Limit(limit + 1)
	}

	var files []*models.File
	if err := query.Find(&files).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if limit > 0 && len(files) > limit {
		nextCursor = files[limit-1].ID
		files = files[:limit]
	}

	return files, nextCursor, nil
}

// Update updates a file
func (r *GormFileRepository) Update(ctx context.Context, file *models.File) error {
	result := r.db.WithContext(ctx).
		Where("version = ?", file.Version).
		Updates(file)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrConflict
	}

	return nil
}

// Delete permanently deletes a file
func (r *GormFileRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Delete(&models.File{}, "id = ?", id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrNotFound
	}

	return nil
}

// SoftDelete soft deletes a file
func (r *GormFileRepository) SoftDelete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Delete(&models.File{}, "id = ?", id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrNotFound
	}

	return nil
}

// CreateVersion creates a new file version
func (r *GormFileRepository) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	return r.db.WithContext(ctx).Create(version).Error
}

// GetLatestVersion gets the latest version of a file
func (r *GormFileRepository) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	var version models.FileVersion
	err := r.db.WithContext(ctx).
		Where("file_id = ?", fileID).
		Order("version_number DESC").
		First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &version, err
}

// ListVersions lists all versions of a file (latest first)
func (r *GormFileRepository) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	var versions []*models.FileVersion
	err := r.db.WithContext(ctx).
		Where("file_id = ?", fileID).
		Order("version_number DESC").
		Find(&versions).Error
	return versions, err
}

// Exists checks if a file exists with the given directory ID and name
func (r *GormFileRepository) Exists(ctx context.Context, dirID, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.File{}).
		Where("directory_id = ? AND name = ? AND deleted_at IS NULL", dirID, name).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Storage decision constants
const (
	JSONThreshold = 16777216 // 16MB
)

// CreateFile creates a new file with content (repository handles storage decision)
func (r *GormFileRepository) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	size := int64(len(content))

	// REPOSITORY DECIDES STORAGE
	if size < JSONThreshold && isJSON(content) {
		file.StorageType = models.StorageTypeJSON
		contentStr := string(content)
		file.JSONContent = &contentStr
	} else if size < JSONThreshold && isValidUTF8(content) {
		file.StorageType = models.StorageTypeText
		contentStr := string(content)
		file.TextContent = &contentStr
	} else {
		// Upload to S3
		file.StorageType = models.StorageTypeS3
		key := fmt.Sprintf("files/%s/%s", file.DirectoryID, uuid.New().String())
		if err := r.storage.Put(ctx, key, bytes.NewReader(content)); err != nil {
			return fmt.Errorf("failed to upload to S3: %w", err)
		}
		file.S3Key = &key
	}

	// Save to MySQL
	return r.db.WithContext(ctx).Create(file).Error
}

// GetFileContent retrieves file content from storage
func (r *GormFileRepository) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	switch file.StorageType {
	case models.StorageTypeJSON:
		if file.JSONContent == nil {
			return nil, fmt.Errorf("JSON content is nil")
		}
		return []byte(*file.JSONContent), nil
	case models.StorageTypeText:
		if file.TextContent == nil {
			return nil, fmt.Errorf("text content is nil")
		}
		return []byte(*file.TextContent), nil
	case models.StorageTypeS3:
		if file.S3Key == nil {
			return nil, fmt.Errorf("S3 key is nil")
		}
		reader, err := r.storage.Get(ctx, *file.S3Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get from S3: %w", err)
		}
		defer reader.Close()

		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, reader); err != nil {
			return nil, fmt.Errorf("failed to read S3 content: %w", err)
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unknown storage type: %s", file.StorageType)
	}
}

// UpdateFile updates a file with new content (repository handles storage decision)
func (r *GormFileRepository) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	size := int64(len(content))
	var oldS3Key *string

	// Save old S3 key for cleanup
	if file.StorageType == models.StorageTypeS3 {
		oldS3Key = file.S3Key
	}

	// Determine new storage type
	if size < JSONThreshold && isJSON(content) {
		file.StorageType = models.StorageTypeJSON
		contentStr := string(content)
		file.JSONContent = &contentStr
		file.TextContent = nil
		file.S3Key = nil
	} else if size < JSONThreshold && isValidUTF8(content) {
		file.StorageType = models.StorageTypeText
		contentStr := string(content)
		file.TextContent = &contentStr
		file.JSONContent = nil
		file.S3Key = nil
	} else {
		// Upload to S3
		file.StorageType = models.StorageTypeS3
		key := fmt.Sprintf("files/%s/%s", file.DirectoryID, uuid.New().String())
		if err := r.storage.Put(ctx, key, bytes.NewReader(content)); err != nil {
			return fmt.Errorf("failed to upload to S3: %w", err)
		}
		file.S3Key = &key
		file.JSONContent = nil
		file.TextContent = nil
	}

	// Update in MySQL
	if err := r.db.WithContext(ctx).Save(file).Error; err != nil {
		return err
	}

	// Cleanup old S3 key
	if oldS3Key != nil && (file.StorageType != models.StorageTypeS3 || *file.S3Key != *oldS3Key) {
		go r.storage.Delete(context.Background(), *oldS3Key)
	}

	return nil
}

// Helper functions

// isJSON checks if content is valid JSON
func isJSON(content []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(content, &js) == nil
}

// isValidUTF8 checks if content is valid UTF-8 text (not binary)
func isValidUTF8(content []byte) bool {
	return strings.ToValidUTF8(string(content), "") == string(content)
}

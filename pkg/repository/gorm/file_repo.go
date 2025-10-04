package gorm

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/repository"
	"gorm.io/gorm"
)

// GormFileRepository implements FileRepository using GORM
type GormFileRepository struct {
	db *gorm.DB
}

// NewGormFileRepository creates a new GORM file repository
func NewGormFileRepository(db *gorm.DB) *GormFileRepository {
	return &GormFileRepository{db: db}
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
		return nil, repository.ErrNotFound
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
		return nil, repository.ErrNotFound
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
		return repository.ErrConflict
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
		return repository.ErrNotFound
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
		return repository.ErrNotFound
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
		Order("version DESC").
		First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, repository.ErrNotFound
	}
	return &version, err
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

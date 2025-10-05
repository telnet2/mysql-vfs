package mysql

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormDirectoryRepository implements DirectoryRepository using GORM
type GormDirectoryRepository struct {
	db *gorm.DB
}

// NewGormDirectoryRepository creates a new GORM directory repository
func NewGormDirectoryRepository(db *gorm.DB) *GormDirectoryRepository {
	return &GormDirectoryRepository{db: db}
}

// Create creates a new directory
func (r *GormDirectoryRepository) Create(ctx context.Context, dir *models.Directory) error {
	return r.db.WithContext(ctx).Create(dir).Error
}

// FindByID finds a directory by ID
func (r *GormDirectoryRepository) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	var dir models.Directory
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&dir).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &dir, err
}

// FindByPath finds a directory by its full path
func (r *GormDirectoryRepository) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	var dir models.Directory
	err := r.db.WithContext(ctx).
		Where("path = ? AND deleted_at IS NULL", path).
		First(&dir).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &dir, err
}

// FindByParentID finds all directories under a parent directory
func (r *GormDirectoryRepository) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	query := r.db.WithContext(ctx).
		Where("parent_id = ? AND deleted_at IS NULL", parentID).
		Order("name ASC")

	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	if limit > 0 {
		query = query.Limit(limit + 1) // Fetch one extra to determine if there's a next page
	}

	var dirs []*models.Directory
	if err := query.Find(&dirs).Error; err != nil {
		return nil, "", err
	}

	// Determine next cursor
	var nextCursor string
	if limit > 0 && len(dirs) > limit {
		nextCursor = dirs[limit-1].ID
		dirs = dirs[:limit]
	}

	return dirs, nextCursor, nil
}

// Update updates a directory
func (r *GormDirectoryRepository) Update(ctx context.Context, dir *models.Directory) error {
	result := r.db.WithContext(ctx).
		Where("version = ?", dir.Version).
		Updates(dir)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrConflict // Optimistic locking failure
	}

	return nil
}

// Delete permanently deletes a directory
func (r *GormDirectoryRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Delete(&models.Directory{}, "id = ?", id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrNotFound
	}

	return nil
}

// SoftDelete soft deletes a directory
func (r *GormDirectoryRepository) SoftDelete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Delete(&models.Directory{}, "id = ?", id)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrNotFound
	}

	return nil
}

// LockPaths acquires locks on the specified paths (for tree locking)
func (r *GormDirectoryRepository) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	gormTx := tx.GetDB().(*gorm.DB)

	for _, p := range paths {
		var dir models.Directory
		err := gormTx.WithContext(ctx).
			Where("path = ? AND deleted_at IS NULL", p).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&dir).Error

		// It's OK if the directory doesn't exist (we're locking ancestors)
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
	}

	return nil
}

// Exists checks if a directory exists at the given path
func (r *GormDirectoryRepository) Exists(ctx context.Context, path string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Directory{}).
		Where("path = ? AND deleted_at IS NULL", path).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

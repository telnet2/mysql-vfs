package models

import (
	"time"

	"gorm.io/gorm"
)

// Directory represents a directory in the VFS
type Directory struct {
	ID           string         `gorm:"type:char(36);primaryKey"`
	ParentID     *string        `gorm:"type:char(36);index:idx_parent_name"`
	Name         string         `gorm:"type:varchar(255);not null;index:idx_parent_name"`
	Path         string         `gorm:"type:text;not null"` // Changed to text, use path_hash for uniqueness
	PathHash     string         `gorm:"type:char(64);not null;uniqueIndex"` // SHA256 hash for uniqueness
	Version      int64          `gorm:"not null;default:1"`
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	Parent   *Directory  `gorm:"-"`
	Children []Directory `gorm:"-"`
	Files    []File      `gorm:"-"`
}

func (Directory) TableName() string {
	return "directories"
}

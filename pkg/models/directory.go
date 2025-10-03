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
	Path         string         `gorm:"type:varchar(4096);not null;uniqueIndex"`
	Version      int64          `gorm:"not null;default:1"`
	OPAPolicyID  *string        `gorm:"type:char(36)"`
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	Parent   *Directory  `gorm:"-:migration"`
	Children []Directory `gorm:"-:migration"`
	Files    []File      `gorm:"-:migration"`
	OPAPolicy *OPAPolicy `gorm:"-:migration"`
}

func (Directory) TableName() string {
	return "directories"
}

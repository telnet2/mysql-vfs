package models

import (
	"time"
)

// FileRelation represents a parent-derivative relationship between files
type FileRelation struct {
	ID               string    `gorm:"type:char(36);primaryKey"`
	ParentFileID     string    `gorm:"type:char(36);not null;uniqueIndex:idx_parent_derivative"`
	DerivativeFileID string    `gorm:"type:char(36);not null;uniqueIndex:idx_parent_derivative"`
	RelationType     string    `gorm:"type:varchar(100);not null"`
	MetadataJSON     *string   `gorm:"type:json"`
	CreatedAt        time.Time `gorm:"not null"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	ParentFile     *File `gorm:"-"`
	DerivativeFile *File `gorm:"-"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

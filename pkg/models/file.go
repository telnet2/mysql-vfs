package models

import (
	"time"

	"gorm.io/gorm"
)

// StorageType represents where file content is stored
type StorageType string

const (
	StorageTypeJSON StorageType = "json"
	StorageTypeText StorageType = "text"
	StorageTypeS3   StorageType = "s3"
)

// File represents a file in the VFS
type File struct {
	ID           string         `gorm:"type:char(36);primaryKey"`
	DirectoryID  string         `gorm:"type:char(36);not null;index:idx_dir_name"`
	Name         string         `gorm:"type:varchar(255);not null;index:idx_dir_name"`
	ContentType  string         `gorm:"type:varchar(255);not null"`
	SizeBytes    int64          `gorm:"not null"`
	StorageType  StorageType    `gorm:"type:varchar(10);not null"`
	JSONContent  *string        `gorm:"type:json"`
	TextContent  *string        `gorm:"type:mediumtext"`
	S3Key        *string        `gorm:"type:varchar(1024)"`
	ChecksumSHA256 string       `gorm:"type:char(64);not null;index"`
	Version      int64          `gorm:"not null;default:1"`
	Metadata     *string        `gorm:"type:json"` // JSON metadata: owner, creator, system flags
	CreatedAt    time.Time      `gorm:"not null"`
	UpdatedAt    time.Time      `gorm:"not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	Directory         *Directory      `gorm:"-"`
	Versions          []FileVersion   `gorm:"-"`
	ParentRelations   []FileRelation  `gorm:"-"`
	DerivativeRelations []FileRelation `gorm:"-"`
}

func (File) TableName() string {
	return "files"
}

// BeforeCreate hook to validate constraints
func (f *File) BeforeCreate(tx *gorm.DB) error {
	// Validate 100MB limit
	if f.SizeBytes > 104857600 {
		return gorm.ErrInvalidData
	}

	// Validate storage type consistency
	if f.StorageType == StorageTypeJSON && f.JSONContent == nil {
		return gorm.ErrInvalidData
	}
	if f.StorageType == StorageTypeText {
		if f.TextContent == nil {
			return gorm.ErrInvalidData
		}
		// Text content limited to 100MB (same as JSON)
		if f.SizeBytes > 104857600 {
			return gorm.ErrInvalidData
		}
	}
	if f.StorageType == StorageTypeS3 && f.S3Key == nil {
		return gorm.ErrInvalidData
	}

	return nil
}

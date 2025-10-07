package models

import (
	"time"
)

// FileVersion represents an immutable version of a file
type FileVersion struct {
	ID             string      `gorm:"type:char(36);primaryKey"`
	FileID         string      `gorm:"type:char(36);not null;index:idx_file_version"`
	VersionNumber  int64       `gorm:"not null;index:idx_file_version"`
	ContentType    string      `gorm:"type:varchar(255);not null"`
	SizeBytes      int64       `gorm:"not null"`
	StorageType    StorageType `gorm:"type:varchar(10);not null"`
	JSONContent    *string     `gorm:"type:json"`
	TextContent    *string     `gorm:"type:mediumtext"`
	S3Key          *string     `gorm:"type:varchar(1024)"`
	ChecksumSHA256 string      `gorm:"type:char(64);not null"`
	Metadata       *string     `gorm:"type:json"` // JSON metadata: owner, creator, system flags
	CreatedAt      time.Time   `gorm:"not null"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	File *File `gorm:"-"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

package models

import (
	"time"
)

// AuditLog represents an audit trail entry for system operations
type AuditLog struct {
	ID           string    `gorm:"type:char(36);primaryKey"`
	RequestID    string    `gorm:"type:char(36);not null;index"`
	UserID       *string   `gorm:"type:char(36);index:idx_created_user"`
	Action       string    `gorm:"type:varchar(100);not null"`
	ResourceType string    `gorm:"type:varchar(50);not null"`
	ResourceID   string    `gorm:"type:char(36);not null;index"`
	IPAddress    string    `gorm:"type:varchar(45)"`
	UserAgent    *string   `gorm:"type:varchar(512)"`
	Status       string    `gorm:"type:varchar(20);not null"`
	DurationMS   int64     `gorm:"not null"`
	CreatedAt    time.Time `gorm:"not null;index:idx_created_user"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

// DeadLetterQueue stores events and jobs that failed repeatedly
type DeadLetterQueue struct {
	ID            string    `gorm:"type:char(36);primaryKey"`
	OriginalTable string    `gorm:"type:varchar(50);not null"`
	OriginalID    string    `gorm:"type:char(36);not null"`
	Payload       string    `gorm:"type:json;not null"`
	FailureReason string    `gorm:"type:text;not null"`
	FailureCount  int       `gorm:"not null"`
	MovedAt       time.Time `gorm:"not null;index"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

package models

import (
	"time"
)

// IdempotencyRecord stores cached responses for idempotent operations
type IdempotencyRecord struct {
	RequestID    string    `gorm:"type:char(36);primaryKey"`
	ResponseHash string    `gorm:"type:char(64);not null"`
	ResponseBody string    `gorm:"type:text;not null"`
	ExpiresAt    time.Time `gorm:"not null;index"`
	CreatedAt    time.Time `gorm:"not null"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

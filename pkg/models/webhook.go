package models

import (
	"time"

	"gorm.io/gorm"
)

// CircuitState represents the state of a circuit breaker
type CircuitState string

const (
	CircuitStateClosed   CircuitState = "closed"
	CircuitStateOpen     CircuitState = "open"
	CircuitStateHalfOpen CircuitState = "half_open"
)

// WebhookConfig represents a webhook endpoint configuration
type WebhookConfig struct {
	ID                  string       `gorm:"type:char(36);primaryKey"`
	DirectoryID         *string      `gorm:"type:char(36);index"`
	EventType           string       `gorm:"type:varchar(100);not null;index:idx_event_active"`
	TargetURL           string       `gorm:"type:varchar(2048);not null"`
	Secret              string       `gorm:"type:varchar(255);not null"`
	IsActive            bool         `gorm:"not null;default:true;index:idx_event_active"`
	CircuitState        CircuitState `gorm:"type:varchar(20);not null;default:'closed'"`
	CircuitOpenedAt     *time.Time
	ConsecutiveFailures int       `gorm:"not null;default:0"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
	DeletedAt           gorm.DeletedAt `gorm:"index"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	Directory    *Directory   `gorm:"-"`
	WebhookJobs  []WebhookJob `gorm:"-"`
}

func (WebhookConfig) TableName() string {
	return "webhook_configs"
}

// WebhookJobStatus represents the delivery status of a webhook
type WebhookJobStatus string

const (
	WebhookJobStatusPending      WebhookJobStatus = "pending"
	WebhookJobStatusSent         WebhookJobStatus = "sent"
	WebhookJobStatusAcknowledged WebhookJobStatus = "acknowledged"
	WebhookJobStatusFailed       WebhookJobStatus = "failed"
)

// WebhookJob represents a webhook delivery attempt
type WebhookJob struct {
	ID              string           `gorm:"type:char(36);primaryKey"`
	EventID         string           `gorm:"type:char(36);not null;index"`
	WebhookConfigID string           `gorm:"type:char(36);not null;index"`
	IdempotencyKey  string           `gorm:"type:varchar(100);uniqueIndex"`
	Status          WebhookJobStatus `gorm:"type:varchar(20);not null;index:idx_status_retry"`
	AttemptCount    int              `gorm:"not null;default:0"`
	NextRetryAt     *time.Time       `gorm:"index:idx_status_retry"`
	LastError       *string          `gorm:"type:text"`
	CreatedAt       time.Time        `gorm:"not null"`
	UpdatedAt       time.Time        `gorm:"not null"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	Event         *Event         `gorm:"-"`
	WebhookConfig *WebhookConfig `gorm:"-"`
}

func (WebhookJob) TableName() string {
	return "webhook_jobs"
}

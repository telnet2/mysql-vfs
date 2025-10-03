package models

import (
	"time"

	"gorm.io/gorm"
)

// EventStatus represents the processing status of an event
type EventStatus string

const (
	EventStatusPending     EventStatus = "pending"
	EventStatusProcessing  EventStatus = "processing"
	EventStatusCompleted   EventStatus = "completed"
	EventStatusFailed      EventStatus = "failed"
	EventStatusDeadLetter  EventStatus = "dead_letter"
)

// Event represents a system event in the transactional outbox
type Event struct {
	ID                  string      `gorm:"type:char(36);primaryKey"`
	EventType           string      `gorm:"type:varchar(100);not null;index:idx_status_visible"`
	AggregateID         string      `gorm:"type:char(36);not null;index"`
	Payload             string      `gorm:"type:json;not null"`
	RequestID           string      `gorm:"type:char(36);not null;index"`
	Status              EventStatus `gorm:"type:varchar(20);not null;index:idx_status_visible"`
	VisibleAt           time.Time   `gorm:"not null;index:idx_status_visible"`
	ProcessingStartedAt *time.Time
	CompletedAt         *time.Time
	RetryCount          int         `gorm:"not null;default:0"`
	ErrorMessage        *string     `gorm:"type:text"`
	CreatedAt           time.Time   `gorm:"not null"`

	// Relations (no DB-level foreign keys - referential integrity managed in application code)
	WebhookJobs []WebhookJob `gorm:"-:migration"`
}

func (Event) TableName() string {
	return "events"
}

// BeforeCreate sets the visible_at timestamp (5 seconds delay)
func (e *Event) BeforeCreate(tx *gorm.DB) error {
	if e.VisibleAt.IsZero() {
		e.VisibleAt = time.Now().Add(5 * time.Second)
	}
	return nil
}

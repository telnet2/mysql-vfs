package models

import (
	"time"

	"gorm.io/gorm"
)

// CronJob represents a scheduled job configuration
type CronJob struct {
	ID              string         `gorm:"type:char(36);primaryKey"`
	Name            string         `gorm:"type:varchar(255);not null;uniqueIndex"`
	CronExpression  string         `gorm:"type:varchar(100);not null"`
	Timezone        string         `gorm:"type:varchar(50);not null;default:'UTC'"`
	HandlerType     string         `gorm:"type:varchar(100);not null"`
	Payload         string         `gorm:"type:json"`
	SkipMissedRuns  bool           `gorm:"not null;default:true"`
	IsActive        bool           `gorm:"not null;default:true;index"`
	CreatedAt       time.Time      `gorm:"not null"`
	UpdatedAt       time.Time      `gorm:"not null"`
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	// Relations
	Executions []CronExecution `gorm:"foreignKey:CronJobID"`
}

func (CronJob) TableName() string {
	return "cron_jobs"
}

// CronExecutionStatus represents the execution status
type CronExecutionStatus string

const (
	CronExecutionStatusPending   CronExecutionStatus = "pending"
	CronExecutionStatusRunning   CronExecutionStatus = "running"
	CronExecutionStatusCompleted CronExecutionStatus = "completed"
	CronExecutionStatusFailed    CronExecutionStatus = "failed"
	CronExecutionStatusRecovered CronExecutionStatus = "recovered"
)

// CronExecution represents a single execution of a cron job
type CronExecution struct {
	ID             string              `gorm:"type:char(36);primaryKey"`
	CronJobID      string              `gorm:"type:char(36);not null;index"`
	ExecutionKey   string              `gorm:"type:varchar(100);uniqueIndex"`
	ScheduledAt    time.Time           `gorm:"not null"`
	LeaseHolderID  *string             `gorm:"type:varchar(100)"`
	LeaseExpiresAt *time.Time          `gorm:"index:idx_status_lease"`
	HeartbeatAt    *time.Time
	Status         CronExecutionStatus `gorm:"type:varchar(20);not null;index:idx_status_lease"`
	StartedAt      *time.Time
	CompletedAt    *time.Time
	ErrorMessage   *string             `gorm:"type:text"`
	CreatedAt      time.Time           `gorm:"not null"`

	// Relations
	CronJob *CronJob `gorm:"foreignKey:CronJobID"`
}

func (CronExecution) TableName() string {
	return "cron_executions"
}

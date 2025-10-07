package models

import "time"

// WorkflowAudit records workflow operations performed on files
type WorkflowAudit struct {
	ID             string    `gorm:"type:char(36);primaryKey"`
	FilePath       string    `gorm:"type:varchar(512);not null;index:idx_file_path"`
	WorkflowPath   string    `gorm:"type:varchar(512);not null;index:idx_workflow_path"`
	FromState      string    `gorm:"type:varchar(100);not null"`
	ToState        string    `gorm:"type:varchar(100);not null"`
	Operation      string    `gorm:"type:varchar(50);not null"`
	Actor          string    `gorm:"type:varchar(100);not null;index:idx_actor"`
	ActorGroups    string    `gorm:"type:json"`
	GatesEvaluated string    `gorm:"type:json"`
	Success        bool      `gorm:"not null"`
	ErrorMessage   *string   `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"not null;index:idx_created_at"`
}

// TableName is removed - GORM will use the naming strategy (which includes table prefix)

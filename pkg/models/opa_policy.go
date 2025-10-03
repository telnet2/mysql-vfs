package models

import (
	"time"

	"gorm.io/gorm"
)

// OPAPolicy represents an Open Policy Agent REGO policy
type OPAPolicy struct {
	ID                string         `gorm:"type:char(36);primaryKey"`
	Name              string         `gorm:"type:varchar(255);not null;uniqueIndex"`
	RegoScript        string         `gorm:"type:text;not null"`
	CompiledAt        *time.Time
	IsValid           bool           `gorm:"not null;default:false;index"`
	CompilationError  *string        `gorm:"type:text"`
	TimeoutMS         int            `gorm:"not null;default:200"`
	CreatedAt         time.Time      `gorm:"not null"`
	UpdatedAt         time.Time      `gorm:"not null"`
	DeletedAt         gorm.DeletedAt `gorm:"index"`

	// Relations
	Directories []Directory `gorm:"foreignKey:OPAPolicyID"`
}

func (OPAPolicy) TableName() string {
	return "opa_policies"
}

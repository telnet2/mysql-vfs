package mysql

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
)

// GormWorkflowAuditRepository persists workflow audit records using GORM
type GormWorkflowAuditRepository struct {
	db *gorm.DB
}

// NewGormWorkflowAuditRepository creates a new workflow audit repository
func NewGormWorkflowAuditRepository(db *gorm.DB) *GormWorkflowAuditRepository {
	return &GormWorkflowAuditRepository{db: db}
}

// Create inserts a new workflow audit record
func (r *GormWorkflowAuditRepository) Create(ctx context.Context, audit *models.WorkflowAudit) error {
	if audit == nil {
		return gorm.ErrInvalidData
	}
	if audit.ID == "" {
		audit.ID = uuid.NewString()
	}
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(audit).Error
}

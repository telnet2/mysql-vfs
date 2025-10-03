package observability

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/pkg/models"
)

// AuditLogger handles audit log creation
type AuditLogger struct {
	db *gorm.DB
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(db *gorm.DB) *AuditLogger {
	return &AuditLogger{db: db}
}

// AuditAction represents types of auditable actions
type AuditAction string

const (
	ActionDirectoryCreate AuditAction = "directory.create"
	ActionDirectoryDelete AuditAction = "directory.delete"
	ActionDirectoryMove   AuditAction = "directory.move"
	ActionDirectoryList   AuditAction = "directory.list"

	ActionFileCreate AuditAction = "file.create"
	ActionFileRead   AuditAction = "file.read"
	ActionFileUpdate AuditAction = "file.update"
	ActionFileDelete AuditAction = "file.delete"
	ActionFileMove   AuditAction = "file.move"

	ActionFileRelationCreate AuditAction = "file_relation.create"
	ActionFileRelationDelete AuditAction = "file_relation.delete"

	ActionWebhookCreate AuditAction = "webhook.create"
	ActionWebhookDelete AuditAction = "webhook.delete"
	ActionWebhookTrigger AuditAction = "webhook.trigger"

	ActionCronJobCreate AuditAction = "cron_job.create"
	ActionCronJobUpdate AuditAction = "cron_job.update"
	ActionCronJobDelete AuditAction = "cron_job.delete"
	ActionCronJobExecute AuditAction = "cron_job.execute"

	ActionIntegrityCheck AuditAction = "integrity.check"
	ActionIntegrityRepair AuditAction = "integrity.repair"
)

// AuditStatus represents the result status
type AuditStatus string

const (
	StatusSuccess AuditStatus = "success"
	StatusFailure AuditStatus = "failure"
	StatusDenied  AuditStatus = "denied"
)

// AuditEntry represents detailed audit information
type AuditEntry struct {
	RequestID    string                 `json:"request_id,omitempty"`
	UserID       string                 `json:"user_id,omitempty"`
	Action       AuditAction            `json:"action"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	Status       AuditStatus            `json:"status"`
	DurationMS   int64                  `json:"duration_ms"`
	Details      map[string]interface{} `json:"details,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// Log creates an audit log entry
func (a *AuditLogger) Log(ctx context.Context, entry AuditEntry) error {
	var userID *string
	if entry.UserID != "" {
		userID = &entry.UserID
	}

	var userAgent *string
	if entry.UserAgent != "" {
		userAgent = &entry.UserAgent
	}

	auditLog := &models.AuditLog{
		RequestID:    entry.RequestID,
		UserID:       userID,
		Action:       string(entry.Action),
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		IPAddress:    entry.IPAddress,
		UserAgent:    userAgent,
		Status:       string(entry.Status),
		DurationMS:   entry.DurationMS,
	}

	return a.db.WithContext(ctx).Create(auditLog).Error
}

// LogDirectoryCreate logs directory creation
func (a *AuditLogger) LogDirectoryCreate(ctx context.Context, requestID, userID, directoryID, path string, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionDirectoryCreate,
		ResourceType: "directory",
		ResourceID:   directoryID,
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"path": path,
		},
		ErrorMessage: errMsg,
	})
}

// LogDirectoryDelete logs directory deletion
func (a *AuditLogger) LogDirectoryDelete(ctx context.Context, requestID, userID, path string, recursive bool, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionDirectoryDelete,
		ResourceType: "directory",
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"path":      path,
			"recursive": recursive,
		},
		ErrorMessage: errMsg,
	})
}

// LogFileCreate logs file creation
func (a *AuditLogger) LogFileCreate(ctx context.Context, requestID, userID, fileID, path string, sizeBytes int64, contentType string, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionFileCreate,
		ResourceType: "file",
		ResourceID:   fileID,
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"path":         path,
			"size_bytes":   sizeBytes,
			"content_type": contentType,
		},
		ErrorMessage: errMsg,
	})
}

// LogFileRead logs file read/download
func (a *AuditLogger) LogFileRead(ctx context.Context, requestID, userID, fileID, path string, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionFileRead,
		ResourceType: "file",
		ResourceID:   fileID,
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"path": path,
		},
		ErrorMessage: errMsg,
	})
}

// LogFileUpdate logs file update
func (a *AuditLogger) LogFileUpdate(ctx context.Context, requestID, userID, fileID string, oldVersion, newVersion int64, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionFileUpdate,
		ResourceType: "file",
		ResourceID:   fileID,
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"old_version": oldVersion,
			"new_version": newVersion,
		},
		ErrorMessage: errMsg,
	})
}

// LogFileDelete logs file deletion
func (a *AuditLogger) LogFileDelete(ctx context.Context, requestID, userID, fileID, path string, durationMS int64, err error) {
	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailure
		errMsg = err.Error()
	}

	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       ActionFileDelete,
		ResourceType: "file",
		ResourceID:   fileID,
		Status:       status,
		DurationMS:   durationMS,
		Details: map[string]interface{}{
			"path": path,
		},
		ErrorMessage: errMsg,
	})
}

// LogAccessDenied logs access denied events
func (a *AuditLogger) LogAccessDenied(ctx context.Context, requestID, userID, action, resourceType, resourceID, reason string) {
	a.Log(ctx, AuditEntry{
		RequestID:    requestID,
		UserID:       userID,
		Action:       AuditAction(action),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Status:       StatusDenied,
		ErrorMessage: reason,
	})
}

// QueryOptions represents audit log query options
type QueryOptions struct {
	UserID       string
	Action       AuditAction
	ResourceType string
	ResourceID   string
	Status       AuditStatus
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}

// Query retrieves audit logs with filters
func (a *AuditLogger) Query(ctx context.Context, opts QueryOptions) ([]*models.AuditLog, error) {
	query := a.db.WithContext(ctx).Model(&models.AuditLog{})

	if opts.UserID != "" {
		query = query.Where("user_id = ?", opts.UserID)
	}

	if opts.Action != "" {
		query = query.Where("action = ?", string(opts.Action))
	}

	if opts.ResourceType != "" {
		query = query.Where("resource_type = ?", opts.ResourceType)
	}

	if opts.ResourceID != "" {
		query = query.Where("resource_id = ?", opts.ResourceID)
	}

	if opts.Status != "" {
		query = query.Where("status = ?", string(opts.Status))
	}

	if opts.StartTime != nil {
		query = query.Where("created_at >= ?", opts.StartTime)
	}

	if opts.EndTime != nil {
		query = query.Where("created_at <= ?", opts.EndTime)
	}

	query = query.Order("created_at DESC")

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	} else {
		query = query.Limit(100) // Default limit
	}

	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	var logs []*models.AuditLog
	err := query.Find(&logs).Error

	return logs, err
}

// GetByRequestID retrieves all audit logs for a specific request
func (a *AuditLogger) GetByRequestID(ctx context.Context, requestID string) ([]*models.AuditLog, error) {
	var logs []*models.AuditLog
	err := a.db.WithContext(ctx).
		Where("request_id = ?", requestID).
		Order("created_at ASC").
		Find(&logs).Error

	return logs, err
}

// GetStats returns audit log statistics
func (a *AuditLogger) GetStats(ctx context.Context, startTime, endTime time.Time) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total count
	var total int64
	if err := a.db.WithContext(ctx).Model(&models.AuditLog{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// Count by action
	var actionCounts []struct {
		Action string
		Count  int64
	}
	if err := a.db.WithContext(ctx).Model(&models.AuditLog{}).
		Select("action, COUNT(*) as count").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Group("action").
		Scan(&actionCounts).Error; err != nil {
		return nil, err
	}

	actionStats := make(map[string]int64)
	for _, ac := range actionCounts {
		actionStats[ac.Action] = ac.Count
	}
	stats["by_action"] = actionStats

	// Count by status
	var statusCounts []struct {
		Status string
		Count  int64
	}
	if err := a.db.WithContext(ctx).Model(&models.AuditLog{}).
		Select("status, COUNT(*) as count").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return nil, err
	}

	statusStats := make(map[string]int64)
	for _, sc := range statusCounts {
		statusStats[sc.Status] = sc.Count
	}
	stats["by_status"] = statusStats

	// Average duration
	var avgDuration float64
	if err := a.db.WithContext(ctx).Model(&models.AuditLog{}).
		Select("AVG(duration_ms) as avg").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Scan(&avgDuration).Error; err != nil {
		return nil, err
	}
	stats["avg_duration_ms"] = avgDuration

	return stats, nil
}

// Cleanup removes old audit logs beyond retention period
func (a *AuditLogger) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	result := a.db.WithContext(ctx).
		Where("created_at < ?", cutoffDate).
		Delete(&models.AuditLog{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup audit logs: %w", result.Error)
	}

	return result.RowsAffected, nil
}

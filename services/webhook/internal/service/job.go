package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type JobService struct {
	DB *gorm.DB
}

type AckInput struct {
	DeliveryID   string
	Success      bool
	Response     string
	ErrorMessage string
}

func NewJobService(db *gorm.DB) *JobService {
	return &JobService{DB: db}
}

func (s *JobService) MarkDelivered(ctx context.Context, jobID string) error {
	return s.DB.WithContext(ctx).
		Model(&db.WebhookJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":          "delivered",
			"retry_count":     gorm.Expr("retry_count"),
			"next_attempt_at": time.Now().Add(15 * time.Second),
			"updated_at":      time.Now(),
		}).Error
}

func (s *JobService) MarkFailed(ctx context.Context, jobID string, increment bool, errMsg string, backoff time.Duration) error {
	updates := map[string]any{
		"status":          "retry",
		"last_error":      errMsg,
		"next_attempt_at": time.Now().Add(backoff),
		"updated_at":      time.Now(),
	}
	if increment {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	return s.DB.WithContext(ctx).
		Model(&db.WebhookJob{}).
		Where("id = ?", jobID).
		Updates(updates).Error
}

func (s *JobService) Ack(ctx context.Context, in AckInput) error {
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if in.Success {
		updates["status"] = "acknowledged"
		updates["last_error"] = gorm.Expr("NULL")
		updates["next_attempt_at"] = time.Now()
	} else {
		updates["status"] = "retry"
		updates["last_error"] = in.ErrorMessage
		updates["next_attempt_at"] = time.Now().Add(30 * time.Second)
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	return s.DB.WithContext(ctx).
		Model(&db.WebhookJob{}).
		Where("id = ?", in.DeliveryID).
		Updates(updates).Error
}

func (s *JobService) FetchPending(ctx context.Context, limit int) ([]db.WebhookJob, error) {
	if limit <= 0 {
		limit = 1
	}
	var jobs []db.WebhookJob
	err := s.DB.WithContext(ctx).
		Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "retry"}, time.Now()).
		Order("next_attempt_at ASC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

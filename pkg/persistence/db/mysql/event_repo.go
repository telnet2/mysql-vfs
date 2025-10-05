package mysql

import (
	"context"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"gorm.io/gorm"
)

// GormEventRepository implements EventRepository using GORM
type GormEventRepository struct {
	db *gorm.DB
}

// NewGormEventRepository creates a new GORM event repository
func NewGormEventRepository(db *gorm.DB) *GormEventRepository {
	return &GormEventRepository{db: db}
}

// Create creates a new event
func (r *GormEventRepository) Create(ctx context.Context, event *models.Event) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// FindByID finds an event by ID
func (r *GormEventRepository) FindByID(ctx context.Context, id string) (*models.Event, error) {
	var event models.Event
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&event).Error
	if err == gorm.ErrRecordNotFound {
		return nil, db.ErrNotFound
	}
	return &event, err
}

// FindByAggregateID finds events for a specific aggregate
func (r *GormEventRepository) FindByAggregateID(ctx context.Context, aggregateID string, limit int) ([]*models.Event, error) {
	query := r.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var events []*models.Event
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// FindPending finds pending events to be processed
func (r *GormEventRepository) FindPending(ctx context.Context, limit int) ([]*models.Event, error) {
	query := r.db.WithContext(ctx).
		Where("processed_at IS NULL").
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var events []*models.Event
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// MarkProcessed marks an event as processed
func (r *GormEventRepository) MarkProcessed(ctx context.Context, eventID string) error {
	result := r.db.WithContext(ctx).
		Model(&models.Event{}).
		Where("id = ?", eventID).
		Update("processed_at", time.Now())

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return db.ErrNotFound
	}

	return nil
}

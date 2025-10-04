package worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type EventProcessor struct {
	DB       *gorm.DB
	Interval time.Duration
}

func NewEventProcessor(dbConn *gorm.DB) *EventProcessor {
	return &EventProcessor{
		DB:       dbConn,
		Interval: 2 * time.Second,
	}
}

func (p *EventProcessor) Start(ctx context.Context) {
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.drainOnce(ctx)
		}
	}
}

type eventEnvelope struct {
	EventType  string          `json:"event_type"`
	SubjectID  string          `json:"subject_id"`
	Data       json.RawMessage `json:"data"`
	RecordedAt time.Time       `json:"recorded_at"`
	Scopes     struct {
		Directories []string `json:"directories"`
		Files       []string `json:"files"`
	} `json:"scopes"`
}

func (p *EventProcessor) drainOnce(ctx context.Context) {
	for {
		event, envelope, err := p.claimEvent(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return
			}
			return
		}
		if event == nil {
			return
		}
		if err := p.processEvent(ctx, event, envelope); err != nil {
			p.scheduleRetry(ctx, event)
		}
	}
}

func (p *EventProcessor) claimEvent(ctx context.Context) (*db.Event, eventEnvelope, error) {
	var event db.Event
	err := p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Only claim workflow-trigger events in this worker
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("type = ? AND status IN ? AND next_attempt_at <= ?", "ext.workflow.triggered", []string{"pending", "retry"}, time.Now()).
			Order("next_attempt_at ASC").
			First(&event).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.Event{}).
			Where("id = ?", event.ID).
			Updates(map[string]any{
				"status":     "processing",
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		event.Status = "processing"
		return nil
	})
	if err != nil {
		return nil, eventEnvelope{}, err
	}

	var env eventEnvelope
	if err := json.Unmarshal(event.Payload, &env); err != nil {
		// Mark as completed to avoid retry loops for malformed payloads
		_ = p.DB.WithContext(ctx).
			Model(&db.Event{}).
			Where("id = ?", event.ID).
			Updates(map[string]any{
				"status":     "completed",
				"updated_at": time.Now(),
			}).Error
		return nil, eventEnvelope{}, err
	}
	return &event, env, nil
}

func (p *EventProcessor) processEvent(ctx context.Context, event *db.Event, env eventEnvelope) error {
	// Minimal v1 executor: acknowledge workflow trigger events.
	// Future: lookup .workflow manifest and dispatch actual steps.
	return p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&db.Event{}).
			Where("id = ?", event.ID).
			Updates(map[string]any{
				"status":          "completed",
				"retry_count":     gorm.Expr("retry_count"),
				"next_attempt_at": time.Now(),
				"updated_at":      time.Now(),
			}).Error
	})
}

func (p *EventProcessor) scheduleRetry(ctx context.Context, event *db.Event) {
	backoff := computeBackoff(event.RetryCount)
	_ = p.DB.WithContext(ctx).
		Model(&db.Event{}).
		Where("id = ?", event.ID).
		Updates(map[string]any{
			"status":          "retry",
			"retry_count":     gorm.Expr("retry_count + 1"),
			"next_attempt_at": time.Now().Add(backoff),
			"updated_at":      time.Now(),
		}).Error
}

func computeBackoff(retry int) time.Duration {
	if retry < 0 {
		retry = 0
	}
	if retry > 6 {
		retry = 6
	}
	return time.Duration(1<<retry) * time.Second
}
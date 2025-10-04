package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type EventProcessor struct {
	DB       *gorm.DB
	Interval time.Duration
}

func NewEventProcessor(db *gorm.DB) *EventProcessor {
	return &EventProcessor{
		DB:       db,
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "retry"}, time.Now()).
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
		// Mark as completed to prevent retry loops on malformed payloads.
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
	return p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		configs, err := p.matchingConfigs(ctx, tx, env)
		if err != nil {
			return err
		}

		now := time.Now()
		for _, cfg := range configs {
			if !allowsEvent(cfg, env.EventType) {
				continue
			}
			jobPayload, err := json.Marshal(map[string]any{
				"event_id":    event.ID,
				"event_type":  env.EventType,
				"subject_id":  env.SubjectID,
				"data":        json.RawMessage(env.Data),
				"recorded_at": env.RecordedAt,
				"config_id":   cfg.ID,
				"scopes": map[string]any{
					"directories": env.Scopes.Directories,
					"files":       env.Scopes.Files,
				},
			})
			if err != nil {
				return err
			}

			job := db.WebhookJob{
				ID:             uuid.NewString(),
				EventID:        event.ID,
				ConfigID:       cfg.ID,
				Payload:        jobPayload,
				Status:         "pending",
				RetryCount:     0,
				NextAttemptAt:  now,
				IdempotencyKey: hashIdentifier(event.ID + ":" + cfg.ID),
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).
				Create(&job).Error; err != nil {
				return err
			}
		}

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

func (p *EventProcessor) matchingConfigs(ctx context.Context, tx *gorm.DB, env eventEnvelope) ([]db.WebhookConfig, error) {
	query := tx.WithContext(ctx).Model(&db.WebhookConfig{}).
		Where("scope_type = ?", "global")
	if len(env.Scopes.Directories) > 0 {
		query = query.Or("scope_type = ? AND scope_id IN ?", "directory", env.Scopes.Directories)
	}
	if len(env.Scopes.Files) > 0 {
		query = query.Or("scope_type = ? AND scope_id IN ?", "file", env.Scopes.Files)
	}
	var configs []db.WebhookConfig
	if err := query.Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
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

func hashIdentifier(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func allowsEvent(cfg db.WebhookConfig, eventType string) bool {
	if strings.TrimSpace(cfg.EventTypes) == "" {
		return true
	}
	for _, evt := range strings.Split(cfg.EventTypes, ",") {
		evt = strings.TrimSpace(evt)
		if evt == "" {
			continue
		}
		if evt == "*" || evt == eventType {
			return true
		}
	}
	return false
}

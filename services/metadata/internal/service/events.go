package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
)

type EventPayload struct {
	EventType      string
	SubjectID      string
	RequestID      string
	IdempotencyKey string
	Data           any
	Scopes         ScopeSet
	Source         string
}

type ScopeSet struct {
	DirectoryIDs []string
	FileIDs      []string
}

func (s ScopeSet) Normalize() ScopeSet {
	return ScopeSet{
		DirectoryIDs: dedupeScopes(s.DirectoryIDs...),
		FileIDs:      dedupeScopes(s.FileIDs...),
	}
}

func persistEvent(ctx context.Context, tx *gorm.DB, payload EventPayload) (db.Event, error) {
	reqID := strings.TrimSpace(payload.RequestID)
	idKey := strings.TrimSpace(payload.IdempotencyKey)
	dedupeKey := idKey
	if dedupeKey == "" {
		dedupeKey = reqID
	}
	if dedupeKey == "" {
		dedupeKey = uuid.NewString()
	}

	existing := db.Event{}
	if err := tx.WithContext(ctx).
		Where("request_id = ? AND type = ?", dedupeKey, payload.EventType).
		First(&existing).Error; err != nil && err != gorm.ErrRecordNotFound {
		return db.Event{}, err
	} else if err == nil {
		return existing, nil
	}

	scopePayload := payload.Scopes.Normalize()
	envelope := map[string]any{
		"event_type": payload.EventType,
		"subject_id": payload.SubjectID,
		"data":       payload.Data,
		"scopes": map[string]any{
			"directories": scopePayload.DirectoryIDs,
			"files":       scopePayload.FileIDs,
		},
		"recorded_at": time.Now().UTC(),
	}
	if reqID != "" {
		envelope["request_id"] = reqID
	}
	if payload.Source != "" {
		envelope["source"] = payload.Source
	}
	if dedupeKey != "" {
		envelope["idempotency_key"] = dedupeKey
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return db.Event{}, err
	}

	event := db.Event{
		ID:            uuid.NewString(),
		Type:          payload.EventType,
		SubjectID:     payload.SubjectID,
		Payload:       body,
		Status:        "pending",
		RetryCount:    0,
		NextAttemptAt: time.Now(),
		RequestID:     dedupeKey,
	}

	if err := tx.Create(&event).Error; err != nil {
		return db.Event{}, err
	}
	return event, nil
}

func dedupeScopes(values ...string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

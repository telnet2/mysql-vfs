package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internaldb "github.com/telnet2/mysql-vfs/internal/db"
)

func setupSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := internaldb.AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestWorkflowEventProcessor_CompleteTriggeredEvent(t *testing.T) {
	db := setupSchedulerTestDB(t)
	ctx := context.Background()

	// Build event envelope payload similar to metadata persistEvent format
	payload, _ := json.Marshal(map[string]any{
		"event_type": "ext.workflow.triggered",
		"subject_id": "subj-1",
		"data":       map[string]any{"action": map[string]any{"workflow": "sync_profile"}},
		"scopes": map[string]any{
			"directories": []string{"dir-123"},
			"files":       []string{"file-xyz"},
		},
		"recorded_at": time.Now().UTC(),
	})

	evt := internaldb.Event{
		ID:            "evt-workflow-1",
		Type:          "ext.workflow.triggered",
		SubjectID:     "subj-1",
		Payload:       payload,
		Status:        "pending",
		RetryCount:    0,
		NextAttemptAt: time.Now(),
		RequestID:     "req-123",
	}
	if err := db.WithContext(ctx).Create(&evt).Error; err != nil {
		t.Fatalf("create event: %v", err)
	}

	var env eventEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal env: %v", err)
	}

	processor := NewEventProcessor(db)
	if err := processor.processEvent(ctx, &evt, env); err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	var stored internaldb.Event
	if err := db.WithContext(ctx).Where("id = ?", evt.ID).First(&stored).Error; err != nil {
		t.Fatalf("fetch event: %v", err)
	}
	if stored.Status != "completed" {
		t.Fatalf("expected event status completed, got %s", stored.Status)
	}
}

func TestWorkflowEventProcessor_ClaimEventFiltersType(t *testing.T) {
	db := setupSchedulerTestDB(t)
	ctx := context.Background()

	// Insert a non-workflow event that should be ignored by claimEvent
	evt := internaldb.Event{
		ID:            "evt-other-1",
		Type:          "ext.webhook.triggered",
		SubjectID:     "subj-2",
		Payload:       []byte(`{"event_type":"ext.webhook.triggered"}`),
		Status:        "pending",
		RetryCount:    0,
		NextAttemptAt: time.Now(),
		RequestID:     "req-999",
	}
	if err := db.WithContext(ctx).Create(&evt).Error; err != nil {
		t.Fatalf("create non-workflow event: %v", err)
	}

	processor := NewEventProcessor(db)
	e, _, err := processor.claimEvent(ctx)
	if err == nil || e != nil {
		t.Fatalf("expected no workflow event to be claimed, got %+v, err=%v", e, err)
	}
}

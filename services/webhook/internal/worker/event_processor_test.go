package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internaldb "github.com/telnet2/mysql-vfs/internal/db"
)

func setupWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := internaldb.AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestProcessEvent_CallWebhookCreatesJobs(t *testing.T) {
	db := setupWebhookTestDB(t)
	ctx := context.Background()

	// Global subscription config should fan-out for all events
	globalCfg := internaldb.WebhookConfig{
		ID:         "cfg-global",
		ScopeType:  "global",
		ScopeID:    nil,
		EventTypes: "*",
		TargetURL:  "https://subscriber.example/test",
		Secret:     "",
	}
	if err := db.WithContext(ctx).Create(&globalCfg).Error; err != nil {
		t.Fatalf("create global config: %v", err)
	}

	// Prepare ext.webhook.triggered event envelope with a direct action URL
	actionURL := "https://direct.example/hook"
	data, _ := json.Marshal(map[string]any{
		"action": map[string]any{
			"webhook": actionURL,
		},
	})

	env := eventEnvelope{
		EventType:  "ext.webhook.triggered",
		SubjectID:  "subject-1",
		Data:       json.RawMessage(data),
		RecordedAt: time.Now(),
	}
	env.Scopes.Directories = []string{"dir-1"}

	evt := internaldb.Event{
		ID:        "evt-1",
		Type:      env.EventType,
		SubjectID: env.SubjectID,
		Payload:   []byte(`{}`),
		Status:    "pending",
	}
	if err := db.WithContext(ctx).Create(&evt).Error; err != nil {
		t.Fatalf("create event row: %v", err)
	}

	processor := NewEventProcessor(db)
	if err := processor.processEvent(ctx, &evt, env); err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	// Expect two jobs:
	// - One for the direct-action webhook (ensureActionConfig)
	// - One for the global subscription config
	var jobs []internaldb.WebhookJob
	if err := db.WithContext(ctx).Find(&jobs).Error; err != nil {
		t.Fatalf("find jobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 webhook jobs, got %d", len(jobs))
	}

	// Verify that an action-scoped config was created for the direct URL
	var actionCfgCount int64
	if err := db.WithContext(ctx).
		Model(&internaldb.WebhookConfig{}).
		Where("scope_type = ? AND target_url = ?", "action", actionURL).
		Count(&actionCfgCount).Error; err != nil {
		t.Fatalf("count action configs: %v", err)
	}
	if actionCfgCount != 1 {
		t.Fatalf("expected 1 action-scoped config for direct URL, got %d", actionCfgCount)
	}
}

func TestEnsureActionConfig_Idempotent(t *testing.T) {
	db := setupWebhookTestDB(t)
	ctx := context.Background()
	processor := NewEventProcessor(db)

	url := "https://direct.example/once"
	var first, second internaldb.WebhookConfig
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = processor.ensureActionConfig(ctx, tx, url)
		if err != nil {
			return err
		}
		second, err = processor.ensureActionConfig(ctx, tx, url)
		return err
	}); err != nil {
		t.Fatalf("ensureActionConfig: %v", err)
	}
	if first.ID == "" || second.ID == "" || first.ID != second.ID {
		t.Fatalf("expected stable idempotent config IDs, got %q and %q", first.ID, second.ID)
	}

	var count int64
	if err := db.WithContext(ctx).
		Model(&internaldb.WebhookConfig{}).
		Where("scope_type = ? AND target_url = ?", "action", url).
		Count(&count).Error; err != nil {
		t.Fatalf("count configs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one config row, got %d", count)
	}
}
func TestProcessEvent_GlobalFanoutOnMalformedActionData(t *testing.T) {
	db := setupWebhookTestDB(t)
	ctx := context.Background()

	// Global subscription config should still fan-out even if action payload is malformed
	globalCfg := internaldb.WebhookConfig{
		ID:         "cfg-global-2",
		ScopeType:  "global",
		ScopeID:    nil,
		EventTypes: "*",
		TargetURL:  "https://subscriber.example/fanout",
		Secret:     "",
	}
	if err := db.WithContext(ctx).Create(&globalCfg).Error; err != nil {
		t.Fatalf("create global config: %v", err)
	}

	// Malformed data for direct action; should be ignored while global fan-out proceeds
	badData := json.RawMessage([]byte("{not-json}"))

	env := eventEnvelope{
		EventType:  "ext.webhook.triggered",
		SubjectID:  "subject-2",
		Data:       badData,
		RecordedAt: time.Now(),
	}
	env.Scopes.Directories = []string{"dir-2"}

	evt := internaldb.Event{
		ID:        "evt-2",
		Type:      env.EventType,
		SubjectID: env.SubjectID,
		Payload:   []byte(`{}`),
		Status:    "pending",
	}
	if err := db.WithContext(ctx).Create(&evt).Error; err != nil {
		t.Fatalf("create event row: %v", err)
	}

	processor := NewEventProcessor(db)
	if err := processor.processEvent(ctx, &evt, env); err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	var jobs []internaldb.WebhookJob
	if err := db.WithContext(ctx).Find(&jobs).Error; err != nil {
		t.Fatalf("find jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 webhook job (global fan-out), got %d", len(jobs))
	}
}

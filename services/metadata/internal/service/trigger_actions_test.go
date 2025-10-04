package service

import (
	"context"
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internaldb "github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

func setupEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := internaldb.AutoMigrate(database); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return database
}

func TestEnqueueTriggerActionsCreatesEvents(t *testing.T) {
	db := setupEventTestDB(t)
	ctx := context.Background()
	match := policy.TriggerMatch{
		Manifest: policy.Manifest{
			Name:        ".events",
			SourcePath:  "/team/.events",
			DirectoryID: "dir-123",
			Scope:       policy.ScopeDirectory,
			Inheritance: policy.InheritanceCascade,
			Type:        policy.TypeEvents,
		},
		Trigger: &policy.EventTrigger{
			Name: "test-trigger",
			On:   "file.created",
			Match: policy.EventTriggerMatch{
				"file_name":    []string{"*.json"},
				"storage_mode": []string{"inline_json"},
			},
			Actions: []policy.EventAction{
				{Type: "emit_event", EventType: "ext.file.audit"},
				{Type: "invoke_workflow", Workflow: "sync_profile"},
				{Type: "call_webhook", Webhook: "https://example.com/hook"},
			},
		},
	}

	input := policy.TriggerContext{
		DirectoryID: "dir-xyz",
		EventType:   "file.created",
		Scope:       policy.ScopeFile,
		Actor:       "alice",
		RequestID:   "req-1",
		FileID:      "file-1",
		FileName:    "profile.json",
		FilePath:    "/profiles/profile.json",
		StorageMode: "inline_json",
		Attributes:  map[string]any{"env": "prod"},
		Metadata:    map[string]any{"tenant": "acme"},
		Payload:     map[string]any{"name": "Alice"},
	}

	var firstEvents []internaldb.Event
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		events, err := enqueueTriggerActions(ctx, tx, []policy.TriggerMatch{match}, input)
		if err != nil {
			return err
		}
		if len(events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(events))
		}
		firstEvents = make([]internaldb.Event, len(events))
		copy(firstEvents, events)

		eventsAgain, err := enqueueTriggerActions(ctx, tx, []policy.TriggerMatch{match}, input)
		if err != nil {
			return err
		}
		if len(eventsAgain) != 3 {
			t.Fatalf("expected 3 events on second enqueue, got %d", len(eventsAgain))
		}
		for i := range events {
			if events[i].ID != eventsAgain[i].ID {
				t.Fatalf("expected idempotent event IDs to match")
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction: %v", err)
	}

	var count int64
	if err := db.Model(&internaldb.Event{}).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 stored events, got %d", count)
	}

	for _, evt := range firstEvents {
		var payload map[string]any
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			t.Fatalf("unmarshal event payload: %v", err)
		}
		contextData := payload["data"].(map[string]any)["context"].(map[string]any)
		if contextData["file_name"].(string) != "profile.json" {
			t.Fatalf("unexpected file_name in context")
		}
		if payload["source"].(string) != "extension" {
			t.Fatalf("expected source extension, got %v", payload["source"])
		}
		if payload["idempotency_key"].(string) == "" {
			t.Fatalf("expected idempotency key in payload")
		}
	}
}

func TestEnqueueTriggerActionsUnknownType(t *testing.T) {
	db := setupEventTestDB(t)
	ctx := context.Background()
	match := policy.TriggerMatch{
		Manifest: policy.Manifest{DirectoryID: "dir", Type: policy.TypeEvents},
		Trigger: &policy.EventTrigger{
			On: "file.updated",
			Actions: []policy.EventAction{
				{Type: "unknown"},
			},
		},
	}

	input := policy.TriggerContext{DirectoryID: "dir"}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := enqueueTriggerActions(ctx, tx, []policy.TriggerMatch{match}, input)
		return err
	})
	if err == nil {
		t.Fatalf("expected error for unknown action type")
	}
}

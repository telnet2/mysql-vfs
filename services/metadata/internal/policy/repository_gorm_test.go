package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	internaldb "github.com/telnet2/mysql-vfs/internal/db"
)

func setupTestDB(t *testing.T) *gorm.DB {
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

func TestGormManifestRepositoryJSONSchema(t *testing.T) {
	db := setupTestDB(t)

	dir := internaldb.Directory{ID: "dir", Name: "dir", Path: "/dir", PathHash: "hash-dir", Version: 1}
	if err := db.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	versionID := "ver1"
	schemaPayload := `{"schema":{"type":"object","properties":{"name":{"type":"string"}}},"scope":"directory","inheritance":"override","applies_to":["*.json"]}`
	file := internaldb.File{ID: "file", DirectoryID: dir.ID, Name: ".jsonschema", Path: "/dir/.jsonschema", CurrentVersionID: versionID, Version: 1}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	version := internaldb.FileVersion{ID: versionID, FileID: file.ID, StorageMode: "inline_json", JSONPayload: datatypes.JSON(schemaPayload)}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	repo := NewGormManifestRepository(db)
	manifests, err := repo.ListManifests(context.Background(), dir.ID)
	if err != nil {
		t.Fatalf("list manifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	m := manifests[0]
	if m.Type != TypeJSONSchema {
		t.Fatalf("expected jsonschema manifest, got %s", m.Type)
	}
	if m.Scope != ScopeDirectory {
		t.Fatalf("expected scope directory, got %s", m.Scope)
	}
	if m.Inheritance != InheritanceOverride {
		t.Fatalf("expected inheritance override, got %s", m.Inheritance)
	}
	if len(m.AppliesTo) != 1 || m.AppliesTo[0] != "*.json" {
		t.Fatalf("unexpected applies_to: %+v", m.AppliesTo)
	}
	expectedSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	if !bytes.Equal([]byte(m.Schema), []byte(expectedSchema)) {
		t.Fatalf("unexpected schema: %s", string(m.Schema))
	}
}

func TestGormManifestRepositoryJSONSchemaNoWrapper(t *testing.T) {
	db := setupTestDB(t)

	dir := internaldb.Directory{ID: "dir2", Name: "dir2", Path: "/dir2", PathHash: "hash-dir2", Version: 1}
	if err := db.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	versionID := "ver2"
	schemaOnly := `{"type":"array","items":{"type":"string"}}`
	file := internaldb.File{ID: "file2", DirectoryID: dir.ID, Name: ".jsonschema", Path: "/dir2/.jsonschema", CurrentVersionID: versionID, Version: 1}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	version := internaldb.FileVersion{ID: versionID, FileID: file.ID, StorageMode: "inline_json", JSONPayload: datatypes.JSON(schemaOnly)}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	repo := NewGormManifestRepository(db)
	manifests, err := repo.ListManifests(context.Background(), dir.ID)
	if err != nil {
		t.Fatalf("list manifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	m := manifests[0]
	if m.Type != TypeJSONSchema {
		t.Fatalf("expected jsonschema manifest, got %s", m.Type)
	}
	if !bytes.Equal([]byte(m.Schema), []byte(schemaOnly)) {
		t.Fatalf("unexpected schema without wrapper: %s", string(m.Schema))
	}
}

func TestGormManifestRepositoryEvents(t *testing.T) {
	db := setupTestDB(t)

	dir := internaldb.Directory{ID: "dir-events", Name: "dir-events", Path: "/events", PathHash: "hash-events", Version: 1}
	if err := db.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	versionID := "ver-events"
	pl := `{
		"scope": "tree",
		"triggers": [
			{
				"on": "file.updated",
				"match": {"file_name": ["*.json", "*.yaml"]},
				"actions": [
					{"type": "emit_event", "event_type": "ext.changed"}
				]
			}
		]
	}`
	file := internaldb.File{ID: "file-events", DirectoryID: dir.ID, Name: ".events", Path: "/events/.events", CurrentVersionID: versionID, Version: 1}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	version := internaldb.FileVersion{ID: versionID, FileID: file.ID, StorageMode: "inline_json", JSONPayload: datatypes.JSON(pl)}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	repo := NewGormManifestRepository(db)
	manifests, err := repo.ListManifests(context.Background(), dir.ID)
	if err != nil {
		t.Fatalf("list manifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	m := manifests[0]
	if m.Type != TypeEvents {
		t.Fatalf("expected events manifest, got %s", m.Type)
	}
	if m.Scope != ScopeTree {
		t.Fatalf("expected scope tree override, got %s", m.Scope)
	}
	if m.Events == nil {
		t.Fatalf("expected events configuration")
	}
	if len(m.Events.Triggers) != 1 {
		t.Fatalf("expected one trigger, got %d", len(m.Events.Triggers))
	}
	trigger := m.Events.Triggers[0]
	if trigger.On != "file.updated" {
		t.Fatalf("unexpected trigger on: %s", trigger.On)
	}
	if trigger.Match == nil {
		t.Fatalf("expected trigger match parsed")
	}
	values := trigger.Match["file_name"]
	if len(values) != 2 || values[0] != "*.json" || values[1] != "*.yaml" {
		t.Fatalf("unexpected match values: %+v", values)
	}
	if len(trigger.Actions) != 1 || trigger.Actions[0].EventType != "ext.changed" {
		t.Fatalf("unexpected action: %+v", trigger.Actions)
	}
}

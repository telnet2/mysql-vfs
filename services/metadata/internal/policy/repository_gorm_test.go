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

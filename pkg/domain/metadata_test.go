package domain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectoryMetadataPopulation(t *testing.T) {
	db := setupTestDB(t)
	dirService := NewDirectoryService(db)

	// Create context with auth info
	authCtx := &AuthContext{
		UserID: "alice",
		Groups: []string{"developers"},
	}
	ctx := context.WithValue(context.Background(), "authContext", authCtx)

	// Create directory
	dir, err := dirService.CreateDirectory(ctx, "/", "testdir")
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}

	// Verify metadata was populated
	if dir.Metadata == nil {
		t.Fatal("metadata is nil")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*dir.Metadata), &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Check required fields
	if metadata["owner"] != "alice" {
		t.Errorf("expected owner=alice, got %v", metadata["owner"])
	}
	if metadata["creator"] != "alice" {
		t.Errorf("expected creator=alice, got %v", metadata["creator"])
	}
	if metadata["system"] != false {
		t.Errorf("expected system=false, got %v", metadata["system"])
	}
}

func TestFileMetadataPopulation(t *testing.T) {
	db := setupTestDB(t)
	mockStorage := &MockStorage{}
	fileService := NewFileService(db, mockStorage)

	// Create context with auth info
	authCtx := &AuthContext{
		UserID: "bob",
		Groups: []string{"admins"},
	}
	ctx := context.WithValue(context.Background(), "authContext", authCtx)

	// Create file
	content := strings.NewReader("hello world")
	file, err := fileService.CreateFile(ctx, "/", "test.txt", "text/plain", 11, content)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	// Verify metadata was populated
	if file.Metadata == nil {
		t.Fatal("metadata is nil")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*file.Metadata), &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Check required fields
	if metadata["owner"] != "bob" {
		t.Errorf("expected owner=bob, got %v", metadata["owner"])
	}
	if metadata["creator"] != "bob" {
		t.Errorf("expected creator=bob, got %v", metadata["creator"])
	}
	if metadata["system"] != false {
		t.Errorf("expected system=false, got %v", metadata["system"])
	}
}

func TestDelegatedFileCreation(t *testing.T) {
	db := setupTestDB(t)
	mockStorage := &MockStorage{}
	fileService := NewFileService(db, mockStorage)

	// Create context with delegation
	authCtx := &AuthContext{
		UserID:           "service-account",
		Groups:           []string{"service-accounts"},
		PrincipalUserID:  "alice",
		DelegationReason: "automated-backup",
	}
	ctx := context.WithValue(context.Background(), "authContext", authCtx)

	// Create file
	content := strings.NewReader("delegated content")
	file, err := fileService.CreateFile(ctx, "/", "delegated.txt", "text/plain", 17, content)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	// Verify metadata shows delegation
	if file.Metadata == nil {
		t.Fatal("metadata is nil")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*file.Metadata), &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Check delegation fields
	if metadata["owner"] != "alice" {
		t.Errorf("expected owner=alice (principal), got %v", metadata["owner"])
	}
	if metadata["creator"] != "service-account" {
		t.Errorf("expected creator=service-account (actor), got %v", metadata["creator"])
	}
	if metadata["delegated"] != true {
		t.Errorf("expected delegated=true, got %v", metadata["delegated"])
	}
	if metadata["delegation_reason"] != "automated-backup" {
		t.Errorf("expected delegation_reason=automated-backup, got %v", metadata["delegation_reason"])
	}
}

func TestFileUpdateMetadataTracking(t *testing.T) {
	db := setupTestDB(t)
	mockStorage := &MockStorage{}
	fileService := NewFileService(db, mockStorage)

	// Create file with one user
	authCtx1 := &AuthContext{
		UserID: "alice",
		Groups: []string{"developers"},
	}
	ctx1 := context.WithValue(context.Background(), "authContext", authCtx1)

	content1 := strings.NewReader("version 1")
	file, err := fileService.CreateFile(ctx1, "/", "updated.txt", "text/plain", 9, content1)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	// Update file with different user
	authCtx2 := &AuthContext{
		UserID: "bob",
		Groups: []string{"developers"},
	}
	ctx2 := context.WithValue(context.Background(), "authContext", authCtx2)

	content2 := strings.NewReader("version 2")
	file, err = fileService.UpdateFile(ctx2, "/updated.txt", "text/plain", 9, content2, 0)
	if err != nil {
		t.Fatalf("UpdateFile failed: %v", err)
	}

	// Verify metadata tracks update
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*file.Metadata), &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Original creator should remain
	if metadata["creator"] != "alice" {
		t.Errorf("expected creator=alice (original), got %v", metadata["creator"])
	}

	// Updated_by should be new user
	if metadata["updated_by"] != "bob" {
		t.Errorf("expected updated_by=bob, got %v", metadata["updated_by"])
	}

	// Should have updated_at timestamp
	if _, ok := metadata["updated_at"]; !ok {
		t.Error("expected updated_at timestamp")
	}
}

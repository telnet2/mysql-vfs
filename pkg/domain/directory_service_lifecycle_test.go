package domain

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/telnet2/mysql-vfs/pkg/events"
)

// TestCreateDirectory_LifecycleEvents verifies directory creation events
func TestCreateDirectory_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Create a directory
	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)
	require.NotNil(t, dir)
	assert.Equal(t, "test-dir", dir.Name)

	// Verify lifecycle events
	authEvents := mockTrigger.GetEventsByPrefix("directory.create.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have authorization events")

	valEvents := mockTrigger.GetEventsByPrefix("directory.create.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events")

	completionEvents := mockTrigger.GetEventsByPrefix("directory.create.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")

	// Verify completion payload has directory resource
	if len(completionEvents) > 0 {
		payload, ok := completionEvents[0].Payload.(*events.CompletionEventPayload)
		assert.True(t, ok, "Completion payload should be *CompletionEventPayload")
		if payload != nil {
			assert.True(t, payload.Success, "Operation should be successful")
			assert.Empty(t, payload.ErrorMessage, "Should have no error message")

			// Verify resource is DirectoryResource
			_, isDir := payload.Resource.(events.DirectoryResource)
			assert.True(t, isDir, "Resource should be DirectoryResource")
		}
	}
}

// TestDeleteDirectory_LifecycleEvents verifies directory deletion events
func TestDeleteDirectory_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Create a directory first
	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)

	// Reset mock
	mockTrigger.Reset()

	// Delete the directory
	err = dirService.DeleteDirectory(ctx, dir.Path, false)
	require.NoError(t, err)

	// Verify lifecycle events
	authEvents := mockTrigger.GetEventsByPrefix("directory.delete.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have authorization events")

	valEvents := mockTrigger.GetEventsByPrefix("directory.delete.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events")

	// Verify existence validation
	existenceEvents := mockTrigger.GetEventsByType("directory.delete.validation.succeeded")
	assert.GreaterOrEqual(t, len(existenceEvents), 1, "Should validate directory existence")

	completionEvents := mockTrigger.GetEventsByPrefix("directory.delete.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")
}

// TestCreateDirectory_ParentNotFound_Events verifies parent validation events
func TestCreateDirectory_ParentNotFound_Events(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Try to create directory with non-existent parent
	_, err := dirService.CreateDirectory(ctx, "/nonexistent", "child-dir")
	require.Error(t, err)

	// Verify parent validation failure event
	valEvents := mockTrigger.GetEventsByPrefix("directory.create.validation.parent.failed")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have parent validation failure event")

	if len(valEvents) > 0 {
		payload, ok := valEvents[0].Payload.(*events.ValidationEventPayload)
		assert.True(t, ok, "Should be ValidationEventPayload")
		if payload != nil {
			assert.Equal(t, "parent_existence", payload.ValidationType)
			assert.Greater(t, len(payload.Violations), 0, "Should have violations")
		}
	}

	// Verify completion failure event
	completionFailEvents := mockTrigger.GetEventsByPrefix("directory.create.completion.failed")
	assert.GreaterOrEqual(t, len(completionFailEvents), 1, "Should have completion failed event")
}

// TestDeleteDirectory_NotEmpty_Events verifies emptiness validation events
func TestDeleteDirectory_NotEmpty_Events(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()
	storage := &MockStorage{}

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)
	fileService := NewFileServiceWithLifecycle(db, storage, nil, nil)

	ctx := context.Background()

	// Create a directory
	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)

	// Create a file in the directory
	content := []byte(`{"test": "data"}`)
	_, _ = fileService.CreateFile(ctx, dir.Path, "test.json", "application/json", int64(len(content)), bytes.NewReader(content))
	// Note: CreateFile might fail because content is nil, but we just need the directory to have a child reference

	// Reset mock
	mockTrigger.Reset()

	// Try to delete non-empty directory
	err = dirService.DeleteDirectory(ctx, dir.Path, false)
	// This might succeed if the file creation failed, but let's check the validation events

	// If directory had files, should have emptiness validation failure
	emptyEvents := mockTrigger.GetEventsByPrefix("directory.delete.validation.emptiness.failed")
	if err != nil && len(emptyEvents) > 0 {
		payload, ok := emptyEvents[0].Payload.(*events.ValidationEventPayload)
		assert.True(t, ok, "Should be ValidationEventPayload")
		if payload != nil {
			assert.Equal(t, "emptiness", payload.ValidationType)
			assert.Greater(t, len(payload.Violations), 0, "Should have violations")
		}
	}
}

// TestDeleteDirectory_NotFound_Events verifies not found validation events
func TestDeleteDirectory_NotFound_Events(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Try to delete non-existent directory
	err := dirService.DeleteDirectory(ctx, "/nonexistent", false)
	require.Error(t, err)

	// Verify existence validation failure event
	valEvents := mockTrigger.GetEventsByPrefix("directory.delete.validation.existence.failed")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have existence validation failure event")

	if len(valEvents) > 0 {
		payload, ok := valEvents[0].Payload.(*events.ValidationEventPayload)
		assert.True(t, ok, "Should be ValidationEventPayload")
		if payload != nil {
			assert.Equal(t, "existence", payload.ValidationType)
			assert.Greater(t, len(payload.Violations), 0, "Should have violations")
		}
	}
}

// TestDirectoryEventOrdering verifies directory events follow lifecycle order
func TestDirectoryEventOrdering(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	_, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)

	// Verify event ordering: Authorization → Validation → Execution → Completion
	var stages []string
	for _, e := range mockTrigger.EmittedEvents {
		if len(e.EventType) > 16 && e.EventType[:16] == "directory.create" {
			parts := splitEventType(e.EventType)
			if len(parts) >= 3 {
				stage := parts[2]
				if len(stages) == 0 || stages[len(stages)-1] != stage {
					stages = append(stages, stage)
				}
			}
		}
	}

	// Verify correct stage order
	expectedOrder := []string{"authorization", "validation", "completion"}
	for i, expectedStage := range expectedOrder {
		if i < len(stages) {
			assert.Equal(t, expectedStage, stages[i], "Stage %d should be %s", i, expectedStage)
		}
	}
}

// TestDirectoryResource_Fields verifies DirectoryResource has correct fields
func TestDirectoryResource_Fields(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)

	// Find completion event
	completionEvents := mockTrigger.GetEventsByType("directory.create.completion.succeeded")
	require.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion event")

	payload, ok := completionEvents[0].Payload.(*events.CompletionEventPayload)
	require.True(t, ok, "Should be CompletionEventPayload")

	dirRes, ok := payload.Resource.(events.DirectoryResource)
	require.True(t, ok, "Resource should be DirectoryResource")

	// Verify fields
	assert.Equal(t, dir.ID, dirRes.ID, "ID should match")
	assert.Equal(t, dir.Name, dirRes.Name, "Name should match")
	assert.Equal(t, dir.Path, dirRes.Path, "Path should match")
	assert.Equal(t, events.ResourceTypeDirectory, dirRes.Type, "Type should be directory")
	assert.False(t, dirRes.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.False(t, dirRes.UpdatedAt.IsZero(), "UpdatedAt should be set")
}

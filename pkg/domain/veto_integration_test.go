package domain

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

// TestVeto_CreateFile_Authorization verifies veto blocks file creation at authorization stage
func TestVeto_CreateFile_Authorization(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	// Configure mock to veto at authorization stage
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.create.authorization.started"
	mockTrigger.VetoMessage = "User not authorized to create files"

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	// Try to create file - should be vetoed
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")
	assert.Nil(t, file, "File should not be created")

	// Verify error is VetoError
	var vetoErr *events.VetoError
	assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")
	if vetoErr != nil {
		assert.Equal(t, "User not authorized to create files", vetoErr.Message)
		assert.Equal(t, "TEST_VETO", vetoErr.Code)
	}

	// Verify authorization event was emitted but not completion
	authEvents := mockTrigger.GetEventsByPrefix("file.create.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 1, "Should have authorization events")

	completionEvents := mockTrigger.GetEventsByPrefix("file.create.completion")
	assert.Equal(t, 0, len(completionEvents), "Should have no completion events (vetoed before completion)")
}

// TestVeto_UpdateFile_Validation verifies veto blocks file update at validation stage
func TestVeto_UpdateFile_Validation(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file first (without veto)
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock and configure veto at validation.succeeded (general validation, not schema-specific)
	mockTrigger.Reset()
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.update.validation.succeeded"
	mockTrigger.VetoMessage = "Validation vetoed"

	// Try to update - should be vetoed
	updateContent := bytes.NewReader([]byte(`{"test": "updated"}`))
	updatedFile, err := fileService.UpdateFile(ctx, "/test.json", "application/json", 18, updateContent, file.Version)

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")
	assert.Nil(t, updatedFile, "File should not be updated")

	// Verify error is VetoError
	var vetoErr *events.VetoError
	assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")

	// Verify the file was NOT actually updated in database
	var dbFile models.File
	err = db.Where("id = ?", file.ID).First(&dbFile).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), dbFile.Version, "File version should still be 1 (not updated)")
}

// TestVeto_DeleteFile verifies veto blocks file deletion
func TestVeto_DeleteFile(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file first
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock and configure veto
	mockTrigger.Reset()
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.delete.authorization.started"
	mockTrigger.VetoMessage = "Delete not allowed"

	// Try to delete - should be vetoed
	err = fileService.DeleteFile(ctx, "/test.json")

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")

	// Verify error is VetoError
	var vetoErr *events.VetoError
	assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")

	// Verify the file still exists in database
	var dbFile models.File
	err = db.Where("id = ? AND deleted_at IS NULL", file.ID).First(&dbFile).Error
	assert.NoError(t, err, "File should still exist (not deleted)")
}

// TestVeto_MoveFile verifies veto blocks file move
func TestVeto_MoveFile(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file first
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	file, err := fileService.CreateFile(ctx, "/", "old-name.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock and configure veto
	mockTrigger.Reset()
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.move.authorization.started"
	mockTrigger.VetoMessage = "Move not allowed"

	// Try to move - should be vetoed
	movedFile, err := fileService.MoveFile(ctx, "/old-name.json", "/new-name.json")

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")
	assert.Nil(t, movedFile, "File should not be moved")

	// Verify the file still has old name in database
	var dbFile models.File
	err = db.Where("id = ?", file.ID).First(&dbFile).Error
	require.NoError(t, err)
	assert.Equal(t, "old-name.json", dbFile.Name, "File name should still be old name (not moved)")
}

// TestVeto_CreateDirectory verifies veto blocks directory creation
func TestVeto_CreateDirectory(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	// Configure veto
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "directory.create.authorization.started"
	mockTrigger.VetoMessage = "Directory creation not allowed"

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Try to create directory - should be vetoed
	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")
	assert.Nil(t, dir, "Directory should not be created")

	// Verify error is VetoError
	var vetoErr *events.VetoError
	assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")

	// Verify the directory does not exist in database
	var count int64
	db.Model(&models.Directory{}).Where("name = ? AND deleted_at IS NULL", "test-dir").Count(&count)
	assert.Equal(t, int64(0), count, "Directory should not exist in database")
}

// TestVeto_DeleteDirectory verifies veto blocks directory deletion
func TestVeto_DeleteDirectory(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Create a directory first (without veto)
	dir, err := dirService.CreateDirectory(ctx, "/", "test-dir")
	require.NoError(t, err)

	// Reset mock and configure veto
	mockTrigger.Reset()
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "directory.delete.authorization.started"
	mockTrigger.VetoMessage = "Directory deletion not allowed"

	// Try to delete - should be vetoed
	err = dirService.DeleteDirectory(ctx, dir.Path, false)

	// Verify operation was blocked
	assert.Error(t, err, "Operation should be blocked by veto")

	// Verify error is VetoError
	var vetoErr *events.VetoError
	assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")

	// Verify the directory still exists in database
	var dbDir models.Directory
	err = db.Where("id = ? AND deleted_at IS NULL", dir.ID).First(&dbDir).Error
	assert.NoError(t, err, "Directory should still exist (not deleted)")
}

// TestNoVeto_OperationSucceeds verifies operations succeed when no veto occurs
func TestNoVeto_OperationSucceeds(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	// No veto configured
	mockTrigger.ShouldVeto = false

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	// Create file - should succeed
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)

	// Verify operation succeeded
	assert.NoError(t, err, "Operation should succeed without veto")
	assert.NotNil(t, file, "File should be created")

	// Verify completion event was emitted
	completionEvents := mockTrigger.GetEventsByPrefix("file.create.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")
}

// TestVeto_MultipleStages verifies veto can occur at different stages
func TestVeto_MultipleStages(t *testing.T) {
	testCases := []struct {
		name          string
		vetoEventType string
		expectVeto    bool
	}{
		{
			name:          "Veto at authorization.started",
			vetoEventType: "file.create.authorization.started",
			expectVeto:    true,
		},
		{
			name:          "Veto at authorization.succeeded",
			vetoEventType: "file.create.authorization.succeeded",
			expectVeto:    true,
		},
		{
			name:          "Veto at validation.succeeded",
			vetoEventType: "file.create.validation.succeeded",
			expectVeto:    true,
		},
		{
			name:          "No veto - completion event",
			vetoEventType: "file.create.completion.succeeded",
			expectVeto:    false, // Completion happens after operation completes
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			storage := &MockStorage{}
			mockTrigger := NewMockEventTrigger()

			mockTrigger.ShouldVeto = true
			mockTrigger.VetoEventType = tc.vetoEventType
			mockTrigger.VetoMessage = "Vetoed at " + tc.vetoEventType

			fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

			ctx := context.Background()
			content := bytes.NewReader([]byte(`{"test": "data"}`))

			file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)

			if tc.expectVeto {
				assert.Error(t, err, "Should be vetoed")
				assert.Nil(t, file, "File should not be created")

				var vetoErr *events.VetoError
				assert.ErrorAs(t, err, &vetoErr, "Error should be VetoError")
			} else {
				// Completion events are async/after the fact, so they don't block
				assert.NoError(t, err, "Should not be vetoed at completion")
				assert.NotNil(t, file, "File should be created")
			}
		})
	}
}

// TestVeto_EventSequence verifies event sequence when veto occurs
func TestVeto_EventSequence(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	// Veto at validation stage
	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.create.validation.succeeded"
	mockTrigger.VetoMessage = "Validation veto"

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	_, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.Error(t, err)

	// Verify event sequence stops at veto point
	authEvents := mockTrigger.GetEventsByPrefix("file.create.authorization")
	assert.Greater(t, len(authEvents), 0, "Authorization events should be emitted before veto")

	valEvents := mockTrigger.GetEventsByPrefix("file.create.validation")
	assert.Greater(t, len(valEvents), 0, "Validation events should be emitted up to veto point")

	// Execution should not happen if vetoed during validation
	execEvents := mockTrigger.GetEventsByPrefix("file.create.execution")
	assert.Equal(t, 0, len(execEvents), "Execution events should not be emitted after veto")

	// Note: completion.failed event IS emitted when veto occurs (to track the failure)
	// but completion.succeeded should NOT be emitted
	completionSuccessEvents := mockTrigger.GetEventsByType("file.create.completion.succeeded")
	assert.Equal(t, 0, len(completionSuccessEvents), "Completion success events should not be emitted after veto")
}

// TestVeto_FileNotCreatedInDatabase verifies vetoed operations don't persist to database
func TestVeto_FileNotCreatedInDatabase(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "file.create.authorization.started"
	mockTrigger.VetoMessage = "Blocked"

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	// Try to create file (will be vetoed)
	_, err := fileService.CreateFile(ctx, "/", "blocked.json", "application/json", 16, content)
	require.Error(t, err)

	// Verify no files exist in database
	var count int64
	db.Model(&models.File{}).Where("name = ?", "blocked.json").Count(&count)
	assert.Equal(t, int64(0), count, "No files should exist in database after veto")

	// Verify no file versions exist
	db.Model(&models.FileVersion{}).Count(&count)
	assert.Equal(t, int64(0), count, "No file versions should exist after veto")
}

// TestVeto_DirectoryNotCreatedInDatabase verifies vetoed directory operations don't persist
func TestVeto_DirectoryNotCreatedInDatabase(t *testing.T) {
	db := setupTestDB(t)
	mockTrigger := NewMockEventTrigger()

	mockTrigger.ShouldVeto = true
	mockTrigger.VetoEventType = "directory.create.authorization.started"
	mockTrigger.VetoMessage = "Blocked"

	dirService := NewDirectoryServiceWithLifecycle(db, mockTrigger)

	ctx := context.Background()

	// Try to create directory (will be vetoed)
	_, err := dirService.CreateDirectory(ctx, "/", "blocked-dir")
	require.Error(t, err)

	// Verify directory doesn't exist in database (excluding root)
	var count int64
	db.Model(&models.Directory{}).Where("name = ?", "blocked-dir").Count(&count)
	assert.Equal(t, int64(0), count, "Directory should not exist in database after veto")
}

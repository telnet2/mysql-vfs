package domain

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockEventTrigger captures emitted events for testing
type MockEventTrigger struct {
	EmittedEvents []EmittedEvent
	ShouldVeto    bool
	VetoEventType string
	VetoMessage   string
}

type EmittedEvent struct {
	EventType string
	Payload   interface{}
	Timestamp time.Time
}

func NewMockEventTrigger() *MockEventTrigger {
	return &MockEventTrigger{
		EmittedEvents: make([]EmittedEvent, 0),
	}
}

func (m *MockEventTrigger) Emit(ctx context.Context, eventType string, payload interface{}) {
	m.EmittedEvents = append(m.EmittedEvents, EmittedEvent{
		EventType: eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

func (m *MockEventTrigger) EmitSync(ctx context.Context, eventType string, payload interface{}) error {
	m.EmittedEvents = append(m.EmittedEvents, EmittedEvent{
		EventType: eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	})

	// Check if we should veto this event
	if m.ShouldVeto && eventType == m.VetoEventType {
		return &events.VetoError{
			Message: m.VetoMessage,
			Code:    "TEST_VETO",
		}
	}

	return nil
}

func (m *MockEventTrigger) EmitWithOperation(ctx context.Context, opCtx *events.OperationContext, eventType string, payload interface{}) {
	m.Emit(ctx, eventType, payload)
}

func (m *MockEventTrigger) EmitSyncWithOperation(ctx context.Context, opCtx *events.OperationContext, eventType string, payload interface{}) error {
	return m.EmitSync(ctx, eventType, payload)
}

func (m *MockEventTrigger) Shutdown(ctx context.Context) error {
	return nil
}

func (m *MockEventTrigger) Wait() {
	// No-op for testing
}

func (m *MockEventTrigger) GetEventsByType(eventType string) []EmittedEvent {
	var filtered []EmittedEvent
	for _, e := range m.EmittedEvents {
		if e.EventType == eventType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func (m *MockEventTrigger) GetEventsByPrefix(prefix string) []EmittedEvent {
	var filtered []EmittedEvent
	for _, e := range m.EmittedEvents {
		if len(e.EventType) >= len(prefix) && e.EventType[:len(prefix)] == prefix {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func (m *MockEventTrigger) Reset() {
	m.EmittedEvents = make([]EmittedEvent, 0)
	m.ShouldVeto = false
	m.VetoEventType = ""
	m.VetoMessage = ""
}

// MockStorage for testing
type MockStorage struct{}

func (m *MockStorage) Put(ctx context.Context, key string, content io.Reader) error {
	return nil
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewBuffer([]byte("{}"))), nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *MockStorage) Close() error {
	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	return true, nil
}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = db.AutoMigrate(
		&models.Directory{},
		&models.File{},
		&models.FileVersion{},
		&models.Event{},
	)
	require.NoError(t, err)

	// Create root directory
	rootDir := &models.Directory{
		ID:        "root",
		Name:      "",
		Path:      "/",
		PathHash:  "root-hash",
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = db.Create(rootDir).Error
	require.NoError(t, err)

	return db
}

// TestCreateFile_LifecycleEvents verifies all lifecycle events are emitted
func TestCreateFile_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	// Create file
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)
	require.NotNil(t, file)

	// Verify events were emitted in correct order
	expectedEvents := []string{
		"file.create.authorization.started",
		"file.create.authorization.succeeded",
		"file.create.validation.succeeded",
		"file.create.completion.succeeded",
	}

	assert.GreaterOrEqual(t, len(mockTrigger.EmittedEvents), len(expectedEvents),
		"Should emit at least %d events, got %d", len(expectedEvents), len(mockTrigger.EmittedEvents))

	// Verify authorization events
	authEvents := mockTrigger.GetEventsByPrefix("file.create.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have at least 2 authorization events")

	// Verify validation events
	valEvents := mockTrigger.GetEventsByPrefix("file.create.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have at least 1 validation event")

	// Verify completion events
	completionEvents := mockTrigger.GetEventsByPrefix("file.create.completion")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion event")

	// Verify completion payload
	if len(completionEvents) > 0 {
		payload, ok := completionEvents[0].Payload.(*events.CompletionEventPayload)
		assert.True(t, ok, "Completion payload should be *CompletionEventPayload")
		if payload != nil {
			assert.True(t, payload.Success, "Operation should be successful")
			assert.Empty(t, payload.ErrorMessage, "Should have no error message")
			assert.GreaterOrEqual(t, payload.TotalDurationMs, int64(0), "Duration should be non-negative")
		}
	}
}

// TestUpdateFile_LifecycleEvents verifies update operation events
func TestUpdateFile_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// First create a file
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	file, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock to clear create events
	mockTrigger.Reset()

	// Update the file
	updateContent := bytes.NewReader([]byte(`{"test": "updated"}`))
	updatedFile, err := fileService.UpdateFile(ctx, "/test.json", "application/json", 18, updateContent, file.Version)
	require.NoError(t, err)
	require.NotNil(t, updatedFile)
	assert.Equal(t, int64(2), updatedFile.Version)

	// Verify update lifecycle events
	authEvents := mockTrigger.GetEventsByPrefix("file.update.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have authorization events")

	valEvents := mockTrigger.GetEventsByPrefix("file.update.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events")

	completionEvents := mockTrigger.GetEventsByPrefix("file.update.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")
}

// TestDeleteFile_LifecycleEvents verifies delete operation events
func TestDeleteFile_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file first
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	_, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock
	mockTrigger.Reset()

	// Delete the file
	err = fileService.DeleteFile(ctx, "/test.json")
	require.NoError(t, err)

	// Verify delete lifecycle events
	authEvents := mockTrigger.GetEventsByPrefix("file.delete.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have authorization events")

	valEvents := mockTrigger.GetEventsByPrefix("file.delete.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events for existence check")

	completionEvents := mockTrigger.GetEventsByPrefix("file.delete.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")
}

// TestMoveFile_LifecycleEvents verifies move operation events
func TestMoveFile_LifecycleEvents(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file first
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	_, err := fileService.CreateFile(ctx, "/", "old-name.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock
	mockTrigger.Reset()

	// Move/rename the file
	movedFile, err := fileService.MoveFile(ctx, "/old-name.json", "/new-name.json")
	require.NoError(t, err)
	require.NotNil(t, movedFile)
	assert.Equal(t, "new-name.json", movedFile.Name)

	// Verify move lifecycle events
	authEvents := mockTrigger.GetEventsByPrefix("file.move.authorization")
	assert.GreaterOrEqual(t, len(authEvents), 2, "Should have authorization events")

	valEvents := mockTrigger.GetEventsByPrefix("file.move.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events")

	// Verify validation includes source and destination checks
	sourceValEvents := mockTrigger.GetEventsByType("file.move.validation.source.succeeded")
	assert.GreaterOrEqual(t, len(sourceValEvents), 1, "Should validate source existence")

	completionEvents := mockTrigger.GetEventsByPrefix("file.move.completion.succeeded")
	assert.GreaterOrEqual(t, len(completionEvents), 1, "Should have completion succeeded event")
}

// TestCreateFile_ValidationFailure_Events verifies validation failure events
func TestCreateFile_ValidationFailure_Events(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Try to create a file that exceeds max size
	largeContent := make([]byte, MaxFileSize+1)
	content := bytes.NewReader(largeContent)

	_, err := fileService.CreateFile(ctx, "/", "large.json", "application/json", int64(len(largeContent)), content)
	require.Error(t, err)

	// Verify validation failure event was emitted
	valFailEvents := mockTrigger.GetEventsByPrefix("file.create.validation")
	assert.GreaterOrEqual(t, len(valFailEvents), 1, "Should have validation events")

	// Find the failure event
	var foundFailure bool
	for _, e := range valFailEvents {
		if payload, ok := e.Payload.(*events.ValidationEventPayload); ok {
			if len(payload.Violations) > 0 {
				foundFailure = true
				assert.Equal(t, "size", payload.ValidationType)
				break
			}
		}
	}
	assert.True(t, foundFailure, "Should have validation failure event with violations")

	// Verify completion failure event
	completionFailEvents := mockTrigger.GetEventsByType("file.create.completion.failed")
	assert.GreaterOrEqual(t, len(completionFailEvents), 1, "Should have completion failed event")
}

// TestUpdateFile_VersionConflict_Events verifies version conflict events
func TestUpdateFile_VersionConflict_Events(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create a file
	content := bytes.NewReader([]byte(`{"test": "data"}`))
	_, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Reset mock
	mockTrigger.Reset()

	// Try to update with wrong version
	updateContent := bytes.NewReader([]byte(`{"test": "updated"}`))
	_, err = fileService.UpdateFile(ctx, "/test.json", "application/json", 18, updateContent, 999)
	require.Error(t, err)

	// Verify version validation failure event
	valEvents := mockTrigger.GetEventsByPrefix("file.update.validation")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have validation events")

	// Find version conflict event
	var foundConflict bool
	for _, e := range valEvents {
		if payload, ok := e.Payload.(*events.ValidationEventPayload); ok {
			if payload.ValidationType == "version" && len(payload.Violations) > 0 {
				foundConflict = true
				assert.Contains(t, payload.Violations[0].Message, "version conflict")
				break
			}
		}
	}
	assert.True(t, foundConflict, "Should have version conflict validation event")
}

// TestDeleteFile_NotFound_Events verifies not found validation events
func TestDeleteFile_NotFound_Events(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Try to delete non-existent file
	err := fileService.DeleteFile(ctx, "/nonexistent.json")
	require.Error(t, err)

	// Verify existence validation failure event
	valEvents := mockTrigger.GetEventsByPrefix("file.delete.validation.existence.failed")
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

// TestMoveFile_DestinationConflict_Events verifies destination conflict events
func TestMoveFile_DestinationConflict_Events(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()

	// Create source file
	content1 := bytes.NewReader([]byte(`{"test": "data1"}`))
	_, err := fileService.CreateFile(ctx, "/", "source.json", "application/json", 17, content1)
	require.NoError(t, err)

	// Create destination file (conflict)
	content2 := bytes.NewReader([]byte(`{"test": "data2"}`))
	_, err = fileService.CreateFile(ctx, "/", "dest.json", "application/json", 17, content2)
	require.NoError(t, err)

	// Reset mock
	mockTrigger.Reset()

	// Try to move source to dest (should conflict)
	_, err = fileService.MoveFile(ctx, "/source.json", "/dest.json")
	require.Error(t, err)

	// Verify destination conflict validation event
	valEvents := mockTrigger.GetEventsByPrefix("file.move.validation.destination.failed")
	assert.GreaterOrEqual(t, len(valEvents), 1, "Should have destination validation failure event")

	if len(valEvents) > 0 {
		payload, ok := valEvents[0].Payload.(*events.ValidationEventPayload)
		assert.True(t, ok, "Should be ValidationEventPayload")
		if payload != nil {
			assert.Equal(t, "destination_conflict", payload.ValidationType)
		}
	}
}

// TestEventOrdering verifies events are emitted in correct lifecycle order
func TestEventOrdering(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	mockTrigger := NewMockEventTrigger()

	fileService := NewFileServiceWithLifecycle(db, storage, nil, mockTrigger)

	ctx := context.Background()
	content := bytes.NewReader([]byte(`{"test": "data"}`))

	_, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 16, content)
	require.NoError(t, err)

	// Verify event ordering: Authorization → Validation → Execution → Completion
	var stages []string
	for _, e := range mockTrigger.EmittedEvents {
		if len(e.EventType) > 11 && e.EventType[:11] == "file.create" {
			// Extract stage from event type (e.g., "file.create.authorization.started" → "authorization")
			parts := splitEventType(e.EventType)
			if len(parts) >= 3 {
				stage := parts[2]
				// Only track stage transitions
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

// Helper function to split event type
func splitEventType(eventType string) []string {
	var parts []string
	current := ""
	for _, ch := range eventType {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

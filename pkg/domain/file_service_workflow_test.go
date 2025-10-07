package domain

import (
	"bytes"
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workflowValidatorStub struct {
	createErr    error
	moveErr      error
	deleteErr    error
	directoryErr error

	createCalls []workflowCreateCall
	moveCalls   []workflowMoveCall
	deleteCalls []workflowDeleteCall
	dirCalls    []workflowDirectoryCall
}

type workflowCreateCall struct {
	Path  string
	Actor WorkflowActor
}

type workflowMoveCall struct {
	SourcePath string
	DestPath   string
	Actor      WorkflowActor
	Metadata   map[string]interface{}
}

type workflowDeleteCall struct {
	Path     string
	Actor    WorkflowActor
	Metadata map[string]interface{}
}

type workflowDirectoryCall struct {
	Path      string
	Operation string
	Actor     WorkflowActor
}

func (s *workflowValidatorStub) ValidateCreateOperation(ctx context.Context, filePath string, actor WorkflowActor) error {
	s.createCalls = append(s.createCalls, workflowCreateCall{Path: filePath, Actor: actor})
	return s.createErr
}

func (s *workflowValidatorStub) ValidateMoveOperation(ctx context.Context, sourcePath, destPath string, actor WorkflowActor, metadata map[string]interface{}) error {
	s.moveCalls = append(s.moveCalls, workflowMoveCall{SourcePath: sourcePath, DestPath: destPath, Actor: actor, Metadata: metadata})
	return s.moveErr
}

func (s *workflowValidatorStub) ValidateDeleteOperation(ctx context.Context, filePath string, actor WorkflowActor, metadata map[string]interface{}) error {
	s.deleteCalls = append(s.deleteCalls, workflowDeleteCall{Path: filePath, Actor: actor, Metadata: metadata})
	return s.deleteErr
}

func (s *workflowValidatorStub) ValidateDirectoryOperation(ctx context.Context, dirPath, operation string, actor WorkflowActor) error {
	s.dirCalls = append(s.dirCalls, workflowDirectoryCall{Path: dirPath, Operation: operation, Actor: actor})
	return s.directoryErr
}

func (s *workflowValidatorStub) GetValidTransitions(ctx context.Context, filePath string, actor WorkflowActor) ([]string, error) {
	return nil, nil
}

func TestFileService_CreateFile_WorkflowBlocked(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	trigger := NewMockEventTrigger()
	service := NewFileServiceWithLifecycle(db, storage, nil, trigger)
	stub := &workflowValidatorStub{
		createErr: newWorkflowValidationError(ErrWorkflowTransitionDenied, "create blocked", map[string]interface{}{"state": "draft"}),
	}
	service.SetWorkflowEngine(stub)

	ctx := context.WithValue(context.Background(), "authContext", &AuthContext{UserID: "alice", Groups: []string{"editors"}})
	data := []byte("hello")

	file, err := service.CreateFile(ctx, "/", "test.txt", "text/plain", int64(len(data)), bytes.NewReader(data))
	require.Error(t, err)
	assert.Nil(t, file)
	assert.ErrorIs(t, err, stub.createErr)
	require.Len(t, stub.createCalls, 1)
	assert.Equal(t, "/test.txt", stub.createCalls[0].Path)

	events := trigger.GetEventsByType("workflow.create.blocked")
	require.Len(t, events, 1)
	payload, ok := events[0].Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "/test.txt", payload["path"])
}

func TestFileService_MoveFile_WorkflowBlocked(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	trigger := NewMockEventTrigger()
	service := NewFileServiceWithLifecycle(db, storage, nil, trigger)
	stub := &workflowValidatorStub{}
	service.SetWorkflowEngine(stub)

	ctx := context.WithValue(context.Background(), "authContext", &AuthContext{UserID: "bob", Groups: []string{"editors"}})
	data := []byte("hello")
	_, err := service.CreateFile(ctx, "/", "draft.txt", "text/plain", int64(len(data)), bytes.NewReader(data))
	require.NoError(t, err)

	stub.moveErr = newWorkflowValidationError(ErrWorkflowTransitionDenied, "move blocked", map[string]interface{}{"from": "draft"})
	_, err = service.MoveFile(ctx, "/draft.txt", "/review.txt")
	require.Error(t, err)
	assert.ErrorIs(t, err, stub.moveErr)
	require.Len(t, stub.moveCalls, 1)
	assert.Equal(t, "/draft.txt", stub.moveCalls[0].SourcePath)
	assert.Equal(t, "/review.txt", stub.moveCalls[0].DestPath)
	require.NotNil(t, stub.moveCalls[0].Metadata)

	events := trigger.GetEventsByType("workflow.transition.failed")
	require.Len(t, events, 1)
	payload, ok := events[0].Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "/draft.txt", payload["path"])
}

func TestFileService_DeleteFile_WorkflowBlocked(t *testing.T) {
	db := setupTestDB(t)
	storage := &MockStorage{}
	trigger := NewMockEventTrigger()
	service := NewFileServiceWithLifecycle(db, storage, nil, trigger)
	stub := &workflowValidatorStub{}
	service.SetWorkflowEngine(stub)

	ctx := context.WithValue(context.Background(), "authContext", &AuthContext{UserID: "carol", Groups: []string{"editors"}})
	data := []byte("hello")
	_, err := service.CreateFile(ctx, "/", "draft.txt", "text/plain", int64(len(data)), bytes.NewReader(data))
	require.NoError(t, err)

	stub.deleteErr = newWorkflowValidationError(ErrWorkflowGateDenied, "delete blocked", map[string]interface{}{"state": "draft"})
	err = service.DeleteFile(ctx, "/draft.txt")
	require.Error(t, err)
	assert.ErrorIs(t, err, stub.deleteErr)
	require.Len(t, stub.deleteCalls, 1)
	assert.Equal(t, "/draft.txt", stub.deleteCalls[0].Path)
	require.NotNil(t, stub.deleteCalls[0].Metadata)

	events := trigger.GetEventsByType("workflow.deletion.blocked")
	require.Len(t, events, 1)
	payload, ok := events[0].Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "/draft.txt", payload["path"])
}

func TestDirectoryService_DeleteDirectory_WorkflowBlocked(t *testing.T) {
	db := setupTestDB(t)
	trigger := NewMockEventTrigger()
	service := NewDirectoryServiceWithLifecycle(db, trigger)
	stub := &workflowValidatorStub{}
	service.SetWorkflowEngine(stub)

	ctx := context.WithValue(context.Background(), "authContext", &AuthContext{UserID: "dave", Groups: []string{"editors"}})
	dir, err := service.CreateDirectory(ctx, "/", "draft")
	require.NoError(t, err)

	stub.directoryErr = newWorkflowValidationError(ErrWorkflowStateProtected, "delete blocked", map[string]interface{}{"state": "draft"})
	err = service.DeleteDirectory(ctx, dir.Path, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, stub.directoryErr)
	require.Len(t, stub.dirCalls, 1)
	assert.Equal(t, "delete", stub.dirCalls[0].Operation)
	assert.Equal(t, path.Clean(dir.Path), stub.dirCalls[0].Path)

	events := trigger.GetEventsByType("workflow.state_dir.delete.blocked")
	require.Len(t, events, 1)
	payload, ok := events[0].Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, dir.Path, payload["path"])
}

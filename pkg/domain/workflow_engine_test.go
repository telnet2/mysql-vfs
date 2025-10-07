package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/telnet2/mysql-vfs/pkg/models"
	mysqlrepo "github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

func TestWorkflowEngine_ValidateCreateAllowed(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)

	err := engine.ValidateCreateOperation(ctx, "/docs/draft/newfile.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}})
	require.NoError(t, err)

	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.True(t, record.Success)
	assert.Equal(t, "draft", record.FromState)
	assert.Equal(t, "draft", record.ToState)
	assert.Equal(t, "create", record.Operation)
}

func TestWorkflowEngine_ValidateCreateDeniedOutsideInitial(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)

	err := engine.ValidateCreateOperation(ctx, "/docs/review/file.txt", WorkflowActor{ID: "bob", Groups: []string{"authors"}})
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrWorkflowInitialStateOnly, verr.Code)

	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.False(t, record.Success)
	assert.Equal(t, "create", record.Operation)
}

func TestWorkflowEngine_ValidateMove_GateAllows(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool {
		return input.Transition.To == "review"
	}}
	engine.gateEvaluator = mockEvaluator

	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file.txt", "/docs/review/file.txt", WorkflowActor{ID: "carol", Groups: []string{"authors"}}, map[string]interface{}{"version": 1})
	require.NoError(t, err)

	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.True(t, record.Success)
	assert.Equal(t, "draft", record.FromState)
	assert.Equal(t, "review", record.ToState)
	assert.Equal(t, "move", record.Operation)
	require.NotEmpty(t, mockEvaluator.inputs)
	assert.Equal(t, "review", mockEvaluator.inputs[0].Transition.To)
}

func TestWorkflowEngine_ValidateMove_GateDenied(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool { return false }}
	engine.gateEvaluator = mockEvaluator

	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file.txt", "/docs/review/file.txt", WorkflowActor{ID: "dave", Groups: []string{"authors"}}, nil)
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrWorkflowGateDenied, verr.Code)

	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.False(t, record.Success)
	assert.Equal(t, "move", record.Operation)
}

func TestWorkflowEngine_ValidateMove_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file.txt", "/docs/unknown/file.txt", WorkflowActor{ID: "erin", Groups: []string{"authors"}}, nil)
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrTransitionStateNotFound, verr.Code)
}

func TestWorkflowEngine_SystemAdminBypass(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool { return false }}
	engine.gateEvaluator = mockEvaluator

	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file.txt", "/docs/review/file.txt", WorkflowActor{ID: "frank", Groups: []string{"system-admin"}}, nil)
	require.NoError(t, err)
	require.Len(t, auditRepo.records, 1)
	assert.Len(t, mockEvaluator.inputs, 0)
}

func TestWorkflowEngine_ValidateDelete_GateDenied(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool { return false }}
	engine.gateEvaluator = mockEvaluator

	err := engine.ValidateDeleteOperation(ctx, "/docs/draft/file.txt", WorkflowActor{ID: "gina", Groups: []string{"authors"}}, map[string]interface{}{"owner": "gina"})
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrWorkflowGateDenied, verr.Code)

	require.Len(t, auditRepo.records, 1)
	assert.False(t, auditRepo.records[0].Success)
}

func TestWorkflowEngine_ValidateDirectoryOperation_Protected(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)

	err := engine.ValidateDirectoryOperation(ctx, "/docs/draft", "delete", WorkflowActor{ID: "harry", Groups: []string{"authors"}})
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrWorkflowStateProtected, verr.Code)

	require.Len(t, auditRepo.records, 1)
	assert.False(t, auditRepo.records[0].Success)
}

func TestWorkflowEngine_GetValidTransitions(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool {
		return input.Transition.To == "review"
	}}
	engine.gateEvaluator = mockEvaluator

	transitions, err := engine.GetValidTransitions(ctx, "/docs/draft/file.txt", WorkflowActor{ID: "ivy", Groups: []string{"authors"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"review"}, transitions)
}

type mockGateEvaluator struct {
	allowFunc func(*WorkflowGateInput) bool
	inputs    []*WorkflowGateInput
}

func (m *mockGateEvaluator) Evaluate(_ context.Context, _ *WorkflowDefinition, input *WorkflowGateInput) (*GateEvaluationResult, error) {
	m.inputs = append(m.inputs, input)
	allowed := false
	if m.allowFunc != nil {
		allowed = m.allowFunc(input)
	}
	return &GateEvaluationResult{Allowed: allowed, PolicyType: "mock"}, nil
}

type mockAuditRepository struct {
	records []*models.WorkflowAudit
}

func (m *mockAuditRepository) Create(_ context.Context, audit *models.WorkflowAudit) error {
	if audit.ID == "" {
		audit.ID = "mock"
	}
	m.records = append(m.records, audit)
	return nil
}

func setupWorkflowEngine(t *testing.T, ctx context.Context) (*WorkflowEngine, *mockAuditRepository) {
	db := setupTestDB(t)
	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	docs := createTestDirectory(t, db, root, "docs")
	createTestDirectory(t, db, docs, "draft")
	createTestDirectory(t, db, docs, "review")

	workflowContent := strings.TrimSpace(`
state_directories:
  draft: "draft"
  review: "review"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
  review:
    transitions: []
`) + "\n"
	createWorkflowFile(t, db, docs, workflowContent)

	// ensure definition cached for operations
	_, err = loader.LoadForPath(ctx, "/docs/draft/file.txt")
	require.NoError(t, err)

	auditRepo := &mockAuditRepository{}
	engine := NewWorkflowEngine(loader, nil, fileRepo, dirRepo, auditRepo, nil) // nil eventDispatcher for tests
	return engine, auditRepo
}

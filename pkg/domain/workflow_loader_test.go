package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mysqlrepo "github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

func TestWorkflowLoader_LoadForPath(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	documents := createTestDirectory(t, db, root, "documents")
	createTestDirectory(t, db, documents, "drafts")
	createTestDirectory(t, db, documents, "review")

	workflowContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
  review: "review"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
  review:
    transitions: []
`) + "\n"
	createWorkflowFile(t, db, documents, workflowContent)

	definition, err := loader.LoadForPath(ctx, "/documents/drafts/report.txt")
	require.NoError(t, err)
	require.NotNil(t, definition)

	assert.Equal(t, "/documents/.workflow", definition.WorkflowPath)
	assert.Equal(t, "draft", definition.InitialState)
	assert.Equal(t, "/documents/drafts", definition.StateDirectories["draft"])
	assert.Equal(t, "/documents/review", definition.StateDirectories["review"])

	state, err := definition.GetCurrentState("/documents/drafts/report.txt")
	require.NoError(t, err)
	assert.Equal(t, "draft", state)

	state, err = definition.GetCurrentState("/documents/review/notes.txt")
	require.NoError(t, err)
	assert.Equal(t, "review", state)

	assert.True(t, definition.IsStateDirectory("/documents/drafts"))
	assert.False(t, definition.IsStateDirectory("/documents"))

	stateDir, err := definition.GetStateDirectoryPath("review")
	require.NoError(t, err)
	assert.Equal(t, "/documents/review", stateDir)

	definitionCached, err := loader.LoadForPath(ctx, "/documents/review/summary.txt")
	require.NoError(t, err)
	assert.True(t, definition == definitionCached, "expected cached workflow definition")
}

func TestWorkflowLoader_StateDirectoryMissing(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	documents := createTestDirectory(t, db, root, "documents")
	createTestDirectory(t, db, documents, "drafts")

	workflowContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
  review: "review"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
  review:
    transitions: []
`) + "\n"
	createWorkflowFile(t, db, documents, workflowContent)

	_, err = loader.LoadForPath(ctx, "/documents/drafts/file.txt")
	require.Error(t, err)

	var wErr *WorkflowValidationError
	require.True(t, errors.As(err, &wErr))
	assert.Equal(t, ErrStateDirectoryNotFound, wErr.Code)
}

func TestWorkflowLoader_GatePolicyRefMissing(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	documents := createTestDirectory(t, db, root, "documents")
	createTestDirectory(t, db, documents, "drafts")

	workflowContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
initial_state: draft
states:
  draft:
    transitions: []
gate_policy_ref: ".workflow.rego"
`) + "\n"
	createWorkflowFile(t, db, documents, workflowContent)

	_, err = loader.LoadForPath(ctx, "/documents/drafts/file.txt")
	require.Error(t, err)
	var wErr *WorkflowValidationError
	require.True(t, errors.As(err, &wErr))
	assert.Equal(t, ErrGatePolicyNotFound, wErr.Code)
}

func TestWorkflowLoader_NestedWorkflowDetection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	documents := createTestDirectory(t, db, root, "documents")
	subDir := createTestDirectory(t, db, documents, "sub")

	workflowContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
initial_state: draft
states:
  draft:
    transitions: []
`) + "\n"
	createTestDirectory(t, db, documents, "drafts")
	createWorkflowFile(t, db, documents, workflowContent)
	createWorkflowFile(t, db, subDir, workflowContent)

	_, err = loader.LoadForPath(ctx, "/documents/drafts/file.txt")
	require.Error(t, err)
	var wErr *WorkflowValidationError
	require.True(t, errors.As(err, &wErr))
	assert.Equal(t, ErrNestedWorkflow, wErr.Code)
}

func TestWorkflowLoader_CacheInvalidation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	documents := createTestDirectory(t, db, root, "documents")
	createTestDirectory(t, db, documents, "drafts")
	createTestDirectory(t, db, documents, "review")

	initialContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
  review: "review"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
  review:
    transitions: []
`) + "\n"
	workflowFile := createWorkflowFile(t, db, documents, initialContent)

	definition1, err := loader.LoadForPath(ctx, "/documents/drafts/file.txt")
	require.NoError(t, err)
	assert.Equal(t, "draft", definition1.InitialState)

	updatedContent := strings.TrimSpace(`
state_directories:
  draft: "drafts"
  review: "review"
initial_state: review
states:
  draft:
    transitions:
      - to: review
  review:
    transitions: []
`) + "\n"
	updateWorkflowFile(t, db, workflowFile, updatedContent)

	definition2, err := loader.LoadForPath(ctx, "/documents/review/file.txt")
	require.NoError(t, err)
	assert.True(t, definition1 == definition2)
	assert.Equal(t, "draft", definition2.InitialState)

	loader.Invalidate("/documents/.workflow")

	definition3, err := loader.LoadForPath(ctx, "/documents/review/file.txt")
	require.NoError(t, err)
	assert.NotEqual(t, definition1, definition3)
	assert.Equal(t, "review", definition3.InitialState)
}

func TestWorkflowLoader_NoWorkflowFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	createTestDirectory(t, db, root, "documents")

	_, err = loader.LoadForPath(ctx, "/documents/file.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestWorkflowDefinition_GetCurrentStateOutsideScope(t *testing.T) {
	definition := &WorkflowDefinition{
		WorkflowPath: "/docs/.workflow",
		WorkflowHome: "/docs",
		StateDirectories: map[string]string{
			"draft":  "/docs/drafts",
			"review": "/docs/review",
		},
		stateDirectoryList: []stateDirectoryEntry{
			{state: "draft", path: "/docs/drafts"},
			{state: "review", path: "/docs/review"},
		},
	}

	_, err := definition.GetCurrentState("/other/file.txt")
	assert.Error(t, err)
}

// helper functions moved to workflow_test_helpers_test.go

package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflowDefinition_GetWorkflowHome tests GetWorkflowHome method
func TestWorkflowDefinition_GetWorkflowHome(t *testing.T) {
	t.Run("returns workflow home", func(t *testing.T) {
		def := &WorkflowDefinition{
			WorkflowHome: "/documents",
		}
		assert.Equal(t, "/documents", def.GetWorkflowHome())
	})

	t.Run("returns empty string for nil definition", func(t *testing.T) {
		var def *WorkflowDefinition
		assert.Equal(t, "", def.GetWorkflowHome())
	})
}

// TestWorkflowDefinition_GetStateDirectory tests GetStateDirectory method
func TestWorkflowDefinition_GetStateDirectory(t *testing.T) {
	def := &WorkflowDefinition{
		StateDirectories: map[string]string{
			"draft":  "/documents/drafts",
			"review": "/documents/review",
		},
	}

	t.Run("returns state directory path", func(t *testing.T) {
		path, err := def.GetStateDirectory("draft")
		require.NoError(t, err)
		assert.Equal(t, "/documents/drafts", path)
	})

	t.Run("returns error for unknown state", func(t *testing.T) {
		_, err := def.GetStateDirectory("unknown")
		require.Error(t, err)
		var verr *WorkflowValidationError
		require.True(t, errors.As(err, &verr))
		assert.Equal(t, ErrStateDirectoryNotFound, verr.Code)
	})

	t.Run("returns error for nil definition", func(t *testing.T) {
		var def *WorkflowDefinition
		_, err := def.GetStateDirectory("draft")
		require.Error(t, err)
	})
}

// TestWorkflowLoader_InvalidateAll tests InvalidateAll method
func TestWorkflowLoader_InvalidateAll(t *testing.T) {
	// Just test that InvalidateAll doesn't panic on nil loader
	var loader *WorkflowLoader
	loader.InvalidateAll() // Should not panic

	// Test with actual loader
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Create loader from engine's workflowLoader field
	// We're testing that InvalidateAll can be called without panicking
	if engine.workflowLoader != nil {
		engine.workflowLoader.InvalidateAll()
	}
}

// TestWorkflowLoader_NilLoaderSafety tests nil loader safety
func TestWorkflowLoader_NilLoaderSafety(t *testing.T) {
	var loader *WorkflowLoader

	// Invalidate should not panic
	loader.Invalidate("/test/.workflow")

	// InvalidateAll should not panic
	loader.InvalidateAll()
}

// TestWorkflowEngine_ValidateDeleteOperation_Success tests successful delete validation
func TestWorkflowEngine_ValidateDeleteOperation_Success(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)
	mockEvaluator := &mockGateEvaluator{allowFunc: func(input *WorkflowGateInput) bool { return true }}
	engine.gateEvaluator = mockEvaluator

	err := engine.ValidateDeleteOperation(ctx, "/docs/draft/file.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}}, map[string]interface{}{"version": 1})
	require.NoError(t, err)

	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.True(t, record.Success)
	assert.Equal(t, "delete", record.Operation)
}

// TestWorkflowEngine_ValidateDeleteOperation_NoWorkflow tests delete with no workflow
func TestWorkflowEngine_ValidateDeleteOperation_NoWorkflow(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Try to delete from non-workflow directory
	err := engine.ValidateDeleteOperation(ctx, "/other/file.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}}, nil)
	require.NoError(t, err) // Should succeed as there's no workflow to enforce
}

// TestWorkflowEngine_ValidateDirectoryOperation_NonStateDirectory tests directory operations on non-state directories
func TestWorkflowEngine_ValidateDirectoryOperation_NonStateDirectory(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Try to delete a non-state directory within workflow
	err := engine.ValidateDirectoryOperation(ctx, "/docs/other", "delete", WorkflowActor{ID: "alice", Groups: []string{"authors"}})
	require.NoError(t, err) // Should succeed as it's not a state directory
}

// TestWorkflowEngine_ValidateDirectoryOperation_NoWorkflow tests directory operations with no workflow
func TestWorkflowEngine_ValidateDirectoryOperation_NoWorkflow(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Try to delete a directory outside workflow scope
	err := engine.ValidateDirectoryOperation(ctx, "/other/dir", "delete", WorkflowActor{ID: "alice", Groups: []string{"authors"}})
	require.NoError(t, err) // Should succeed as there's no workflow
}

// TestWorkflowEngine_GetValidTransitions_SystemAdmin tests system admin gets all transitions
func TestWorkflowEngine_GetValidTransitions_SystemAdmin(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	actor := WorkflowActor{ID: "admin", Groups: []string{"system-admin"}}
	transitions, err := engine.GetValidTransitions(ctx, "/docs/draft/file.txt", actor)
	require.NoError(t, err)

	// System admin should see all transitions from draft (which is only "review" in test setup)
	assert.Contains(t, transitions, "review")
	assert.Len(t, transitions, 1) // Only one transition defined in test setup
}

// TestWorkflowEngine_GetValidTransitions_NoWorkflow tests getting transitions with no workflow
func TestWorkflowEngine_GetValidTransitions_NoWorkflow(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	actor := WorkflowActor{ID: "user", Groups: []string{"authors"}}
	transitions, err := engine.GetValidTransitions(ctx, "/other/file.txt", actor)
	require.NoError(t, err)
	assert.Empty(t, transitions) // No workflow means no transitions
}

// TestWorkflowEngine_SameStateMove tests moving within the same state
func TestWorkflowEngine_SameStateMove(t *testing.T) {
	ctx := context.Background()
	engine, auditRepo := setupWorkflowEngine(t, ctx)

	// Move within draft state (rename)
	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file1.txt", "/docs/draft/file2.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}}, nil)
	require.NoError(t, err)

	// Should succeed - same state moves are allowed
	require.Len(t, auditRepo.records, 1)
	record := auditRepo.records[0]
	assert.True(t, record.Success)
	assert.Equal(t, "draft", record.FromState)
	assert.Equal(t, "draft", record.ToState)
}

// TestWorkflowEngine_MoveOutsideWorkflow tests moving to outside workflow scope
func TestWorkflowEngine_MoveOutsideWorkflow(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Try to move from workflow to outside
	err := engine.ValidateMoveOperation(ctx, "/docs/draft/file.txt", "/other/file.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}}, nil)
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrWorkflowScopeViolation, verr.Code)
}

// TestWorkflowEngine_MoveFromOutsideToWorkflow tests moving from outside to workflow
func TestWorkflowEngine_MoveFromOutsideToWorkflow(t *testing.T) {
	ctx := context.Background()
	engine, _ := setupWorkflowEngine(t, ctx)

	// Move from outside to workflow initial state should succeed
	// Workflow doesn't govern files outside its scope
	err := engine.ValidateMoveOperation(ctx, "/other/file.txt", "/docs/draft/file.txt", WorkflowActor{ID: "alice", Groups: []string{"authors"}}, nil)
	require.NoError(t, err) // Should succeed - files entering workflow go to initial state
}

// TestWorkflowActor_IsSystemAdmin tests the IsSystemAdmin method
func TestWorkflowActor_IsSystemAdmin(t *testing.T) {
	t.Run("returns true for system-admin", func(t *testing.T) {
		actor := WorkflowActor{ID: "admin", Groups: []string{"system-admin"}}
		assert.True(t, actor.IsSystemAdmin())
	})

	t.Run("returns true with mixed groups", func(t *testing.T) {
		actor := WorkflowActor{ID: "admin", Groups: []string{"editors", "system-admin", "viewers"}}
		assert.True(t, actor.IsSystemAdmin())
	})

	t.Run("returns false without system-admin", func(t *testing.T) {
		actor := WorkflowActor{ID: "user", Groups: []string{"editors", "viewers"}}
		assert.False(t, actor.IsSystemAdmin())
	})

	t.Run("returns false with empty groups", func(t *testing.T) {
		actor := WorkflowActor{ID: "user", Groups: []string{}}
		assert.False(t, actor.IsSystemAdmin())
	})
}

// TestWorkflowValidationError_Error tests the Error method
func TestWorkflowValidationError_Error(t *testing.T) {
	t.Run("returns message without details", func(t *testing.T) {
		err := &WorkflowValidationError{
			Code:    ErrWorkflowGateDenied,
			Message: "gate denied access",
		}
		assert.Contains(t, err.Error(), "gate denied access")
	})

	t.Run("returns message with details", func(t *testing.T) {
		err := &WorkflowValidationError{
			Code:    ErrWorkflowGateDenied,
			Message: "gate denied access",
			Details: map[string]interface{}{
				"gate": "require_approval",
				"user": "alice",
			},
		}
		errStr := err.Error()
		assert.Contains(t, errStr, "gate denied access")
		// Details are not included in Error() output, they're for structured logging
	})

	t.Run("returns message without code when message is empty", func(t *testing.T) {
		err := &WorkflowValidationError{
			Code: ErrWorkflowGateDenied,
		}
		errStr := err.Error()
		assert.Equal(t, string(ErrWorkflowGateDenied), errStr)
	})
}

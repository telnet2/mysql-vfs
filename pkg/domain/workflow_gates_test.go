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

func TestWorkflowGateEvaluator_InlinePolicy(t *testing.T) {
	ctx := context.Background()
	evaluator := NewWorkflowGateEvaluator(nil, time.Minute)

	workflow := &WorkflowDefinition{
		WorkflowPath:             "/docs/.workflow",
		WorkflowHome:             "/docs",
		InitialState:             "draft",
		StateDirectories:         map[string]string{"draft": "/docs/draft", "review": "/docs/review"},
		RelativeStateDirectories: map[string]string{"draft": "draft", "review": "review"},
		States: map[string]StateDefinition{
			"draft": {
				Transitions: []TransitionDefinition{{To: "review"}},
			},
			"review": {
				Transitions: []TransitionDefinition{},
			},
		},
		GatePolicy: strings.TrimSpace(`
package vfs.workflow.gates

allow {
	"reviewers" == input.user.groups[_]
}
`),
	}

	input := &WorkflowGateInput{
		User:       WorkflowGateUser{ID: "alice", Groups: []string{"reviewers"}},
		Transition: WorkflowGateTransition{Operation: "move", From: "draft", To: "review"},
		File:       WorkflowGateFile{Path: "/docs/draft/file.txt", Name: "file.txt"},
		Workflow:   WorkflowGateWorkflow{Path: workflow.WorkflowPath, Home: workflow.WorkflowHome, InitialState: workflow.InitialState, States: workflow.States, StateDirectories: workflow.StateDirectories, RelativeDirs: workflow.RelativeStateDirectories},
	}

	result, err := evaluator.Evaluate(ctx, workflow, input)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "inline", result.PolicyType)

	input.User.Groups = []string{"authors"}
	result, err = evaluator.Evaluate(ctx, workflow, input)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestWorkflowGateEvaluator_ExternalPolicy(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)
	loader := NewWorkflowLoader(fileRepo, dirRepo, time.Minute)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	docs := createTestDirectory(t, db, root, "docs")
	draftDir := createTestDirectory(t, db, docs, "draft")
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
gate_policy_ref: ".workflow.rego"
`) + "\n"
	createWorkflowFile(t, db, docs, workflowContent)

	regoContent := strings.TrimSpace(`
package vfs.workflow.gates

allow {
	input.transition.to == "review"
	"reviewers" == input.user.groups[_]
}
`) + "\n"
	createTestFile(t, db, docs, ".workflow.rego", regoContent, "application/octet-stream")

	definition, err := loader.LoadForPath(ctx, draftDir.Path+"/file.txt")
	require.NoError(t, err)
	require.NotNil(t, definition)

	evaluator := NewWorkflowGateEvaluator(fileRepo, time.Minute)

	input := &WorkflowGateInput{
		User:       WorkflowGateUser{ID: "carol", Groups: []string{"reviewers"}},
		Transition: WorkflowGateTransition{Operation: "move", From: "draft", To: "review", SourcePath: draftDir.Path + "/file.txt"},
		File:       WorkflowGateFile{Path: draftDir.Path + "/file.txt", Name: "file.txt"},
		Workflow:   WorkflowGateWorkflow{Path: definition.WorkflowPath, Home: definition.WorkflowHome, InitialState: definition.InitialState, States: definition.States, StateDirectories: definition.StateDirectories, RelativeDirs: definition.RelativeStateDirectories},
	}

	result, err := evaluator.Evaluate(ctx, definition, input)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "external", result.PolicyType)

	input.User.Groups = []string{"authors"}
	result, err = evaluator.Evaluate(ctx, definition, input)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestWorkflowGateEvaluator_CacheInvalidation(t *testing.T) {
	ctx := context.Background()
	evaluator := NewWorkflowGateEvaluator(nil, time.Minute)

	workflow := &WorkflowDefinition{
		WorkflowPath:             "/docs/.workflow",
		WorkflowHome:             "/docs",
		InitialState:             "draft",
		StateDirectories:         map[string]string{"draft": "/docs/draft"},
		RelativeStateDirectories: map[string]string{"draft": "draft"},
		States:                   map[string]StateDefinition{"draft": {Transitions: []TransitionDefinition{{To: "review"}}}},
		GatePolicy: `package vfs.workflow.gates
allow { true }
`,
	}

	input := &WorkflowGateInput{
		User:       WorkflowGateUser{ID: "eve", Groups: []string{"reviewers"}},
		Transition: WorkflowGateTransition{Operation: "move", From: "draft", To: "review"},
		File:       WorkflowGateFile{Path: "/docs/draft/file.txt", Name: "file.txt"},
		Workflow:   WorkflowGateWorkflow{Path: workflow.WorkflowPath, Home: workflow.WorkflowHome, InitialState: workflow.InitialState, States: workflow.States, StateDirectories: workflow.StateDirectories, RelativeDirs: workflow.RelativeStateDirectories},
	}

	_, err := evaluator.Evaluate(ctx, workflow, input)
	require.NoError(t, err)

	count := 0
	evaluator.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 1, count)

	evaluator.Invalidate(workflow.WorkflowPath)
	count = 0
	evaluator.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count)
}

func TestWorkflowGateEvaluator_MissingExternalPolicy(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	dirRepo := mysqlrepo.NewGormDirectoryRepository(db)
	fileRepo := mysqlrepo.NewGormFileRepository(db, nil)

	root, err := dirRepo.FindByPath(ctx, "/")
	require.NoError(t, err)

	docs := createTestDirectory(t, db, root, "docs")
	createTestDirectory(t, db, docs, "draft")
	createTestDirectory(t, db, docs, "review")

	definition := &WorkflowDefinition{
		WorkflowPath:             docs.Path + "/.workflow",
		WorkflowHome:             docs.Path,
		InitialState:             "draft",
		StateDirectories:         map[string]string{"draft": docs.Path + "/draft", "review": docs.Path + "/review"},
		RelativeStateDirectories: map[string]string{"draft": "draft", "review": "review"},
		States: map[string]StateDefinition{
			"draft":  {Transitions: []TransitionDefinition{{To: "review"}}},
			"review": {Transitions: []TransitionDefinition{}},
		},
		GatePolicyRef:       ".workflow.rego",
		WorkflowDirectoryID: docs.ID,
	}

	evaluator := NewWorkflowGateEvaluator(fileRepo, time.Minute)
	input := &WorkflowGateInput{
		User:       WorkflowGateUser{ID: "mallory", Groups: []string{"reviewers"}},
		Transition: WorkflowGateTransition{Operation: "move", From: "draft", To: "review"},
		File:       WorkflowGateFile{Path: docs.Path + "/draft/file.txt", Name: "file.txt"},
		Workflow:   WorkflowGateWorkflow{Path: definition.WorkflowPath, Home: definition.WorkflowHome, InitialState: definition.InitialState, States: definition.States, StateDirectories: definition.StateDirectories, RelativeDirs: definition.RelativeStateDirectories},
	}

	_, err = evaluator.Evaluate(ctx, definition, input)
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrGatePolicyNotFound, verr.Code)
}

func TestWorkflowGateEvaluator_InvalidPolicy(t *testing.T) {
	ctx := context.Background()
	evaluator := NewWorkflowGateEvaluator(nil, time.Minute)
	workflow := &WorkflowDefinition{
		WorkflowPath:             "/docs/.workflow",
		WorkflowHome:             "/docs",
		InitialState:             "draft",
		StateDirectories:         map[string]string{"draft": "/docs/draft"},
		RelativeStateDirectories: map[string]string{"draft": "draft"},
		States:                   map[string]StateDefinition{"draft": {Transitions: []TransitionDefinition{{To: "review"}}}},
		GatePolicy:               "package vfs.workflow.gates\nallow { invalid syntax }",
	}

	input := &WorkflowGateInput{
		User:       WorkflowGateUser{ID: "trent", Groups: []string{"reviewers"}},
		Transition: WorkflowGateTransition{Operation: "move", From: "draft", To: "review"},
		File:       WorkflowGateFile{Path: "/docs/draft/file.txt", Name: "file.txt"},
		Workflow:   WorkflowGateWorkflow{Path: workflow.WorkflowPath, Home: workflow.WorkflowHome, InitialState: workflow.InitialState, States: workflow.States, StateDirectories: workflow.StateDirectories, RelativeDirs: workflow.RelativeStateDirectories},
	}

	_, err := evaluator.Evaluate(ctx, workflow, input)
	require.Error(t, err)
	var verr *WorkflowValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, ErrInvalidGatePolicy, verr.Code)
}

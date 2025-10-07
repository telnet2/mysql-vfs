package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/events"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// WorkflowActor represents the actor performing an operation
type WorkflowActor struct {
	ID     string
	Groups []string
}

// IsSystemAdmin returns true if the actor has system-admin privileges
func (a WorkflowActor) IsSystemAdmin() bool {
	for _, group := range a.Groups {
		if group == "system-admin" {
			return true
		}
	}
	return false
}

// WorkflowValidator defines workflow validation capabilities required by services
type WorkflowValidator interface {
	ValidateCreateOperation(ctx context.Context, filePath string, actor WorkflowActor) error
	ValidateMoveOperation(ctx context.Context, sourcePath, destPath string, actor WorkflowActor, metadata map[string]interface{}) error
	ValidateDeleteOperation(ctx context.Context, filePath string, actor WorkflowActor, metadata map[string]interface{}) error
	ValidateDirectoryOperation(ctx context.Context, dirPath, operation string, actor WorkflowActor) error
	GetValidTransitions(ctx context.Context, filePath string, actor WorkflowActor) ([]string, error)
}

type gateEvaluator interface {
	Evaluate(ctx context.Context, workflow *WorkflowDefinition, input *WorkflowGateInput) (*GateEvaluationResult, error)
}

// WorkflowEngine coordinates workflow validation and gate evaluation
type WorkflowEngine struct {
	workflowLoader  *WorkflowLoader
	gateEvaluator   gateEvaluator
	fileRepo        db.FileRepository
	dirRepo         db.DirectoryRepository
	auditRepo       db.WorkflowAuditRepository
	eventDispatcher events.EventTrigger
}

// NewWorkflowEngine creates a workflow engine instance
func NewWorkflowEngine(loader *WorkflowLoader, evaluator *WorkflowGateEvaluator, fileRepo db.FileRepository, dirRepo db.DirectoryRepository, auditRepo db.WorkflowAuditRepository, eventDispatcher events.EventTrigger) *WorkflowEngine {
	return &WorkflowEngine{
		workflowLoader:  loader,
		gateEvaluator:   evaluator,
		fileRepo:        fileRepo,
		dirRepo:         dirRepo,
		auditRepo:       auditRepo,
		eventDispatcher: eventDispatcher,
	}
}

// ValidateCreateOperation ensures new files are created in the initial state directory
func (e *WorkflowEngine) ValidateCreateOperation(ctx context.Context, filePath string, actor WorkflowActor) error {
	definition, err := e.loadWorkflowDefinition(ctx, filePath)
	if err != nil || definition == nil {
		return err
	}

	state, err := definition.GetCurrentState(filePath)
	if err != nil {
		return err
	}

	if actor.IsSystemAdmin() {
		return e.recordAudit(ctx, definition, filePath, definition.InitialState, definition.InitialState, "create", actor, &GateEvaluationResult{Allowed: true, PolicyType: "bypass"}, true, nil)
	}

	if state != definition.InitialState {
		err := newWorkflowValidationError(ErrWorkflowInitialStateOnly, fmt.Sprintf("files must be created in initial state '%s'", definition.InitialState), map[string]interface{}{"file_path": filePath, "state": state})
		return e.recordFailure(ctx, definition, filePath, state, state, "create", actor, &GateEvaluationResult{Allowed: false, PolicyType: "none"}, err)
	}

	return e.recordAudit(ctx, definition, filePath, state, state, "create", actor, &GateEvaluationResult{Allowed: true, PolicyType: "none"}, true, nil)
}

// ValidateMoveOperation validates transitions between workflow states
func (e *WorkflowEngine) ValidateMoveOperation(ctx context.Context, sourcePath, destPath string, actor WorkflowActor, metadata map[string]interface{}) error {
	definition, err := e.loadWorkflowDefinition(ctx, sourcePath)
	if err != nil || definition == nil {
		return err
	}

	fromState, err := definition.GetCurrentState(sourcePath)
	if err != nil {
		return err
	}

	if !isDescendantPath(destPath, definition.WorkflowHome) {
		err := newWorkflowValidationError(ErrWorkflowScopeViolation, "destination path escapes workflow scope", map[string]interface{}{"source": sourcePath, "destination": destPath})
		return e.recordFailure(ctx, definition, sourcePath, fromState, fromState, "move", actor, &GateEvaluationResult{Allowed: false, PolicyType: "none"}, err)
	}

	toState, err := definition.GetCurrentState(destPath)
	if err != nil {
		err := newWorkflowValidationError(ErrTransitionStateNotFound, fmt.Sprintf("destination path '%s' does not map to a workflow state", destPath), map[string]interface{}{"destination": destPath})
		return e.recordFailure(ctx, definition, sourcePath, fromState, fromState, "move", actor, &GateEvaluationResult{Allowed: false, PolicyType: "none"}, err)
	}

	if toState == fromState {
		return e.recordAudit(ctx, definition, sourcePath, fromState, toState, "move", actor, &GateEvaluationResult{Allowed: true, PolicyType: "none"}, true, nil)
	}

	if !hasTransition(definition, fromState, toState) {
		err := newWorkflowValidationError(ErrWorkflowTransitionDenied, fmt.Sprintf("transition from '%s' to '%s' not allowed", fromState, toState), map[string]interface{}{"source": sourcePath, "destination": destPath})
		return e.recordFailure(ctx, definition, sourcePath, fromState, toState, "move", actor, &GateEvaluationResult{Allowed: false, PolicyType: "none"}, err)
	}

	if actor.IsSystemAdmin() {
		return e.recordAudit(ctx, definition, sourcePath, fromState, toState, "move", actor, &GateEvaluationResult{Allowed: true, PolicyType: "bypass"}, true, nil)
	}

	result, err := e.evaluateGate(ctx, definition, WorkflowGateInput{
		User: WorkflowGateUser{ID: actor.ID, Groups: actor.Groups},
		Transition: WorkflowGateTransition{
			Operation:       "move",
			From:            fromState,
			To:              toState,
			SourcePath:      sourcePath,
			DestinationPath: destPath,
		},
		File: WorkflowGateFile{
			Path:     sourcePath,
			Name:     path.Base(sourcePath),
			Metadata: metadata,
		},
		Workflow: WorkflowGateWorkflow{
			Path:             definition.WorkflowPath,
			Home:             definition.WorkflowHome,
			InitialState:     definition.InitialState,
			States:           definition.States,
			StateDirectories: definition.StateDirectories,
			RelativeDirs:     definition.RelativeStateDirectories,
		},
	})
	if err != nil {
		return e.recordFailure(ctx, definition, sourcePath, fromState, toState, "move", actor, &GateEvaluationResult{Allowed: false, PolicyType: "error"}, err)
	}

	if !result.Allowed {
		err := newWorkflowValidationError(ErrWorkflowGateDenied, fmt.Sprintf("gate denied transition from '%s' to '%s'", fromState, toState), map[string]interface{}{"source": sourcePath, "destination": destPath})
		return e.recordFailure(ctx, definition, sourcePath, fromState, toState, "move", actor, result, err)
	}

	return e.recordAudit(ctx, definition, sourcePath, fromState, toState, "move", actor, result, true, nil)
}

// ValidateDeleteOperation validates deletion operations within a workflow
func (e *WorkflowEngine) ValidateDeleteOperation(ctx context.Context, filePath string, actor WorkflowActor, metadata map[string]interface{}) error {
	definition, err := e.loadWorkflowDefinition(ctx, filePath)
	if err != nil || definition == nil {
		return err
	}

	state, err := definition.GetCurrentState(filePath)
	if err != nil {
		return err
	}

	if actor.IsSystemAdmin() {
		return e.recordAudit(ctx, definition, filePath, state, state, "delete", actor, &GateEvaluationResult{Allowed: true, PolicyType: "bypass"}, true, nil)
	}

	result, err := e.evaluateGate(ctx, definition, WorkflowGateInput{
		User: WorkflowGateUser{ID: actor.ID, Groups: actor.Groups},
		Transition: WorkflowGateTransition{
			Operation:  "delete",
			From:       state,
			To:         state,
			SourcePath: filePath,
		},
		File: WorkflowGateFile{
			Path:     filePath,
			Name:     path.Base(filePath),
			Metadata: metadata,
		},
		Workflow: WorkflowGateWorkflow{
			Path:             definition.WorkflowPath,
			Home:             definition.WorkflowHome,
			InitialState:     definition.InitialState,
			States:           definition.States,
			StateDirectories: definition.StateDirectories,
			RelativeDirs:     definition.RelativeStateDirectories,
		},
	})
	if err != nil {
		return e.recordFailure(ctx, definition, filePath, state, state, "delete", actor, &GateEvaluationResult{Allowed: false, PolicyType: "error"}, err)
	}

	if !result.Allowed {
		err := newWorkflowValidationError(ErrWorkflowGateDenied, "gate denied delete operation", map[string]interface{}{"file_path": filePath, "state": state})
		return e.recordFailure(ctx, definition, filePath, state, state, "delete", actor, result, err)
	}

	return e.recordAudit(ctx, definition, filePath, state, state, "delete", actor, result, true, nil)
}

// ValidateDirectoryOperation prevents protected state directory modifications
func (e *WorkflowEngine) ValidateDirectoryOperation(ctx context.Context, dirPath, operation string, actor WorkflowActor) error {
	definition, err := e.loadWorkflowDefinition(ctx, dirPath)
	if err != nil || definition == nil {
		return err
	}

	state, stateErr := definition.GetCurrentState(dirPath)
	if stateErr != nil {
		state = ""
	}

	if definition.IsStateDirectory(dirPath) {
		err := newWorkflowValidationError(ErrWorkflowStateProtected, fmt.Sprintf("state directory '%s' cannot be %sed", dirPath, operation), map[string]interface{}{"operation": operation})
		return e.recordFailure(ctx, definition, dirPath, state, state, "directory", actor, &GateEvaluationResult{Allowed: false, PolicyType: "none"}, err)
	}

	return e.recordAudit(ctx, definition, dirPath, state, state, "directory", actor, &GateEvaluationResult{Allowed: true, PolicyType: "none"}, true, nil)
}

// GetValidTransitions returns transitions allowed by the gate policy for a file
func (e *WorkflowEngine) GetValidTransitions(ctx context.Context, filePath string, actor WorkflowActor) ([]string, error) {
	definition, err := e.loadWorkflowDefinition(ctx, filePath)
	if err != nil || definition == nil {
		return nil, err
	}

	state, err := definition.GetCurrentState(filePath)
	if err != nil {
		return nil, err
	}

	transitions := definition.States[state].Transitions
	if len(transitions) == 0 {
		return nil, nil
	}

	if actor.IsSystemAdmin() {
		result := make([]string, 0, len(transitions))
		for _, tr := range transitions {
			result = append(result, tr.To)
		}
		return result, nil
	}

	allowed := make([]string, 0, len(transitions))
	for _, tr := range transitions {
		result, evalErr := e.evaluateGate(ctx, definition, WorkflowGateInput{
			User: WorkflowGateUser{ID: actor.ID, Groups: actor.Groups},
			Transition: WorkflowGateTransition{
				Operation:  "move",
				From:       state,
				To:         tr.To,
				SourcePath: filePath,
			},
			File: WorkflowGateFile{
				Path: filePath,
				Name: path.Base(filePath),
			},
			Workflow: WorkflowGateWorkflow{
				Path:             definition.WorkflowPath,
				Home:             definition.WorkflowHome,
				InitialState:     definition.InitialState,
				States:           definition.States,
				StateDirectories: definition.StateDirectories,
				RelativeDirs:     definition.RelativeStateDirectories,
			},
		})
		if evalErr != nil {
			return nil, evalErr
		}
		if result.Allowed {
			allowed = append(allowed, tr.To)
		}
	}

	return allowed, nil
}

func (e *WorkflowEngine) evaluateGate(ctx context.Context, workflow *WorkflowDefinition, input WorkflowGateInput) (*GateEvaluationResult, error) {
	if e.gateEvaluator == nil {
		return &GateEvaluationResult{Allowed: true, PolicyType: "none"}, nil
	}
	return e.gateEvaluator.Evaluate(ctx, workflow, &input)
}

func (e *WorkflowEngine) loadWorkflowDefinition(ctx context.Context, path string) (*WorkflowDefinition, error) {
	if e.workflowLoader == nil {
		return nil, nil
	}
	definition, err := e.workflowLoader.LoadForPath(ctx, path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return definition, nil
}

func (e *WorkflowEngine) recordFailure(ctx context.Context, workflow *WorkflowDefinition, filePath, fromState, toState, operation string, actor WorkflowActor, gateResult *GateEvaluationResult, opErr error) error {
	if err := e.recordAudit(ctx, workflow, filePath, fromState, toState, operation, actor, gateResult, false, opErr); err != nil {
		return errors.Join(opErr, err)
	}
	return opErr
}

func (e *WorkflowEngine) recordAudit(ctx context.Context, workflow *WorkflowDefinition, filePath, fromState, toState, operation string, actor WorkflowActor, gateResult *GateEvaluationResult, success bool, opErr error) error {
	if e.auditRepo == nil {
		return nil
	}

	actorJSON, err := json.Marshal(actor.Groups)
	if err != nil {
		return err
	}
	gateJSON, err := json.Marshal(map[string]interface{}{
		"allowed":     gateResult != nil && gateResult.Allowed,
		"policy_type": gateResultPolicyType(gateResult),
	})
	if err != nil {
		return err
	}

	var errorMessage *string
	if opErr != nil {
		msg := opErr.Error()
		errorMessage = &msg
	}

	audit := &models.WorkflowAudit{
		FilePath:       normalizePath(filePath),
		WorkflowPath:   workflow.WorkflowPath,
		FromState:      fallbackState(fromState, workflow.InitialState),
		ToState:        fallbackState(toState, workflow.InitialState),
		Operation:      operation,
		Actor:          actor.ID,
		ActorGroups:    string(actorJSON),
		GatesEvaluated: string(gateJSON),
		Success:        success,
		ErrorMessage:   errorMessage,
		CreatedAt:      time.Now(),
	}

	// Create audit record
	if err := e.auditRepo.Create(ctx, audit); err != nil {
		return err
	}

	// Emit real-time events for observability and automation
	if e.eventDispatcher != nil {
		errMsg := ""
		if errorMessage != nil {
			errMsg = *errorMessage
		}

		payload := events.WorkflowEventPayload{
			FilePath:     audit.FilePath,
			WorkflowPath: audit.WorkflowPath,
			FromState:    audit.FromState,
			ToState:      audit.ToState,
			Operation:    audit.Operation,
			Actor:        events.WorkflowActorContext{ID: actor.ID, Groups: actor.Groups},
			ErrorMessage: errMsg,
			Timestamp:    audit.CreatedAt,
		}

		if success {
			e.eventDispatcher.Emit(ctx, events.EventWorkflowTransitionSucceeded, payload)
		} else {
			e.eventDispatcher.Emit(ctx, events.EventWorkflowTransitionFailed, payload)
		}
	}

	return nil
}

func fallbackState(state, defaultState string) string {
	if strings.TrimSpace(state) == "" {
		return defaultState
	}
	return state
}

func gateResultPolicyType(result *GateEvaluationResult) string {
	if result == nil {
		return "none"
	}
	if strings.TrimSpace(result.PolicyType) == "" {
		if result.Allowed {
			return "none"
		}
		return "unknown"
	}
	return result.PolicyType
}

func hasTransition(definition *WorkflowDefinition, fromState, toState string) bool {
	stateDef, ok := definition.States[fromState]
	if !ok {
		return false
	}
	for _, tr := range stateDef.Transitions {
		if tr.To == toState {
			return true
		}
	}
	return false
}

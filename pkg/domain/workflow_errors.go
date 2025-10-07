package domain

import "fmt"

type WorkflowErrorCode string

const (
	ErrInvalidYAML              WorkflowErrorCode = "WORKFLOW_INVALID_YAML"
	ErrSchemaViolation          WorkflowErrorCode = "WORKFLOW_SCHEMA_VIOLATION"
	ErrInvalidStateName         WorkflowErrorCode = "WORKFLOW_INVALID_STATE_NAME"
	ErrInvalidStatePath         WorkflowErrorCode = "WORKFLOW_INVALID_STATE_PATH"
	ErrInitialStateNotFound     WorkflowErrorCode = "WORKFLOW_INITIAL_STATE_NOT_FOUND"
	ErrTransitionStateNotFound  WorkflowErrorCode = "WORKFLOW_TRANSITION_STATE_NOT_FOUND"
	ErrStateDirectoryNotFound   WorkflowErrorCode = "WORKFLOW_STATE_DIR_NOT_FOUND"
	ErrOrphanedState            WorkflowErrorCode = "WORKFLOW_ORPHANED_STATE"
	ErrNestedWorkflow           WorkflowErrorCode = "WORKFLOW_NESTING_PROHIBITED"
	ErrInvalidGatePolicy        WorkflowErrorCode = "WORKFLOW_INVALID_GATE_POLICY"
	ErrGatePolicyNotFound       WorkflowErrorCode = "WORKFLOW_GATE_POLICY_NOT_FOUND"
	ErrBothGatePolicies         WorkflowErrorCode = "WORKFLOW_BOTH_GATE_POLICIES"
	ErrWorkflowNotFound         WorkflowErrorCode = "WORKFLOW_NOT_FOUND"
	ErrWorkflowScopeViolation   WorkflowErrorCode = "WORKFLOW_SCOPE_VIOLATION"
	ErrWorkflowTransitionDenied WorkflowErrorCode = "WORKFLOW_TRANSITION_DENIED"
	ErrWorkflowGateDenied       WorkflowErrorCode = "WORKFLOW_GATE_DENIED"
	ErrWorkflowInitialStateOnly WorkflowErrorCode = "WORKFLOW_INITIAL_STATE_ONLY"
	ErrWorkflowStateProtected   WorkflowErrorCode = "WORKFLOW_STATE_PROTECTED"
)

// WorkflowValidationError represents a structured validation error with context
type WorkflowValidationError struct {
	Code    WorkflowErrorCode
	Message string
	Details map[string]interface{}
}

func (e *WorkflowValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return string(e.Code)
}

func newWorkflowValidationError(code WorkflowErrorCode, message string, details map[string]interface{}) error {
	return &WorkflowValidationError{Code: code, Message: message, Details: details}
}

package events

import (
	"fmt"
	"strings"
	"time"
)

// EventCategory represents the category of an event
type EventCategory string

const (
	CategoryFile      EventCategory = "file"
	CategoryDirectory EventCategory = "directory"
)

// Operation represents the operation being performed
type Operation string

const (
	OperationCreate Operation = "create"
	OperationRead   Operation = "read"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
	OperationMove   Operation = "move"
	OperationList   Operation = "list"
)

// EventStage represents the lifecycle stage of an operation
type EventStage string

const (
	StageAuthorization EventStage = "authorization"
	StageValidation    EventStage = "validation"
	StageExecution     EventStage = "execution"
	StageCompletion    EventStage = "completion"
)

// Substage represents sub-stages within a lifecycle stage
type Substage string

const (
	// Authorization substages
	SubstageAuthPolicy     Substage = "policy"
	SubstageAuthPermission Substage = "permission"
	SubstageAuthRole       Substage = "role"

	// Validation substages
	SubstageValidationSchema  Substage = "schema"
	SubstageValidationQuota   Substage = "quota"
	SubstageValidationContent Substage = "content"
	SubstageValidationSize    Substage = "size"

	// Execution substages
	SubstageExecLock        Substage = "lock"
	SubstageExecTransaction Substage = "transaction"
	SubstageExecStorage     Substage = "storage"
)

// Action represents the action within a substage
type Action string

const (
	ActionStarted   Action = "started"
	ActionChecking  Action = "checking"
	ActionChecked   Action = "checked"
	ActionAcquiring Action = "acquiring"
	ActionAcquired  Action = "acquired"
	ActionWriting   Action = "writing"
	ActionWritten   Action = "written"
	ActionCommitted Action = "committed"
	ActionRolledBack Action = "rolled_back"
)

// Outcome represents the result of a stage or substage
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeAborted   Outcome = "aborted"
	OutcomeTimeout   Outcome = "timeout"
)

// LifecycleEventType builds a hierarchical event type from components
type LifecycleEventType struct {
	Category  EventCategory `json:"category"`
	Operation Operation     `json:"operation"`
	Stage     EventStage    `json:"stage"`
	Substage  Substage      `json:"substage,omitempty"`
	Action    Action        `json:"action,omitempty"`
	Outcome   Outcome       `json:"outcome,omitempty"`
}

// String returns the full event type string
// Format: {category}.{operation}.{stage}[.{substage}][.{action}][.{outcome}]
func (t *LifecycleEventType) String() string {
	parts := []string{
		string(t.Category),
		string(t.Operation),
		string(t.Stage),
	}

	if t.Substage != "" {
		parts = append(parts, string(t.Substage))
	}
	if t.Action != "" {
		parts = append(parts, string(t.Action))
	}
	if t.Outcome != "" {
		parts = append(parts, string(t.Outcome))
	}

	return strings.Join(parts, ".")
}

// ParseLifecycleEventType parses an event type string into components
func ParseLifecycleEventType(eventType string) (*LifecycleEventType, error) {
	parts := strings.Split(eventType, ".")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid event type: must have at least category.operation.stage")
	}

	evt := &LifecycleEventType{
		Category:  EventCategory(parts[0]),
		Operation: Operation(parts[1]),
		Stage:     EventStage(parts[2]),
	}

	// Parse optional components
	if len(parts) > 3 {
		evt.Substage = Substage(parts[3])
	}
	if len(parts) > 4 {
		evt.Action = Action(parts[4])
	}
	if len(parts) > 5 {
		evt.Outcome = Outcome(parts[5])
	}

	return evt, nil
}

// OperationContext tracks the entire operation lifecycle
type OperationContext struct {
	OperationID   string        `json:"operation_id"`
	Category      EventCategory `json:"category"`
	Operation     Operation     `json:"operation"`
	ResourcePath  string        `json:"resource_path"`
	UserID        string        `json:"user_id"`
	StartedAt     time.Time     `json:"started_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
	CurrentStage  EventStage    `json:"current_stage"`
	Status        string        `json:"status"` // "in_progress", "succeeded", "failed", "aborted"

	// Stage tracking
	AuthorizationStartedAt *time.Time `json:"authorization_started_at,omitempty"`
	AuthorizationEndedAt   *time.Time `json:"authorization_ended_at,omitempty"`
	ValidationStartedAt    *time.Time `json:"validation_started_at,omitempty"`
	ValidationEndedAt      *time.Time `json:"validation_ended_at,omitempty"`
	ExecutionStartedAt     *time.Time `json:"execution_started_at,omitempty"`
	ExecutionEndedAt       *time.Time `json:"execution_ended_at,omitempty"`

	// Error tracking
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorStage   string `json:"error_stage,omitempty"`
}

// StageInfo represents information about a specific stage
type StageInfo struct {
	Stage       string    `json:"stage"`
	Substage    string    `json:"substage,omitempty"`
	Action      string    `json:"action,omitempty"`
	DurationMs  int64     `json:"duration_ms,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// LifecycleEvent represents the enhanced event structure for lifecycle events
type LifecycleEvent struct {
	ID          string              `json:"id"`
	Category    EventCategory       `json:"category"`
	Operation   Operation           `json:"operation"`
	Stage       EventStage          `json:"stage"`
	Substage    Substage            `json:"substage,omitempty"`
	Action      Action              `json:"action,omitempty"`
	Outcome     Outcome             `json:"outcome,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
	OperationID string              `json:"operation_id"`
	StageInfo   *StageInfo          `json:"stage_info,omitempty"`
}

// GetEventType returns the full event type string
func (e *LifecycleEvent) GetEventType() string {
	t := &LifecycleEventType{
		Category:  e.Category,
		Operation: e.Operation,
		Stage:     e.Stage,
		Substage:  e.Substage,
		Action:    e.Action,
		Outcome:   e.Outcome,
	}
	return t.String()
}

// AuthorizationEventPayload represents the payload for authorization events
type AuthorizationEventPayload struct {
	Event           LifecycleEvent `json:"event"`
	Resource        interface{}    `json:"resource"` // FileResource or DirectoryResource
	User            UserContext    `json:"user"`
	Metadata        EventMetadata  `json:"metadata"`

	// Authorization-specific fields
	PolicyName          string   `json:"policy_name,omitempty"`
	RequiredRole        string   `json:"required_role,omitempty"`
	RequiredPermissions []string `json:"required_permissions,omitempty"`
	Decision            string   `json:"decision"` // "allow" or "deny"
	Reason              string   `json:"reason,omitempty"`
	DeniedBy            string   `json:"denied_by,omitempty"` // Which check denied (policy, role, permission)
}

// ValidationEventPayload represents the payload for validation events
type ValidationEventPayload struct {
	Event      LifecycleEvent `json:"event"`
	Resource   interface{}    `json:"resource"` // FileResource or DirectoryResource
	User       UserContext    `json:"user"`
	Metadata   EventMetadata  `json:"metadata"`

	// Validation-specific fields
	ValidationType string      `json:"validation_type"` // "schema", "quota", "content", "size"
	SchemaPath     string      `json:"schema_path,omitempty"`
	Violations     []Violation `json:"violations,omitempty"`
	QuotaUsed      *QuotaUsage `json:"quota_used,omitempty"`
	QuotaLimit     *QuotaLimit `json:"quota_limit,omitempty"`
}

// Violation represents a validation violation
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// QuotaUsage represents current quota usage
type QuotaUsage struct {
	Files     int   `json:"files"`
	SizeBytes int64 `json:"size_bytes"`
	Depth     int   `json:"depth,omitempty"`
}

// QuotaLimit represents quota limits
type QuotaLimit struct {
	MaxFiles     int   `json:"max_files"`
	MaxSizeBytes int64 `json:"max_size_bytes"`
	MaxDepth     int   `json:"max_depth,omitempty"`
	MaxFileSize  int64 `json:"max_file_size,omitempty"`
}

// ExecutionEventPayload represents the payload for execution events
type ExecutionEventPayload struct {
	Event        LifecycleEvent `json:"event"`
	Resource     FileResource   `json:"resource"`
	User         UserContext    `json:"user"`
	Metadata     EventMetadata  `json:"metadata"`

	// Execution-specific fields
	TransactionID string         `json:"transaction_id,omitempty"`
	LockID        string         `json:"lock_id,omitempty"`
	AffectedRows  int            `json:"affected_rows,omitempty"`
	BytesWritten  int64          `json:"bytes_written,omitempty"`
	StoragePath   string         `json:"storage_path,omitempty"`
}

// CompletionEventPayload represents the payload for completion events
type CompletionEventPayload struct {
	Event            LifecycleEvent    `json:"event"`
	Resource         interface{}       `json:"resource"` // FileResource or DirectoryResource
	User             UserContext       `json:"user"`
	Metadata         EventMetadata     `json:"metadata"`
	OperationContext *OperationContext `json:"operation_context"`

	// Completion-specific fields
	Success           bool   `json:"success"`
	TotalDurationMs   int64  `json:"total_duration_ms"`
	ErrorMessage      string `json:"error_message,omitempty"`
	FailedStage       string `json:"failed_stage,omitempty"`
	RollbackPerformed bool   `json:"rollback_performed,omitempty"`
}

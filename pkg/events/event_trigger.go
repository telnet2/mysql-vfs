package events

import (
	"context"
	"fmt"
)

// HandlerResponse represents the response from a handler
type HandlerResponse struct {
	Success bool   `json:"success"`
	Veto    bool   `json:"veto"`    // If true, abort the operation
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"` // Error code if veto
}

// SuccessResponse returns a successful handler response
func SuccessResponse() HandlerResponse {
	return HandlerResponse{
		Success: true,
		Veto:    false,
	}
}

// VetoResponse returns a veto response that aborts the operation
func VetoResponse(message, code string) HandlerResponse {
	return HandlerResponse{
		Success: false,
		Veto:    true,
		Message: message,
		Code:    code,
	}
}

// ErrorResponse returns an error response (non-veto, just logs error)
func ErrorResponse(message string) HandlerResponse {
	return HandlerResponse{
		Success: false,
		Veto:    false,
		Message: message,
	}
}

// EventTrigger is the interface for emitting lifecycle events
type EventTrigger interface {
	// Emit emits an event asynchronously (fire and forget)
	// Used for observability events that don't affect operation flow
	Emit(ctx context.Context, eventType string, payload interface{})

	// EmitSync emits an event synchronously and waits for handler responses
	// Returns error if any handler vetoes the operation
	// Used for critical checks (authorization, validation) that can abort operations
	EmitSync(ctx context.Context, eventType string, payload interface{}) error

	// EmitWithOperation emits an event with operation context tracking
	EmitWithOperation(ctx context.Context, opCtx *OperationContext, eventType string, payload interface{})

	// EmitSyncWithOperation emits an event synchronously with operation context
	EmitSyncWithOperation(ctx context.Context, opCtx *OperationContext, eventType string, payload interface{}) error

	// Wait waits for all pending async handlers to complete
	Wait()

	// Shutdown gracefully shuts down the event trigger
	Shutdown(ctx context.Context) error
}

// VetoError represents an error when a handler vetoes an operation
type VetoError struct {
	HandlerName string
	EventType   string
	Message     string
	Code        string
}

func (e *VetoError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("operation vetoed by handler '%s' on event '%s': %s (code: %s)",
			e.HandlerName, e.EventType, e.Message, e.Code)
	}
	return fmt.Sprintf("operation vetoed by handler '%s' on event '%s': %s",
		e.HandlerName, e.EventType, e.Message)
}

// PatternMatcher matches event types against patterns
type PatternMatcher interface {
	// Match returns true if the event type matches the pattern
	Match(pattern string, eventType string) bool

	// CompilePattern compiles a pattern for faster matching (optional optimization)
	CompilePattern(pattern string) (interface{}, error)

	// MatchCompiled matches using a pre-compiled pattern
	MatchCompiled(compiled interface{}, eventType string) bool
}

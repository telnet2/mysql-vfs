package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/telnet2/mysql-vfs/pkg/domain"
)

// WorkflowHandler handles HTTP requests for workflow operations
type WorkflowHandler struct {
	workflowLoader *domain.WorkflowLoader
	workflowEngine *domain.WorkflowEngine
	fileService    *domain.FileService
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(
	workflowLoader *domain.WorkflowLoader,
	workflowEngine *domain.WorkflowEngine,
	fileService *domain.FileService,
) *WorkflowHandler {
	return &WorkflowHandler{
		workflowLoader: workflowLoader,
		workflowEngine: workflowEngine,
		fileService:    fileService,
	}
}

// WorkflowInfoResponse represents workflow information for a file
type WorkflowInfoResponse struct {
	Active       bool                         `json:"active"`
	WorkflowPath string                       `json:"workflow_path,omitempty"`
	WorkflowHome string                       `json:"workflow_home,omitempty"`
	CurrentState string                       `json:"current_state,omitempty"`
	InitialState string                       `json:"initial_state,omitempty"`
	States       map[string]StateInfoResponse `json:"states,omitempty"`
}

// StateInfoResponse represents information about a workflow state
type StateInfoResponse struct {
	Name        string   `json:"name"`
	Directory   string   `json:"directory"`
	Transitions []string `json:"transitions"`
}

// TransitionsResponse represents available transitions for a file
type TransitionsResponse struct {
	CurrentState     string           `json:"current_state"`
	AvailableStates  []string         `json:"available_states"`
	ValidTransitions []TransitionInfo `json:"valid_transitions"`
}

// TransitionInfo represents information about a single transition
type TransitionInfo struct {
	ToState       string   `json:"to_state"`
	TargetPath    string   `json:"target_path"`
	RequiresGates bool     `json:"requires_gates"`
	GatePolicies  []string `json:"gate_policies,omitempty"`
}

// TransitionRequest represents a request to transition a file to a new state
type TransitionRequest struct {
	TargetState       string `json:"target_state"`
	PreserveStructure *bool  `json:"preserve_structure,omitempty"` // default: true
}

// TransitionResponse represents the response after a successful transition
type TransitionResponse struct {
	Success   bool   `json:"success"`
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	OldPath   string `json:"old_path"`
	NewPath   string `json:"new_path"`
	Message   string `json:"message"`
}

// GetWorkflowInfo handles GET /api/v1/workflows/*filepath/info
func (h *WorkflowHandler) GetWorkflowInfo(ctx context.Context, c *app.RequestContext) {
	// Extract file path from URL parameter
	filePath := c.Param("filepath")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "file path is required",
		})
		return
	}

	// Ensure path starts with /
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	// Load workflow for this path
	workflow, err := h.workflowLoader.LoadForPath(ctx, filePath)
	if err != nil {
		if err == domain.ErrNotFound {
			// No workflow found
			c.JSON(http.StatusOK, WorkflowInfoResponse{
				Active: false,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to load workflow: %v", err),
		})
		return
	}

	if workflow == nil {
		c.JSON(http.StatusOK, WorkflowInfoResponse{
			Active: false,
		})
		return
	}

	// Extract current state
	currentState, err := workflow.GetCurrentState(filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to determine current state: %v", err),
		})
		return
	}

	// Build states information
	states := make(map[string]StateInfoResponse, len(workflow.States))
	for stateName, stateDef := range workflow.States {
		transitions := make([]string, 0, len(stateDef.Transitions))
		for _, trans := range stateDef.Transitions {
			transitions = append(transitions, trans.To)
		}

		stateDir, _ := workflow.GetStateDirectory(stateName)
		states[stateName] = StateInfoResponse{
			Name:        stateName,
			Directory:   stateDir,
			Transitions: transitions,
		}
	}

	response := WorkflowInfoResponse{
		Active:       true,
		WorkflowPath: workflow.WorkflowPath,
		WorkflowHome: workflow.WorkflowHome,
		CurrentState: currentState,
		InitialState: workflow.InitialState,
		States:       states,
	}

	c.JSON(http.StatusOK, response)
}

// GetValidTransitions handles GET /api/v1/workflows/*filepath/transitions
func (h *WorkflowHandler) GetValidTransitions(ctx context.Context, c *app.RequestContext) {
	// Extract file path from URL parameter
	filePath := c.Param("filepath")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "file path is required",
		})
		return
	}

	// Ensure path starts with /
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	// Get user context
	userCtx := extractUserContext(c)
	if userCtx == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "user context not found",
		})
		return
	}

	// Load workflow for this path
	workflow, err := h.workflowLoader.LoadForPath(ctx, filePath)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "no workflow found for this path",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to load workflow: %v", err),
		})
		return
	}

	if workflow == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "no workflow found for this path",
		})
		return
	}

	// Extract current state
	currentState, err := workflow.GetCurrentState(filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to determine current state: %v", err),
		})
		return
	}

	// Create workflow actor from user context
	actor := domain.WorkflowActor{
		ID:     userCtx.UserID,
		Groups: userCtx.Groups,
	}

	// Get valid transitions from workflow engine
	validStates, err := h.workflowEngine.GetValidTransitions(ctx, filePath, actor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to get valid transitions: %v", err),
		})
		return
	}

	// Build transition information
	transitions := make([]TransitionInfo, 0, len(validStates))
	for _, toState := range validStates {
		// Construct target path
		stateDir, _ := workflow.GetStateDirectory(toState)

		// Preserve subdirectory structure
		currentStateDir, _ := workflow.GetStateDirectory(currentState)
		relPath := strings.TrimPrefix(filePath, currentStateDir)
		relPath = strings.TrimPrefix(relPath, "/")
		targetPath := path.Join(stateDir, relPath)

		// Check if this transition exists in the workflow definition
		requiresGates := false
		if stateDef, ok := workflow.States[currentState]; ok {
			for _, trans := range stateDef.Transitions {
				if trans.To == toState {
					// Transitions with gates are evaluated by the workflow engine
					// We indicate that gates may be required but don't expose policy details
					requiresGates = true
					break
				}
			}
		}

		transitions = append(transitions, TransitionInfo{
			ToState:       toState,
			TargetPath:    targetPath,
			RequiresGates: requiresGates,
			GatePolicies:  []string{}, // Gate policies are internal to the workflow system
		})
	}

	// Get all available states
	availableStates := make([]string, 0, len(workflow.States))
	for stateName := range workflow.States {
		availableStates = append(availableStates, stateName)
	}

	response := TransitionsResponse{
		CurrentState:     currentState,
		AvailableStates:  availableStates,
		ValidTransitions: transitions,
	}

	c.JSON(http.StatusOK, response)
}

// TransitionToState handles POST /api/v1/workflows/*filepath/next
func (h *WorkflowHandler) TransitionToState(ctx context.Context, c *app.RequestContext) {
	// Extract file path from URL parameter
	filePath := c.Param("filepath")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "file path is required",
		})
		return
	}

	// Ensure path starts with /
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	// Parse request body
	var req TransitionRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	if req.TargetState == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "target_state is required",
		})
		return
	}

	// Get user context
	userCtx := extractUserContext(c)
	if userCtx == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "user context not found",
		})
		return
	}

	// Load workflow for this path
	workflow, err := h.workflowLoader.LoadForPath(ctx, filePath)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "no workflow found for this path",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to load workflow: %v", err),
		})
		return
	}

	if workflow == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "no workflow found for this path",
		})
		return
	}

	// Extract current state
	currentState, err := workflow.GetCurrentState(filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("failed to determine current state: %v", err),
		})
		return
	}

	// Construct destination path
	preserveStructure := true
	if req.PreserveStructure != nil {
		preserveStructure = *req.PreserveStructure
	}

	var destPath string
	if preserveStructure {
		// Preserve subdirectory structure
		currentStateDir, _ := workflow.GetStateDirectory(currentState)
		targetStateDir, err := workflow.GetStateDirectory(req.TargetState)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("invalid target state: %v", err),
			})
			return
		}

		relPath := strings.TrimPrefix(filePath, currentStateDir)
		relPath = strings.TrimPrefix(relPath, "/")
		destPath = path.Join(targetStateDir, relPath)
	} else {
		// Just move to target state directory root
		targetStateDir, err := workflow.GetStateDirectory(req.TargetState)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("invalid target state: %v", err),
			})
			return
		}
		fileName := path.Base(filePath)
		destPath = path.Join(targetStateDir, fileName)
	}

	// Call FileService.MoveFile to perform the transition
	// This will trigger workflow validation
	_, err = h.fileService.MoveFile(ctx, filePath, destPath)
	if err != nil {
		// Check if it's a workflow validation error
		if strings.Contains(err.Error(), "workflow") {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Error: fmt.Sprintf("transition denied: %v", err),
			})
			return
		}

		statusCode := mapErrorToStatus(err)
		c.JSON(statusCode, ErrorResponse{
			Error: mapErrorToMessage(err),
		})
		return
	}

	response := TransitionResponse{
		Success:   true,
		FromState: currentState,
		ToState:   req.TargetState,
		OldPath:   filePath,
		NewPath:   destPath,
		Message:   fmt.Sprintf("Successfully transitioned from %s to %s", currentState, req.TargetState),
	}

	c.JSON(http.StatusOK, response)
}

// extractUserContext extracts user context from the request context
func extractUserContext(c *app.RequestContext) *UserContextInfo {
	// Try to get from context (set by auth middleware)
	if val, exists := c.Get("user_context"); exists {
		if userCtx, ok := val.(*UserContextInfo); ok {
			return userCtx
		}
	}

	// Fallback: try to extract from headers
	userID := string(c.GetHeader("X-User-ID"))
	if userID == "" {
		userID = "anonymous"
	}

	groupsHeader := string(c.GetHeader("X-User-Groups"))
	groups := []string{"user"}
	if groupsHeader != "" {
		groups = strings.Split(groupsHeader, ",")
		for i := range groups {
			groups[i] = strings.TrimSpace(groups[i])
		}
	}

	return &UserContextInfo{
		UserID: userID,
		Groups: groups,
	}
}

// UserContextInfo represents user information extracted from request
type UserContextInfo struct {
	UserID string
	Groups []string
}

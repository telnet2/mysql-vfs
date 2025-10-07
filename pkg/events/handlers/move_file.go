package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/telnet2/mysql-vfs/pkg/events"
)

// MoveFileHandler implements the move_file action handler
// Uses interface{} types to avoid circular dependencies
type MoveFileHandler struct {
	fileService    interface{} // Should be *domain.FileService
	workflowLoader interface{} // Should be *domain.WorkflowLoader
}

// NewMoveFileHandler creates a new move_file handler
func NewMoveFileHandler(fileService interface{}, workflowLoader interface{}) *MoveFileHandler {
	return &MoveFileHandler{
		fileService:    fileService,
		workflowLoader: workflowLoader,
	}
}

// Handle executes the move_file action
func (h *MoveFileHandler) Handle(ctx context.Context, handler *events.EventHandler, payload interface{}) events.HandlerResponse {
	// Parse config
	config, err := h.parseConfig(handler.Config)
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("invalid move_file config: %v", err),
		}
	}

	// Extract file path from payload
	filePath, err := h.extractFilePath(payload)
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("failed to extract file path: %v", err),
		}
	}

	// Load workflow for this file using reflection
	type workflowLoaderInterface interface {
		LoadForPath(ctx context.Context, filePath string) (interface{}, error)
	}
	loader, ok := h.workflowLoader.(workflowLoaderInterface)
	if !ok {
		return events.HandlerResponse{
			Success: false,
			Message: "workflow loader not properly initialized",
		}
	}

	workflowDef, err := loader.LoadForPath(ctx, filePath)
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("failed to load workflow: %v", err),
		}
	}

	// Check if workflow is nil (no workflow active)
	if workflowDef == nil {
		return events.HandlerResponse{
			Success: false,
			Message: "no workflow active for file",
		}
	}

	// Get current state from file path
	currentState, err := h.getCurrentState(workflowDef, filePath)
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("failed to get current state: %v", err),
		}
	}

	// Construct destination path
	destPath, err := h.constructDestPath(workflowDef, filePath, currentState, config.TargetState, config.ShouldPreserveStructure())
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("failed to construct destination path: %v", err),
		}
	}

	// Execute move using type assertion
	type fileServiceInterface interface {
		MoveFile(ctx context.Context, sourcePath, destPath string) (interface{}, error)
	}
	svc, ok := h.fileService.(fileServiceInterface)
	if !ok {
		return events.HandlerResponse{
			Success: false,
			Message: "file service not properly initialized",
		}
	}

	_, err = svc.MoveFile(ctx, filePath, destPath)
	if err != nil {
		return events.HandlerResponse{
			Success: false,
			Message: fmt.Sprintf("move failed: %v", err),
		}
	}

	return events.HandlerResponse{
		Success: true,
		Message: fmt.Sprintf("moved file from %s to %s", filePath, destPath),
	}
}

// Type returns the handler type
func (h *MoveFileHandler) Type() events.HandlerType {
	return events.HandlerTypeMoveFile
}

// parseConfig parses the handler config into MoveFileConfig
func (h *MoveFileHandler) parseConfig(config interface{}) (*events.MoveFileConfig, error) {
	// Config might be a map[string]interface{} from JSON
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("config must be an object")
	}

	// Marshal and unmarshal to get proper types
	data, err := json.Marshal(configMap)
	if err != nil {
		return nil, err
	}

	var moveConfig events.MoveFileConfig
	if err := json.Unmarshal(data, &moveConfig); err != nil {
		return nil, err
	}

	if moveConfig.TargetState == "" {
		return nil, fmt.Errorf("target_state is required")
	}

	return &moveConfig, nil
}

// extractFilePath extracts the file path from the event payload
func (h *MoveFileHandler) extractFilePath(payload interface{}) (string, error) {
	switch p := payload.(type) {
	case *events.FileEventPayload:
		return p.Resource.Path, nil
	case *events.MoveEventPayload:
		return p.NewPath, nil
	case map[string]interface{}:
		// Try to extract from generic map
		if resource, ok := p["resource"].(map[string]interface{}); ok {
			if filePath, ok := resource["path"].(string); ok {
				return filePath, nil
			}
		}
		return "", fmt.Errorf("path not found in payload")
	default:
		return "", fmt.Errorf("unsupported payload type: %T", payload)
	}
}

// getCurrentState extracts the current state from file path
func (h *MoveFileHandler) getCurrentState(workflowDef interface{}, filePath string) (string, error) {
	// Use reflection to call GetCurrentState on workflowDef
	// For now, we'll use a simple path-based approach
	// This assumes workflow definition has a method like GetCurrentState

	// Type assertion to access workflow definition methods
	type WorkflowDef interface {
		GetCurrentState(filePath string) (string, error)
	}

	if wd, ok := workflowDef.(WorkflowDef); ok {
		return wd.GetCurrentState(filePath)
	}

	return "", fmt.Errorf("workflow definition does not implement GetCurrentState")
}

// constructDestPath builds the destination path for the target state
func (h *MoveFileHandler) constructDestPath(workflowDef interface{}, filePath, fromState, toState string, preserveStructure bool) (string, error) {
	// Type assertion to access workflow definition methods
	type WorkflowDef interface {
		GetWorkflowHome() string
		GetStateDirectory(state string) (string, error)
	}

	wd, ok := workflowDef.(WorkflowDef)
	if !ok {
		return "", fmt.Errorf("workflow definition does not implement required methods")
	}

	workflowHome := wd.GetWorkflowHome()
	targetStateDir, err := wd.GetStateDirectory(toState)
	if err != nil {
		return "", fmt.Errorf("invalid target state: %w", err)
	}

	fileName := path.Base(filePath)

	if !preserveStructure {
		// Move directly to target state directory
		return path.Join(targetStateDir, fileName), nil
	}

	// Preserve subdirectory structure
	// Extract relative path from current state directory
	currentStateDir, err := wd.GetStateDirectory(fromState)
	if err != nil {
		return "", fmt.Errorf("invalid current state: %w", err)
	}

	// Remove workflow home prefix
	relPath := strings.TrimPrefix(filePath, workflowHome)
	relPath = strings.TrimPrefix(relPath, "/")

	// Remove current state directory prefix
	stateBaseName := path.Base(currentStateDir)
	if strings.HasPrefix(relPath, stateBaseName+"/") {
		relPath = strings.TrimPrefix(relPath, stateBaseName+"/")
	}

	// Construct destination: targetStateDir + subdirs + filename
	return path.Join(targetStateDir, relPath), nil
}

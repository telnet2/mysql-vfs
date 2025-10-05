package handlers

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/telnet2/mysql-vfs/pkg/domain"
)

// DirectoryHandler handles HTTP requests for directory operations
type DirectoryHandler struct {
	domainService *domain.DirectoryService
}

// NewDirectoryHandler creates a new directory handler
func NewDirectoryHandler(domainService *domain.DirectoryService) *DirectoryHandler {
	return &DirectoryHandler{
		domainService: domainService,
	}
}

// CreateDirectoryRequest represents the request to create a directory
type CreateDirectoryRequest struct {
	ParentPath string `json:"parent_path"`
	Name       string `json:"name"`
}

// DirectoryResponse represents a directory in API responses
type DirectoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	ParentID  *string   `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListDirectoryResponse represents a list of directories
type ListDirectoryResponse struct {
	Directories []DirectoryResponse `json:"directories"`
	NextCursor  *string             `json:"next_cursor,omitempty"`
}

// CreateDirectory handles directory creation
func (h *DirectoryHandler) CreateDirectory(ctx context.Context, c *app.RequestContext) {
	// Get validated request from context (set by validation middleware)
	var req CreateDirectoryRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Get request ID from context
	requestID, _ := ctx.Value("request_id").(string)

	// Call domain service
	dir, err := h.domainService.CreateDirectory(ctx, req.ParentPath, req.Name)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		c.JSON(statusCode, ErrorResponse{
			Error:     mapErrorToMessage(err),
			RequestID: requestID,
		})
		return
	}

	// Format response
	response := DirectoryResponse{
		ID:        dir.ID,
		Name:      dir.Name,
		Path:      dir.Path,
		ParentID:  dir.ParentID,
		CreatedAt: dir.CreatedAt,
		UpdatedAt: dir.UpdatedAt,
	}

	c.JSON(201, response)
}

// ListDirectory handles listing directory contents
func (h *DirectoryHandler) ListDirectory(ctx context.Context, c *app.RequestContext) {
	// Get path parameter
	path := c.Query("path")
	if path == "" {
		path = "/"
	}

	// Get pagination parameters
	limit := 100
	if limitParam := c.Query("limit"); limitParam != "" {
		// Parse limit (simple implementation)
		// In production, use strconv.Atoi with validation
	}

	cursor := c.Query("cursor")

	// Get request ID
	requestID, _ := ctx.Value("request_id").(string)

	// Call domain service
	directories, files, nextCursor, err := h.domainService.ListDirectory(path, limit, cursor)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		c.JSON(statusCode, ErrorResponse{
			Error:     mapErrorToMessage(err),
			RequestID: requestID,
		})
		return
	}

	// Format response
	dirResponses := make([]DirectoryResponse, len(directories))
	for i, dir := range directories {
		dirResponses[i] = DirectoryResponse{
			ID:        dir.ID,
			Name:      dir.Name,
			Path:      dir.Path,
			ParentID:  dir.ParentID,
			CreatedAt: dir.CreatedAt,
			UpdatedAt: dir.UpdatedAt,
		}
	}

	response := ListDirectoryResponse{
		Directories: dirResponses,
	}

	if nextCursor != "" {
		response.NextCursor = &nextCursor
	}

	// Note: files are currently not included in the response, but they are available if needed
	_ = files

	c.JSON(200, response)
}

// DeleteDirectory handles directory deletion
func (h *DirectoryHandler) DeleteDirectory(ctx context.Context, c *app.RequestContext) {
	// Get path parameter
	path := c.Param("path")
	if path == "" {
		c.JSON(400, ErrorResponse{Error: "path parameter is required"})
		return
	}

	// Check for recursive flag
	recursive := c.Query("recursive") == "true"

	// Get request ID
	requestID, _ := ctx.Value("request_id").(string)

	// Note: userRole parameter is no longer used in DeleteDirectory
	// Authorization now happens via groups in middleware
	err := h.domainService.DeleteDirectory(ctx, path, recursive)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		c.JSON(statusCode, ErrorResponse{
			Error:     mapErrorToMessage(err),
			RequestID: requestID,
		})
		return
	}

	c.JSON(200, map[string]string{"message": "directory deleted successfully"})
}

// GetDirectory handles retrieving a single directory
func (h *DirectoryHandler) GetDirectory(ctx context.Context, c *app.RequestContext) {
	// Get path parameter
	path := c.Param("path")
	if path == "" {
		c.JSON(400, ErrorResponse{Error: "path parameter is required"})
		return
	}

	// Get request ID
	requestID, _ := ctx.Value("request_id").(string)

	// Call domain service
	dir, err := h.domainService.GetDirectory(path)
	if err != nil {
		statusCode := mapErrorToStatus(err)
		c.JSON(statusCode, ErrorResponse{
			Error:     mapErrorToMessage(err),
			RequestID: requestID,
		})
		return
	}

	// Format response
	response := DirectoryResponse{
		ID:        dir.ID,
		Name:      dir.Name,
		Path:      dir.Path,
		ParentID:  dir.ParentID,
		CreatedAt: dir.CreatedAt,
		UpdatedAt: dir.UpdatedAt,
	}

	c.JSON(200, response)
}

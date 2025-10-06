package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxFileSize = 100 * 1024 * 1024 // 100MB
)

// Client is the VFS HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
	authToken  string

	// Delegation headers for on-behalf-of operations
	onBehalfOf       string
	delegationReason string
}

// NewClient creates a new VFS client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAuthToken sets the authentication token
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
}

// SetOnBehalfOf sets the on-behalf-of delegation headers
func (c *Client) SetOnBehalfOf(principalUserID, reason string) {
	c.onBehalfOf = principalUserID
	c.delegationReason = reason
}

// ClearOnBehalfOf clears the delegation headers
func (c *Client) ClearOnBehalfOf() {
	c.onBehalfOf = ""
	c.delegationReason = ""
}

// request makes an HTTP request with common headers
func (c *Client) request(method, path string, body interface{}, requestID string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	fullURL := c.baseURL + path
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	// Set delegation headers if configured
	if c.onBehalfOf != "" {
		req.Header.Set("X-VFS-On-Behalf-Of", c.onBehalfOf)
		if c.delegationReason != "" {
			req.Header.Set("X-VFS-Delegation-Reason", c.delegationReason)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// DirectoryEntry represents a directory entry
type DirectoryEntry struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"` // "directory" or "file"
	SizeBytes  int64     `json:"size_bytes"`
	Version    int64     `json:"version"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ListDirectoryResponse is the response from listing a directory
type ListDirectoryResponse struct {
	Entries    []DirectoryEntry `json:"entries"`
	NextCursor *string          `json:"next_cursor"`
}

// ListDirectory lists directory contents
func (c *Client) ListDirectory(path string, limit int, cursor string) (*ListDirectoryResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}

	queryString := ""
	if len(params) > 0 {
		queryString = "?" + params.Encode()
	}

	resp, err := c.request("GET", "/api/v1/directories"+path+queryString, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list directory: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result ListDirectoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// CreateDirectoryRequest is the request to create a directory
type CreateDirectoryRequest struct {
	ParentPath string `json:"parent_path"`
	Name       string `json:"name"`
}

// CreateDirectoryResponse is the response from creating a directory
type CreateDirectoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	ParentID  *string   `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateDirectory creates a new directory
func (c *Client) CreateDirectory(parentPath, name string) (*CreateDirectoryResponse, error) {
	requestID := uuid.New().String()

	req := CreateDirectoryRequest{
		ParentPath: parentPath,
		Name:       name,
	}

	resp, err := c.request("POST", "/api/v1/directories", req, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create directory: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result CreateDirectoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteDirectory deletes a directory
func (c *Client) DeleteDirectory(path string, recursive bool) error {
	requestID := uuid.New().String()

	params := url.Values{}
	if recursive {
		params.Set("recursive", "true")
	}

	queryString := ""
	if len(params) > 0 {
		queryString = "?" + params.Encode()
	}

	resp, err := c.request("DELETE", "/api/v1/directories"+path+queryString, nil, requestID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete directory: %s (status: %d)", string(body), resp.StatusCode)
	}

	return nil
}

// CreateFileRequest is the request to create a file
type CreateFileRequest struct {
	DirectoryPath string `json:"directory_path"`
	Name          string `json:"name"`
	ContentType   string `json:"content_type"`
	Content       string `json:"content"`
}

// CreateFileResponse is the response from creating a file
type CreateFileResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StorageType string    `json:"storage_type"`
	Checksum    string    `json:"checksum"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateFileRequest is the request to update a file
type UpdateFileRequest struct {
	ContentType     string `json:"content_type"`
	Content         string `json:"content"`
	ExpectedVersion int64  `json:"expected_version"`
}

// UpdateFileResponse is the response from updating a file
type UpdateFileResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Version     int64     `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FileVersion represents a version of a file
type FileVersion struct {
	Version     int64     `json:"version"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StorageType string    `json:"storage_type"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateFile creates a new file
func (c *Client) CreateFile(directoryPath, name, contentType, content string) (*CreateFileResponse, error) {
	requestID := uuid.New().String()

	req := CreateFileRequest{
		DirectoryPath: directoryPath,
		Name:          name,
		ContentType:   contentType,
		Content:       content,
	}

	resp, err := c.request("POST", "/api/v1/files", req, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		// Try to parse structured error response
		var errorResp struct {
			Error   string   `json:"error"`
			Details []string `json:"details,omitempty"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			if len(errorResp.Details) > 0 {
				// Format field-specific validation errors
				details := strings.Join(errorResp.Details, "; ")
				return nil, fmt.Errorf("failed to create file: %s (%s)", errorResp.Error, details)
			}
			return nil, fmt.Errorf("failed to create file: %s", errorResp.Error)
		}
		return nil, fmt.Errorf("failed to create file: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result CreateFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UpdateFile updates an existing file
func (c *Client) UpdateFile(path, contentType, content string, expectedVersion int64) (*UpdateFileResponse, error) {
	requestID := uuid.New().String()

	req := UpdateFileRequest{
		ContentType:     contentType,
		Content:         content,
		ExpectedVersion: expectedVersion,
	}

	resp, err := c.request("PUT", "/api/v1/files"+path, req, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Try to parse structured error response
		var errorResp struct {
			Error   string   `json:"error"`
			Details []string `json:"details,omitempty"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			if len(errorResp.Details) > 0 {
				// Format field-specific validation errors
				details := strings.Join(errorResp.Details, "; ")
				return nil, fmt.Errorf("failed to update file: %s (%s)", errorResp.Error, details)
			}
			return nil, fmt.Errorf("failed to update file: %s", errorResp.Error)
		}
		return nil, fmt.Errorf("failed to update file: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result UpdateFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListVersions lists all versions of a file
func (c *Client) ListVersions(path string) ([]*FileVersion, error) {
	resp, err := c.request("GET", "/api/v1/files-version"+path, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list versions: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result []*FileVersion
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetFile retrieves a file's content
func (c *Client) GetFile(path string) ([]byte, string, int64, time.Time, error) {
	resp, err := c.request("GET", "/api/v1/files"+path, nil, "")
	if err != nil {
		return nil, "", 0, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", 0, time.Time{}, fmt.Errorf("failed to get file: %s (status: %d)", string(body), resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	versionStr := resp.Header.Get("X-File-Version")
	modifiedStr := resp.Header.Get("X-File-Modified-At")

	var version int64 = 1 // Default to 1
	if versionStr != "" {
		if parsed, err := strconv.ParseInt(versionStr, 10, 64); err == nil {
			version = parsed
		}
	}

	var modifiedAt time.Time
	if modifiedStr != "" {
		if parsed, err := time.Parse(time.RFC3339, modifiedStr); err == nil {
			modifiedAt = parsed
		}
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", 0, time.Time{}, fmt.Errorf("failed to read file content: %w", err)
	}

	return content, contentType, version, modifiedAt, nil
}

// GetFileStream retrieves a file's content as a stream
func (c *Client) GetFileStream(path string) (io.ReadCloser, string, error) {
	resp, err := c.request("GET", "/api/v1/files"+path, nil, "")
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, "", fmt.Errorf("failed to get file: %s (status: %d)", string(body), resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	return resp.Body, contentType, nil
}

// DeleteFile deletes a file
func (c *Client) DeleteFile(path string) error {
	requestID := uuid.New().String()

	resp, err := c.request("DELETE", "/api/v1/files"+path, nil, requestID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete file: %s (status: %d)", string(body), resp.StatusCode)
	}

	return nil
}

// MoveFileRequest is the request to move a file
type MoveFileRequest struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
}

// MoveFileResponse is the response from moving a file
type MoveFileResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MoveFile moves or renames a file
func (c *Client) MoveFile(sourcePath, destinationPath string) (*MoveFileResponse, error) {
	requestID := uuid.New().String()

	req := MoveFileRequest{
		SourcePath:      sourcePath,
		DestinationPath: destinationPath,
	}

	resp, err := c.request("POST", "/api/v1/files/move", req, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to move file: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result MoveFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// HealthCheck checks the service health
func (c *Client) HealthCheck() (bool, error) {
	resp, err := c.request("GET", "/health", nil, "")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// LoginRequest is the request to authenticate
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the response from login
type LoginResponse struct {
	Token string `json:"token"`
}

// Login authenticates with username/password
func (c *Client) Login(username, password string) (string, error) {
	req := LoginRequest{
		Username: username,
		Password: password,
	}

	resp, err := c.request("POST", "/api/v1/auth/login", req, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Token, nil
}

// UpdateFileMetadata updates only the metadata of a file
func (c *Client) UpdateFileMetadata(path, contentType string) error {
	requestID := uuid.New().String()

	req := map[string]string{
		"content_type": contentType,
	}

	resp, err := c.request("PATCH", "/api/v1/files"+path, req, requestID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Try to parse structured error response
		var errorResp struct {
			Error   string   `json:"error"`
			Details []string `json:"details,omitempty"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			if len(errorResp.Details) > 0 {
				// Format field-specific validation errors
				details := strings.Join(errorResp.Details, "; ")
				return fmt.Errorf("failed to update file metadata: %s (%s)", errorResp.Error, details)
			}
			return fmt.Errorf("failed to update file metadata: %s", errorResp.Error)
		}
		return fmt.Errorf("failed to update file metadata: %s (status: %d)", string(body), resp.StatusCode)
	}

	return nil
}

// GetFileMetadata retrieves file metadata without content
func (c *Client) GetFileMetadata(path string, version int64) (map[string]interface{}, error) {
	params := url.Values{}
	if version > 0 {
		params.Set("version", fmt.Sprintf("%d", version))
	}
	queryString := ""
	if len(params) > 0 {
		queryString = "?" + params.Encode()
	}

	resp, err := c.request("GET", "/api/v1/files-metadata"+path+queryString, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get file metadata: %s (status: %d)", string(body), resp.StatusCode)
	}

	var metadata map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode metadata response: %w", err)
	}

	return metadata, nil
}

// GetFileVersion retrieves a specific version of a file
func (c *Client) GetFileVersion(path string, version int64) ([]byte, string, error) {
	params := url.Values{}
	params.Set("version", fmt.Sprintf("%d", version))
	queryString := "?" + params.Encode()

	resp, err := c.request("GET", "/api/v1/files"+path+queryString, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("failed to get file: %s (status: %d)", string(body), resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file content: %w", err)
	}

	return content, contentType, nil
}

// Note: User/group management is handled via .user files and super user tokens.
// There are no database-backed user tables or traditional login endpoints.

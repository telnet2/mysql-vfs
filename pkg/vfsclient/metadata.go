package vfsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MetadataClient handles metadata service operations.
type MetadataClient struct {
	baseURL    string
	httpClient *http.Client
	actor      string
}

const actorHeader = "X-VFS-Actor"

// NewMetadataClient creates a new metadata service client.
func NewMetadataClient(baseURL string, actor string, httpClient *http.Client) *MetadataClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &MetadataClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		actor:      actor,
	}
}

// ResolveDirectory resolves a directory by path.
func (m *MetadataClient) ResolveDirectory(ctx context.Context, path string) (DirectoryDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/directories/resolve?path=%s", m.baseURL, url.QueryEscape(path))
	var out DirectoryDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return DirectoryDTO{}, err
	}
	return out, nil
}

// CreateDirectory creates a new directory.
func (m *MetadataClient) CreateDirectory(ctx context.Context, name string, parentID *string) (DirectoryDTO, error) {
	payload := map[string]any{"name": name}
	if parentID != nil {
		payload["parent_id"] = parentID
	}
	endpoint := fmt.Sprintf("%s/api/v1/directories", m.baseURL)
	var out DirectoryDTO
	if err := m.do(ctx, http.MethodPost, endpoint, payload, &out); err != nil {
		return DirectoryDTO{}, err
	}
	return out, nil
}

// DeleteDirectory deletes a directory.
func (m *MetadataClient) DeleteDirectory(ctx context.Context, id string, force bool) error {
	endpoint := fmt.Sprintf("%s/api/v1/directories/%s", m.baseURL, url.PathEscape(id))
	if force {
		endpoint += "?force=true"
	}
	return m.do(ctx, http.MethodDelete, endpoint, nil, nil)
}

// UpdateDirectory updates a directory.
func (m *MetadataClient) UpdateDirectory(ctx context.Context, id string, req UpdateDirectoryRequest) (DirectoryDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/directories/%s", m.baseURL, url.PathEscape(id))
	var out DirectoryDTO
	if err := m.do(ctx, http.MethodPatch, endpoint, req, &out); err != nil {
		return DirectoryDTO{}, err
	}
	return out, nil
}

// ListDirectory lists directories and files under a parent.
func (m *MetadataClient) ListDirectory(ctx context.Context, parentID string, recursive bool) (ListDirectoryResponse, error) {
	params := url.Values{}
	if parentID != "" {
		params.Set("parent_id", parentID)
	}
	if recursive {
		params.Set("recursive", "true")
	}
	endpoint := fmt.Sprintf("%s/api/v1/directories", m.baseURL)
	if enc := params.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	var out ListDirectoryResponse
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return ListDirectoryResponse{}, err
	}
	return out, nil
}

// ResolveFile resolves a file by path.
func (m *MetadataClient) ResolveFile(ctx context.Context, path string) (FileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/resolve?path=%s", m.baseURL, url.QueryEscape(path))
	var out FileDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return FileDTO{}, err
	}
	return out, nil
}

// CreateFile creates a new file.
func (m *MetadataClient) CreateFile(ctx context.Context, req CreateFileRequest) (FileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files", m.baseURL)
	var out FileDTO
	if err := m.do(ctx, http.MethodPost, endpoint, req, &out); err != nil {
		return FileDTO{}, err
	}
	return out, nil
}

// DeleteFile deletes a file.
func (m *MetadataClient) DeleteFile(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	return m.do(ctx, http.MethodDelete, endpoint, nil, nil)
}

// UpdateFile updates a file.
func (m *MetadataClient) UpdateFile(ctx context.Context, id string, req UpdateFileRequest) (FileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	var out FileDTO
	if err := m.do(ctx, http.MethodPatch, endpoint, req, &out); err != nil {
		return FileDTO{}, err
	}
	return out, nil
}

// GetFile retrieves file metadata by ID.
func (m *MetadataClient) GetFile(ctx context.Context, id string) (FileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	var out FileDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return FileDTO{}, err
	}
	return out, nil
}

// ListFileVersions lists all versions of a file.
func (m *MetadataClient) ListFileVersions(ctx context.Context, id string) ([]FileVersionDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s/versions", m.baseURL, url.PathEscape(id))
	var out []FileVersionDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolvePolicy resolves policy for a directory or path.
func (m *MetadataClient) ResolvePolicy(ctx context.Context, directoryID, path, typ string) (PolicyResolutionDTO, error) {
	params := url.Values{}
	if strings.TrimSpace(directoryID) != "" {
		params.Set("directory_id", directoryID)
	}
	if strings.TrimSpace(path) != "" {
		params.Set("path", path)
	}
	if strings.TrimSpace(typ) != "" {
		params.Set("type", typ)
	}
	endpoint := fmt.Sprintf("%s/api/v1/policies/resolve", m.baseURL)
	if enc := params.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	var out PolicyResolutionDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return PolicyResolutionDTO{}, err
	}
	return out, nil
}

func (m *MetadataClient) do(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
		reader = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(m.actor) != "" {
		req.Header.Set(actorHeader, m.actor)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseAPIError(resp *http.Response) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(data, &payload)
	message := strings.TrimSpace(payload.Error.Message)
	if message == "" && len(data) > 0 {
		message = strings.TrimSpace(string(data))
	}
	if message == "" {
		message = resp.Status
	}
	return APIError{Status: resp.StatusCode, Code: payload.Error.Code, Message: message}
}

// UpdateDirectoryRequest represents a directory update request.
type UpdateDirectoryRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
	Version  *int64  `json:"version,omitempty"`
}

// CreateFileRequest represents a file creation request.
type CreateFileRequest struct {
	DirectoryID string           `json:"directory_id"`
	Name        string           `json:"name"`
	StorageMode string           `json:"storage_mode"`
	BlobKey     *string          `json:"blob_key,omitempty"`
	JSONPayload *json.RawMessage `json:"json_payload,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Checksum    *string          `json:"checksum,omitempty"`
	Size        *int64           `json:"size,omitempty"`
	MimeType    *string          `json:"mime_type,omitempty"`
	Actor       string           `json:"actor"`
}

// UpdateFileRequest represents a file update request.
type UpdateFileRequest struct {
	DirectoryID *string          `json:"directory_id,omitempty"`
	Name        *string          `json:"name,omitempty"`
	StorageMode *string          `json:"storage_mode,omitempty"`
	BlobKey     *string          `json:"blob_key,omitempty"`
	JSONPayload *json.RawMessage `json:"json_payload,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Checksum    *string          `json:"checksum,omitempty"`
	Size        *int64           `json:"size,omitempty"`
	MimeType    *string          `json:"mime_type,omitempty"`
	Version     *int64           `json:"version,omitempty"`
	Actor       string           `json:"actor"`
}

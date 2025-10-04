package vfsclient

import (
	"encoding/json"
	"time"
)

// DirectoryDTO represents a directory in the VFS.
type DirectoryDTO struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id"`
	Path      string     `json:"path"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// FileDTO represents a file in the VFS.
type FileDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	DirectoryID  string          `json:"directory_id"`
	Path         string          `json:"path"`
	Version      int64           `json:"version"`
	OriginFileID *string         `json:"origin_file_id"`
	StorageMode  string          `json:"storage_mode"`
	BlobKey      *string         `json:"blob_key"`
	InlineJSON   json.RawMessage `json:"inline_json"`
	Checksum     *string         `json:"checksum"`
	Size         *int64          `json:"size"`
	MimeType     *string         `json:"mime_type"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at"`
}

// ListDirectoryResponse contains directories and files in a listing.
type ListDirectoryResponse struct {
	Directories []DirectoryDTO `json:"directories"`
	Files       []FileDTO      `json:"files"`
}

// UploadResponse contains the result of a content upload.
type UploadResponse struct {
	StorageMode string          `json:"storage_mode"`
	BlobKey     *string         `json:"blob_key"`
	JSONPayload json.RawMessage `json:"json_payload"`
	Checksum    string          `json:"checksum"`
	Size        int64           `json:"size"`
	MimeType    string          `json:"mime_type"`
}

// FileVersionDTO represents a specific version of a file.
type FileVersionDTO struct {
	ID          string          `json:"id"`
	Index       int             `json:"index"`
	StorageMode string          `json:"storage_mode"`
	BlobKey     *string         `json:"blob_key"`
	JSONPayload json.RawMessage `json:"json_payload"`
	Metadata    map[string]any  `json:"metadata"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

// PolicyManifestDTO represents a policy manifest.
type PolicyManifestDTO struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	SourcePath  string         `json:"source_path"`
	DirectoryID string         `json:"directory_id"`
	Scope       string         `json:"scope"`
	Inheritance string         `json:"inheritance"`
	AppliesTo   []string       `json:"applies_to,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PolicyUserDTO represents a user in the policy system.
type PolicyUserDTO struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Email       string         `json:"email,omitempty"`
	Groups      []string       `json:"groups,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// PolicyGroupDTO represents a group in the policy system.
type PolicyGroupDTO struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Description string         `json:"description,omitempty"`
	Members     []string       `json:"members,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// PrincipalSetDTO contains users and groups.
type PrincipalSetDTO struct {
	Users  []PolicyUserDTO  `json:"users,omitempty"`
	Groups []PolicyGroupDTO `json:"groups,omitempty"`
}

// PolicyResolutionDTO contains resolved policy information.
type PolicyResolutionDTO struct {
	DirectoryID string              `json:"directory_id"`
	Manifests   []PolicyManifestDTO `json:"manifests"`
	Principals  PrincipalSetDTO     `json:"principals"`
}

// APIError represents an error returned by the VFS API.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

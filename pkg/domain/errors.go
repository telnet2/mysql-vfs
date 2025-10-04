package domain

import "errors"

var (
	// ErrDepthLimitExceeded is returned when directory depth exceeds the maximum
	ErrDepthLimitExceeded = errors.New("directory depth limit exceeded")

	// ErrParentNotFound is returned when the parent directory doesn't exist
	ErrParentNotFound = errors.New("parent directory not found")

	// ErrDirectoryNotEmpty is returned when trying to delete a non-empty directory
	ErrDirectoryNotEmpty = errors.New("directory is not empty")

	// ErrInvalidPath is returned when a path is invalid
	ErrInvalidPath = errors.New("invalid path")

	// ErrInvalidName is returned when a name is invalid
	ErrInvalidName = errors.New("invalid name")

	// ErrFileTooLarge is returned when a file exceeds the maximum size
	ErrFileTooLarge = errors.New("file too large")

	// ErrFileNotFound is returned when a file is not found
	ErrFileNotFound = errors.New("file not found")

	// ErrDirectoryNotFound is returned when a directory is not found
	ErrDirectoryNotFound = errors.New("directory not found")

	// ErrAlreadyExists is returned when a resource already exists
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrVersionConflict is returned when there's a version conflict (optimistic locking)
	ErrVersionConflict = errors.New("version conflict")

	// ErrNotImplemented is returned when a feature is not yet implemented
	ErrNotImplemented = errors.New("not implemented")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrNotFound is a generic not found error
	ErrNotFound = errors.New("not found")

	// ErrPermissionDenied is returned when user lacks required permissions
	ErrPermissionDenied = errors.New("permission denied")

	// ErrUnknownSpecialFileType is returned for unrecognized special file types
	ErrUnknownSpecialFileType = errors.New("unknown special file type")

	// ErrInvalidSpecialFileContent is returned when special file content is invalid
	ErrInvalidSpecialFileContent = errors.New("invalid special file content")

	// ErrQuotaExceeded is returned when resource quota is exceeded
	ErrQuotaExceeded = errors.New("quota exceeded")
)

package handlers

import (
	"errors"
	"net/http"

	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// mapErrorToStatus maps domain and repository errors to HTTP status codes
func mapErrorToStatus(err error) int {
	// Domain errors
	switch {
	case errors.Is(err, domain.ErrDepthLimitExceeded):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrParentNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrDirectoryNotEmpty):
		return http.StatusConflict
	case errors.Is(err, domain.ErrInvalidPath):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidName):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrFileTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, domain.ErrFileNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrDirectoryNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, domain.ErrVersionConflict):
		return http.StatusConflict
	}

	// Repository errors
	switch {
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, db.ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, db.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, db.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, db.ErrTransactionFailed):
		return http.StatusInternalServerError
	case errors.Is(err, db.ErrLockFailed):
		return http.StatusConflict
	}

	// Default to internal server error
	return http.StatusInternalServerError
}

// mapErrorToMessage returns a user-friendly error message
func mapErrorToMessage(err error) string {
	// Domain errors
	switch {
	case errors.Is(err, domain.ErrDepthLimitExceeded):
		return "directory depth limit exceeded"
	case errors.Is(err, domain.ErrParentNotFound):
		return "parent directory not found"
	case errors.Is(err, domain.ErrDirectoryNotEmpty):
		return "directory is not empty"
	case errors.Is(err, domain.ErrInvalidPath):
		return "invalid path"
	case errors.Is(err, domain.ErrInvalidName):
		return "invalid name"
	case errors.Is(err, domain.ErrFileTooLarge):
		return "file size exceeds maximum allowed"
	case errors.Is(err, domain.ErrFileNotFound):
		return "file not found"
	case errors.Is(err, domain.ErrDirectoryNotFound):
		return "directory not found"
	case errors.Is(err, domain.ErrAlreadyExists):
		return "resource already exists"
	case errors.Is(err, domain.ErrVersionConflict):
		return "version conflict - resource was modified"
	}

	// Repository errors
	switch {
	case errors.Is(err, db.ErrNotFound):
		return "resource not found"
	case errors.Is(err, db.ErrAlreadyExists):
		return "resource already exists"
	case errors.Is(err, db.ErrConflict):
		return "conflict detected"
	case errors.Is(err, db.ErrInvalidInput):
		return "invalid input"
	case errors.Is(err, db.ErrTransactionFailed):
		return "transaction failed"
	case errors.Is(err, db.ErrLockFailed):
		return "failed to acquire lock"
	}

	// Default error message
	return err.Error()
}

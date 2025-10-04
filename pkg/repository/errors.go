package repository

import "errors"

var (
	// ErrNotFound is returned when a resource is not found
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists is returned when trying to create a resource that already exists
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrConflict is returned when there's a conflict (e.g., optimistic locking failure)
	ErrConflict = errors.New("conflict detected")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrTransactionFailed is returned when a transaction operation fails
	ErrTransactionFailed = errors.New("transaction failed")

	// ErrLockFailed is returned when acquiring a lock fails
	ErrLockFailed = errors.New("failed to acquire lock")
)

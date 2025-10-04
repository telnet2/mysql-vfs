package repository

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/models"
)

// DirectoryRepository defines the interface for directory data access
type DirectoryRepository interface {
	// Create creates a new directory
	Create(ctx context.Context, dir *models.Directory) error

	// FindByID finds a directory by ID
	FindByID(ctx context.Context, id string) (*models.Directory, error)

	// FindByPath finds a directory by its full path
	FindByPath(ctx context.Context, path string) (*models.Directory, error)

	// FindByParentID finds all directories under a parent directory
	FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error)

	// Update updates a directory
	Update(ctx context.Context, dir *models.Directory) error

	// Delete permanently deletes a directory
	Delete(ctx context.Context, id string) error

	// SoftDelete soft deletes a directory
	SoftDelete(ctx context.Context, id string) error

	// LockPaths acquires locks on the specified paths (for tree locking)
	LockPaths(ctx context.Context, tx Transaction, paths []string) error

	// Exists checks if a directory exists at the given path
	Exists(ctx context.Context, path string) (bool, error)
}

// FileRepository defines the interface for file data access
type FileRepository interface {
	// Create creates a new file
	Create(ctx context.Context, file *models.File) error

	// FindByID finds a file by ID
	FindByID(ctx context.Context, id string) (*models.File, error)

	// FindByDirectoryAndName finds a file by directory ID and name
	FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error)

	// FindByDirectoryID finds all files in a directory
	FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error)

	// Update updates a file
	Update(ctx context.Context, file *models.File) error

	// Delete permanently deletes a file
	Delete(ctx context.Context, id string) error

	// SoftDelete soft deletes a file
	SoftDelete(ctx context.Context, id string) error

	// CreateVersion creates a new file version
	CreateVersion(ctx context.Context, version *models.FileVersion) error

	// GetLatestVersion gets the latest version of a file
	GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error)

	// Exists checks if a file exists with the given directory ID and name
	Exists(ctx context.Context, dirID, name string) (bool, error)
}

// EventRepository defines the interface for event data access
type EventRepository interface {
	// Create creates a new event
	Create(ctx context.Context, event *models.Event) error

	// FindByID finds an event by ID
	FindByID(ctx context.Context, id string) (*models.Event, error)

	// FindByAggregateID finds events for a specific aggregate
	FindByAggregateID(ctx context.Context, aggregateID string, limit int) ([]*models.Event, error)

	// FindPending finds pending events to be processed
	FindPending(ctx context.Context, limit int) ([]*models.Event, error)

	// MarkProcessed marks an event as processed
	MarkProcessed(ctx context.Context, eventID string) error
}

// Transaction represents a database transaction
type Transaction interface {
	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error

	// GetDB returns the underlying database connection (GORM-specific)
	// This is needed for nested repository calls within a transaction
	GetDB() interface{}
}

// UnitOfWork provides a way to coordinate multiple repository operations
// within a single transaction
type UnitOfWork interface {
	// BeginTransaction starts a new transaction
	BeginTransaction(ctx context.Context) (Transaction, error)

	// Directories returns the directory repository
	Directories() DirectoryRepository

	// Files returns the file repository
	Files() FileRepository

	// Events returns the event repository
	Events() EventRepository
}

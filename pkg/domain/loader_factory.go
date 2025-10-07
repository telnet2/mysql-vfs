package domain

import (
	"time"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// LoaderFactory creates all special file loaders with consistent configuration
type LoaderFactory struct {
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cacheTTL time.Duration
}

// NewLoaderFactory creates a factory with default configuration (5-minute cache TTL)
func NewLoaderFactory(
	fileRepo db.FileRepository,
	dirRepo db.DirectoryRepository,
) *LoaderFactory {
	return &LoaderFactory{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		cacheTTL: 5 * time.Minute, // Default cache TTL
	}
}

// WithCacheTTL sets a custom cache TTL and returns the factory for chaining
func (f *LoaderFactory) WithCacheTTL(ttl time.Duration) *LoaderFactory {
	f.cacheTTL = ttl
	return f
}

// NewPolicyLoader creates a PolicyLoader
func (f *LoaderFactory) NewPolicyLoader() *PolicyLoader {
	return NewPolicyLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewEventsLoader creates an EventsLoader
func (f *LoaderFactory) NewEventsLoader() *EventsLoader {
	return NewEventsLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewWorkflowLoader creates a WorkflowLoader
func (f *LoaderFactory) NewWorkflowLoader() *WorkflowLoader {
	return NewWorkflowLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewFilesLoader creates a FilesLoader
func (f *LoaderFactory) NewFilesLoader() *FilesLoader {
	return NewFilesLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewUserLoader creates a UserLoader
func (f *LoaderFactory) NewUserLoader() *UserLoader {
	return NewUserLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewGroupLoader creates a GroupLoader
func (f *LoaderFactory) NewGroupLoader() *GroupLoader {
	return NewGroupLoader(f.fileRepo, f.dirRepo, f.cacheTTL)
}

// NewOwnerLoader creates an OwnerLoader
// Note: OwnerLoader requires a GroupLoader for cross-validation
func (f *LoaderFactory) NewOwnerLoader(groupLoader *GroupLoader) *OwnerLoader {
	return NewOwnerLoader(f.fileRepo, f.dirRepo, groupLoader, f.cacheTTL)
}

// Note: VFSSchemaLoader is not included here as it has a different constructor pattern
// (requires context and cache as arguments). Create it directly with NewVFSSchemaLoader if needed.

// SpecialFileLoaders holds all special file loaders
type SpecialFileLoaders struct {
	Policy   *PolicyLoader
	Events   *EventsLoader
	Workflow *WorkflowLoader
	Files    *FilesLoader
	User     *UserLoader
	Group    *GroupLoader
	Owner    *OwnerLoader
}

// CreateAll creates all loaders at once
// Note: Creates loaders in dependency order (Group before Owner)
func (f *LoaderFactory) CreateAll() *SpecialFileLoaders {
	groupLoader := f.NewGroupLoader()

	return &SpecialFileLoaders{
		Policy:   f.NewPolicyLoader(),
		Events:   f.NewEventsLoader(),
		Workflow: f.NewWorkflowLoader(),
		Files:    f.NewFilesLoader(),
		User:     f.NewUserLoader(),
		Group:    groupLoader,
		Owner:    f.NewOwnerLoader(groupLoader), // Depends on GroupLoader
	}
}

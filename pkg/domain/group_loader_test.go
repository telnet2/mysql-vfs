package domain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// MockFileRepository is a mock for FileRepository
type MockFileRepository struct {
	mock.Mock
}

func (m *MockFileRepository) Create(ctx context.Context, file *models.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepository) FindByID(ctx context.Context, id string) (*models.File, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.File), args.Error(1)
}

func (m *MockFileRepository) FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error) {
	args := m.Called(ctx, dirID, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.File), args.Error(1)
}

func (m *MockFileRepository) Update(ctx context.Context, file *models.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockFileRepository) SoftDelete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockFileRepository) Exists(ctx context.Context, dirID, name string) (bool, error) {
	args := m.Called(ctx, dirID, name)
	return args.Bool(0), args.Error(1)
}

func (m *MockFileRepository) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

func (m *MockFileRepository) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.FileVersion), args.Error(1)
}

func (m *MockFileRepository) GetVersion(ctx context.Context, fileID string, version int) (*models.FileVersion, error) {
	args := m.Called(ctx, fileID, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.FileVersion), args.Error(1)
}

func (m *MockFileRepository) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.FileVersion), args.Error(1)
}

func (m *MockFileRepository) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	args := m.Called(ctx, dirID, limit, cursor)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).([]*models.File), args.String(1), args.Error(2)
}

func (m *MockFileRepository) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	args := m.Called(ctx, file, content)
	return args.Error(0)
}

func (m *MockFileRepository) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	args := m.Called(ctx, file)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockFileRepository) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	args := m.Called(ctx, file, content)
	return args.Error(0)
}

// MockDirectoryRepository is a mock for DirectoryRepository
type MockDirectoryRepository struct {
	mock.Mock
}

func (m *MockDirectoryRepository) Create(ctx context.Context, dir *models.Directory) error {
	args := m.Called(ctx, dir)
	return args.Error(0)
}

func (m *MockDirectoryRepository) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Directory), args.Error(1)
}

func (m *MockDirectoryRepository) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Directory), args.Error(1)
}

func (m *MockDirectoryRepository) Update(ctx context.Context, dir *models.Directory) error {
	args := m.Called(ctx, dir)
	return args.Error(0)
}

func (m *MockDirectoryRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDirectoryRepository) SoftDelete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDirectoryRepository) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	args := m.Called(ctx, tx, paths)
	return args.Error(0)
}

func (m *MockDirectoryRepository) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	args := m.Called(ctx, parentID, limit, cursor)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).([]*models.Directory), args.String(1), args.Error(2)
}

func (m *MockDirectoryRepository) Exists(ctx context.Context, path string) (bool, error) {
	args := m.Called(ctx, path)
	return args.Bool(0), args.Error(1)
}

func TestGroupLoader_Load(t *testing.T) {
	ctx := context.Background()

	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice", "bob"}},
			{GroupID: "user", Members: []string{"charlie", "david"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	t.Run("load from current directory", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		config, err := loader.Load(ctx, "dir-1")

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Groups, 2)
		assert.Equal(t, "admin", config.Groups[0].GroupID)
		mockFileRepo.AssertExpectations(t)
	})

	t.Run("load from parent directory", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		parentID := "parent-dir"
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".group").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "dir-1").
			Return(&models.Directory{
				ID:       "dir-1",
				ParentID: &parentID,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "parent-dir", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "parent-dir",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		config, err := loader.Load(ctx, "dir-1")

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Groups, 2)
		mockFileRepo.AssertExpectations(t)
		mockDirRepo.AssertExpectations(t)
	})

	t.Run("cache hit", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil).Once()

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		// First load - should hit repository
		config1, err := loader.Load(ctx, "dir-1")
		assert.NoError(t, err)
		assert.NotNil(t, config1)

		// Second load - should hit cache
		config2, err := loader.Load(ctx, "dir-1")
		assert.NoError(t, err)
		assert.NotNil(t, config2)
		assert.Equal(t, config1, config2)

		mockFileRepo.AssertExpectations(t)
	})
}

func TestGroupLoader_LoadFromRoot(t *testing.T) {
	ctx := context.Background()

	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	mockFileRepo := new(MockFileRepository)
	mockDirRepo := new(MockDirectoryRepository)

	mockDirRepo.On("FindByPath", ctx, "/").
		Return(&models.Directory{
			ID:   "root-id",
			Path: "/",
		}, nil)
	mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
		Return(&models.File{
			ID:          "file-1",
			DirectoryID: "root-id",
			Name:        ".group",
			JSONContent: &groupContent,
		}, nil)

	loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
	config, err := loader.LoadFromRoot(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Len(t, config.Groups, 1)
	mockFileRepo.AssertExpectations(t)
	mockDirRepo.AssertExpectations(t)
}

func TestGroupLoader_GetUserGroups(t *testing.T) {
	ctx := context.Background()

	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice", "bob"}},
			{GroupID: "user", Members: []string{"charlie", "alice"}},
			{GroupID: "reviewer", Members: []string{"david"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	t.Run("user in multiple groups", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		groups, err := loader.GetUserGroups(ctx, "alice")

		assert.NoError(t, err)
		assert.Len(t, groups, 2)
		assert.Contains(t, groups, "admin")
		assert.Contains(t, groups, "user")
	})

	t.Run("user in one group", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		groups, err := loader.GetUserGroups(ctx, "david")

		assert.NoError(t, err)
		assert.Len(t, groups, 1)
		assert.Equal(t, "reviewer", groups[0])
	})

	t.Run("user in no groups", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		groups, err := loader.GetUserGroups(ctx, "unknown")

		assert.NoError(t, err)
		assert.Len(t, groups, 0)
	})

	t.Run("no .group file", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "root-id").
			Return(&models.Directory{ID: "root-id", Path: "/", ParentID: nil}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		groups, err := loader.GetUserGroups(ctx, "alice")

		assert.NoError(t, err)
		assert.Len(t, groups, 0)
	})
}

func TestGroupLoader_GroupExists(t *testing.T) {
	ctx := context.Background()

	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice"}},
			{GroupID: "user", Members: []string{"bob"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	t.Run("group exists", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		exists, err := loader.GroupExists(ctx, "admin")

		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("group does not exist", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		exists, err := loader.GroupExists(ctx, "unknown")

		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("no .group file", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "root-id").
			Return(&models.Directory{ID: "root-id", Path: "/", ParentID: nil}, nil)

		loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		exists, err := loader.GroupExists(ctx, "admin")

		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestGroupLoader_InvalidateCache(t *testing.T) {
	ctx := context.Background()

	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	mockFileRepo := new(MockFileRepository)
	mockDirRepo := new(MockDirectoryRepository)

	mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".group").
		Return(&models.File{
			ID:          "file-1",
			DirectoryID: "dir-1",
			Name:        ".group",
			JSONContent: &groupContent,
		}, nil).Twice()

	loader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

	// First load
	config1, err := loader.Load(ctx, "dir-1")
	assert.NoError(t, err)
	assert.NotNil(t, config1)

	// Invalidate cache
	loader.InvalidateCache("dir-1")

	// Second load should hit repository again
	config2, err := loader.Load(ctx, "dir-1")
	assert.NoError(t, err)
	assert.NotNil(t, config2)

	mockFileRepo.AssertExpectations(t)
}

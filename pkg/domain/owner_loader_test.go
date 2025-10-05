package domain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

func TestOwnerLoader_Load(t *testing.T) {
	ctx := context.Background()

	ownerConfig := OwnerConfig{
		Owners: []string{"admin", "project-team"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	t.Run("load from current directory", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		config, err := loader.Load(ctx, "dir-1")

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Owners, 2)
		assert.Equal(t, "admin", config.Owners[0])
		assert.Equal(t, "project-team", config.Owners[1])
		mockFileRepo.AssertExpectations(t)
	})

	t.Run("load from parent directory", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		parentID := "parent-dir"
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "dir-1").
			Return(&models.Directory{
				ID:       "dir-1",
				ParentID: &parentID,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "parent-dir", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "parent-dir",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		config, err := loader.Load(ctx, "dir-1")

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Owners, 2)
		mockFileRepo.AssertExpectations(t)
		mockDirRepo.AssertExpectations(t)
	})

	t.Run("cache hit", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil).Once()

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)

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

func TestOwnerLoader_LoadByPath(t *testing.T) {
	ctx := context.Background()

	ownerConfig := OwnerConfig{
		Owners: []string{"admin"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	mockFileRepo := new(MockFileRepository)
	mockDirRepo := new(MockDirectoryRepository)
	mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

	mockDirRepo.On("FindByPath", ctx, "/projects/alpha").
		Return(&models.Directory{
			ID:   "dir-1",
			Path: "/projects/alpha",
		}, nil)
	mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
		Return(&models.File{
			ID:          "file-1",
			DirectoryID: "dir-1",
			Name:        ".owner",
			JSONContent: &ownerContent,
		}, nil)

	loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
	config, err := loader.LoadByPath(ctx, "/projects/alpha")

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Len(t, config.Owners, 1)
	mockFileRepo.AssertExpectations(t)
	mockDirRepo.AssertExpectations(t)
}

func TestOwnerLoader_GetOwnerGroups(t *testing.T) {
	ctx := context.Background()

	ownerConfig := OwnerConfig{
		Owners: []string{"admin", "project-team", "reviewers"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	t.Run("get owner groups", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		groups, err := loader.GetOwnerGroups(ctx, "dir-1")

		assert.NoError(t, err)
		assert.Len(t, groups, 3)
		assert.Contains(t, groups, "admin")
		assert.Contains(t, groups, "project-team")
		assert.Contains(t, groups, "reviewers")
	})

	t.Run("no .owner file", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "dir-1").
			Return(&models.Directory{ID: "dir-1", ParentID: nil}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		groups, err := loader.GetOwnerGroups(ctx, "dir-1")

		assert.NoError(t, err)
		assert.Len(t, groups, 0)
	})
}

func TestOwnerLoader_IsUserOwner(t *testing.T) {
	ctx := context.Background()

	ownerConfig := OwnerConfig{
		Owners: []string{"admin", "project-team"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	t.Run("user is owner - single group match", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"admin", "user"}
		isOwner, err := loader.IsUserOwner(ctx, "dir-1", userGroups)

		assert.NoError(t, err)
		assert.True(t, isOwner)
	})

	t.Run("user is owner - multiple group match", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"admin", "project-team", "user"}
		isOwner, err := loader.IsUserOwner(ctx, "dir-1", userGroups)

		assert.NoError(t, err)
		assert.True(t, isOwner)
	})

	t.Run("user is not owner", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"user", "guest"}
		isOwner, err := loader.IsUserOwner(ctx, "dir-1", userGroups)

		assert.NoError(t, err)
		assert.False(t, isOwner)
	})

	t.Run("no .owner file - allow access", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(nil, db.ErrNotFound)
		mockDirRepo.On("FindByID", ctx, "dir-1").
			Return(&models.Directory{ID: "dir-1", ParentID: nil}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"user"}
		isOwner, err := loader.IsUserOwner(ctx, "dir-1", userGroups)

		assert.NoError(t, err)
		assert.True(t, isOwner) // No restriction means allow
	})

	t.Run("user has no groups", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{}
		isOwner, err := loader.IsUserOwner(ctx, "dir-1", userGroups)

		assert.NoError(t, err)
		assert.False(t, isOwner)
	})
}

func TestOwnerLoader_CanUserAccessDirectory(t *testing.T) {
	ctx := context.Background()

	// Setup group config
	groupConfig := GroupConfig{
		Groups: []GroupDefinition{
			{GroupID: "admin", Members: []string{"alice", "bob"}},
			{GroupID: "project-team", Members: []string{"charlie", "alice"}},
			{GroupID: "user", Members: []string{"david"}},
		},
	}
	groupJSON, _ := json.Marshal(groupConfig)
	groupContent := string(groupJSON)

	// Setup owner config
	ownerConfig := OwnerConfig{
		Owners: []string{"admin", "project-team"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	t.Run("user can access - in owner group", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-group",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-owner",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		canAccess, err := loader.CanUserAccessDirectory(ctx, "dir-1", "alice")

		assert.NoError(t, err)
		assert.True(t, canAccess)
	})

	t.Run("user cannot access - not in owner group", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)

		mockDirRepo.On("FindByPath", ctx, "/").
			Return(&models.Directory{ID: "root-id", Path: "/"}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "root-id", ".group").
			Return(&models.File{
				ID:          "file-group",
				DirectoryID: "root-id",
				Name:        ".group",
				JSONContent: &groupContent,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-owner",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent,
			}, nil)

		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)
		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		canAccess, err := loader.CanUserAccessDirectory(ctx, "dir-1", "david")

		assert.NoError(t, err)
		assert.False(t, canAccess)
	})
}

func TestOwnerLoader_FilterVisibleDirectories(t *testing.T) {
	ctx := context.Background()

	// Directory 1 owned by admin
	ownerConfig1 := OwnerConfig{Owners: []string{"admin"}}
	ownerJSON1, _ := json.Marshal(ownerConfig1)
	ownerContent1 := string(ownerJSON1)

	// Directory 2 owned by project-team
	ownerConfig2 := OwnerConfig{Owners: []string{"project-team"}}
	ownerJSON2, _ := json.Marshal(ownerConfig2)
	ownerContent2 := string(ownerJSON2)

	// Directory 3 owned by guest
	ownerConfig3 := OwnerConfig{Owners: []string{"guest"}}
	ownerJSON3, _ := json.Marshal(ownerConfig3)
	ownerContent3 := string(ownerJSON3)

	t.Run("filter directories by ownership", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent1,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-2", ".owner").
			Return(&models.File{
				ID:          "file-2",
				DirectoryID: "dir-2",
				Name:        ".owner",
				JSONContent: &ownerContent2,
			}, nil)
		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-3", ".owner").
			Return(&models.File{
				ID:          "file-3",
				DirectoryID: "dir-3",
				Name:        ".owner",
				JSONContent: &ownerContent3,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"admin", "project-team"}
		dirIDs := []string{"dir-1", "dir-2", "dir-3"}

		visibleDirs, err := loader.FilterVisibleDirectories(ctx, dirIDs, userGroups)

		assert.NoError(t, err)
		assert.Len(t, visibleDirs, 2)
		assert.Contains(t, visibleDirs, "dir-1")
		assert.Contains(t, visibleDirs, "dir-2")
		assert.NotContains(t, visibleDirs, "dir-3")
	})

	t.Run("no directories match", func(t *testing.T) {
		mockFileRepo := new(MockFileRepository)
		mockDirRepo := new(MockDirectoryRepository)
		mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

		mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
			Return(&models.File{
				ID:          "file-1",
				DirectoryID: "dir-1",
				Name:        ".owner",
				JSONContent: &ownerContent1,
			}, nil)

		loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)
		userGroups := []string{"guest"}
		dirIDs := []string{"dir-1"}

		visibleDirs, err := loader.FilterVisibleDirectories(ctx, dirIDs, userGroups)

		assert.NoError(t, err)
		assert.Len(t, visibleDirs, 0)
	})
}

func TestOwnerLoader_InvalidateCache(t *testing.T) {
	ctx := context.Background()

	ownerConfig := OwnerConfig{
		Owners: []string{"admin"},
	}
	ownerJSON, _ := json.Marshal(ownerConfig)
	ownerContent := string(ownerJSON)

	mockFileRepo := new(MockFileRepository)
	mockDirRepo := new(MockDirectoryRepository)
	mockGroupLoader := NewGroupLoader(mockFileRepo, mockDirRepo, 5*time.Minute)

	mockFileRepo.On("FindByDirectoryAndName", ctx, "dir-1", ".owner").
		Return(&models.File{
			ID:          "file-1",
			DirectoryID: "dir-1",
			Name:        ".owner",
			JSONContent: &ownerContent,
		}, nil).Twice()

	loader := NewOwnerLoader(mockFileRepo, mockDirRepo, mockGroupLoader, 5*time.Minute)

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

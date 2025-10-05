package domain

import (
	"context"
	"testing"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// Mock repositories for testing
type mockPolicyFileRepo struct {
	files    map[string]*models.File
	versions map[string]*models.FileVersion
}

func (m *mockPolicyFileRepo) FindByDirectoryAndName(ctx context.Context, directoryID, name string) (*models.File, error) {
	key := directoryID + "/" + name
	if file, ok := m.files[key]; ok {
		return file, nil
	}
	return nil, db.ErrNotFound
}

func (m *mockPolicyFileRepo) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	if version, ok := m.versions[fileID]; ok {
		return version, nil
	}
	return nil, db.ErrNotFound
}

// Unused methods
func (m *mockPolicyFileRepo) Create(ctx context.Context, file *models.File) error {
	return nil
}
func (m *mockPolicyFileRepo) Update(ctx context.Context, file *models.File) error {
	return nil
}
func (m *mockPolicyFileRepo) FindByID(ctx context.Context, id string) (*models.File, error) {
	return nil, db.ErrNotFound
}
func (m *mockPolicyFileRepo) FindByDirectory(ctx context.Context, directoryID string) ([]*models.File, error) {
	return nil, nil
}
func (m *mockPolicyFileRepo) SoftDelete(ctx context.Context, id string) error {
	return nil
}
func (m *mockPolicyFileRepo) Exists(ctx context.Context, directoryID, name string) (bool, error) {
	return false, nil
}
func (m *mockPolicyFileRepo) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	return nil
}
func (m *mockPolicyFileRepo) GetVersion(ctx context.Context, fileID string, version int64) (*models.FileVersion, error) {
	return nil, db.ErrNotFound
}
func (m *mockPolicyFileRepo) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	return nil, nil
}
func (m *mockPolicyFileRepo) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockPolicyFileRepo) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	return nil, "", nil
}
func (m *mockPolicyFileRepo) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	return nil
}
func (m *mockPolicyFileRepo) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	return nil, db.ErrNotFound
}
func (m *mockPolicyFileRepo) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	return nil
}

type mockPolicyDirRepo struct {
	dirs map[string]*models.Directory
}

func (m *mockPolicyDirRepo) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	if dir, ok := m.dirs[path]; ok {
		return dir, nil
	}
	return nil, db.ErrNotFound
}

func (m *mockPolicyDirRepo) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	for _, dir := range m.dirs {
		if dir.ID == id {
			return dir, nil
		}
	}
	return nil, db.ErrNotFound
}

// Unused methods
func (m *mockPolicyDirRepo) Create(ctx context.Context, dir *models.Directory) error {
	return nil
}
func (m *mockPolicyDirRepo) Update(ctx context.Context, dir *models.Directory) error {
	return nil
}
func (m *mockPolicyDirRepo) SoftDelete(ctx context.Context, id string) error {
	return nil
}
func (m *mockPolicyDirRepo) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	return nil, "", nil
}
func (m *mockPolicyDirRepo) Exists(ctx context.Context, path string) (bool, error) {
	_, ok := m.dirs[path]
	return ok, nil
}
func (m *mockPolicyDirRepo) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockPolicyDirRepo) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	return nil
}

func TestPolicyLoader_LoadPolicy(t *testing.T) {
	tests := []struct {
		name          string
		directoryPath string
		setupMocks    func(*mockPolicyFileRepo, *mockPolicyDirRepo)
		wantErr       bool
		wantPolicy    string
	}{
		{
			name:          "load policy from directory",
			directoryPath: "/projects/secret",
			setupMocks: func(fileRepo *mockPolicyFileRepo, dirRepo *mockPolicyDirRepo) {
				dirID := "dir-secret"
				dirRepo.dirs["/projects/secret"] = &models.Directory{
					ID:   dirID,
					Name: "secret",
					Path: "/projects/secret",
				}

				fileID := "file-rego-1"
				fileRepo.files[dirID+"/.rego"] = &models.File{
					ID:          fileID,
					DirectoryID: dirID,
					Name:        ".rego",
				}

				policy := "package vfs.authz\nallow { input.user.role == \"admin\" }"
				fileRepo.versions[fileID] = &models.FileVersion{
					ID:          "version-1",
					FileID:      fileID,
					JSONContent: &policy,
				}
			},
			wantErr:    false,
			wantPolicy: "package vfs.authz\nallow { input.user.role == \"admin\" }",
		},
		{
			name:          "inherit policy from parent",
			directoryPath: "/projects/secret/data",
			setupMocks: func(fileRepo *mockPolicyFileRepo, dirRepo *mockPolicyDirRepo) {
				// Parent directory
				parentID := "dir-secret"
				dirRepo.dirs["/projects/secret"] = &models.Directory{
					ID:   parentID,
					Name: "secret",
					Path: "/projects/secret",
				}

				// Child directory (no .rego file)
				childID := "dir-data"
				dirRepo.dirs["/projects/secret/data"] = &models.Directory{
					ID:       childID,
					ParentID: &parentID,
					Name:     "data",
					Path:     "/projects/secret/data",
				}

				// Policy in parent directory
				fileID := "file-rego-1"
				fileRepo.files[parentID+"/.rego"] = &models.File{
					ID:          fileID,
					DirectoryID: parentID,
					Name:        ".rego",
				}

				policy := "package vfs.authz\nallow { input.user.role == \"admin\" }"
				fileRepo.versions[fileID] = &models.FileVersion{
					ID:          "version-1",
					FileID:      fileID,
					JSONContent: &policy,
				}
			},
			wantErr:    false,
			wantPolicy: "package vfs.authz\nallow { input.user.role == \"admin\" }",
		},
		{
			name:          "no policy found",
			directoryPath: "/public",
			setupMocks: func(fileRepo *mockPolicyFileRepo, dirRepo *mockPolicyDirRepo) {
				dirRepo.dirs["/public"] = &models.Directory{
					ID:   "dir-public",
					Name: "public",
					Path: "/public",
				}
				// No .rego file
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileRepo := &mockPolicyFileRepo{
				files:    make(map[string]*models.File),
				versions: make(map[string]*models.FileVersion),
			}
			dirRepo := &mockPolicyDirRepo{
				dirs: make(map[string]*models.Directory),
			}

			tt.setupMocks(fileRepo, dirRepo)

			loader := NewPolicyLoader(fileRepo, dirRepo, 5*time.Minute)
			ctx := context.Background()

			policy, err := loader.LoadPolicy(ctx, tt.directoryPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPolicy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && policy != tt.wantPolicy {
				t.Errorf("LoadPolicy() = %v, want %v", policy, tt.wantPolicy)
			}
		})
	}
}

func TestPolicyLoader_Caching(t *testing.T) {
	fileRepo := &mockPolicyFileRepo{
		files:    make(map[string]*models.File),
		versions: make(map[string]*models.FileVersion),
	}
	dirRepo := &mockPolicyDirRepo{
		dirs: make(map[string]*models.Directory),
	}

	// Setup mock data
	dirID := "dir-test"
	dirRepo.dirs["/test"] = &models.Directory{
		ID:   dirID,
		Name: "test",
		Path: "/test",
	}

	fileID := "file-rego-1"
	fileRepo.files[dirID+"/.rego"] = &models.File{
		ID:          fileID,
		DirectoryID: dirID,
		Name:        ".rego",
	}

	policy := "package vfs.authz\nallow { true }"
	fileRepo.versions[fileID] = &models.FileVersion{
		ID:          "version-1",
		FileID:      fileID,
		JSONContent: &policy,
	}

	loader := NewPolicyLoader(fileRepo, dirRepo, 5*time.Minute)
	ctx := context.Background()

	// First load - should hit the repository
	policy1, err := loader.LoadPolicy(ctx, "/test")
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Second load - should hit the cache
	policy2, err := loader.LoadPolicy(ctx, "/test")
	if err != nil {
		t.Fatalf("Second load failed: %v", err)
	}

	if policy1 != policy2 {
		t.Errorf("Cached policy doesn't match: got %v, want %v", policy2, policy1)
	}

	// Invalidate cache
	loader.Invalidate("/test")

	// Third load - should hit the repository again
	policy3, err := loader.LoadPolicy(ctx, "/test")
	if err != nil {
		t.Fatalf("Third load failed: %v", err)
	}

	if policy1 != policy3 {
		t.Errorf("Policy after invalidation doesn't match: got %v, want %v", policy3, policy1)
	}
}

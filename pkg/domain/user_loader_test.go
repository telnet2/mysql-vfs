package domain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"golang.org/x/crypto/bcrypt"
)

// Mock repositories for testing
type mockFileRepo struct {
	files map[string]map[string]*models.File // dirID -> fileName -> file
}

type mockDirRepo struct {
	dirs map[string]*models.Directory // path -> dir
}

func newMockFileRepo() *mockFileRepo {
	return &mockFileRepo{
		files: make(map[string]map[string]*models.File),
	}
}

// Implement repository.FileRepository interface
func (r *mockFileRepo) Create(ctx context.Context, file *models.File) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) FindByID(ctx context.Context, id string) (*models.File, error) {
	return nil, db.ErrNotFound // Not used in these tests
}

func (r *mockFileRepo) FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error) {
	if dirFiles, ok := r.files[dirID]; ok {
		if file, ok := dirFiles[name]; ok {
			return file, nil
		}
	}
	return nil, db.ErrNotFound
}

func (r *mockFileRepo) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	return nil, "", db.ErrNotFound // Not used in these tests
}

func (r *mockFileRepo) Update(ctx context.Context, file *models.File) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) Delete(ctx context.Context, id string) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) SoftDelete(ctx context.Context, id string) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	return []*models.FileVersion{}, nil // Not used in these tests
}

func (r *mockFileRepo) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	return nil, db.ErrNotFound // Not used in these tests
}

func (r *mockFileRepo) Exists(ctx context.Context, dirID, name string) (bool, error) {
	return false, nil // Not used in these tests
}

func (r *mockFileRepo) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	return nil, db.ErrNotFound // Not used in these tests
}

func (r *mockFileRepo) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	return nil // Not used in these tests
}

func (r *mockFileRepo) addFile(dirID, name string, content interface{}) {
	if r.files[dirID] == nil {
		r.files[dirID] = make(map[string]*models.File)
	}
	jsonBytes, _ := json.Marshal(content)
	jsonStr := string(jsonBytes)
	r.files[dirID][name] = &models.File{
		Name:        name,
		JSONContent: &jsonStr,
	}
}

func newMockDirRepo() *mockDirRepo {
	return &mockDirRepo{
		dirs: make(map[string]*models.Directory),
	}
}

// Implement repository.DirectoryRepository interface
func (r *mockDirRepo) Create(ctx context.Context, dir *models.Directory) error {
	return nil // Not used in these tests
}

func (r *mockDirRepo) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	if dir, ok := r.dirs[path]; ok {
		return dir, nil
	}
	return nil, db.ErrNotFound
}

func (r *mockDirRepo) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	for _, dir := range r.dirs {
		if dir.ID == id {
			return dir, nil
		}
	}
	return nil, db.ErrNotFound
}

func (r *mockDirRepo) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	return nil, "", db.ErrNotFound // Not used in these tests
}

func (r *mockDirRepo) Update(ctx context.Context, dir *models.Directory) error {
	return nil // Not used in these tests
}

func (r *mockDirRepo) Delete(ctx context.Context, id string) error {
	return nil // Not used in these tests
}

func (r *mockDirRepo) SoftDelete(ctx context.Context, id string) error {
	return nil // Not used in these tests
}

func (r *mockDirRepo) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	return nil // Not used in these tests
}

func (r *mockDirRepo) Exists(ctx context.Context, path string) (bool, error) {
	return false, nil // Not used in these tests
}

func (r *mockDirRepo) CountByParentID(ctx context.Context, parentID string) (int64, error) {
	return 0, nil // Not used in these tests
}

func (r *mockDirRepo) addDir(id, path string, parentID *string) {
	r.dirs[path] = &models.Directory{
		ID:       id,
		Path:     path,
		ParentID: parentID,
	}
}

// Helper to create password hash
func hashPassword(password string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash)
}

func TestUserLoader_LoadUser(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup test data
	dirRepo.addDir("dir1", "/users", nil)

	userConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID:       "alice",
				PasswordHash: hashPassword("password123"),
				Groups:       []string{"admin"},
			},
			{
				UserID:       "bob",
				PasswordHash: hashPassword("secret"),
				Groups:       []string{"user"},
				Token:        "token123",
			},
		},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	tests := []struct {
		name      string
		dirPath   string
		userID    string
		wantError bool
		checkUser func(*testing.T, *UserCredential)
	}{
		{
			name:      "load existing user alice",
			dirPath:   "/users",
			userID:    "alice",
			wantError: false,
			checkUser: func(t *testing.T, user *UserCredential) {
				if user.UserID != "alice" {
					t.Errorf("Expected user_id alice, got %s", user.UserID)
				}
				if len(user.Groups) == 0 || user.Groups[0] != "admin" {
					t.Errorf("Expected group admin, got %v", user.Groups)
				}
			},
		},
		{
			name:      "load existing user bob",
			dirPath:   "/users",
			userID:    "bob",
			wantError: false,
			checkUser: func(t *testing.T, user *UserCredential) {
				if user.UserID != "bob" {
					t.Errorf("Expected user_id bob, got %s", user.UserID)
				}
				if user.Token != "token123" {
					t.Errorf("Expected token token123, got %s", user.Token)
				}
			},
		},
		{
			name:      "load non-existent user",
			dirPath:   "/users",
			userID:    "charlie",
			wantError: true,
		},
		{
			name:      "directory not found",
			dirPath:   "/nonexistent",
			userID:    "alice",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := loader.LoadUser(ctx, tt.dirPath, tt.userID)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if tt.checkUser != nil {
					tt.checkUser(t, user)
				}
			}
		})
	}
}

func TestUserLoader_LoadUserByToken(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup test data
	dirRepo.addDir("dir1", "/api", nil)

	userConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID: "service1",
				Token:  "service-token-abc123",
				Groups: []string{"service"},
			},
			{
				UserID:       "user1",
				PasswordHash: hashPassword("password"),
				Groups:       []string{"user"},
			},
			{
				UserID: "service2",
				Token:  "service-token-xyz789",
				Groups: []string{"service"},
			},
		},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	tests := []struct {
		name      string
		dirPath   string
		token     string
		wantError bool
		wantUser  string
	}{
		{
			name:      "valid token for service1",
			dirPath:   "/api",
			token:     "service-token-abc123",
			wantError: false,
			wantUser:  "service1",
		},
		{
			name:      "valid token for service2",
			dirPath:   "/api",
			token:     "service-token-xyz789",
			wantError: false,
			wantUser:  "service2",
		},
		{
			name:      "invalid token",
			dirPath:   "/api",
			token:     "wrong-token",
			wantError: true,
		},
		{
			name:      "empty token",
			dirPath:   "/api",
			token:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := loader.LoadUserByToken(ctx, tt.dirPath, tt.token)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if user.UserID != tt.wantUser {
					t.Errorf("Expected user %s, got %s", tt.wantUser, user.UserID)
				}
			}
		})
	}
}

func TestUserLoader_ValidatePassword(t *testing.T) {
	loader := NewUserLoader(nil, nil, 5*time.Minute)

	tests := []struct {
		name      string
		user      *UserCredential
		password  string
		wantError bool
	}{
		{
			name: "valid password",
			user: &UserCredential{
				UserID:       "alice",
				PasswordHash: hashPassword("correct-password"),
			},
			password:  "correct-password",
			wantError: false,
		},
		{
			name: "invalid password",
			user: &UserCredential{
				UserID:       "alice",
				PasswordHash: hashPassword("correct-password"),
			},
			password:  "wrong-password",
			wantError: true,
		},
		{
			name: "user has no password",
			user: &UserCredential{
				UserID:       "alice",
				PasswordHash: "",
			},
			password:  "any-password",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.ValidatePassword(tt.user, tt.password)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestUserLoader_Caching(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup test data
	dirRepo.addDir("dir1", "/users", nil)

	userConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID: "alice",
				Groups: []string{"admin"},
			},
		},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	// Create loader with short TTL
	loader := NewUserLoader(fileRepo, dirRepo, 100*time.Millisecond)

	// First load - should hit the file repo
	user1, err := loader.LoadUser(ctx, "/users", "alice")
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Second load - should hit cache
	user2, err := loader.LoadUser(ctx, "/users", "alice")
	if err != nil {
		t.Fatalf("Second load failed: %v", err)
	}

	// Both should return same user
	if user1.UserID != user2.UserID {
		t.Error("Cache returned different user")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Update the file
	newConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID: "alice",
				Groups: []string{"superadmin"}, // Changed role
			},
		},
	}
	fileRepo.addFile("dir1", ".user", newConfig)

	// Load again - should get updated data
	user3, err := loader.LoadUser(ctx, "/users", "alice")
	if err != nil {
		t.Fatalf("Third load failed: %v", err)
	}

	if len(user3.Groups) == 0 || user3.Groups[0] != "superadmin" {
		t.Errorf("Expected updated group superadmin, got %v", user3.Groups)
	}
}

func TestUserLoader_CacheInvalidation(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup test data
	dirRepo.addDir("dir1", "/users", nil)

	userConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID: "alice",
				Groups: []string{"admin"},
			},
		},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	// Load to populate cache
	_, err := loader.LoadUser(ctx, "/users", "alice")
	if err != nil {
		t.Fatalf("Initial load failed: %v", err)
	}

	// Update the file
	newConfig := UserConfig{
		Users: []UserCredential{
			{
				UserID: "alice",
				Groups: []string{"superadmin"},
			},
		},
	}
	fileRepo.addFile("dir1", ".user", newConfig)

	// Invalidate cache
	loader.InvalidateCache("dir1")

	// Load again - should get updated data
	user, err := loader.LoadUser(ctx, "/users", "alice")
	if err != nil {
		t.Fatalf("Load after invalidation failed: %v", err)
	}

	if len(user.Groups) == 0 || user.Groups[0] != "superadmin" {
		t.Errorf("Expected group superadmin after cache invalidation, got %v", user.Groups)
	}
}

func TestUserLoader_InvalidUserFile(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup test data with invalid JSON
	dirRepo.addDir("dir1", "/users", nil)

	// Add file with invalid content
	if fileRepo.files["dir1"] == nil {
		fileRepo.files["dir1"] = make(map[string]*models.File)
	}
	invalidJSON := "{ invalid json }"
	fileRepo.files["dir1"][".user"] = &models.File{
		Name:        ".user",
		JSONContent: &invalidJSON,
	}

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	// Should fail to load
	_, err := loader.LoadUser(ctx, "/users", "alice")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestUserLoader_MissingUserFile(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup directory but no .user file
	dirRepo.addDir("dir1", "/users", nil)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	// Should fail - no .user file
	_, err := loader.LoadUser(ctx, "/users", "alice")
	if err == nil {
		t.Error("Expected error for missing .user file, got nil")
	}
}

func TestUserLoader_EmptyUserFile(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup with empty user list
	dirRepo.addDir("dir1", "/users", nil)

	userConfig := UserConfig{
		Users: []UserCredential{},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	// Should fail - user not found
	_, err := loader.LoadUser(ctx, "/users", "alice")
	if err == nil {
		t.Error("Expected error for user not in empty list, got nil")
	}
}

func TestUserLoader_MultipleUsers(t *testing.T) {
	ctx := context.Background()
	fileRepo := newMockFileRepo()
	dirRepo := newMockDirRepo()

	// Setup with multiple users
	dirRepo.addDir("dir1", "/team", nil)

	userConfig := UserConfig{
		Users: []UserCredential{
			{UserID: "alice", Groups: []string{"admin"}},
			{UserID: "bob", Groups: []string{"developer"}},
			{UserID: "charlie", Groups: []string{"designer"}},
			{UserID: "dave", Groups: []string{"developer"}},
		},
	}
	fileRepo.addFile("dir1", ".user", userConfig)

	loader := NewUserLoader(fileRepo, dirRepo, 5*time.Minute)

	// Load all users
	users := []string{"alice", "bob", "charlie", "dave"}
	for _, userID := range users {
		user, err := loader.LoadUser(ctx, "/team", userID)
		if err != nil {
			t.Errorf("Failed to load user %s: %v", userID, err)
		}
		if user.UserID != userID {
			t.Errorf("Expected user %s, got %s", userID, user.UserID)
		}
	}
}

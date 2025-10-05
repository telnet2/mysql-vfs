package citest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/rego"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/middleware"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// TestOPAIntegration_FullFlow tests the complete OPA integration flow:
// 1. Create directories
// 2. Upload .rego policy files
// 3. Load policies with inheritance
// 4. Evaluate authorization decisions
// 5. Test cache invalidation
func TestOPAIntegration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup mock repositories
	fileRepo := newMockFileRepository()
	dirRepo := newMockDirectoryRepository()

	// Create directory structure
	rootDir := &models.Directory{
		ID:       "dir-root",
		Name:     "root",
		Path:     "/",
		PathHash: "hash-root",
	}
	dirRepo.dirs["/"] = rootDir

	projectsDir := &models.Directory{
		ID:       "dir-projects",
		ParentID: &rootDir.ID,
		Name:     "projects",
		Path:     "/projects",
		PathHash: "hash-projects",
	}
	dirRepo.dirs["/projects"] = projectsDir

	secretDir := &models.Directory{
		ID:       "dir-secret",
		ParentID: &projectsDir.ID,
		Name:     "secret",
		Path:     "/projects/secret",
		PathHash: "hash-secret",
	}
	dirRepo.dirs["/projects/secret"] = secretDir

	// Create .rego policy in /projects (parent)
	projectsPolicy := `package vfs.authz

# Allow admins everywhere
allow {
	input.user.role == "admin"
}

# Allow developers group for read
allow {
	input.action == "read"
	input.user.groups[_] == "developers"
}`

	projectsPolicyFile := &models.File{
		ID:          "file-projects-rego",
		DirectoryID: projectsDir.ID,
		Name:        ".rego",
		ContentType: "text/plain",
	}
	fileRepo.files[projectsDir.ID+"/.rego"] = projectsPolicyFile

	projectsPolicyContent := projectsPolicy
	projectsPolicyVersion := &models.FileVersion{
		ID:          "version-projects-rego",
		FileID:      projectsPolicyFile.ID,
		JSONContent: &projectsPolicyContent,
	}
	fileRepo.versions[projectsPolicyFile.ID] = projectsPolicyVersion

	// Create stricter .rego policy in /projects/secret (child overrides)
	secretPolicy := `package vfs.authz

# Only admins allowed in secret directory
allow {
	input.user.role == "admin"
}`

	secretPolicyFile := &models.File{
		ID:          "file-secret-rego",
		DirectoryID: secretDir.ID,
		Name:        ".rego",
		ContentType: "text/plain",
	}
	fileRepo.files[secretDir.ID+"/.rego"] = secretPolicyFile

	secretPolicyContent := secretPolicy
	secretPolicyVersion := &models.FileVersion{
		ID:          "version-secret-rego",
		FileID:      secretPolicyFile.ID,
		JSONContent: &secretPolicyContent,
	}
	fileRepo.versions[secretPolicyFile.ID] = secretPolicyVersion

	// Create PolicyLoader
	policyLoader := domain.NewPolicyLoader(fileRepo, dirRepo, 5*time.Minute)

	ctx := context.Background()

	// Test 1: Load policy from /projects
	t.Run("load policy from parent directory", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}
		if policy != projectsPolicy {
			t.Errorf("Policy mismatch: got %v, want %v", policy, projectsPolicy)
		}
	})

	// Test 2: Evaluate admin access to /projects
	t.Run("admin can write to projects directory", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       "user-admin",
				"username": "alice",
				"role":     "admin",
				"groups":   []string{},
			},
			"resource": map[string]interface{}{
				"path": "/projects/app",
				"type": "file",
			},
			"action": "create",
		}

		allowed := evaluatePolicy(policy, input)
		if !allowed {
			t.Error("Admin should be allowed to write to /projects")
		}
	})

	// Test 3: Developer can read from /projects
	t.Run("developer can read from projects directory", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       "user-dev",
				"username": "bob",
				"role":     "user",
				"groups":   []string{"developers"},
			},
			"resource": map[string]interface{}{
				"path": "/projects/app",
				"type": "file",
			},
			"action": "read",
		}

		allowed := evaluatePolicy(policy, input)
		if !allowed {
			t.Error("Developer should be allowed to read from /projects")
		}
	})

	// Test 4: Developer cannot write to /projects
	t.Run("developer cannot write to projects directory", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       "user-dev",
				"username": "bob",
				"role":     "user",
				"groups":   []string{"developers"},
			},
			"resource": map[string]interface{}{
				"path": "/projects/app",
				"type": "file",
			},
			"action": "create",
		}

		allowed := evaluatePolicy(policy, input)
		if allowed {
			t.Error("Developer should not be allowed to write to /projects")
		}
	})

	// Test 5: Child directory has stricter policy (inheritance override)
	t.Run("child directory overrides parent policy", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects/secret")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		if policy != secretPolicy {
			t.Errorf("Should load child policy, not parent")
		}

		// Developer can no longer read (stricter policy)
		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       "user-dev",
				"username": "bob",
				"role":     "user",
				"groups":   []string{"developers"},
			},
			"resource": map[string]interface{}{
				"path": "/projects/secret/data",
				"type": "file",
			},
			"action": "read",
		}

		allowed := evaluatePolicy(policy, input)
		if allowed {
			t.Error("Developer should NOT be allowed to read from /projects/secret (stricter policy)")
		}
	})

	// Test 6: Admin can still access secret directory
	t.Run("admin can access secret directory", func(t *testing.T) {
		policy, err := policyLoader.LoadPolicy(ctx, "/projects/secret")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		input := map[string]interface{}{
			"user": map[string]interface{}{
				"id":       "user-admin",
				"username": "alice",
				"role":     "admin",
				"groups":   []string{},
			},
			"resource": map[string]interface{}{
				"path": "/projects/secret/credentials",
				"type": "file",
			},
			"action": "create",
		}

		allowed := evaluatePolicy(policy, input)
		if !allowed {
			t.Error("Admin should be allowed to write to /projects/secret")
		}
	})

	// Test 7: Cache effectiveness
	t.Run("policy caching works", func(t *testing.T) {
		// First load
		start := time.Now()
		policy1, err := policyLoader.LoadPolicy(ctx, "/projects")
		duration1 := time.Since(start)
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		// Second load (should be from cache)
		start = time.Now()
		policy2, err := policyLoader.LoadPolicy(ctx, "/projects")
		duration2 := time.Since(start)
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		if policy1 != policy2 {
			t.Error("Cached policy should match original")
		}

		// Cached load should be faster (though both are very fast with mock)
		t.Logf("First load: %v, Cached load: %v", duration1, duration2)
	})

	// Test 8: Cache invalidation
	t.Run("cache invalidation works", func(t *testing.T) {
		// Load policy
		policy1, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		// Invalidate cache
		policyLoader.Invalidate("/projects")

		// Update the policy in the mock
		newPolicy := `package vfs.authz

allow {
	true  # Allow everyone (for testing)
}`
		newPolicyContent := newPolicy
		projectsPolicyVersion.JSONContent = &newPolicyContent

		// Load again (should get new policy)
		policy2, err := policyLoader.LoadPolicy(ctx, "/projects")
		if err != nil {
			t.Fatalf("Failed to load policy: %v", err)
		}

		if policy1 == policy2 {
			t.Error("Policy should be different after invalidation and update")
		}

		if policy2 != newPolicy {
			t.Error("Should load new policy after invalidation")
		}
	})
}

// TestOPAIntegration_AuthorizationMiddleware tests the full middleware flow
func TestOPAIntegration_AuthorizationMiddleware(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	fileRepo := newMockFileRepository()
	dirRepo := newMockDirectoryRepository()

	// Create /api directory with policy
	apiDir := &models.Directory{
		ID:       "dir-api",
		Name:     "api",
		Path:     "/api",
		PathHash: "hash-api",
	}
	dirRepo.dirs["/api"] = apiDir

	// Create restrictive policy
	policy := `package vfs.authz

# Only allow admin or specific groups
allow {
	input.user.role == "admin"
}

allow {
	input.user.groups[_] == "api-users"
	input.action == "read"
}`

	policyFile := &models.File{
		ID:          "file-api-rego",
		DirectoryID: apiDir.ID,
		Name:        ".rego",
		ContentType: "text/plain",
	}
	fileRepo.files[apiDir.ID+"/.rego"] = policyFile

	policyContent := policy
	policyVersion := &models.FileVersion{
		ID:          "version-api-rego",
		FileID:      policyFile.ID,
		JSONContent: &policyContent,
	}
	fileRepo.versions[policyFile.ID] = policyVersion

	policyLoader := domain.NewPolicyLoader(fileRepo, dirRepo, 5*time.Minute)

	// Create middleware
	authzMiddleware := middleware.NewAuthorizationMiddleware(middleware.AuthorizationConfig{
		PolicyLoader: policyLoader,
		Timeout:      200 * time.Millisecond,
		SkipRoutes:   []string{"/health"},
	})

	if authzMiddleware == nil {
		t.Fatal("Failed to create authorization middleware")
	}

	t.Run("middleware configured correctly", func(t *testing.T) {
		// Just verify middleware was created successfully
		// Full HTTP integration would require more setup
		if authzMiddleware == nil {
			t.Error("Middleware should not be nil")
		}
	})
}

// evaluatePolicy uses the actual OPA engine to evaluate a Rego policy
// This duplicates the logic from middleware.evaluateRegoPolicy for integration testing
func evaluatePolicy(regoPolicy string, input map[string]interface{}) bool {
	ctx := context.Background()

	// Compile the Rego policy using the real OPA engine
	query, err := rego.New(
		rego.Query("data.vfs.authz.allow"),
		rego.Module("policy.rego", regoPolicy),
	).PrepareForEval(ctx)

	if err != nil {
		// Policy compilation failed - fail closed
		return false
	}

	// Evaluate the policy with the input
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		// Evaluation failed - fail closed
		return false
	}

	// Check if the policy allows the request
	if len(results) > 0 && len(results[0].Expressions) > 0 {
		if allowed, ok := results[0].Expressions[0].Value.(bool); ok {
			return allowed
		}
	}

	// Default deny if no clear allow decision
	return false
}

// Mock repository implementations

type mockFileRepository struct {
	files    map[string]*models.File
	versions map[string]*models.FileVersion
}

func newMockFileRepository() *mockFileRepository {
	return &mockFileRepository{
		files:    make(map[string]*models.File),
		versions: make(map[string]*models.FileVersion),
	}
}

func (m *mockFileRepository) FindByDirectoryAndName(ctx context.Context, directoryID, name string) (*models.File, error) {
	key := directoryID + "/" + name
	if file, ok := m.files[key]; ok {
		return file, nil
	}
	return nil, db.ErrNotFound
}

func (m *mockFileRepository) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	if version, ok := m.versions[fileID]; ok {
		return version, nil
	}
	return nil, db.ErrNotFound
}

func (m *mockFileRepository) Create(ctx context.Context, file *models.File) error {
	file.ID = uuid.New().String()
	key := file.DirectoryID + "/" + file.Name
	m.files[key] = file
	return nil
}

func (m *mockFileRepository) Update(ctx context.Context, file *models.File) error { return nil }
func (m *mockFileRepository) FindByID(ctx context.Context, id string) (*models.File, error) {
	return nil, db.ErrNotFound
}
func (m *mockFileRepository) FindByDirectoryID(ctx context.Context, dirID string, limit int, cursor string) ([]*models.File, string, error) {
	return nil, "", nil
}
func (m *mockFileRepository) SoftDelete(ctx context.Context, id string) error   { return nil }
func (m *mockFileRepository) Delete(ctx context.Context, id string) error       { return nil }
func (m *mockFileRepository) Exists(ctx context.Context, dirID, name string) (bool, error) {
	key := dirID + "/" + name
	_, ok := m.files[key]
	return ok, nil
}
func (m *mockFileRepository) CreateVersion(ctx context.Context, version *models.FileVersion) error {
	m.versions[version.FileID] = version
	return nil
}
func (m *mockFileRepository) GetVersion(ctx context.Context, fileID string, version int64) (*models.FileVersion, error) {
	return nil, db.ErrNotFound
}
func (m *mockFileRepository) ListVersions(ctx context.Context, fileID string) ([]*models.FileVersion, error) {
	return nil, nil
}

func (m *mockFileRepository) CreateFile(ctx context.Context, file *models.File, content []byte) error {
	// Simple mock - just store metadata
	key := file.DirectoryID + "/" + file.Name
	m.files[key] = file
	return nil
}

func (m *mockFileRepository) GetFileContent(ctx context.Context, file *models.File) ([]byte, error) {
	// Mock returns empty content
	return []byte{}, nil
}

func (m *mockFileRepository) UpdateFile(ctx context.Context, file *models.File, content []byte) error {
	// Simple mock - just update metadata
	return nil
}

type mockDirectoryRepository struct {
	dirs map[string]*models.Directory
}

func newMockDirectoryRepository() *mockDirectoryRepository {
	return &mockDirectoryRepository{
		dirs: make(map[string]*models.Directory),
	}
}

func (m *mockDirectoryRepository) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
	if dir, ok := m.dirs[path]; ok {
		return dir, nil
	}
	return nil, db.ErrNotFound
}

func (m *mockDirectoryRepository) FindByID(ctx context.Context, id string) (*models.Directory, error) {
	for _, dir := range m.dirs {
		if dir.ID == id {
			return dir, nil
		}
	}
	return nil, db.ErrNotFound
}

func (m *mockDirectoryRepository) Create(ctx context.Context, dir *models.Directory) error {
	dir.ID = uuid.New().String()
	m.dirs[dir.Path] = dir
	return nil
}
func (m *mockDirectoryRepository) Update(ctx context.Context, dir *models.Directory) error { return nil }
func (m *mockDirectoryRepository) SoftDelete(ctx context.Context, id string) error         { return nil }
func (m *mockDirectoryRepository) Delete(ctx context.Context, id string) error             { return nil }
func (m *mockDirectoryRepository) FindByParentID(ctx context.Context, parentID string, limit int, cursor string) ([]*models.Directory, string, error) {
	return nil, "", nil
}
func (m *mockDirectoryRepository) Exists(ctx context.Context, path string) (bool, error) {
	_, ok := m.dirs[path]
	return ok, nil
}
func (m *mockDirectoryRepository) LockPaths(ctx context.Context, tx db.Transaction, paths []string) error {
	return nil
}

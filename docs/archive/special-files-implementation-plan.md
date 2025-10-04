# Special Files Implementation Plan

**Date:** 2025-10-03
**Status:** Ready for Implementation
**Design:** See `docs/special-files-design.md`

---

## Summary

Implement file-based schema and policy management using special files (`.jsonschema` and `.rego`) stored directly in the VFS. Only super-users can create/modify special files (any file starting with `.`).

---

## Cleaned Up (Removed Previous Approach)

✅ Removed `pkg/models/content_schema.go`
✅ Removed `pkg/repository/gorm/schema_repo.go`
✅ Removed `services/vfs/handlers/schema.go`
✅ Removed `SchemaRepository` interface from repositories
✅ Removed `Schemas()` from UnitOfWork
✅ Removed `schema_id` and `enforce_schema` from Directory model
✅ Removed `pkg/domain/file_service.go` (placeholder)
✅ Updated `ContentValidator` to not depend on repository

---

## Implementation Steps

### Phase 1: Special File Detection & Authorization ⏳

**Files to Create:**
1. `pkg/domain/special_files.go` - Constants and helper functions

```go
package domain

const (
    SpecialFilePrefix   = "."
    SchemaFileName      = ".jsonschema"
    PolicyFileName      = ".rego"
)

// IsSpecialFile checks if filename is a special file
func IsSpecialFile(name string) bool {
    return strings.HasPrefix(name, SpecialFilePrefix)
}

// IsSuperUser checks if user has super-user role
func IsSuperUser(ctx context.Context) bool {
    user, ok := ctx.Value("user").(map[string]interface{})
    if !ok {
        return false
    }

    roles, ok := user["roles"].([]interface{})
    if !ok {
        return false
    }

    for _, role := range roles {
        if roleStr, ok := role.(string); ok {
            if roleStr == "super-user" || roleStr == "admin" {
                return true
            }
        }
    }
    return false
}
```

**Files to Modify:**
1. Update `pkg/services/file_service.go`:
   - Add check in `CreateFile()` to block special files for non-super-users
   - Add check in `UpdateFile()` to block special files for non-super-users
   - Add check in `DeleteFile()` to block special files for non-super-users

```go
// In CreateFile(), before other validations:
if domain.IsSpecialFile(name) && !domain.IsSuperUser(ctx) {
    return nil, fmt.Errorf("only super-users can create special files")
}
```

---

### Phase 2: Schema Loader with Caching ⏳

**Files to Create:**
1. `pkg/domain/schema_loader.go`

```go
package domain

import (
    "context"
    "path/filepath"
    "sync"
    "time"

    "github.com/telnet2/mysql-vfs/pkg/repository"
    "github.com/xeipuuv/gojsonschema"
)

type SchemaLoader struct {
    uow      repository.UnitOfWork
    cache    map[string]*cachedSchema
    cacheTTL time.Duration
    mu       sync.RWMutex
}

type cachedSchema struct {
    schema   *gojsonschema.Schema
    content  string
    loadedAt time.Time
}

func NewSchemaLoader(uow repository.UnitOfWork) *SchemaLoader {
    return &SchemaLoader{
        uow:      uow,
        cache:    make(map[string]*cachedSchema),
        cacheTTL: 5 * time.Minute,
    }
}

// LoadSchemaForDirectory loads .jsonschema with parent inheritance
func (s *SchemaLoader) LoadSchemaForDirectory(ctx context.Context, dirPath string) (string, error) {
    // Check cache
    if cached, ok := s.getFromCache(dirPath); ok {
        return cached.content, nil
    }

    // Try current directory
    content, err := s.loadSchemaFile(ctx, dirPath)
    if err == nil {
        s.cacheSchema(dirPath, content)
        return content, nil
    }

    // Try parent directories (inheritance)
    parentPath := filepath.Dir(dirPath)
    if parentPath != dirPath && parentPath != "." && parentPath != "/" {
        return s.LoadSchemaForDirectory(ctx, parentPath)
    }

    return "", ErrNotFound
}

func (s *SchemaLoader) loadSchemaFile(ctx context.Context, dirPath string) (string, error) {
    // Find directory
    dir, err := s.uow.Directories().FindByPath(ctx, dirPath)
    if err != nil {
        return "", err
    }

    // Find .jsonschema file
    file, err := s.uow.Files().FindByDirectoryAndName(ctx, dir.ID, SchemaFileName)
    if err != nil {
        return "", err
    }

    // Load file content based on storage type
    if file.StorageType == models.StorageTypeJSON && file.JSONContent != nil {
        return *file.JSONContent, nil
    }

    // TODO: Handle S3 storage
    return "", fmt.Errorf("schema stored in S3 not yet supported")
}

// InvalidateCache invalidates cache for directory
func (s *SchemaLoader) InvalidateCache(dirPath string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.cache, dirPath)
}
```

---

### Phase 3: Policy Loader (Similar to Schema Loader) ⏳

**Files to Create:**
1. `pkg/domain/policy_loader.go` - Similar structure to SchemaLoader

---

### Phase 4: Special File Validation ⏳

**Files to Modify:**
1. Update `pkg/services/file_service.go`:
   - Add `validateSpecialFileContent()` function
   - Call it when creating/updating special files

```go
func (s *FileService) validateSpecialFileContent(name string, content []byte) error {
    switch name {
    case domain.SchemaFileName:
        // Validate it's valid JSON Schema
        var schema map[string]interface{}
        if err := json.Unmarshal(content, &schema); err != nil {
            return fmt.Errorf("invalid JSON: %w", err)
        }

        // Try to compile schema
        loader := gojsonschema.NewStringLoader(string(content))
        if _, err := gojsonschema.NewSchema(loader); err != nil {
            return fmt.Errorf("invalid JSON schema: %w", err)
        }
        return nil

    case domain.PolicyFileName:
        // Validate it's valid Rego
        // TODO: Use OPA SDK to validate
        return nil

    default:
        // Other special files - just require super-user (already checked)
        return nil
    }
}
```

---

### Phase 5: Schema Validation Integration ⏳

**Files to Modify:**
1. Update `pkg/services/file_service.go`:
   - In `CreateFile()`, after authorization, check for `.jsonschema` and validate
   - In `UpdateFile()`, same validation logic

```go
// In CreateFile(), after finding directory:
// Check if directory has schema
schemaContent, err := s.schemaLoader.LoadSchemaForDirectory(ctx, directoryPath)
if err == nil {
    // Schema found - validate content
    if err := s.validator.ValidateAgainstSchema(schemaContent, string(contentBytes), contentType); err != nil {
        return nil, err
    }
}
```

---

### Phase 6: Cache Invalidation ⏳

**Files to Modify:**
1. Update `pkg/services/file_service.go`:
   - When `.jsonschema` is created/updated/deleted, invalidate schema cache
   - When `.rego` is created/updated/deleted, invalidate policy cache

```go
// In UpdateFile(), after success:
if file.Name == domain.SchemaFileName {
    s.schemaLoader.InvalidateCache(file.DirectoryPath)
}
if file.Name == domain.PolicyFileName {
    s.policyLoader.InvalidateCache(file.DirectoryPath)
}
```

---

## Testing Plan

### Unit Tests

```go
// Test special file detection
func TestIsSpecialFile(t *testing.T) {
    tests := []struct{
        name     string
        filename string
        want     bool
    }{
        {".jsonschema", ".jsonschema", true},
        {".rego", ".rego", true},
        {".anything", ".anything", true},
        {"normal.json", "normal.json", false},
    }
}

// Test super-user check
func TestIsSuperUser(t *testing.T) {
    // Test with admin role
    // Test with super-user role
    // Test with regular user
    // Test with no user in context
}

// Test schema loader with inheritance
func TestSchemaLoader_LoadWithInheritance(t *testing.T) {
    // Create schema in /data
    // Try to load from /data/users (should inherit)
    // Create schema in /data/users
    // Try to load from /data/users (should use specific, not parent)
}
```

### Integration Tests

```bash
# Test 1: Super-user creates schema
POST /api/v1/files (as super-user)
{
  "directory_path": "/data/users",
  "name": ".jsonschema",
  "content": "{...}"
}
→ 201 Created

# Test 2: Regular user tries to create schema
POST /api/v1/files (as regular user)
{
  "directory_path": "/data/users",
  "name": ".jsonschema",
  "content": "{...}"
}
→ 403 Forbidden

# Test 3: Upload file with validation
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "user.json",
  "content": "{\"email\":\"test@example.com\"}"
}
→ 201 (if valid) or 400 (if invalid)

# Test 4: Schema inheritance
# Create schema in /data
# Upload file to /data/users/
# Should validate against /data/.jsonschema
```

---

## API Examples

### Create Schema (Super-user)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {super-user-token}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data/users",
    "name": ".jsonschema",
    "content_type": "application/json",
    "content": "{\"type\":\"object\",\"required\":[\"email\",\"name\"]}"
  }'
```

### Upload File (Auto-validated)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data/users",
    "name": "john.json",
    "content_type": "application/json",
    "content": "{\"email\":\"john@example.com\",\"name\":\"John\"}"
  }'
```

### View Schema

```bash
curl http://localhost:8080/api/v1/files/data/users/.jsonschema
```

### Delete Schema (Disables Validation)

```bash
curl -X DELETE http://localhost:8080/api/v1/files/data/users/.jsonschema \
  -H "Authorization: Bearer {super-user-token}"
```

---

## Advantages Over Previous Approach

1. **Simplicity** - No new tables, no schema management endpoints
2. **Flexibility** - Schemas are versionable files, can be copied/moved
3. **Consistency** - Everything is a file
4. **Power User Friendly** - Can manage via CLI, version control
5. **Inheritance** - Schemas cascade from parent directories
6. **Less Code** - ~500 lines vs ~1500 lines for previous approach

---

## Migration Notes

- No database migration needed (we removed the schema table before it was created)
- No breaking changes to existing APIs
- Backward compatible (directories without `.jsonschema` work as before)

---

## Next Session Tasks

1. ✅ Complete Phase 1: Special file detection & authorization
2. ✅ Complete Phase 2: Schema loader with caching
3. ✅ Complete Phase 3: Policy loader
4. ⏳ Complete Phase 4: Special file validation
5. ⏳ Complete Phase 5: Schema validation integration
6. ⏳ Complete Phase 6: Cache invalidation
7. ⏳ Write tests
8. ⏳ Update documentation

---

**Status:** Design complete, ready for implementation
**Estimated Effort:** 1-2 days
**Breaking Changes:** 0

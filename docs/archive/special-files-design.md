# Special Files Design (.jsonschema and .rego)

**Feature:** File-based Schema and Policy Management
**Author:** Claude (Sonnet 4.5)
**Date:** 2025-10-03
**Status:** Design

---

## Overview

Instead of separate management endpoints, use special files within the VFS itself:
- `.jsonschema` - JSON Schema for validating directory content
- `.rego` - OPA policy for directory authorization

**Key Principles:**
1. Special files start with `.` (dot prefix)
2. Only super-users can create/modify special files
3. Regular users cannot create any `.xxx` files
4. Schemas and policies are discovered by reading these files from directories

---

## Use Cases

### 1. Directory with Schema Validation

```
/data/users/
├── .jsonschema          ← Defines validation schema
├── john.json            ← Must comply with .jsonschema
├── jane.json            ← Must comply with .jsonschema
└── invalid.json         ← Upload fails if doesn't match schema
```

**`.jsonschema` content:**
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["email", "name"],
  "properties": {
    "email": {"type": "string", "format": "email"},
    "name": {"type": "string", "minLength": 1},
    "age": {"type": "integer", "minimum": 0}
  }
}
```

### 2. Directory with Custom Authorization

```
/projects/secret/
├── .rego                ← Custom authorization policy
├── .jsonschema          ← Optional schema
└── data.json
```

**`.rego` content:**
```rego
package vfs.authz

# Only allow access to team members
allow {
    input.user.team == "security"
    input.action in ["read", "write"]
}

# Admins have full access
allow {
    input.user.role == "admin"
}
```

### 3. Nested Policies (Inheritance)

```
/data/
├── .rego                ← Base policy: admin-only
└── users/
    ├── .rego            ← Override: team-members can read
    ├── .jsonschema      ← User schema
    └── john.json
```

**Lookup order:**
1. Check `/data/users/.rego` (most specific)
2. If not found, check `/data/.rego` (parent)
3. If not found, use default policy

---

## Architecture

### File Discovery Flow

```
User uploads file to /data/users/john.json
    ↓
FileService.CreateFile()
    ↓
1. Check authorization:
   - Look for .rego in /data/users/
   - If not found, check /data/
   - If not found, use default policy
   ↓
2. Validate content:
   - Look for .jsonschema in /data/users/
   - If found, validate john.json against schema
   - If not found, skip validation
   ↓
3. Create file (if authorized and valid)
```

### Special File Rules

```go
// Special file naming rules
const (
    SpecialFileSchema = ".jsonschema"
    SpecialFilePolicy = ".rego"
    SpecialFilePrefix = "."
)

// Rules:
// 1. Only super-users can create files starting with "."
// 2. Special files are loaded and cached
// 3. Special files are version-controlled (like regular files)
// 4. Deleting special file disables feature for that directory
```

---

## API Design

### No New Endpoints Needed! ✅

Use existing file endpoints with special file handling:

#### Create Schema (Super-user only)

```http
POST /api/v1/files
Authorization: Bearer {super-user-token}

{
  "directory_path": "/data/users",
  "name": ".jsonschema",
  "content_type": "application/json",
  "content": "{\"type\":\"object\",\"required\":[\"email\"]}"
}

Response (201):
{
  "id": "file123",
  "name": ".jsonschema",
  "path": "/data/users/.jsonschema",
  "is_special": true
}
```

#### Upload File (Validated automatically)

```http
POST /api/v1/files

{
  "directory_path": "/data/users",
  "name": "john.json",
  "content_type": "application/json",
  "content": "{\"email\":\"john@example.com\",\"name\":\"John\"}"
}

# Automatically validated against /data/users/.jsonschema
# If valid: 201 Created
# If invalid: 400 Bad Request with validation errors
```

#### Regular User Tries to Create Special File (Rejected)

```http
POST /api/v1/files
Authorization: Bearer {regular-user-token}

{
  "directory_path": "/data/users",
  "name": ".custom",
  "content": "anything"
}

Response (403):
{
  "error": "only super-users can create special files (files starting with '.')"
}
```

---

## Implementation

### 1. Special File Detection

```go
// pkg/domain/file_service.go

const (
    SpecialFilePrefix   = "."
    SchemaFileName      = ".jsonschema"
    PolicyFileName      = ".rego"
)

// IsSpecialFile checks if a filename is a special file
func IsSpecialFile(name string) bool {
    return strings.HasPrefix(name, SpecialFilePrefix)
}

// IsSuperUser checks if user has super-user privileges
func IsSuperUser(ctx context.Context) bool {
    user := ctx.Value("user")
    if user == nil {
        return false
    }
    // Check if user has "super-user" role
    userObj := user.(map[string]interface{})
    roles := userObj["roles"].([]string)
    for _, role := range roles {
        if role == "super-user" || role == "admin" {
            return true
        }
    }
    return false
}
```

### 2. File Creation with Special File Check

```go
// pkg/domain/file_service.go

func (s *FileService) CreateFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
    // Check if special file
    if IsSpecialFile(req.Name) {
        if !IsSuperUser(ctx) {
            return nil, ErrPermissionDenied
        }
        // Validate special file content
        if err := s.validateSpecialFile(req.Name, req.Content); err != nil {
            return nil, err
        }
    }

    // Find directory
    dir, err := s.uow.Directories().FindByPath(ctx, req.DirectoryPath)
    if err != nil {
        return nil, err
    }

    // For non-special files, check schema validation
    if !IsSpecialFile(req.Name) {
        if err := s.validateAgainstDirectorySchema(ctx, dir, req); err != nil {
            return nil, err
        }
    }

    // Continue with file creation...
}
```

### 3. Schema Discovery and Caching

```go
// pkg/domain/schema_loader.go

type SchemaLoader struct {
    uow   repository.UnitOfWork
    cache map[string]*gojsonschema.Schema // Cache schemas by directory path
    mu    sync.RWMutex
}

// LoadSchemaForDirectory loads .jsonschema from directory (with parent lookup)
func (s *SchemaLoader) LoadSchemaForDirectory(ctx context.Context, dirPath string) (*gojsonschema.Schema, error) {
    // Check cache
    s.mu.RLock()
    if schema, ok := s.cache[dirPath]; ok {
        s.mu.RUnlock()
        return schema, nil
    }
    s.mu.RUnlock()

    // Try current directory
    schema, err := s.loadSchemaFile(ctx, dirPath)
    if err == nil {
        s.cacheSchema(dirPath, schema)
        return schema, nil
    }

    // Try parent directories (inheritance)
    parentPath := filepath.Dir(dirPath)
    if parentPath != dirPath && parentPath != "." {
        return s.LoadSchemaForDirectory(ctx, parentPath)
    }

    return nil, ErrSchemaNotFound
}

func (s *SchemaLoader) loadSchemaFile(ctx context.Context, dirPath string) (*gojsonschema.Schema, error) {
    // Find .jsonschema file in directory
    dir, err := s.uow.Directories().FindByPath(ctx, dirPath)
    if err != nil {
        return nil, err
    }

    file, err := s.uow.Files().FindByDirectoryAndName(ctx, dir.ID, SchemaFileName)
    if err != nil {
        return nil, err
    }

    // Load file content
    content, err := s.loadFileContent(ctx, file)
    if err != nil {
        return nil, err
    }

    // Parse schema
    schemaLoader := gojsonschema.NewStringLoader(content)
    return gojsonschema.NewSchema(schemaLoader)
}
```

### 4. Policy Discovery (Similar Pattern)

```go
// pkg/domain/policy_loader.go

type PolicyLoader struct {
    uow   repository.UnitOfWork
    cache map[string]string // Cache policies by directory path
    mu    sync.RWMutex
}

// LoadPolicyForDirectory loads .rego from directory (with parent lookup)
func (p *PolicyLoader) LoadPolicyForDirectory(ctx context.Context, dirPath string) (string, error) {
    // Similar to SchemaLoader
    // 1. Check cache
    // 2. Try current directory
    // 3. Try parent directories
    // 4. Return default policy if none found
}
```

### 5. Special File Validation

```go
// pkg/domain/file_service.go

func (s *FileService) validateSpecialFile(name string, content io.Reader) error {
    buf := new(bytes.Buffer)
    if _, err := io.Copy(buf, content); err != nil {
        return err
    }
    contentStr := buf.String()

    switch name {
    case SchemaFileName:
        // Validate it's valid JSON Schema
        var schema map[string]interface{}
        if err := json.Unmarshal([]byte(contentStr), &schema); err != nil {
            return fmt.Errorf("invalid JSON schema: %w", err)
        }
        // Try to compile it
        loader := gojsonschema.NewStringLoader(contentStr)
        if _, err := gojsonschema.NewSchema(loader); err != nil {
            return fmt.Errorf("invalid JSON schema: %w", err)
        }
        return nil

    case PolicyFileName:
        // Validate it's valid Rego
        // Use OPA SDK to compile and validate
        return s.validateRegoPolicy(contentStr)

    default:
        // Other special files - just require super-user
        return nil
    }
}
```

### 6. Cache Invalidation

```go
// pkg/domain/file_service.go

func (s *FileService) UpdateFile(ctx context.Context, req UpdateFileRequest) (*models.File, error) {
    file, err := s.uow.Files().FindByID(ctx, req.FileID)
    if err != nil {
        return nil, err
    }

    // If updating special file, invalidate cache
    if IsSpecialFile(file.Name) {
        if !IsSuperUser(ctx) {
            return nil, ErrPermissionDenied
        }

        // Invalidate caches
        if file.Name == SchemaFileName {
            s.schemaLoader.InvalidateCache(file.DirectoryID)
        }
        if file.Name == PolicyFileName {
            s.policyLoader.InvalidateCache(file.DirectoryID)
        }
    }

    // Continue with update...
}
```

---

## Migration from Previous Design

### Remove (No longer needed):
- ❌ `content_schemas` table
- ❌ `schema_id` field from directories
- ❌ `enforce_schema` field from directories
- ❌ Schema management endpoints
- ❌ SchemaRepository

### Keep (Still useful):
- ✅ ContentValidator domain service (reuse for .jsonschema)
- ✅ File handlers (add special file logic)
- ✅ Directory/File repositories

### Add (New components):
- ✅ SchemaLoader service
- ✅ PolicyLoader service
- ✅ Special file validation
- ✅ Super-user authorization check

---

## Benefits

### 1. Simplicity
- ✅ No separate schema management API
- ✅ Schemas are just files (version-controlled, backed up, etc.)
- ✅ Easy to understand: "create .jsonschema file to enable validation"

### 2. Flexibility
- ✅ Schemas inherit from parent directories
- ✅ Easy to copy schemas between directories (just copy the file)
- ✅ Can edit schemas with any text editor via file API

### 3. Consistency
- ✅ Everything is a file
- ✅ Same permissions model (super-user only)
- ✅ Same versioning (file versions)
- ✅ Same audit trail (file events)

### 4. Power User Friendly
- ✅ Can manage schemas via CLI
- ✅ Can version-control schemas in git (via sync)
- ✅ Can template schemas easily

---

## Security Model

### Authorization Levels

```
Super-User (admin):
  - Can create/update/delete .jsonschema files
  - Can create/update/delete .rego files
  - Can create/update/delete any .xxx files
  - Has full access to all directories

Regular User:
  - Cannot create any files starting with "."
  - Cannot update/delete special files
  - Can read special files (for debugging)
  - Subject to .rego policies
```

### Protection Mechanisms

1. **Creation Block**
   ```go
   if strings.HasPrefix(name, ".") && !IsSuperUser(ctx) {
       return ErrPermissionDenied
   }
   ```

2. **Update Block**
   ```go
   if IsSpecialFile(file.Name) && !IsSuperUser(ctx) {
       return ErrPermissionDenied
   }
   ```

3. **Delete Block**
   ```go
   if IsSpecialFile(file.Name) && !IsSuperUser(ctx) {
       return ErrPermissionDenied
   }
   ```

---

## Examples

### Example 1: User Data Directory

```bash
# Super-user creates schema
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": ".jsonschema",
  "content": "{\"type\":\"object\",\"required\":[\"email\",\"name\"]}"
}

# Regular user uploads valid file (succeeds)
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "john.json",
  "content": "{\"email\":\"john@example.com\",\"name\":\"John\"}"
}
# → 201 Created

# Regular user uploads invalid file (fails)
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "invalid.json",
  "content": "{\"name\":\"Invalid\"}"
}
# → 400 Bad Request: email is required
```

### Example 2: Project Directory with Custom Policy

```bash
# Super-user creates custom policy
POST /api/v1/files
{
  "directory_path": "/projects/secret",
  "name": ".rego",
  "content": "package vfs.authz\nallow { input.user.team == \"security\" }"
}

# Regular user (team=marketing) tries to access (fails)
GET /api/v1/files?path=/projects/secret
# → 403 Forbidden

# Security team member accesses (succeeds)
GET /api/v1/files?path=/projects/secret
# → 200 OK
```

### Example 3: Schema Inheritance

```bash
# Super-user creates base schema in /data
POST /api/v1/files
{
  "directory_path": "/data",
  "name": ".jsonschema",
  "content": "{\"type\":\"object\",\"required\":[\"id\"]}"
}

# Files in /data/users/ inherit schema (no .jsonschema there)
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "test.json",
  "content": "{\"name\":\"Test\"}"
}
# → 400 Bad Request: id is required (inherited from /data/.jsonschema)
```

---

## CLI Usage

```bash
# Using the VFS CLI to manage schemas

# Create schema
$ vfs-cli upload /data/users/.jsonschema schema.json

# View schema
$ vfs-cli cat /data/users/.jsonschema

# Update schema
$ vfs-cli upload /data/users/.jsonschema updated-schema.json

# Delete schema (disables validation)
$ vfs-cli rm /data/users/.jsonschema

# Copy schema to another directory
$ vfs-cli cp /data/users/.jsonschema /data/products/.jsonschema
```

---

## Performance Considerations

### Caching Strategy

```go
type SchemaLoader struct {
    cache     map[string]*cachedSchema
    cacheTTL  time.Duration
    mu        sync.RWMutex
}

type cachedSchema struct {
    schema    *gojsonschema.Schema
    loadedAt  time.Time
    dirPath   string
}

// Cache with TTL
func (s *SchemaLoader) getFromCache(dirPath string) (*gojsonschema.Schema, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    cached, ok := s.cache[dirPath]
    if !ok {
        return nil, false
    }

    // Check TTL
    if time.Since(cached.loadedAt) > s.cacheTTL {
        return nil, false
    }

    return cached.schema, true
}
```

### Cache Invalidation Triggers

1. When `.jsonschema` file is created/updated/deleted
2. When directory is deleted
3. Manual cache clear (admin endpoint)
4. TTL expiration (default: 5 minutes)

---

## Testing Strategy

### Unit Tests

```go
func TestFileService_CreateSpecialFile(t *testing.T) {
    tests := []struct{
        name        string
        filename    string
        isSuperUser bool
        wantErr     error
    }{
        {"super-user creates .jsonschema", ".jsonschema", true, nil},
        {"regular user creates .jsonschema", ".jsonschema", false, ErrPermissionDenied},
        {"super-user creates .custom", ".custom", true, nil},
        {"regular user creates regular file", "data.json", false, nil},
    }
}

func TestSchemaLoader_LoadWithInheritance(t *testing.T) {
    // Test schema inheritance from parent directories
}
```

---

## Documentation Updates Needed

1. **API Documentation**
   - Document special file behavior
   - Document super-user requirement
   - Document schema/policy file formats

2. **User Guide**
   - How to create schemas
   - How to test validation
   - Schema inheritance rules

3. **Admin Guide**
   - Super-user management
   - Schema best practices
   - Policy best practices

---

**Status:** Design Complete
**Next:** Implementation
**Advantages over previous design:**
- ✅ Simpler (no new tables/endpoints)
- ✅ More flexible (inheritance, versioning)
- ✅ More consistent (everything is a file)
- ✅ Better UX (edit files instead of API calls)

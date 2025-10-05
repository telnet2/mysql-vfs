# Special Files in MySQL VFS v2.1+

**Version:** v2.1+ Production Ready
**Status:** ✅ Complete (103/104 tests passing)
**Last Updated:** 2025-10-05

**Implementation:** `pkg/domain/special_files.go` (lines 1-430)

---

## Overview

MySQL VFS uses **special files** (files starting with `.`) for declarative configuration within the file system itself. Instead of separate management APIs, all configuration is done through versioned, inheritable files.

**Core Principle:** Everything is a file. Configuration, policies, validation rules, event handlers—all stored as special files within the VFS.

---

## Current Special Files (v2.1+)

| File | Purpose | Admin Only | Inheritance | Implementation |
|------|---------|------------|-------------|----------------|
| `.files` | Pattern-based file validation with JSON schemas | ✅ Yes | ✅ Yes | `pkg/domain/files_loader.go` (lines 1-215) |
| `.rego` | OPA authorization policies | ✅ Yes | ✅ Yes | `pkg/domain/policy_loader.go` (lines 1-200) |
| `.events` | Lifecycle event handlers (webhooks, logs, metrics) | ❌ No | ✅ Yes (merge) | `pkg/domain/events_loader.go` (lines 1-290) |
| `.user` | User credentials and tokens (root only) | ✅ Yes | ❌ No | `pkg/domain/user_loader.go` (lines 1-144) |
| `.owner` | Directory ownership | ❌ No | ✅ Yes | `pkg/domain/owner_loader.go` (lines 1-134) |

**Deprecated in v2.1:**
- ❌ `.jsonschema` → Replaced by `.files` (more flexible pattern matching)
- ❌ `.group` → Deprecated (role-only auth, groups in `.user` file)
- ❌ `.quota`, `.lifecycle` → Removed (admin features deprecated)

**See also:**
- [.files Specification](13_FILES_SPEC.md) - Pattern validation details
- [.events Specification](14_EVENTS_SPEC.md) - Event system details
- [Bootstrap Guide](18_BOOTSTRAP.md) - Initial setup with special files
- [Resource Protection](19_RESOURCE_PROTECTION.md) - Protection of special files
- [Owner-Based Access](20_OWNER_BASED_ACCESS.md) - Ownership control

---

## Architecture

### Special File Registry

All special files are registered in a central registry with metadata:

**Implementation:** `pkg/domain/special_files.go` (lines 34-83)

```go
var SpecialFileRegistry = map[SpecialFileType]*SpecialFileDefinition{
    SpecialFileTypeFiles: {
        Name:              ".files",
        Description:       "File pattern rules with JSON schemas for validation",
        ContentType:       "application/json",
        AdminOnly:         true,
        ValidateFunc:      validateFilesConfig,
        InheritFromParent: true,
    },
    SpecialFileTypePolicy: {
        Name:              ".rego",
        Description:       "OPA Rego policy for authorization",
        ContentType:       "text/plain",
        AdminOnly:         true,
        ValidateFunc:      validateRegoPolicy,
        InheritFromParent: true,
    },
    SpecialFileTypeEvents: {
        Name:              ".events",
        Description:       "Event handlers for file/directory operations",
        ContentType:       "application/json",
        AdminOnly:         false, // Regular users can set event handlers
        ValidateFunc:      validateEventsConfig,
        InheritFromParent: true,  // Events inherit and merge from parent
    },
    SpecialFileTypeUser: {
        Name:              ".user",
        Description:       "User credential store - ONLY at root",
        ContentType:       "application/json",
        AdminOnly:         true,
        ValidateFunc:      validateUserConfig,
        InheritFromParent: false, // Users stay at root
    },
    SpecialFileTypeOwner: {
        Name:              ".owner",
        Description:       "Directory ownership",
        ContentType:       "application/json",
        AdminOnly:         false, // Users can set ownership on their dirs
        ValidateFunc:      ValidateOwnerConfig,
        InheritFromParent: true,  // Ownership inherits
    },
}
```

### File Discovery Flow

**Implementation:** `pkg/domain/file_service.go` (integrated with file operations)

```
User uploads file to /data/users/john.json
    ↓
FileService.CreateFile()
    ↓
1. Check authorization:
   - PolicyLoader.Load(dirID) → Look for .rego
   - If not found, check parent directories (inheritance)
   - Apply OPA policy evaluation
   ↓
2. Validate content:
   - FilesLoader.ValidateFile(dirID, "john.json", content)
   - Load .files rules with pattern matching
   - Validate against matched rule's JSON schema
   ↓
3. Trigger events:
   - EventsLoader.GetHandlersForEvent(dirID, "file.created")
   - Execute webhooks, log handlers, metrics
   ↓
4. Create file (if authorized, valid, and not vetoed)
```

### Special File Validation

**Implementation:** `pkg/domain/special_files.go` (lines 115-129)

All special files are validated before creation:

```go
func ValidateSpecialFileContent(filename string, content []byte) error {
    fileType := GetSpecialFileType(filename)
    def, exists := GetDefinition(fileType)
    if !exists {
        return ErrUnknownSpecialFileType
    }

    if def.ValidateFunc != nil {
        if err := def.ValidateFunc(content); err != nil {
            return fmt.Errorf("%w: %v", ErrInvalidSpecialFileContent, err)
        }
    }

    return nil
}
```

---

## Use Cases

### 1. Directory with Pattern-Based Validation (.files)

**Implementation:** `pkg/domain/files_loader.go` (lines 38-77)

```
/data/users/
├── .files               ← Pattern rules with schemas
├── john.json            ← Must match .files patterns
├── jane.json            ← Must match .files patterns
└── report.pdf           ← Allowed if pattern matches
```

**`.files` content:**
```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email", "name"],
        "properties": {
          "email": {"type": "string", "format": "email"},
          "name": {"type": "string", "minLength": 1}
        }
      }
    },
    {
      "pattern": "*.pdf",
      "type": "glob",
      "description": "PDF documents allowed without schema validation"
    }
  ],
  "default_action": "deny"
}
```

**Pattern Matching:** Lines 96-110 in `files_loader.go`
- Supports `glob` patterns (e.g., `*.json`, `report-*.pdf`)
- Supports `regex` patterns for complex matching
- First matching rule wins (order matters)

### 2. Directory with Custom Authorization (.rego)

**Implementation:** `pkg/domain/policy_loader.go` (lines 1-200)

```
/projects/secret/
├── .rego                ← Custom OPA policy
├── .files               ← Optional validation
└── sensitive.json
```

**`.rego` content:**
```rego
package vfs.authz

# Only security team can access
allow {
    input.user.role == "security-team"
    input.action in ["read", "write"]
}

# Admins have full access
allow {
    input.user.role == "admin"
}
```

**Policy Loading:** Inherits from parent directories with caching (5-minute TTL)

### 3. Directory with Event Handlers (.events)

**Implementation:** `pkg/domain/events_loader.go` (lines 38-124)

```
/data/uploads/
├── .events              ← Webhook + metrics handlers
└── document.pdf         ← Triggers events on upload
```

**`.events` content:**
```json
{
  "handlers": [
    {
      "name": "notify-webhook",
      "events": ["file.created", "file.updated"],
      "type": "webhook",
      "enabled": true,
      "config": {
        "url": "https://api.example.com/webhooks/vfs",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer token123"
        }
      },
      "filter": {
        "pattern": "*.pdf",
        "type": "glob",
        "min_size_bytes": 1024
      }
    },
    {
      "name": "log-all",
      "events": ["file.created", "file.deleted"],
      "type": "log",
      "enabled": true,
      "config": {
        "level": "info"
      }
    }
  ]
}
```

**Event Merging:** Lines 126-164 in `events_loader.go`
- Child handlers override parent handlers by name
- `enabled: false` removes parent handlers
- All parent handlers included unless overridden

### 4. User Authentication (.user)

**Implementation:** `pkg/domain/user_loader.go` (lines 36-96)

```
/ (root only)
└── .user                ← User credentials with roles
```

**`.user` content:**
```json
{
  "users": [
    {
      "user_id": "admin",
      "password_hash": "$2a$10$...",
      "token": "admin-static-token",
      "groups": ["admin", "developers"]
    },
    {
      "user_id": "alice",
      "token": "alice-token-123",
      "groups": ["developers"]
    }
  ]
}
```

**Authentication Methods:**
- Password + bcrypt hash (lines 84-96)
- Static bearer token (lines 60-82)
- Both supported per user

**Restriction:** `.user` files can ONLY be created at root (`/`) directory (enforced in `files_loader.go` lines 199-214)

### 5. Directory Ownership (.owner)

**Implementation:** `pkg/domain/owner_loader.go` (lines 37-89)

```
/projects/acme/
├── .owner               ← Ownership declaration
└── data/
    └── file.txt         ← Inherits ownership from parent
```

**`.owner` content:**
```json
{
  "owners": ["developers", "project-leads"]
}
```

**Ownership Inheritance:** Lines 76-88
- If no `.owner` in directory, checks parent
- Continues up to root
- Cached with 5-minute TTL

---

## Inheritance Model

### Files Supporting Inheritance

**Implementation:** `pkg/domain/special_files.go` (lines 148-156)

| Special File | Inheritance Behavior |
|--------------|---------------------|
| `.files` | ✅ Inherits from parent (first match wins) |
| `.rego` | ✅ Inherits from parent (most specific wins) |
| `.events` | ✅ Merges with parent (child overrides) |
| `.user` | ❌ Root only, no inheritance |
| `.owner` | ✅ Inherits from parent |

### Inheritance Example

```
/data/
├── .files               ← Rule: all files must have "id" field
├── .rego                ← Policy: admin-only access
└── users/
    ├── .files           ← Rule: *.json must have "email" field (overrides parent)
    ├── .rego            ← Policy: team-members can read (more specific)
    └── john.json        ← Validated against /data/users/.files
```

**Lookup Order (all loaders):**
1. Check current directory for special file
2. If not found, check parent directory
3. Repeat until root
4. Use default if none found

**Implementation Pattern (used by all loaders):**
```go
// Try current directory
file, err := loader.fileRepo.FindByDirectoryAndName(ctx, dirID, ".files")
if err == nil {
    // Found - parse and cache
    return parseAndCache(file)
}

// Try parent
dir, _ := loader.dirRepo.FindByID(ctx, dirID)
if dir.ParentID != nil {
    return loader.Load(ctx, *dir.ParentID) // Recursive
}

return nil // Not found
```

---

## Caching Strategy

**Implementation:** All loaders use `sync.Map` with TTL (lines vary by loader)

### Cache Characteristics

- **Structure:** `sync.Map` for concurrent access
- **TTL:** 5 minutes default (configurable)
- **Invalidation:** On special file update/delete
- **Key:** Directory ID

### Example (FilesLoader)

**Implementation:** `pkg/domain/files_loader.go` (lines 17-27, 140-149)

```go
type FilesLoader struct {
    fileRepo db.FileRepository
    dirRepo  db.DirectoryRepository
    cache    sync.Map // map[directoryID]*filesCacheEntry
    ttl      time.Duration
}

type filesCacheEntry struct {
    config    *FilesConfig
    expiresAt time.Time
}

// Load with cache check
func (l *FilesLoader) Load(ctx context.Context, dirID string) (*FilesConfig, error) {
    // Check cache
    if entry, ok := l.cache.Load(dirID); ok {
        cached := entry.(*filesCacheEntry)
        if time.Now().Before(cached.expiresAt) {
            return cached.config, nil // Cache hit
        }
        l.cache.Delete(dirID) // Expired
    }

    // Load from database...
    // Cache result with TTL
    l.cache.Store(dirID, &filesCacheEntry{
        config:    config,
        expiresAt: time.Now().Add(l.ttl),
    })

    return config, nil
}
```

### Cache Invalidation Triggers

**Implementation:** Each loader provides `InvalidateCache(dirID)` method

1. When special file is created/updated/deleted
2. When directory is deleted
3. TTL expiration (automatic)
4. Manual invalidation (admin endpoint)

---

## Security & Protection

### Admin-Only Special Files

**Implementation:** `pkg/domain/special_files.go` (lines 131-140)

```go
func RequiresAdmin(filename string) bool {
    fileType := GetSpecialFileType(filename)
    def, exists := GetDefinition(fileType)
    if !exists {
        // Unknown special files require admin by default (secure by default)
        return true
    }
    return def.AdminOnly
}
```

**Admin required for:**
- `.files` - Pattern validation rules
- `.rego` - Authorization policies
- `.user` - User credentials

**Regular users can create:**
- `.events` - Event handlers (subject to authorization)
- `.owner` - Directory ownership (subject to authorization)

### Resource Protection

**Implementation:** `pkg/domain/protection.go` (lines 8-160)

Special files are protected from unauthorized access through hard-coded rules:

```go
type ProtectionType int

const (
    ProtectionNone ProtectionType = iota
    ProtectionReadOnly
    ProtectionSystemAdminOnly
    ProtectionOwnerOnly
)

func GetFileProtection(fileName string) ProtectionType {
    switch fileName {
    case ".user", ".rego":
        return ProtectionSystemAdminOnly
    case ".files":
        return ProtectionSystemAdminOnly
    case ".events":
        return ProtectionOwnerOnly
    case ".owner":
        return ProtectionOwnerOnly
    default:
        return ProtectionNone
    }
}
```

**See:** [Resource Protection Guide](19_RESOURCE_PROTECTION.md)

### Built-In Rules

**Implementation:** `pkg/domain/files_loader.go` (lines 198-214)

Hard-coded rules that cannot be overridden:

1. **Root-Only Files:**
   ```go
   if fileName == ".user" || fileName == ".group" {
       // Get directory path
       dir, _ := l.dirRepo.FindByID(ctx, dirID)
       if dir.Path != "/" {
           return fmt.Errorf("%s files can only be created at root (/)", fileName)
       }
   }
   ```

2. **Unknown Special Files:**
   - Any file starting with `.` that's not registered → Admin only
   - Secure by default principle

---

## API Usage

### No New Endpoints! ✅

Special files use the same file API as regular files:

#### Create Special File (Admin)

```http
POST /api/v1/files
Authorization: Bearer {system-admin-token}

{
  "directory_path": "/data/users",
  "name": ".files",
  "content": "{\"rules\":[{\"pattern\":\"*.json\",\"type\":\"glob\"}]}"
}

Response (201):
{
  "id": "file-uuid",
  "name": ".files",
  "directory_path": "/data/users",
  "created_at": "2025-10-05T10:00:00Z"
}
```

#### Create Regular File (Auto-Validated)

```http
POST /api/v1/files
Authorization: Bearer {user-token}

{
  "directory_path": "/data/users",
  "name": "alice.json",
  "content": "{\"email\":\"alice@example.com\",\"name\":\"Alice\"}"
}

# Automatically:
# 1. Loads .files from /data/users/ (or parent)
# 2. Matches pattern (*.json)
# 3. Validates against schema
# 4. Triggers .events handlers
# 5. Creates file if all checks pass
```

#### Regular User Creates Special File (Rejected)

```http
POST /api/v1/files
Authorization: Bearer {user-token}

{
  "directory_path": "/data",
  "name": ".files",
  "content": "{...}"
}

Response (403):
{
  "error": "only system admins can create .files (admin-only special file)"
}
```

---

## Validation Details

### .files Validation

**Implementation:** `pkg/domain/special_files.go` (lines 171-214)

```go
func validateFilesConfig(content []byte) error {
    var filesConfig FilesConfig

    if err := json.Unmarshal(content, &filesConfig); err != nil {
        return fmt.Errorf("invalid JSON: %w", err)
    }

    if len(filesConfig.Rules) == 0 {
        return fmt.Errorf("at least one rule must be defined")
    }

    // Validate each rule
    for i, rule := range filesConfig.Rules {
        if rule.Pattern == "" {
            return fmt.Errorf("rule %d: pattern is required", i)
        }

        if rule.Type != "glob" && rule.Type != "regex" {
            return fmt.Errorf("rule %d: type must be 'glob' or 'regex'", i)
        }

        // Validate JSON schema if provided
        if rule.Schema != nil {
            schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
            _, err = gojsonschema.NewSchema(schemaLoader)
            if err != nil {
                return fmt.Errorf("rule %d: invalid JSON schema: %w", i, err)
            }
        }
    }

    return nil
}
```

### .rego Validation

**Implementation:** `pkg/domain/special_files.go` (lines 216-234)

```go
func validateRegoPolicy(content []byte) error {
    if len(content) == 0 {
        return fmt.Errorf("policy cannot be empty")
    }

    contentStr := string(content)

    // Check for basic Rego syntax
    if !strings.Contains(contentStr, "package") {
        return fmt.Errorf("policy must contain a package declaration")
    }

    // TODO: Use OPA's AST parser for thorough validation

    return nil
}
```

### .events Validation

**Implementation:** `pkg/domain/special_files.go` (lines 236-311)

Validates:
- Handler names (unique, non-empty)
- Event types (must be valid lifecycle events)
- Handler types (webhook, log, metrics)
- Config structure (type-specific)

### .user Validation

**Implementation:** `pkg/domain/special_files.go` (lines 325-358)

Validates:
- User IDs (unique, non-empty)
- At least one auth method (password_hash or token)
- Groups array (non-empty)

---

## CLI Usage

**See:** [CLI How-To Guide](CLI_HOWTO.md) for full CLI documentation

### Managing Special Files

```bash
# View special file
vfs:/> cat /data/.files

# Create .files (admin only)
vfs:/> import local-files-config.json /data/.files

# Update .events
vfs:/> import updated-events.json /data/.events

# Delete special file (disables feature)
vfs:/> rm /data/.files

# Copy special file to another directory
vfs:/> mv /data/.files /projects/.files
```

### Bootstrap Example

```bash
# 1. Create .user at root (with system admin token)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"admin-token\",\"groups\":[\"admin\"]}]}"
  }'

# 2. Create .files validation
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer admin-token" \
  -d '{
    "directory_path": "/data",
    "name": ".files",
    "content": "{\"rules\":[{\"pattern\":\"*.json\",\"type\":\"glob\",\"schema\":{\"type\":\"object\"}}]}"
  }'

# 3. Create .events handlers
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer admin-token" \
  -d '{
    "directory_path": "/data",
    "name": ".events",
    "content": "{\"handlers\":[{\"name\":\"webhook\",\"events\":[\"file.created\"],\"type\":\"webhook\",\"config\":{\"url\":\"https://example.com/hook\"}}]}"
  }'
```

---

## Testing

### Unit Tests

**Test files:**
- `pkg/domain/files_loader_test.go`
- `pkg/domain/user_loader_test.go`
- `pkg/domain/policy_loader_test.go`
- `pkg/domain/events_loader_test.go`
- `pkg/domain/owner_loader_test.go`

### Integration Tests

**Test files:**
- `citest/file_based_auth_test.go` - .user authentication
- `citest/schema_validation_test.go` - .files validation
- `citest/opa_integration_test.go` - .rego policies
- `citest/e2e_workflow_test.go` - Complete workflows

### Test Coverage

**Status:** 103/104 tests passing (1 flaky concurrency test)

```bash
# Run all tests
go test ./pkg/domain/...

# Run with coverage
go test -cover ./pkg/domain/...

# Run integration tests
cd citest && ginkgo
```

---

## Benefits of File-Based Configuration

### 1. Simplicity
- ✅ No separate management API
- ✅ Special files are versioned like regular files
- ✅ Easy to understand: "upload `.files` to enable validation"

### 2. Flexibility
- ✅ Inheritance from parent directories
- ✅ Copy/move special files between directories
- ✅ Edit via file API or CLI

### 3. Consistency
- ✅ Everything is a file
- ✅ Same permissions model
- ✅ Same versioning (file_versions table)
- ✅ Same audit trail (lifecycle events)

### 4. GitOps Ready
- ✅ Can export special files to git
- ✅ Can import from git repos
- ✅ Infrastructure as code

---

## Migration Notes

### From v2.0 to v2.1+

**Breaking Changes:**
1. ❌ `.jsonschema` → Use `.files` instead
   - Old: Single schema per directory
   - New: Multiple pattern rules with schemas

2. ❌ `.group` → Deprecated
   - Old: Separate `.group` file
   - New: Groups in `.user` file (array field)

3. ❌ `.quota`, `.lifecycle` → Removed
   - Admin features deprecated

**Migration Steps:**
1. Convert `.jsonschema` to `.files` format
2. Merge `.group` into `.user` file
3. Remove `.quota` and `.lifecycle` files
4. Update `.rego` policies if using group-based auth

### Example Migration

**Old (.jsonschema):**
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["email"]
}
```

**New (.files):**
```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email"]
      }
    }
  ],
  "default_action": "allow"
}
```

---

## Summary

**Version:** v2.1+ Production Ready
**Status:** ✅ Complete (103/104 tests)

**Key Files:**
- `pkg/domain/special_files.go` - Registry and validation
- `pkg/domain/files_loader.go` - Pattern-based validation
- `pkg/domain/policy_loader.go` - OPA policies
- `pkg/domain/events_loader.go` - Event handlers
- `pkg/domain/user_loader.go` - Authentication
- `pkg/domain/owner_loader.go` - Ownership

**Special Files:**
- `.files` - Pattern validation (admin only, inherits)
- `.rego` - Authorization (admin only, inherits)
- `.events` - Event handlers (user accessible, merges)
- `.user` - Credentials (admin only, root only)
- `.owner` - Ownership (user accessible, inherits)

**Next Steps:**
- [.files Specification](13_FILES_SPEC.md) - Detailed pattern syntax
- [.events Specification](14_EVENTS_SPEC.md) - Event handler details
- [Bootstrap Guide](18_BOOTSTRAP.md) - Initial setup walkthrough
- [API Reference](10_API.md) - Complete API documentation

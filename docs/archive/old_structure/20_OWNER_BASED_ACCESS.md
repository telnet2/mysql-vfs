# Owner-Based Access Control

**Type:** Directory Ownership & Scoped Visibility
**Status:** Implemented

---

## Overview

Implement directory-level ownership using `.owner` special files to control visibility and access. Owners have full control over their directories.

## Core Concept

### `.owner` Special File

**Purpose:** Mark directory ownership to control who can access it

**Format:**
```json
{
  "owner": "alice"
}
```

**Properties:**
- **Inheritable**: Child directories inherit parent's owner unless overridden
- **Visibility Control**: Determines what directories appear in listings
- **Access Control**: Grants full access to owner

**Source**: `pkg/domain/special_files.go` (OwnerConfig struct)

---

## Implementation

### Owner Loader

**Location**: `pkg/domain/owner_loader.go`

```go
type OwnerLoader struct {
    fileRepo    db.FileRepository
    dirRepo     db.DirectoryRepository
    groupLoader *GroupLoader
    cache       sync.Map
    ttl         time.Duration
}

// Load loads .owner config for a directory (with inheritance)
func (l *OwnerLoader) Load(ctx context.Context, dirID string) (*OwnerConfig, error)

// LoadByPath loads .owner config for a directory by path
func (l *OwnerLoader) LoadByPath(ctx context.Context, dirPath string) (*OwnerConfig, error)

// IsOwner checks if a user owns a directory
func (l *OwnerLoader) IsOwner(ctx context.Context, dirID, userID string) (bool, error)
```

**Key Features:**
- **Caching**: TTL-based cache for performance (line 18)
- **Inheritance**: Walks up directory tree to find `.owner` (lines 38-89)
- **Efficient**: Only loads when needed

**Inheritance Logic** (lines 76-88):
1. Check `/users/alice/documents/.owner`
2. If not found, check `/users/alice/.owner`
3. If not found, check `/users/.owner`
4. If not found, check `/.owner`
5. If not found, return "no owner"

### Ownership Configuration

**Source**: `pkg/domain/special_files.go`

```go
type OwnerConfig struct {
    Owner string `json:"owner"` // Single owner user ID
}
```

---

## Hierarchy Example

```
/
├── .user (global users)
├── users/
│   ├── .owner {"owner": "root"}
│   ├── alice/
│   │   ├── .owner {"owner": "alice"}
│   │   ├── documents/
│   │   │   └── (inherits owner: alice)
│   │   └── projects/
│   │       └── (inherits owner: alice)
│   └── bob/
│       ├── .owner {"owner": "bob"}
│       └── data/
│           └── (inherits owner: bob)
```

---

## Access Rules

### 1. Ownership Check

**User alice accesses /users/alice/documents:**
```
1. Load .owner for /users/alice/documents → Not found
2. Walk up to /users/alice → Found: {"owner": "alice"}
3. Check alice == alice → ✅ Access granted
```

**User bob accesses /users/alice/documents:**
```
1. Load .owner for /users/alice/documents → Not found
2. Walk up to /users/alice → Found: {"owner": "alice"}
3. Check bob == alice → ❌ Access denied
```

**System admin accesses any directory:**
```
1. Check role == "system-admin" → ✅ Access granted (bypass)
```

**Implementation**: See `pkg/domain/owner_loader.go` lines 101-134

### 2. Access Control

| User | Directory | .owner | Can Access? |
|------|-----------|--------|-------------|
| alice | /users/alice | alice | ✅ Yes |
| alice | /users/bob | bob | ❌ No |
| bob | /users/bob | bob | ✅ Yes |
| bob | /users/alice | alice | ❌ No |
| system-admin | ANY | ANY | ✅ Yes (bypass) |

### 3. Parent Directory Access

**Problem:** Alice owns `/users/alice` but not `/users`

**Solution:** Users can see parent directories if they lead to owned directories, but with limited access:
- ✅ **List**: Can see parent to navigate to owned dirs
- ❌ **Create**: Cannot create siblings
- ❌ **Delete**: Cannot delete parent
- ✅ **Read metadata**: Can get parent info for navigation

---

## Integration with Authorization

The owner-based access integrates with the rego policy system.

**In rego policies** (`pkg/setup/setup.go` lines 28-48):

```rego
# Owners: Users can write if they own the directory
allow {
    input.user.role == "user"
    input.action == "write"
    is_owner
}

# Owners: Users can delete if they own the directory
allow {
    input.user.role == "user"
    input.action == "delete"
    is_owner
}

# Helper rule: Check if user is the owner
is_owner {
    input.resource.owner == input.user.id
}
```

**Resource context includes owner**:
- Owner loaded via `OwnerLoader.LoadByPath()`
- Added to authorization input as `input.resource.owner`
- Rego policies can use this for access control

**Implementation**: See `pkg/middleware/authorization.go` for how owner is added to rego input

---

## User Workflows

### Workflow 1: User Setup (Bootstrap)

1. **System Admin** creates user directory:
```bash
curl -X POST /api/v1/directories \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{"parent_path": "/users", "name": "alice"}'
```

2. **System Admin** sets ownership:
```bash
curl -X POST /api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/users/alice",
    "name": ".owner",
    "content": "{\"owner\": \"alice\"}"
  }'
```

3. **Alice** can now access only her directory:
```bash
curl -X GET /api/v1/directories/users/alice \
  -H "Authorization: Bearer $ALICE_TOKEN"

# Success: Alice can access her own directory
```

```bash
curl -X GET /api/v1/directories/users/bob \
  -H "Authorization: Bearer $ALICE_TOKEN"

# Denied: Alice cannot access Bob's directory
```

### Workflow 2: User Creates Subdirectory

```bash
# Alice creates a project directory
curl -X POST /api/v1/directories \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -d '{"parent_path": "/users/alice", "name": "projects"}'

# Ownership is automatically inherited from /users/alice
# No need to create .owner file - it inherits "alice" as owner
```

### Workflow 3: Transfer Ownership

```bash
# Alice wants to transfer ownership to Bob
# Get the .owner file ID
FILE_ID=$(curl -s /api/v1/files?directory_path=/users/alice&name=.owner \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq -r '.files[0].id')

# Update .owner file
curl -X PUT /api/v1/files/$FILE_ID \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -d '{
    "content": "{\"owner\": \"bob\"}"
  }'

# Now Bob owns the directory
```

---

## Security Considerations

### 1. Owner Cannot Be Bypassed

- Users CANNOT access directories they don't own
- Even if rego policies allow, ownership check comes first
- Only system-admin bypasses ownership

### 2. Owner File Protection

- `.owner` file can only be created/modified by:
  - Current directory owner
  - System admin
  - Checked in special file creation logic

**Implementation**: See `pkg/domain/file_service.go` for owner file validation

### 3. Ownership Transfer

- Only current owner or system-admin can change `.owner`
- Prevents ownership hijacking

### 4. Parent Directory Traversal

- Users can see parent paths leading to owned directories
- But cannot create/delete/modify parent directories
- Only read-only access for navigation

---

## Configuration

No new environment variables needed. Uses existing:

- `SYSTEM_ADMIN_TOKEN` - For bootstrap and administration
- `AUTH_PROVIDER=file` - Uses `.user` files
- `USER_CACHE_TTL_SECONDS` - Cache TTL for user data
- `OWNER_CACHE_TTL_SECONDS` - Cache TTL for owner data (default: 300)

**Source**: `pkg/config/config.go`

---

## Caching

**Implementation**: `pkg/domain/owner_loader.go` lines 18-25, 39-47

```go
type ownerCacheEntry struct {
    config    *OwnerConfig
    expiresAt time.Time
}

// Check cache (line 40)
if entry, ok := l.cache.Load(dirID); ok {
    cached := entry.(*ownerCacheEntry)
    if time.Now().Before(cached.expiresAt) {
        return cached.config, nil
    }
    l.cache.Delete(dirID)
}
```

**Cache Behavior:**
- TTL-based expiration (default: 5 minutes)
- Automatic invalidation on expiry
- Per-directory caching
- Thread-safe using `sync.Map`

**Configuration**: Set via `NewOwnerLoader(fileRepo, dirRepo, groupLoader, ttl)`

---

## Testing

### Unit Tests

**Location**: `pkg/domain/owner_loader_test.go`

Test coverage:
- `TestOwnerLoader_Load` - Basic loading
- `TestOwnerLoader_LoadByPath` - Path-based loading
- `TestOwnerLoader_IsOwner` - Ownership checking
- `TestOwnerLoader_Inheritance` - Parent directory inheritance
- `TestOwnerLoader_Caching` - Cache TTL behavior

### Integration Tests

**Location**: `citest/directory_access_test.go`

End-to-end scenarios:
- User accessing owned directory
- User blocked from other's directory
- System admin accessing all directories
- Inheritance behavior across directory tree

---

## API Examples

### Create .owner File

```bash
POST /api/v1/files
Authorization: Bearer $ALICE_TOKEN
Content-Type: application/json

{
  "directory_path": "/users/alice/projects",
  "name": ".owner",
  "content_type": "application/json",
  "content": "{\"owner\": \"alice\"}"
}
```

### Read .owner File

```bash
GET /api/v1/files?directory_path=/users/alice&name=.owner
Authorization: Bearer $ALICE_TOKEN
```

### Update .owner File

```bash
PUT /api/v1/files/{file_id}
Authorization: Bearer $ALICE_TOKEN
Content-Type: application/json

{
  "content": "{\"owner\": \"bob\"}"
}
```

### Delete .owner File (Remove Ownership)

```bash
DELETE /api/v1/files/{file_id}
Authorization: Bearer $ALICE_TOKEN
```

---

## Code References

### Core Implementation

- **Owner loader**: `pkg/domain/owner_loader.go`
  - OwnerLoader struct: lines 14-20
  - Load method with inheritance: lines 38-89
  - LoadByPath: lines 91-99
  - IsOwner: lines 101-134
  - Caching logic: lines 39-47, 68-72

- **Special file definitions**: `pkg/domain/special_files.go`
  - OwnerConfig struct
  - SpecialFileTypeOwner constant

- **Authorization integration**: `pkg/middleware/authorization.go`
  - Owner added to rego input context

### Testing

- **Unit tests**: `pkg/domain/owner_loader_test.go`
- **Integration tests**: `citest/directory_access_test.go`

### Related Components

- **Policy loader**: `pkg/domain/policy_loader.go` - Rego policies use owner info
- **User loader**: `pkg/domain/user_loader.go` - User authentication
- **Group loader**: `pkg/domain/group_loader.go` - Group membership (deprecated)

---

## Migration Guide

### For Existing Installations

1. **Add `.owner` files to existing directories**:

```bash
# Script to set ownership based on path
for dir in /users/*; do
  username=$(basename $dir)
  curl -X POST /api/v1/files \
    -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
    -d "{
      \"directory_path\": \"$dir\",
      \"name\": \".owner\",
      \"content\": \"{\\\"owner\\\": \\\"$username\\\"}\"
    }"
done
```

2. **Update rego policies** to use owner information:

```rego
# Before: Only admins can write
allow {
    input.user.role == "admin"
    input.action == "write"
}

# After: Admins and owners can write
allow {
    input.user.role == "admin"
    input.action == "write"
}

allow {
    input.user.id == input.resource.owner
    input.action == "write"
}
```

---

## Open Questions

1. **Should we support multiple owners?**
   - Current design: Single owner per directory
   - Alternative: `{"owners": ["alice", "bob"]}`
   - **Answer**: Use groups for multiple users, keep owner singular

2. **How to handle shared directories?**
   - Option A: No `.owner` file = accessible via rego policies
   - Option B: Special owner value like "public" or "shared"
   - **Answer**: Use Option A - no .owner means policy-based access

3. **Parent directory permissions?**
   - Current: Read-only for navigation
   - Alternative: Configurable via rego policies
   - **Answer**: Keep read-only, use rego for fine-grained control

---

## See Also

- [Authorization Guide](6_AUTHORIZATION.md) - Rego policy integration
- [Bootstrap Guide](18_BOOTSTRAP.md) - Setting up default files
- [Resource Protection](19_RESOURCE_PROTECTION.md) - Protection rules
- [Authentication](5_AUTHENTICATION.md) - User authentication
- [Special Files](4_SPECIAL_FILES.md) - All special file types

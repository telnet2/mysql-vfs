# MySQL VFS - Implementation Roadmap

**Last Updated:** 2025-10-03
**Current Branch:** claude-v1
**Status:** Design Phase Complete, Ready for Implementation

---

## Overview

This document provides the complete roadmap for implementing the special files feature with user/group management in the MySQL VFS project.

---

## Architecture Summary

### Special Files Approach

Instead of separate schema/policy management endpoints, we use **special files** stored directly in the VFS:

- **`.jsonschema`** - JSON Schema for validating file content in a directory
- **`.rego`** - OPA policy for directory authorization
- **`.xxx`** - Any file starting with `.` is considered "special"

**Key Rules:**
1. Only **super-users (admins)** can create/modify/delete special files
2. Regular users **cannot** create any files starting with `.`
3. Special files **inherit** from parent directories (can be overridden)
4. Schemas/policies are **automatically applied** to files in their directory

---

## Complete Feature Set

### 1. Special Files (`.jsonschema` and `.rego`)

**Documents:**
- Design: `docs/special-files-design.md`
- Implementation Plan: `docs/special-files-implementation-plan.md`
- Inheritance Spec: `docs/special-files-inheritance.md`

**Features:**
- ✅ File-based schema validation
- ✅ File-based authorization policies
- ✅ Parent-to-child inheritance
- ✅ Override capability
- ✅ Caching with TTL and invalidation
- ✅ No new database tables needed

**Example:**
```
/data/users/
├── .jsonschema          ← Validates all JSON files in this directory
├── john.json            ← Automatically validated
└── admins/              (no .jsonschema)
    └── root.json        ← Inherits validation from parent
```

---

### 2. User & Group Management

**Document:** `docs/user-group-management.md`

**Features:**
- ✅ User authentication (JWT tokens)
- ✅ Role-based access control (admin/user/readonly)
- ✅ Group memberships
- ✅ Password hashing (bcrypt)
- ✅ Admin bootstrap on first run

**Database Tables:**
- `users` - User accounts
- `groups` - User groups
- `user_groups` - Many-to-many relationship

**Roles:**
- **admin** - Super-user, can create `.xxx` files
- **user** - Regular user
- **readonly** - Read-only access

**API Endpoints:**
```
POST   /api/v1/auth/login              - Login
GET    /api/v1/auth/me                 - Get current user
POST   /api/v1/auth/change-password    - Change password

POST   /api/v1/admin/users             - Create user (admin only)
GET    /api/v1/admin/users             - List users (admin only)
PATCH  /api/v1/admin/users/{id}        - Update user (admin only)
DELETE /api/v1/admin/users/{id}        - Delete user (admin only)

POST   /api/v1/admin/groups            - Create group (admin only)
GET    /api/v1/admin/groups            - List groups (admin only)
```

---

### 3. Inheritance Behavior

**Document:** `docs/special-files-inheritance.md`

**How It Works:**

```
/
├── .jsonschema              ← Root schema (applies to everything)
└── data/
    ├── public/              (no .jsonschema - inherits from /)
    │   └── stats.json       ← Validates against /.jsonschema
    └── users/
        ├── .jsonschema      ← Override (more specific)
        └── john.json        ← Validates against /data/users/.jsonschema
```

**Inheritance Algorithm:**
1. Check current directory for `.xxx` file
2. If not found, check parent directory
3. Continue up to root `/`
4. If found anywhere, use it
5. If not found anywhere, no validation/policy applies

**Cache Strategy:**
- Cache effective schema/policy for each directory
- Invalidate on create/update/delete of special files
- Invalidate all children when parent special file changes
- TTL: 5 minutes (configurable)

---

## Implementation Phases

### Phase 1: User & Group Management (Foundation)

**Estimated Time:** 2-3 days

**Tasks:**
1. Create User/Group/UserGroup models
2. Create repositories for users and groups
3. Implement JWT authentication
4. Create auth middleware
5. Create user/group CRUD endpoints (admin only)
6. Bootstrap initial admin user
7. Write unit tests

**Files to Create:**
- `pkg/models/user.go`
- `pkg/models/group.go`
- `pkg/repository/gorm/user_repo.go`
- `pkg/repository/gorm/group_repo.go`
- `pkg/auth/token.go`
- `pkg/middleware/auth.go`
- `services/vfs/handlers/auth.go`
- `services/vfs/handlers/admin.go`
- `pkg/bootstrap/admin.go`

**Environment Variables:**
```bash
JWT_SECRET=your-secret-key
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-this
ADMIN_EMAIL=admin@example.com
```

---

### Phase 2: Special File Detection & Authorization

**Estimated Time:** 1 day

**Tasks:**
1. Create special file helper functions
2. Update FileService to block special files for non-admins
3. Add special file validation
4. Add audit logging for special file operations
5. Write unit tests

**Files to Create:**
- `pkg/domain/special_files.go`

**Files to Modify:**
- `pkg/services/file_service.go` (add special file checks)

**Code Changes:**
```go
// In CreateFile()
if domain.IsSpecialFile(name) && !domain.IsSuperUser(ctx) {
    return nil, ErrPermissionDenied
}

// Validate special file content
if domain.IsSpecialFile(name) {
    if err := s.validateSpecialFileContent(name, content); err != nil {
        return nil, err
    }
}
```

---

### Phase 3: Schema Loader with Caching

**Estimated Time:** 2 days

**Tasks:**
1. Create SchemaLoader service
2. Implement parent directory lookup (inheritance)
3. Add caching with TTL
4. Add cache invalidation
5. Integrate with ContentValidator
6. Write unit tests

**Files to Create:**
- `pkg/domain/schema_loader.go`

**Files to Modify:**
- `pkg/domain/content_validator.go` (use SchemaLoader)
- `pkg/services/file_service.go` (integrate schema validation)

**Key Methods:**
```go
LoadSchemaForDirectory(ctx, dirPath) (string, error)
InvalidateCache(dirPath)
getFromCache(dirPath) (*cachedSchema, bool)
cacheSchema(dirPath, content string)
```

---

### Phase 4: Policy Loader (Similar to Schema Loader)

**Estimated Time:** 1-2 days

**Tasks:**
1. Create PolicyLoader service (similar structure to SchemaLoader)
2. Implement parent directory lookup
3. Add caching with TTL
4. Add cache invalidation
5. Integrate with OPA authorization
6. Write unit tests

**Files to Create:**
- `pkg/domain/policy_loader.go`

**Files to Modify:**
- `pkg/middleware/authorization.go` (use PolicyLoader)

---

### Phase 5: Schema Validation Integration

**Estimated Time:** 1 day

**Tasks:**
1. Update FileService to load schemas before validation
2. Validate file content if schema exists
3. Handle validation errors with detailed messages
4. Add cache invalidation on schema updates
5. Write integration tests

**Files to Modify:**
- `pkg/services/file_service.go`

**Flow:**
```go
// In CreateFile()
schemaContent, err := schemaLoader.LoadSchemaForDirectory(ctx, dirPath)
if err == nil {
    // Schema found - validate
    if err := validator.ValidateAgainstSchema(schemaContent, content, contentType); err != nil {
        return nil, err  // Return validation errors
    }
}
// No schema found - skip validation
```

---

### Phase 6: Testing & Documentation

**Estimated Time:** 2 days

**Tasks:**
1. Write comprehensive unit tests
2. Write integration tests
3. Write E2E tests
4. Update API documentation
5. Create user guide
6. Create admin guide
7. Create CLI examples

**Test Coverage:**
- Special file authorization
- Schema/policy inheritance
- Cache invalidation
- User/group management
- Authentication/authorization
- Validation errors

---

## Database Migrations

### New Tables

```sql
-- Users table
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(255),
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL
);

-- Groups table
CREATE TABLE groups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL
);

-- User-Group mapping
CREATE TABLE user_groups (
    user_id VARCHAR(36) NOT NULL,
    group_id VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, group_id)
);
```

### Migration Script

```go
// pkg/db/migrate.go
func AutoMigrate(db *gorm.DB) error {
    modelsToMigrate := []interface{}{
        &models.User{},        // NEW
        &models.Group{},       // NEW
        &models.UserGroup{},   // NEW
        &models.OPAPolicy{},
        &models.Directory{},
        &models.File{},
        // ... rest of models
    }
    return db.AutoMigrate(modelsToMigrate...)
}
```

---

## API Examples

### 1. Bootstrap & Login

```bash
# System starts, creates initial admin
docker-compose up

# Admin logs in
curl -X POST http://localhost:8080/api/v1/auth/login \
  -d '{"username":"admin","password":"admin"}'

# Response:
{
  "token": "eyJhbGci...",
  "user": {"id": "...", "username": "admin", "role": "admin"}
}
```

### 2. Create User

```bash
curl -X POST http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer {admin-token}" \
  -d '{
    "username": "john",
    "email": "john@example.com",
    "password": "secure-password",
    "role": "user"
  }'
```

### 3. Create Schema (Admin Only)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {admin-token}" \
  -d '{
    "directory_path": "/data/users",
    "name": ".jsonschema",
    "content_type": "application/json",
    "content": "{\"type\":\"object\",\"required\":[\"email\"]}"
  }'
```

### 4. Upload File (Auto-Validated)

```bash
# Regular user uploads file
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {user-token}" \
  -d '{
    "directory_path": "/data/users",
    "name": "profile.json",
    "content_type": "application/json",
    "content": "{\"email\":\"john@example.com\",\"name\":\"John\"}"
  }'

# Automatically validated against /data/users/.jsonschema
# Returns 201 if valid, 400 with errors if invalid
```

### 5. Regular User Tries Special File (Rejected)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {user-token}" \
  -d '{
    "directory_path": "/data",
    "name": ".custom",
    "content": "anything"
  }'

# Response: 403 Forbidden
{
  "error": "only administrators can create special files"
}
```

---

## Configuration

### Environment Variables

```bash
# Database
MYSQL_DSN=user:password@tcp(localhost:3306)/vfs?parseTime=true

# JWT
JWT_SECRET=your-super-secret-key-change-this

# Bootstrap Admin
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-this-secure-password
ADMIN_EMAIL=admin@example.com

# Cache
SCHEMA_CACHE_TTL=5m
POLICY_CACHE_TTL=5m
```

### docker-compose.yml

```yaml
services:
  vfs-service:
    environment:
      - MYSQL_DSN=root:password@tcp(mysql:3306)/vfs?parseTime=true
      - JWT_SECRET=${JWT_SECRET:-change-this-secret}
      - ADMIN_USERNAME=${ADMIN_USERNAME:-admin}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin}
      - ADMIN_EMAIL=${ADMIN_EMAIL:-admin@example.com}
      - SCHEMA_CACHE_TTL=5m
```

---

## Testing Strategy

### Unit Tests

```go
// Special file detection
func TestIsSpecialFile(t *testing.T) { ... }

// Super-user check
func TestIsSuperUser(t *testing.T) { ... }

// Schema inheritance
func TestSchemaLoader_Inheritance(t *testing.T) { ... }

// Cache invalidation
func TestSchemaLoader_CacheInvalidation(t *testing.T) { ... }

// User authentication
func TestAuthService_Login(t *testing.T) { ... }
```

### Integration Tests

```bash
# User management flow
1. Create admin
2. Login as admin
3. Create regular user
4. Login as user
5. Try to create special file (should fail)

# Schema validation flow
1. Admin creates schema
2. User uploads valid file (succeeds)
3. User uploads invalid file (fails with errors)

# Inheritance flow
1. Admin creates schema in /data
2. User uploads to /data/users (inherits schema)
3. Admin creates schema in /data/users (overrides)
4. User uploads to /data/users (validates against override)
```

---

## Success Criteria

### Phase 1: User Management
- [ ] Admin can be bootstrapped
- [ ] Admin can create users
- [ ] Admin can create groups
- [ ] Users can login and get JWT token
- [ ] Users can access protected endpoints with token

### Phase 2: Special Files
- [ ] Admin can create `.jsonschema` files
- [ ] Admin can create `.rego` files
- [ ] Regular users CANNOT create `.xxx` files
- [ ] Invalid schemas are rejected

### Phase 3: Schema Validation
- [ ] Files validate against `.jsonschema` in same directory
- [ ] Files inherit schema from parent if not in current directory
- [ ] Invalid files are rejected with detailed errors
- [ ] Directories without schema allow any content

### Phase 4: Caching
- [ ] Schemas are cached (avoid repeated DB lookups)
- [ ] Cache is invalidated on schema updates
- [ ] Cache TTL works correctly

### Phase 5: Overall
- [ ] Zero breaking changes to existing APIs
- [ ] All tests pass
- [ ] Documentation is complete
- [ ] Performance is acceptable (< 100ms overhead)

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance overhead from inheritance lookup | Medium | Aggressive caching with 5min TTL |
| Cache invalidation complexity | Medium | Clear invalidation rules, comprehensive tests |
| Security: weak admin password | High | Force password change on first login |
| Breaking existing code | High | Maintain backward compatibility, comprehensive tests |

---

## Timeline

```
Week 1:
  Day 1-3: Phase 1 (User/Group Management)
  Day 4-5: Phase 2 (Special File Detection)

Week 2:
  Day 1-2: Phase 3 (Schema Loader)
  Day 3-4: Phase 4 (Policy Loader)
  Day 5: Phase 5 (Integration)

Week 3:
  Day 1-2: Phase 6 (Testing)
  Day 3-5: Documentation & Polish
```

**Total Estimated Time:** 2-3 weeks

---

## Next Steps

1. ✅ Review roadmap with team
2. ⏳ Set up development branch
3. ⏳ Start Phase 1: User & Group Management
4. ⏳ Daily standups to track progress
5. ⏳ Code reviews after each phase

---

## Related Documents

- [Special Files Design](./special-files-design.md)
- [Special Files Implementation Plan](./special-files-implementation-plan.md)
- [Special Files Inheritance](./special-files-inheritance.md)
- [User & Group Management](./user-group-management.md)
- [Layered Architecture Progress](./layered-architecture-progress.md)

---

**Status:** Ready for Implementation
**Assigned:** Development Team
**Priority:** High
**Breaking Changes:** 0

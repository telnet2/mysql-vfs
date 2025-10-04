# MySQL VFS v2 - Implementation Progress

**Version:** v2.0
**Last Updated:** 2025-10-03
**Branch:** claude-v1
**Status:** Foundation Complete ✅ | Special Files Designed ✅

---

## Vision

MySQL VFS v2 introduces a **file-based approach to schema validation and authorization**. Instead of managing schemas and policies through separate APIs, everything is stored as special files (`.jsonschema`, `.rego`) directly within the VFS, creating a unified, elegant system where **everything is a file**.

---

## What's New in v2

### 1. Special Files System

**Core Concept:** Files starting with `.` are special and control directory behavior.

- **`.jsonschema`** → Validates JSON files in the directory
- **`.rego`** → Defines authorization policy for the directory
- **Only admins** can create/modify special files
- **Inherits from parents** (child directories can override)

**Example:**
```
/data/
├── .jsonschema              ← All files in /data validate against this
├── .rego                    ← All files in /data authorize against this
└── users/
    ├── .jsonschema          ← Override: stricter validation
    ├── alice.json           ← Validates with /data/users/.jsonschema
    └── admins/              (no special files)
        └── root.json        ← Inherits /data/users/.jsonschema
```

### 2. User & Group Management

- JWT-based authentication
- Role-based access (admin/user/readonly)
- Group memberships for team access
- Bootstrap admin on first run

### 3. Clean Layered Architecture

- **Middleware** → Request ID, Auth, Validation, Authorization, Logging
- **Handlers** → HTTP request/response, error mapping
- **Domain** → Pure business logic (DirectoryService, SchemaLoader, PolicyLoader)
- **Repository** → Data access with Unit of Work pattern
- **Infrastructure** → GORM, models, JWT, bcrypt

---

## Implementation Status

### ✅ Completed (Foundation)

#### Phase 1: Layered Architecture Foundation
**Commit:** `a97c0c4`
**Date:** 2025-10-03

**Delivered:**
- Middleware layer (chain, validation, auth, observability)
- Repository layer (interfaces, GORM, Unit of Work)
- Domain layer (DirectoryService)
- Clean separation of concerns

**Metrics:**
- 14 files created
- ~1,500 lines of code
- 0 breaking changes

#### Phase 2: Validation & Handlers
**Commit:** `9bf2309`
**Date:** 2025-10-03

**Delivered:**
- JSON schemas for request validation
- Directory HTTP handlers
- Error mapping utilities
- Request/response DTOs

**Metrics:**
- 7 files created
- ~600 lines of code
- 3 JSON schemas

#### Design Phase: Special Files System
**Date:** 2025-10-03

**Delivered:**
- Complete architecture design
- Inheritance specification
- User/group management design
- Implementation roadmap

**Documents:**
- `special-files-design.md` - Architecture
- `special-files-inheritance.md` - Inheritance rules
- `user-group-management.md` - Auth design
- `IMPLEMENTATION_ROADMAP.md` - Complete plan

---

### 📋 Next: Implementation (2-3 Weeks)

#### Week 1: User & Group Management

**Goal:** Enable admin/user authentication and authorization

**Tasks:**
1. Create User/Group/UserGroup models
2. Create repositories (UserRepository, GroupRepository)
3. Implement JWT token generation/validation
4. Create authentication middleware
5. Create login/logout endpoints
6. Create user/group CRUD endpoints (admin only)
7. Bootstrap initial admin user
8. Write tests

**Files to Create:**
```
pkg/models/user.go
pkg/models/group.go
pkg/repository/gorm/user_repo.go
pkg/repository/gorm/group_repo.go
pkg/auth/token.go
pkg/middleware/auth.go
pkg/bootstrap/admin.go
services/vfs/handlers/auth.go
services/vfs/handlers/admin.go
```

**Database Migration:**
```sql
CREATE TABLE users (...)
CREATE TABLE groups (...)
CREATE TABLE user_groups (...)
```

**Success Criteria:**
- [ ] Admin can login and get JWT token
- [ ] Admin can create users
- [ ] Admin can create groups
- [ ] Users can login with credentials
- [ ] JWT middleware blocks unauthenticated requests

---

#### Week 2: Special Files Core

**Goal:** Enable special file detection, validation, and schema loading

**Tasks:**

**Day 1-2: Special File Detection**
1. Create special file helper functions
2. Update FileService to detect `.xxx` files
3. Block special file creation for non-admins
4. Validate special file content (JSON Schema, Rego)
5. Add audit logging
6. Write tests

**Files to Create:**
```
pkg/domain/special_files.go
```

**Files to Modify:**
```
pkg/services/file_service.go (add special file checks)
pkg/domain/errors.go (add ErrPermissionDenied)
```

**Success Criteria:**
- [ ] Admin can create `.jsonschema` files
- [ ] Regular users CANNOT create `.xxx` files
- [ ] Invalid schemas are rejected
- [ ] Audit log records special file operations

**Day 3-4: Schema Loader**
1. Create SchemaLoader with caching
2. Implement parent directory lookup (inheritance)
3. Add cache with TTL (5 minutes)
4. Add cache invalidation on schema updates
5. Integrate with ContentValidator
6. Write tests

**Files to Create:**
```
pkg/domain/schema_loader.go
```

**Files to Modify:**
```
pkg/domain/content_validator.go (use SchemaLoader)
pkg/services/file_service.go (integrate validation)
```

**Success Criteria:**
- [ ] Schemas load from `.jsonschema` files
- [ ] Inheritance works (child inherits from parent)
- [ ] Cache reduces DB lookups
- [ ] Cache invalidates on schema update

**Day 5: Policy Loader**
1. Create PolicyLoader (similar to SchemaLoader)
2. Implement inheritance
3. Add caching
4. Integrate with OPA middleware
5. Write tests

**Files to Create:**
```
pkg/domain/policy_loader.go
```

**Files to Modify:**
```
pkg/middleware/authorization.go (use PolicyLoader)
```

**Success Criteria:**
- [ ] Policies load from `.rego` files
- [ ] Inheritance works
- [ ] OPA uses loaded policies

---

#### Week 3: Integration & Testing

**Goal:** Integrate everything and ensure quality

**Tasks:**

**Day 1-2: Integration**
1. Wire auth middleware into routes
2. Integrate schema validation into file uploads
3. Integrate policy loader into authorization
4. Update main.go with all components
5. End-to-end testing

**Success Criteria:**
- [ ] Admin creates schema → files validate automatically
- [ ] Regular user uploads file → validates against schema
- [ ] Invalid file rejected with detailed errors
- [ ] Schema inheritance works in real scenarios

**Day 3-4: Testing**
1. Unit tests for all new components
2. Integration tests for workflows
3. E2E tests for special file scenarios
4. Performance tests (caching effectiveness)

**Test Scenarios:**
```
✓ Admin creates schema
✓ User uploads valid file (succeeds)
✓ User uploads invalid file (fails with errors)
✓ Schema inheritance (no schema in dir, inherits from parent)
✓ Schema override (child overrides parent)
✓ Regular user tries to create .xxx (rejected)
✓ Cache invalidation on schema update
```

**Day 5: Documentation**
1. Update API documentation
2. Create user guide (how to use schemas)
3. Create admin guide (how to manage users/schemas)
4. Update CLI examples

---

## Architecture

### System Overview

```
┌─────────────────────────────────────────┐
│   Client (HTTP/CLI)                      │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   HTTP Layer (Hertz)                     │
│   - Routes                               │
│   - Request parsing                      │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Middleware Chain                       │
│   ✅ Request ID                          │
│   ✅ Observability (logging)             │
│   ✅ Request Validation (JSON Schema)    │
│   📋 Authentication (JWT)         NEW    │
│   📋 Authorization (OPA + .rego)  NEW    │
│   ✅ Recovery, CORS, Timeout             │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Handler Layer                          │
│   ✅ Directory handlers                  │
│   📋 Auth handlers               NEW     │
│   📋 Admin handlers              NEW     │
│   ⏳ File handlers (with validation)     │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Domain Layer                           │
│   ✅ DirectoryService                    │
│   ✅ ContentValidator                    │
│   📋 SchemaLoader (caching)      NEW     │
│   📋 PolicyLoader (caching)      NEW     │
│   📋 Special file helpers        NEW     │
│   ⏳ FileService                         │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Repository Layer                       │
│   ✅ DirectoryRepository                 │
│   ✅ FileRepository                      │
│   ✅ EventRepository                     │
│   📋 UserRepository              NEW     │
│   📋 GroupRepository             NEW     │
│   ✅ Unit of Work                        │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Infrastructure                         │
│   ✅ GORM + Models                       │
│   📋 User/Group Models           NEW     │
│   📋 JWT utilities               NEW     │
│   📋 Password hashing (bcrypt)   NEW     │
└─────────────────────────────────────────┘
```

### Data Flow: File Upload with Validation

```
1. User uploads file to /data/users/alice.json
   ↓
2. Auth Middleware
   - Validates JWT token
   - Extracts user info (role, groups)
   ↓
3. Authorization Middleware
   - Load .rego from /data/users/ (or parent)
   - Check if user can write to /data/users/
   ↓
4. File Handler
   - Parse request
   - Call FileService.CreateFile()
   ↓
5. FileService (Domain)
   - Check if special file (.jsonschema)
     → If yes: Block non-admin
     → If yes: Validate schema content
   - Load schema for /data/users/
     → SchemaLoader checks /data/users/.jsonschema
     → If not found, check /data/.jsonschema
     → Cache result
   - Validate alice.json against schema
     → If valid: Continue
     → If invalid: Return 400 with errors
   ↓
6. File Repository
   - Save file to database
   - Save file version
   ↓
7. Response
   - 201 Created (if valid)
   - 400 Bad Request (if validation failed)
   - 403 Forbidden (if not authorized)
```

---

## File Structure

### Current Structure

```
pkg/
├── middleware/          ✅ Complete
│   ├── middleware.go    - Chain pattern
│   ├── validation.go    - JSON schema validation
│   ├── authorization.go - OPA integration
│   └── observability.go - Logging, metrics
│
├── repository/          ✅ Complete (will add User/Group)
│   ├── interfaces.go
│   ├── errors.go
│   └── gorm/
│       ├── transaction.go
│       ├── unit_of_work.go
│       ├── directory_repo.go
│       ├── file_repo.go
│       └── event_repo.go
│
├── domain/              ✅ Partial
│   ├── errors.go
│   ├── directory_service.go
│   └── content_validator.go
│
└── models/              ✅ Complete
    ├── directory.go
    ├── file.go
    └── opa_policy.go

services/vfs/
├── handlers/            ✅ Partial
│   ├── errors.go
│   └── directory.go
└── main.go              ⏳ TODO (wiring)
```

### After v2 Implementation

```
pkg/
├── auth/                📋 NEW
│   └── token.go         - JWT generation/validation
│
├── bootstrap/           📋 NEW
│   └── admin.go         - Initial admin creation
│
├── middleware/          ✅ + NEW
│   ├── middleware.go
│   ├── validation.go
│   ├── authorization.go
│   ├── auth.go          📋 NEW - JWT middleware
│   └── observability.go
│
├── repository/          ✅ + NEW
│   ├── interfaces.go    (updated with User/Group)
│   └── gorm/
│       ├── user_repo.go      📋 NEW
│       ├── group_repo.go     📋 NEW
│       └── ...
│
├── domain/              ✅ + NEW
│   ├── special_files.go      📋 NEW - Helper functions
│   ├── schema_loader.go      📋 NEW - Schema loading + cache
│   ├── policy_loader.go      📋 NEW - Policy loading + cache
│   ├── directory_service.go
│   ├── content_validator.go
│   └── file_service.go       ⏳ TODO
│
└── models/              ✅ + NEW
    ├── user.go          📋 NEW
    ├── group.go         📋 NEW
    └── ...

services/vfs/
├── handlers/            ✅ + NEW
│   ├── auth.go          📋 NEW - Login, logout
│   ├── admin.go         📋 NEW - User/group CRUD
│   ├── directory.go
│   └── file.go          ⏳ TODO
└── main.go              ⏳ TODO
```

---

## Database Schema

### New Tables (v2)

```sql
-- Users
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
    deleted_at TIMESTAMP NULL,
    INDEX idx_username (username),
    INDEX idx_email (email),
    INDEX idx_role (role)
);

-- Groups
CREATE TABLE groups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL,
    INDEX idx_name (name)
);

-- User-Group Mapping
CREATE TABLE user_groups (
    user_id VARCHAR(36) NOT NULL,
    group_id VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, group_id),
    INDEX idx_user_id (user_id),
    INDEX idx_group_id (group_id)
);
```

### Existing Tables (No Changes)

- `directories`
- `files`
- `file_versions`
- `file_relations`
- `events`
- `opa_policies`
- `webhook_configs`
- `webhook_jobs`
- `cron_jobs`
- `cron_executions`
- `idempotency_records`
- `audit_logs`
- `dead_letter_queue`

---

## API Examples

### Authentication

```bash
# Login
POST /api/v1/auth/login
{
  "username": "admin",
  "password": "secure-password"
}

Response:
{
  "token": "eyJhbGci...",
  "user": {
    "id": "...",
    "username": "admin",
    "role": "admin"
  }
}

# Get current user
GET /api/v1/auth/me
Authorization: Bearer eyJhbGci...
```

### User Management (Admin Only)

```bash
# Create user
POST /api/v1/admin/users
Authorization: Bearer {admin-token}
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "secure-password",
  "role": "user"
}

# List users
GET /api/v1/admin/users

# Update user
PATCH /api/v1/admin/users/{id}
{
  "role": "admin"
}
```

### Special Files (Admin Only)

```bash
# Create schema
POST /api/v1/files
Authorization: Bearer {admin-token}
{
  "directory_path": "/data/users",
  "name": ".jsonschema",
  "content_type": "application/json",
  "content": "{\"type\":\"object\",\"required\":[\"email\",\"name\"]}"
}

# Create policy
POST /api/v1/files
Authorization: Bearer {admin-token}
{
  "directory_path": "/projects/secret",
  "name": ".rego",
  "content_type": "text/plain",
  "content": "package vfs.authz\nallow { input.user.role == \"admin\" }"
}
```

### File Upload (Auto-Validated)

```bash
# Upload file (validated against .jsonschema)
POST /api/v1/files
Authorization: Bearer {user-token}
{
  "directory_path": "/data/users",
  "name": "alice.json",
  "content_type": "application/json",
  "content": "{\"email\":\"alice@example.com\",\"name\":\"Alice\"}"
}

# Success (201 Created)
{
  "id": "...",
  "name": "alice.json",
  "path": "/data/users/alice.json"
}

# Failure (400 Bad Request)
{
  "error": "content validation failed",
  "details": [
    "email: Required property is missing",
    "name: String length must be >= 1"
  ]
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

# Admin Bootstrap
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
      - JWT_SECRET=${JWT_SECRET:-change-this}
      - ADMIN_USERNAME=${ADMIN_USERNAME:-admin}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin}
      - ADMIN_EMAIL=${ADMIN_EMAIL:-admin@example.com}
      - SCHEMA_CACHE_TTL=5m
      - POLICY_CACHE_TTL=5m
```

---

## Metrics

| Metric | Phase 1 | Phase 2 | v2 (Est.) | Total |
|--------|---------|---------|-----------|-------|
| Files Created | 14 | 7 | ~20 | ~41 |
| Lines of Code | ~1,500 | ~600 | ~3,000 | ~5,100 |
| New Tables | 0 | 0 | 3 | 3 |
| New Packages | 3 | 2 | 2 | 7 |
| New Interfaces | 5 | 3 | 4 | 12 |
| Breaking Changes | 0 | 0 | 0 | 0 |

---

## Success Criteria

### ✅ Completed

- [x] Middleware layer with chain pattern
- [x] Repository layer with Unit of Work
- [x] Domain layer (DirectoryService)
- [x] JSON schema validation
- [x] Directory handlers
- [x] Error mapping
- [x] Special files design complete
- [x] User/group management design complete
- [x] Inheritance specification complete

### ⏳ In Progress / Pending

**Week 1:**
- [ ] User/Group models and repositories
- [ ] JWT authentication
- [ ] Auth middleware
- [ ] Login/logout endpoints
- [ ] User/group CRUD endpoints
- [ ] Bootstrap admin

**Week 2:**
- [ ] Special file detection
- [ ] Special file validation
- [ ] SchemaLoader with caching
- [ ] PolicyLoader with caching
- [ ] Schema validation integration

**Week 3:**
- [ ] End-to-end integration
- [ ] Comprehensive testing
- [ ] Documentation updates
- [ ] Performance benchmarks

---

## Testing Strategy

### Unit Tests

```go
// Auth
TestJWTTokenGeneration
TestJWTTokenValidation
TestPasswordHashing

// Special Files
TestIsSpecialFile
TestIsSuperUser
TestValidateSpecialFileContent

// Schema Loader
TestSchemaLoader_LoadFromDirectory
TestSchemaLoader_Inheritance
TestSchemaLoader_Caching
TestSchemaLoader_CacheInvalidation

// Policy Loader
TestPolicyLoader_LoadFromDirectory
TestPolicyLoader_Inheritance
```

### Integration Tests

```go
// User Management
TestCreateUser_AdminOnly
TestLogin_Success
TestLogin_InvalidCredentials

// Special Files
TestCreateSchema_AdminOnly
TestCreateSchema_RegularUserRejected
TestFileUpload_ValidatedAgainstSchema

// Inheritance
TestSchemaInheritance_ChildInheritsFromParent
TestSchemaInheritance_ChildOverridesParent
```

### E2E Tests

```bash
# Scenario 1: Admin creates schema, user uploads file
1. Admin logs in
2. Admin creates .jsonschema in /data/users
3. User logs in
4. User uploads valid file → Success
5. User uploads invalid file → Rejected with errors

# Scenario 2: Inheritance
1. Admin creates schema in /data
2. User uploads to /data/users (no schema there)
3. File validates against /data/.jsonschema (inherited)

# Scenario 3: Regular user tries special file
1. User logs in
2. User tries to create .custom file
3. Rejected with "only admins can create special files"
```

---

## Documentation

### For Users

- **User Guide** - How to use schemas for validation
- **API Reference** - All endpoints with examples
- **CLI Guide** - Using special files via CLI

### For Admins

- **Admin Guide** - User/group management
- **Schema Guide** - Creating and managing schemas
- **Security Guide** - Best practices

### For Developers

- **Architecture Guide** - System design
- **Contributing Guide** - How to contribute
- **API Documentation** - OpenAPI/Swagger specs

---

## Migration Path

### v1 → v2

**Zero Breaking Changes** ✅

1. **Database Migration**
   ```bash
   # Run migrations to add users, groups, user_groups tables
   docker-compose run vfs-service migrate
   ```

2. **Bootstrap Admin**
   ```bash
   # First run creates admin user from environment variables
   docker-compose up
   ```

3. **Existing Features**
   - All existing file operations work exactly as before
   - Directories without schemas behave identically to v1
   - Adding schemas is opt-in per directory

4. **Gradual Adoption**
   - Start using authentication (optional, can disable)
   - Add schemas to critical directories
   - Policies can be added incrementally

---

## Timeline

```
COMPLETED:
✅ Oct 3: Phase 1 - Foundation (Middleware, Repository, Domain)
✅ Oct 3: Phase 2 - Validation & Handlers
✅ Oct 3: Design - Special Files System

NEXT 3 WEEKS:
📋 Week 1 (Oct 7-11): User & Group Management
📋 Week 2 (Oct 14-18): Special Files Core
📋 Week 3 (Oct 21-25): Integration & Testing

FUTURE:
📋 Event Middleware
📋 Audit Improvements
📋 Final Documentation
```

---

## Resources

### Design Documents

- [Special Files Design](./special-files-design.md)
- [Special Files Inheritance](./special-files-inheritance.md)
- [User & Group Management](./user-group-management.md)
- [Implementation Roadmap](./IMPLEMENTATION_ROADMAP.md) ⭐ **START HERE**

### Implementation Guides

- [Phase 1 Report](./phase-1-layered-architecture.md)
- [Phase 2 Report](./phase-2-validation-layer.md)
- [Special Files Implementation Plan](./special-files-implementation-plan.md)

### Original Architecture

- [Layered Architecture Plan](./layered-architecture.md)

---

## Risks & Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Performance overhead from inheritance lookup | Medium | Aggressive caching (5min TTL) |
| Cache invalidation bugs | High | Comprehensive tests, clear invalidation rules |
| Weak admin password on bootstrap | High | Force password change on first login |
| Schema validation bypassed | High | Validate in domain layer, not skippable |
| Breaking existing workflows | Critical | 100% backward compatible, comprehensive testing |

---

## Key Principles

1. **Everything is a File** - Schemas and policies are files, not database records
2. **Inheritance** - Child directories inherit from parents (DRY principle)
3. **Admin-Only Special Files** - Security by design
4. **Backward Compatible** - Zero breaking changes
5. **Cacheable** - Performance through intelligent caching
6. **Testable** - Clean architecture enables comprehensive testing

---

**Status:** Foundation Complete ✅ | Design Complete ✅ | Ready for Implementation
**Next:** Week 1 - User & Group Management
**Updated:** 2025-10-03
**Version:** v2.0

---

*For implementation details, see [IMPLEMENTATION_ROADMAP.md](./IMPLEMENTATION_ROADMAP.md)*

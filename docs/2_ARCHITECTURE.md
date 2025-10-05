# 2. System Architecture

**Version:** v2.1+
**Last Updated:** 2025-10-05

[← Back: Overview](1_OVERVIEW.md) | [Index](0_README.md) | [Next: Quick Start →](3_QUICKSTART.md)

**Implementation:** All core components in `pkg/domain/`, `pkg/middleware/`, and `services/vfs/`

---

## Table of Contents

1. [High-Level Architecture](#high-level-architecture)
2. [Layered Design](#layered-design)
3. [Data Flow](#data-flow)
4. [Component Details](#component-details)
5. [Database Schema](#database-schema)

---

## High-Level Architecture

MySQL VFS v2 follows a clean layered architecture with pluggable authentication:

```
┌─────────────────────────────────────────┐
│   External Auth Provider                 │
│   (OAuth, JWT, LDAP, Reverse Proxy)     │
└─────────────────────────────────────────┘
                  ↓ (Headers or Token)
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
│   ✅ Authentication (JWT, OAuth, etc.)   │
│   ✅ Authorization (OPA + .rego)         │
│   ✅ Recovery, CORS, Timeout             │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Handler Layer                          │
│   ✅ Directory handlers                  │
│   ✅ File handlers (with validation)     │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Domain Layer                           │
│   ✅ DirectoryService                    │
│   ✅ FileService                         │
│   ✅ SchemaLoader (caching)              │
│   ✅ PolicyLoader (caching)              │
│   ✅ QuotaLoader (caching)               │
│   ✅ Special file helpers                │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Repository Layer                       │
│   ✅ DirectoryRepository                 │
│   ✅ FileRepository                      │
│   ✅ EventRepository                     │
│   ✅ Unit of Work                        │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Infrastructure                         │
│   ✅ GORM + Models                       │
│   ✅ S3/MinIO Storage                    │
└─────────────────────────────────────────┘
```

---

## Layered Design

### 1. HTTP Layer

**Responsibility:** Route requests, parse inputs, format responses

**Components:**
- Hertz web framework
- Route definitions
- Request/Response DTOs

**Example:**
```go
// services/vfs/main.go
v1 := h.Group("/api/v1")
v1.POST("/files", vfsServer.createFile)
v1.GET("/files/*path", vfsServer.getFile)
```

---

### 2. Middleware Layer

**Responsibility:** Cross-cutting concerns (auth, validation, logging)

**Components:**
- `middleware.AuthMiddleware` - Authentication (file, JWT, headers, proxy)
- `middleware.AuthorizationMiddleware` - OPA policy evaluation
- `middleware.ValidationMiddleware` - JSON schema validation
- `middleware.ObservabilityMiddleware` - Logging, metrics

**Implementation Files:**
- `pkg/middleware/auth.go` - Generic auth middleware (lines 1-100)
- `pkg/middleware/auth_providers.go` - Provider factory and implementations (lines 1-300)
- `pkg/middleware/authorization.go` - OPA-based authorization (lines 1-200)
- `pkg/middleware/default_policy.go` - Fallback authorization policy (lines 1-100)

**Example:**
```go
// Chain middleware
v1.Use(authMiddleware.Handler())
v1.Use(authzMiddleware.Handler())
```

---

### 3. Handler Layer

**Responsibility:** HTTP-specific logic, error handling

**Components:**
- `DirectoryHandler` - Directory CRUD
- `FileHandler` - File CRUD with validation

**Files:** `services/vfs/handlers/`

**Example:**
```go
func (s *VFSServer) createFile(ctx context.Context, c *app.RequestContext) {
    var req CreateFileRequest
    c.BindJSON(&req)

    file, err := s.fileService.CreateFile(ctx, ...)
    if err != nil {
        c.JSON(400, ErrorResponse{Error: err.Error()})
        return
    }

    c.JSON(201, file)
}
```

---

### 4. Domain Layer

**Responsibility:** Business logic, validation, orchestration

**Components:**
- `DirectoryService` - Directory operations with lifecycle events
- `FileService` - File operations with special file handling and events
- `FilesLoader` - Load and cache `.files` patterns (replaces SchemaLoader)
- `PolicyLoader` - Load and cache `.rego` files
- `UserLoader` - Load and cache `.user` files for file-based auth
- `EventsLoader` - Load and cache `.events` handlers
- `OwnerLoader` - Load and cache `.owner` ownership data
- `EventTrigger` - Lifecycle event triggering
- `EventDispatcher` - Event delivery coordination

**Implementation Files:**
- `pkg/domain/file_service.go` - File CRUD with validation (lines 1-500)
- `pkg/domain/directory_service.go` - Directory CRUD with events (lines 1-400)
- `pkg/domain/files_loader.go` - Pattern matching and validation (lines 1-300)
- `pkg/domain/policy_loader.go` - OPA policy loading (lines 1-200)
- `pkg/domain/user_loader.go` - File-based auth (lines 1-150)
- `pkg/domain/events_loader.go` - Event handler config (lines 1-250)
- `pkg/domain/owner_loader.go` - Ownership tracking (lines 1-100)
- `pkg/domain/event_trigger.go` - Event lifecycle management (lines 1-300)
- `pkg/domain/special_file_loader.go` - Generic loader with caching (lines 1-150)

**Example:**
```go
// pkg/domain/file_service.go
func (s *FileService) CreateFile(ctx context.Context, dirPath, name string, ...) (*models.File, error) {
    // 1. Check if special file
    if domain.IsSpecialFile(name) {
        // Validate special file syntax
    }

    // 2. Load schema for directory
    schema, _ := s.schemaLoader.Load(ctx, dirID)

    // 3. Validate content against schema
    if schema != nil {
        s.validateContent(content, schema)
    }

    // 4. Save file
    return s.repository.Create(ctx, file)
}
```

---

### 5. Repository Layer

**Responsibility:** Data access abstraction

**Components:**
- `DirectoryRepository` - Directory data access
- `FileRepository` - File data access
- `EventRepository` - Event data access
- `UnitOfWork` - Transaction coordination

**Files:** `pkg/repository/`

**Example:**
```go
type FileRepository interface {
    Create(ctx context.Context, file *models.File) error
    FindByID(ctx context.Context, id string) (*models.File, error)
    FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error)
    // ...
}
```

---

### 6. Infrastructure Layer

**Responsibility:** External systems (database, storage)

**Components:**
- GORM implementations
- S3/MinIO storage client
- Database models

**Files:** `pkg/repository/gorm/`, `pkg/models/`, `pkg/storage/`

---

## Data Flow

### Example: Upload File with Schema Validation

```
1. External Auth System authenticates user
   ↓
2. Request with auth headers (X-User-ID, X-User-Role, X-User-Groups)
   ↓
3. Authorization Middleware
   - Load .rego from /data/users/ (or parent)
   - Evaluate policy with input.user from headers
   ↓
4. File Handler
   - Parse request
   - Call FileService.CreateFile()
   ↓
5. FileService (Domain)
   - Check if special file (.jsonschema)
     → If yes: Validate syntax
   - Load schema for /data/users/
     → SchemaLoader checks /data/users/.jsonschema
     → If not found, check /data/.jsonschema (inheritance)
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

## Component Details

### Special Files System

Files starting with `.` have special behavior:

| File | Purpose | Validated By |
|------|---------|--------------|
| `.jsonschema` | Validates JSON files in directory | SchemaLoader |
| `.rego` | Authorization policy for directory | PolicyLoader |
| `.quota` | Resource limits for directory | QuotaLoader |

**Inheritance:**
- Child directories inherit parent special files
- Children can override parent rules
- Traverse up tree to find applicable file

**Caching:**
- 5-minute TTL by default (configurable)
- Invalidated on special file change
- Per-directory caching

**See:** [Special Files](4_SPECIAL_FILES.md) for details

---

### Authentication Flow

```
Request → Auth Middleware → Auth Extractor (pluggable) → Auth Context
```

**Supported Auth Methods:**
- **JWT:** Validate signature, extract claims
- **OAuth:** Token introspection with provider
- **mTLS:** Certificate verification
- **Proxy+HMAC:** Verify HMAC signature from reverse proxy
- **Headers:** Development only (unsafe)

**See:** [Authentication](5_AUTHENTICATION.md) for details

---

### Authorization Flow

```
Request → Authorization Middleware → Load .rego → Evaluate with OPA → Allow/Deny
```

**OPA Policy Input:**
```json
{
  "user": {
    "user_id": "alice",
    "role": "admin",
    "groups": ["engineering", "admins"]
  },
  "resource": {
    "path": "/data/users/alice.json",
    "type": "file"
  },
  "action": "read"
}
```

**See:** [Authorization](6_AUTHORIZATION.md) for details

---

## Database Schema

### Core Tables

```sql
-- Directories
CREATE TABLE directories (
    id VARCHAR(36) PRIMARY KEY,
    parent_id VARCHAR(36),
    name VARCHAR(255) NOT NULL,
    path VARCHAR(1024) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL,
    INDEX idx_parent (parent_id),
    INDEX idx_path (path)
);

-- Files
CREATE TABLE files (
    id VARCHAR(36) PRIMARY KEY,
    directory_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    content_type VARCHAR(255),
    size_bytes BIGINT NOT NULL,
    storage_type ENUM('json', 's3') NOT NULL,
    json_content LONGTEXT,
    s3_key VARCHAR(512),
    checksum_sha256 VARCHAR(64),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL,
    UNIQUE KEY idx_dir_name (directory_id, name),
    INDEX idx_directory (directory_id)
);

-- File Versions
CREATE TABLE file_versions (
    id VARCHAR(36) PRIMARY KEY,
    file_id VARCHAR(36) NOT NULL,
    version_number BIGINT NOT NULL,
    content_type VARCHAR(255),
    size_bytes BIGINT NOT NULL,
    storage_type ENUM('json', 's3') NOT NULL,
    json_content LONGTEXT,
    s3_key VARCHAR(512),
    checksum_sha256 VARCHAR(64),
    created_at TIMESTAMP NOT NULL,
    UNIQUE KEY idx_file_version (file_id, version_number),
    INDEX idx_file (file_id)
);
```

**Note:** No user/group tables - authentication is external!

---

## Key Design Principles

1. **Separation of Concerns** - Each layer has clear responsibility
2. **Dependency Inversion** - Depend on interfaces, not implementations
3. **Testability** - Easy to mock and test each layer
4. **Pluggability** - Auth, storage, etc. are pluggable
5. **Clean Code** - No business logic in handlers, no HTTP in domain

---

## File Structure

```
pkg/
├── middleware/          # Cross-cutting concerns
│   ├── auth.go
│   ├── authorization.go
│   └── validation.go
│
├── domain/              # Business logic
│   ├── directory_service.go
│   ├── file_service.go
│   ├── schema_loader.go
│   ├── policy_loader.go
│   └── quota_loader.go
│
├── repository/          # Data access
│   ├── interfaces.go
│   └── gorm/
│       ├── directory_repo.go
│       ├── file_repo.go
│       └── unit_of_work.go
│
├── models/              # Domain models
│   ├── directory.go
│   ├── file.go
│   └── event.go
│
├── config/              # Configuration
│   └── config.go
│
└── storage/             # Storage abstraction
    ├── s3.go
    └── minio.go

services/vfs/
├── handlers/            # HTTP handlers
│   ├── directory.go
│   └── file.go
└── main.go              # App entry point
```

---

[← Back: Overview](1_OVERVIEW.md) | [Index](0_README.md) | [Next: Quick Start →](3_QUICKSTART.md)

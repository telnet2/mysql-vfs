# Design Document

**Version:** v2.1+
**Last Updated:** 2025-10-05

---

## Table of Contents

- [Design Philosophy](#design-philosophy)
- [Motivation](#motivation)
- [High-Level Architecture](#high-level-architecture)
- [Design Details](#design-details)
- [Implementation](#implementation)
- [Future Work](#future-work)
- [Design Decisions Log](#design-decisions-log)

---

## Design Philosophy

### Core Principles

1. **Files as Configuration** - Configuration stored as files, not database records
   - Version controllable
   - Auditable via git
   - Familiar to developers

2. **Policy as Code** - Authorization via OPA policies
   - Declarative, not imperative
   - Testable independently
   - No code changes to update access rules

3. **Inheritance Over Duplication** - Policies/config inherit from parent directories
   - DRY principle
   - Easier to manage
   - Clear override semantics

4. **Fail-Safe Defaults** - Deny by default, explicit allow
   - Security first
   - No accidental exposure
   - Clear intent

5. **No Magic** - Explicit over implicit
   - No hidden behaviors
   - Predictable outcomes
   - Easy to reason about

### Why MySQL VFS?

**Problem Statement:**
- Need file storage with dynamic access control
- Can't rebuild auth system for every policy change
- Need audit trails without application changes
- Want multi-tenant isolation

**Why This Design:**
- Leverage MySQL for metadata + queries
- Leverage S3 for cheap bulk storage
- OPA for flexible, testable authorization
- Special files for self-contained configuration

**Design Constraints:**
- Must work with existing MySQL infrastructure
- Must support S3-compatible storage
- Must be horizontally scalable
- Must support policy inheritance

---

## Motivation

### Problems We're Solving

1. **Configuration Management**
   - Problem: Config in DB → hard to version, review, rollback
   - Solution: Config as files → git, PR reviews, rollback via git

2. **Dynamic Authorization**
   - Problem: Hardcoded RBAC → code changes for new rules
   - Solution: OPA policies → update `.rego` file, no deployment

3. **Schema Validation**
   - Problem: Invalid data reaches application
   - Solution: JSON schema validation at upload time

4. **Multi-Tenancy**
   - Problem: Cross-tenant data leaks
   - Solution: Directory isolation + owner-based access

5. **Audit Trails**
   - Problem: Need to know who did what
   - Solution: Event system + webhooks

### Use Cases

- **SaaS Multi-Tenant Storage** - Each tenant gets isolated directories
- **Regulated Industries** - Immutable audit logs via events
- **Dynamic Access Control** - Change permissions without code deploy
- **Content Validation** - Enforce data schemas at upload time

---

## High-Level Architecture

### System Overview

```
┌─────────────────────────────────────────────┐
│              HTTP API Layer                  │
│  (File CRUD, Directory Ops, Auth)           │
│  → cmd/server/main.go                       │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│           Middleware Stack                   │
│  ┌─────────────────────────────────────┐   │
│  │ 1. Authentication                    │   │
│  │    → pkg/middleware/auth.go          │   │
│  │ 2. Authorization (OPA)              │   │
│  │    → pkg/middleware/authorization.go │   │
│  │ 3. Validation (Schema)              │   │
│  │    → pkg/domain/file_service.go     │   │
│  │ 4. Events (Lifecycle)               │   │
│  │    → pkg/domain/event_trigger.go    │   │
│  └─────────────────────────────────────┘   │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│            Domain Layer                      │
│  ┌──────────┬──────────┬──────────────┐    │
│  │ File     │ Policy   │ User         │    │
│  │ Service  │ Loader   │ Loader       │    │
│  └──────────┴──────────┴──────────────┘    │
│  ┌──────────────────────────────────────┐  │
│  │      Special File Loaders            │  │
│  │  → pkg/domain/*_loader.go            │  │
│  └──────────────────────────────────────┘  │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│         Persistence Layer                    │
│  ┌──────────────┬───────────────────────┐  │
│  │ MySQL        │ S3 Storage            │  │
│  │ (Metadata)   │ (Content)             │  │
│  │ → pkg/persistence/db/mysql/          │  │
│  └──────────────┴───────────────────────┘  │
└─────────────────────────────────────────────┘
```

### Component Responsibilities

**API Layer** (`cmd/server/`)
- HTTP routing and handlers
- Request/response serialization
- Error formatting

**Middleware** (`pkg/middleware/`)
- Authentication (extract user context)
- Authorization (OPA policy evaluation)
- Cross-cutting concerns

**Domain** (`pkg/domain/`)
- Business logic
- Special file loading (policies, users, validation rules)
- Event triggering

**Persistence** (`pkg/persistence/`)
- Database access (MySQL)
- Storage access (S3)
- Repository pattern

### Data Flow

```
1. Request arrives
   ↓
2. Auth middleware extracts user (JWT/headers)
   → pkg/middleware/auth.go:51-179
   ↓
3. Authz middleware loads policy, evaluates with OPA
   → pkg/middleware/authorization.go:68-179
   ↓
4. Domain service validates content (if applicable)
   → pkg/domain/file_service.go
   ↓
5. Domain service triggers lifecycle events
   → pkg/domain/event_trigger.go
   ↓
6. Persistence layer saves to MySQL + S3
   → pkg/persistence/db/mysql/file.go
   ↓
7. Response returned
```

**Special File Changes:**
- Invalidate relevant caches
- See: `pkg/domain/*_loader.go` for cache invalidation

---

## Design Details

### 1. Special Files Architecture

**Design Decision:** Configuration as Files

**Why:**
- Version control (git workflow)
- Auditable (PR reviews, blame)
- Familiar (developers know files)
- Self-contained (no external dependencies)

**Tradeoff:**
- Cache invalidation complexity
- Eventual consistency (cache TTL)

**Alternative Rejected:**
- Database tables → Less flexible, no version control

**Special File Types:**

| File | Purpose | Implementation |
|------|---------|----------------|
| `.rego` | Authorization policies | `pkg/domain/policy_loader.go` |
| `.user` | User credentials | `pkg/domain/user_loader.go` |
| `.group` | Group definitions | `pkg/domain/group_loader.go` |
| `.owner` | Directory ownership | `pkg/domain/owner_loader.go` |
| `.files` | Content validation | `pkg/domain/files_loader.go` |
| `.events` | Lifecycle handlers | `pkg/domain/events_loader.go` |

**Inheritance Model:**

```
/                    (.rego: admin-only)
└── data/           (.rego: users can read)  ← overrides parent
    └── public/     (no .rego)              ← inherits from /data
    └── private/    (.rego: admins only)    ← overrides parent
```

**Lookup Algorithm:**
See: `pkg/domain/policy_loader.go:38-89` (walks up directory tree)

**Implementation Pattern:**
- Generic loader: `pkg/domain/special_file_loader.go`
- 5-minute TTL cache (configurable)
- Automatic invalidation on file changes
- Thread-safe with `sync.Map`

---

### 2. Authorization System

**Design Decision:** OPA (Open Policy Agent)

**Why:**
- Industry standard
- Rego is declarative and testable
- Flexible (time-based, attribute-based, etc.)
- External policy engine (separation of concerns)

**Tradeoff:**
- Learning curve for Rego
- ~1-5ms latency per request

**Alternative Rejected:**
- RBAC in code → Not flexible enough

**Policy Input Structure:**

See: `pkg/middleware/authorization.go:148-160`

```go
input = {
  "user": {
    "id": "alice",
    "username": "alice",
    "groups": ["admin", "engineering"]
  },
  "resource": {
    "path": "/data/file.json",
    "type": "file",
    "owners": ["engineering"]
  },
  "action": "read|write|delete"
}
```

**System Admin Bypass:**
- Users in `"system-admin"` group bypass ALL OPA policies
- Implementation: `pkg/middleware/authorization.go:89-102`
- Rationale: Bootstrap and emergency access
- Tradeoff: Powerful but dangerous if token leaks

**Default Policy:**
- Fallback when no `.rego` found
- See: `pkg/middleware/default_policy.go`
- Also: `pkg/setup/setup.go` (bootstrap policy)

---

### 3. Group-Based Access Control

**Design Decision:** Groups over Roles

**Why:**
- Users often have multiple responsibilities
- Flexible: `["admin", "engineering", "oncall"]`
- Natural mapping to organizational structure

**Tradeoff:**
- Slightly more complex policies (iterate groups array)

**Alternative Rejected:**
- Single role field → Too limiting

**Data Structures:**

```go
// pkg/middleware/auth.go:29-33
type AuthContext struct {
    UserID   string
    Groups   []string
    Metadata map[string]interface{}
}

// pkg/domain/special_files.go:318-323
type UserCredential struct {
    UserID       string   `json:"user_id"`
    PasswordHash string   `json:"password_hash"`
    Token        string   `json:"token,omitempty"`
    Groups       []string `json:"groups"`
}
```

**Policy Pattern:**

See OPA policies in:
- `pkg/middleware/default_policy.go`
- `pkg/setup/setup.go`
- `docs/6_AUTHORIZATION.md` (examples)

---

### 4. Content Validation

**Design Decision:** JSON Schema + Pattern Matching

**Why:**
- JSON Schema is standard and widely supported
- Validation libraries exist
- Declarative schema definition

**Pattern Matching:**
- Glob patterns: `*.json`
- Regex patterns: `admin-.*\.json`
- See: `pkg/domain/files_loader.go`

**Tradeoff:**
- Complex schemas hard to write
- Performance overhead on large files

**Validation Flow:**

```
1. File upload
2. Load .files config for directory (pkg/domain/files_loader.go)
3. Match filename against patterns
4. If match → validate content against schema
5. Reject if validation fails
```

**Schema Storage Options:**

1. **Inline Schema** - Schema embedded in `.files` rule
   ```json
   {
     "rules": [
       {
         "pattern": "*.json",
         "type": "glob",
         "schema": {"type": "object", "properties": {...}}
       }
     ]
   }
   ```

2. **External Schema File** - Schema stored in separate VFS file
   ```json
   {
     "rules": [
       {
         "pattern": "*.json",
         "type": "glob",
         "schema_ref": "/schemas/user.json"
       }
     ]
   }
   ```

3. **Schema with $ref** - *(Future Enhancement)*
   - JSON Schema `$ref` resolution from VFS files is planned but not yet implemented
   - For now, use `schema_ref` to reference external schemas or inline schemas without `$ref`

**Implementation:**
- Validation: `pkg/domain/file_service.go:validateFileContent()`
- Schema loading: `pkg/domain/files_loader.go`
- Schema caching: Built-in cache for external schemas
- Tests: `citest/schema_validation_test.go`, `citest/schema_ref_validation_test.go`

---

### 5. Event System

**Design Decision:** Lifecycle Events + Webhooks

**Why:**
- Extensibility without code changes
- Audit trails
- Integration with external systems

**Event Stages:**
- `starting` → Before operation
- `started` → Operation in progress
- `succeeded` / `failed` → After operation

**Tradeoff:**
- Async complexity
- Webhook reliability (retries?)
- Performance overhead

**Architecture:**

```
Event Trigger → Event Loader → Handlers
    ↓              ↓              ↓
event_trigger.go  events_loader  log.go
                  .go            metrics.go
                                 webhook.go
```

**Event Types:**

See: `pkg/events/types.go` for full list

Format: `{category}.{operation}.{stage}.{outcome}`

Examples:
- `file.create.starting`
- `file.create.succeeded`
- `authorization.policy.checked.failed`

**Handlers:**

Built-in handlers in `pkg/events/handlers/`:
- `log.go` - Log to stdout
- `metrics.go` - Increment counters
- `webhook.go` - HTTP POST to external endpoint

**Configuration:**

See `.events` file format in: `pkg/domain/special_files.go:191-237`

---

### 6. Storage Architecture

**Design Decision:** Dual Storage (MySQL + S3)

**Why:**
- MySQL: Fast queries, transactions, consistency
- S3: Cheap, scalable, durable for large files
- Best of both worlds

**Split Threshold:**
- Small files (< 1MB): Store in MySQL
- Large files (≥ 1MB): Store in S3
- Configurable via environment variable

**Tradeoff:**
- Complexity in keeping them in sync
- Need to handle both storage paths

**Implementation:**

File storage decision: `pkg/persistence/db/mysql/file.go:Create()`

Content retrieval:
- Text content: `text_content` column
- JSON content: `json_content` column
- S3 content: `s3_url` + fetch from S3

**Consistency:**
- Transactions ensure MySQL + S3 stay in sync
- Rollback on S3 upload failure

---

### 7. Caching Strategy

**Cache Layers:**

All special files cached for 5 minutes (configurable):
- `.rego` → `pkg/domain/policy_loader.go`
- `.user` → `pkg/domain/user_loader.go`
- `.files` → `pkg/domain/files_loader.go`
- `.events` → `pkg/domain/events_loader.go`
- `.owner` → `pkg/domain/owner_loader.go`

**Invalidation:**
- On file update/delete
- Per-directory caching
- Automatic via loader pattern

**Tradeoff:**
- **Pro:** Much faster reads (avoid DB query)
- **Con:** Eventual consistency (up to 5-min staleness)
- **Con:** Memory usage

**Thread Safety:**
- `sync.Map` for concurrent access
- See: Generic loader implementation pattern

---

## Implementation

### Technology Stack

- **Language:** Go 1.21+
- **Web Framework:** Hertz (CloudWeGo)
- **ORM:** GORM
- **Database:** MySQL 8.0+
- **Storage:** S3-compatible (MinIO, AWS S3)
- **Policy Engine:** OPA (Open Policy Agent)
- **Validation:** go-jsonschema
- **Testing:** Ginkgo + Gomega

### Project Structure

```
mysql-vfs/
├── cmd/server/              # Main entry point
│   └── main.go
├── pkg/
│   ├── domain/              # Business logic
│   │   ├── file_service.go           # Core file operations
│   │   ├── directory_service.go      # Directory operations
│   │   ├── policy_loader.go          # .rego loader
│   │   ├── user_loader.go            # .user loader
│   │   ├── files_loader.go           # .files loader
│   │   ├── events_loader.go          # .events loader
│   │   ├── owner_loader.go           # .owner loader
│   │   ├── group_loader.go           # .group loader
│   │   ├── event_trigger.go          # Event triggering
│   │   └── special_files.go          # Type definitions
│   ├── middleware/          # HTTP middleware
│   │   ├── auth.go                   # Authentication
│   │   ├── authorization.go          # OPA authorization
│   │   └── default_policy.go         # Fallback policy
│   ├── persistence/         # Data access
│   │   └── db/mysql/
│   │       ├── file.go               # File repository
│   │       ├── directory.go          # Directory repository
│   │       └── migrate.go            # Database migrations
│   ├── events/              # Event system
│   │   ├── types.go                  # Event type definitions
│   │   └── handlers/
│   │       ├── log.go
│   │       ├── metrics.go
│   │       └── webhook.go
│   └── setup/               # Bootstrap utilities
│       └── setup.go                  # Initial setup
├── citest/                  # Integration tests
│   ├── fixtures/                     # Test utilities
│   └── *_test.go                     # E2E test suites
└── docs/                    # Documentation
    ├── README.md
    ├── USER_GUIDE.md
    ├── SECURITY.md
    ├── OPERATIONS.md
    └── DESIGN.md (this file)
```

### Key Design Patterns

**1. Repository Pattern**

Interface definition: `pkg/persistence/db/repository.go`

Separates domain logic from data access.

**2. Generic Loader Pattern**

Template for all special file loaders:
- Cache with TTL
- Automatic invalidation
- Thread-safe
- Directory inheritance

See example: `pkg/domain/policy_loader.go`

**3. Middleware Chain**

Request flows through:
```
HTTP → Auth → Authz → Validation → Events → Handler
```

Configured in: `cmd/server/main.go`

**4. Event-Driven Architecture**

Operations trigger events at each stage:
```
starting → started → succeeded/failed
```

See: `pkg/domain/event_trigger.go`

### Testing Strategy

**Unit Tests:**
- Domain logic: `pkg/domain/*_test.go`
- Middleware: `pkg/middleware/*_test.go`

**Integration Tests:**
- E2E scenarios: `citest/*_test.go`
- Real MySQL + S3 (MinIO)
- Ginkgo BDD style

**Policy Tests:**
- OPA policy validation
- See authorization tests: `pkg/middleware/authorization_test.go`

**Test Coverage:**
- Target: >80% for domain layer
- Current: Check via `go test -cover`

### Performance Considerations

**Optimizations:**
- Special file caching (5-min TTL)
- MySQL connection pooling
- Prepared statements (via GORM)
- Lazy loading of file content
- S3 presigned URLs (future)

**Bottlenecks:**
- OPA policy evaluation (~1-5ms)
- S3 upload/download latency
- Database queries (mitigated by indexes)

**Scalability:**
- **Stateless design** → Horizontal scaling
- **Shared cache** → Could use Redis (future)
- **Read replicas** → MySQL read scaling
- **S3** → Infinitely scalable storage

**Database Indexes:**

See: `pkg/persistence/db/mysql/models.go`

Critical indexes:
- `files(directory_id, name)` - File lookup
- `files(directory_id, deleted_at)` - List files
- `directories(path)` - Directory lookup

---

## Future Work

### Planned Features

**1. Password Authentication** (Q1 2026)
- Login endpoint with password validation
- Session/token management
- Status: Partial (bcrypt hash storage exists)
- See: `pkg/domain/user_loader.go:85-96`

**2. Advanced Event System** (Q2 2026)
- Async event processing (queue-based)
- Event replay for debugging
- Dead letter queue for failed webhooks
- Retry logic with exponential backoff

**3. Performance Improvements**
- Redis cache for special files
- Prepared statement caching
- S3 multipart upload for large files
- CDN integration for public files

**4. Enhanced Authorization**
- Attribute-based access control (ABAC)
- Policy versioning and rollback
- Audit mode (log denies without blocking)
- Policy dry-run testing

**5. Multi-Region Support**
- S3 cross-region replication
- MySQL read replicas in multiple regions
- Geographic routing

### Experimental Features

**1. GraphQL API**
- Status: Design phase
- Rationale: Better for complex queries
- Keep REST API for simplicity

**2. WebSocket Support**
- Real-time file updates
- Live event streaming
- Status: Research

**3. Built-in Search**
- Full-text search in file content
- ElasticSearch integration
- Status: Design

### Known Limitations

**Current Constraints:**

1. **No Distributed Locking**
   - Single-node file locking only
   - Workaround: Use external lock service (Redis)

2. **Cache Invalidation Delay**
   - Up to 5 minutes staleness
   - Workaround: Reduce TTL for critical files

3. **No Built-in Encryption**
   - Files stored unencrypted in S3
   - Workaround: Encrypt client-side before upload

4. **Manual Group Management**
   - No UI for managing groups
   - Workaround: Edit `.group` file via API

5. **No File Versioning UI**
   - Versions exist but no built-in browser
   - Workaround: Use API to list/access versions

### Open Questions

**1. Should we support file locking?**
- **Pros:** Prevent concurrent edits, data consistency
- **Cons:** Distributed state complexity, potential deadlocks
- **Decision:** TBD - evaluate use cases first

**2. Should we add built-in versioning UI?**
- **Pros:** User-friendly, common request
- **Cons:** Scope creep, maintenance burden
- **Decision:** No - keep VFS focused, use API

**3. Should policies be versioned?**
- **Pros:** Rollback capability, audit trail
- **Cons:** Implementation complexity
- **Decision:** Future enhancement (low priority)

**4. Should we support async file processing?**
- **Pros:** Thumbnails, previews, indexing
- **Cons:** Queue infrastructure, retry logic
- **Decision:** Via events + external workers

---

## Design Decisions Log

### Decision 1: Groups over Roles
**Date:** 2025-10-05
**Status:** ✅ Implemented

**Context:**
Need flexible access control for multi-tenant scenarios

**Decision:**
Use array of groups instead of single role field

**Rationale:**
- Users often have multiple responsibilities
- More flexible for organizational hierarchies
- Common pattern in identity systems

**Consequences:**
- Slightly more complex OPA policies
- Need to iterate groups array
- More flexible for users

**Implementation:**
- `pkg/middleware/auth.go:29-33` (AuthContext)
- `pkg/domain/special_files.go:318-323` (UserCredential)

---

### Decision 2: Special Files Not in Database
**Date:** 2024-09-15
**Status:** ✅ Implemented

**Context:**
How to store configuration (policies, users, validation rules)

**Decision:**
Store as regular files, not database tables

**Rationale:**
- Version control via git
- Auditable via PR reviews
- Familiar to developers
- Self-contained

**Consequences:**
- Cache invalidation complexity
- Eventual consistency (TTL-based)
- Memory overhead (caching)

**Alternatives Considered:**
- Database tables → Rejected (not version-controllable)
- External config service → Rejected (external dependency)

**Implementation:**
- `pkg/domain/*_loader.go` (all special file loaders)

---

### Decision 3: OPA for Authorization
**Date:** 2024-08-20
**Status:** ✅ Implemented

**Context:**
Need flexible authorization without code changes

**Decision:**
Use Open Policy Agent (OPA) with Rego policies

**Rationale:**
- Industry standard
- Declarative and testable
- Flexible (time-based, attribute-based, etc.)
- Separate policy from code

**Consequences:**
- Learning curve for Rego
- ~1-5ms latency per request
- External dependency (OPA library)

**Alternatives Considered:**
- Hardcoded RBAC → Rejected (not flexible)
- Casbin → Rejected (less powerful than OPA)
- Custom DSL → Rejected (reinventing wheel)

**Implementation:**
- `pkg/middleware/authorization.go`
- `pkg/domain/policy_loader.go`

---

### Decision 4: Dual Storage (MySQL + S3)
**Date:** 2024-07-10
**Status:** ✅ Implemented

**Context:**
Large files are expensive in MySQL

**Decision:**
Split at 1MB threshold: small files in MySQL, large in S3

**Rationale:**
- Cost-effective
- Leverage MySQL for queries
- Leverage S3 for bulk storage
- Fast access for small files (no S3 call)

**Consequences:**
- Complexity in keeping them in sync
- Need to handle both storage paths
- Transaction complexity

**Alternatives Considered:**
- All in MySQL → Rejected (expensive, size limits)
- All in S3 → Rejected (slow for small files, no querying)

**Implementation:**
- `pkg/persistence/db/mysql/file.go:Create()`
- Threshold configurable via env var

---

### Decision 5: Event System Architecture
**Date:** 2024-10-04
**Status:** ✅ Implemented

**Context:**
Need audit trails and extensibility

**Decision:**
Lifecycle events with configurable handlers

**Rationale:**
- Extensible without code changes
- Audit trails via log handler
- Integration via webhook handler
- Clear event lifecycle (starting → succeeded/failed)

**Consequences:**
- Event overhead on every operation
- Webhook reliability concerns
- Need retry logic (future)

**Alternatives Considered:**
- Database triggers → Rejected (not portable)
- Application logs only → Rejected (not extensible)

**Implementation:**
- `pkg/domain/event_trigger.go`
- `pkg/events/handlers/`

---

## References

### External Resources
- [OPA Documentation](https://www.openpolicyagent.org/)
- [JSON Schema Specification](https://json-schema.org/)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Repository Pattern](https://martinfowler.com/eaaCatalog/repository.html)

### Related Documents
- `README.md` - Quick start
- `USER_GUIDE.md` - API and features
- `SECURITY.md` - Authentication and authorization
- `OPERATIONS.md` - Deployment and configuration

### Prior Art & Inspiration
- **NextCloud** - File storage with plugins
- **MinIO** - S3-compatible storage
- **HashiCorp Vault** - Policy-based access control
- **Kubernetes** - Resource-based authorization (RBAC)

---

**Document Status:** Living document - updated as design evolves
**Last Review:** 2025-10-05
**Next Review:** 2026-01-05

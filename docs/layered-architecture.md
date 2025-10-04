# Layered Architecture Enhancement Plan

## Executive Summary

Refactor the MySQL VFS codebase from a service-oriented architecture to a clean layered architecture with clear separation of concerns. This will improve testability, maintainability, and extensibility.

---

## Current Architecture Problems

### 1. **Mixed Concerns in Services**
```go
// Current: file_service.go lines 75-100
func (s *FileService) CreateFile(...) (*models.File, error) {
    // Validation mixed with business logic
    if size > MaxFileSize { return nil, fmt.Errorf(...) }
    if name == "" || name == "." || name == ".." { return nil, fmt.Errorf(...) }
    if strings.Contains(name, "/") || strings.Contains(name, "\\") { ... }

    // Business logic mixed with data access
    // Event emission embedded in service
    s.emitEvent(ctx, tx, "file.created", ...)
}
```

**Issues:**
- ❌ Validation logic scattered across services
- ❌ Event emission duplicated in every service method
- ❌ Hard to test business logic in isolation
- ❌ Services tightly coupled to GORM and storage implementation
- ❌ No authorization layer integration

### 2. **Missing Cross-Cutting Concerns**
- No unified authorization middleware (OPA service exists but not integrated)
- No schema validation layer
- No structured event emission
- Observability scattered across handlers

### 3. **Testing Challenges**
- Can't unit test business logic without database
- Hard to mock data access
- Integration tests required for basic validation logic

---

## Proposed Layered Architecture

### Layer Structure (Bottom to Top)

```
┌─────────────────────────────────────────┐
│   HTTP/Hertz Layer (Entry Point)        │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Middleware Chain Layer                 │
│   - Request ID                           │
│   - Observability (tracing, metrics)     │
│   - Schema Validation                    │
│   - Authorization (OPA)                  │
│   - Idempotency (existing)               │
│   - Event Emission                       │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Application/Handler Layer              │
│   - Request mapping                      │
│   - Response formatting                  │
│   - Error handling                       │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Domain/Business Logic Layer            │
│   - Pure business rules                  │
│   - No infrastructure dependencies       │
│   - Fully unit testable                  │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Repository Layer (Data Access)         │
│   - Interface-based abstraction          │
│   - GORM implementation                  │
│   - Storage abstraction                  │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Infrastructure (DB, Storage, Models)   │
└─────────────────────────────────────────┘
```

---

## Detailed Layer Design

### 1. Middleware Chain Layer (`pkg/middleware/`)

#### **Authorization Middleware** (`authorization.go`)
```go
package middleware

type AuthorizationMiddleware struct {
    opaService *services.OPAService
}

func (m *AuthorizationMiddleware) Handler() app.HandlerFunc {
    return func(ctx context.Context, c *app.RequestContext) {
        // Extract user context from headers/token
        userCtx := extractUserContext(c)

        // Extract resource path from request
        resourcePath := extractResourcePath(c)

        // Determine action from HTTP method + route
        action := determineAction(c)

        // Check authorization via OPA
        allowed, err := m.opaService.CheckAccess(ctx, resourcePath, action, userCtx)
        if err != nil || !allowed {
            c.JSON(403, map[string]string{"error": "forbidden"})
            c.Abort()
            return
        }

        // Add authorization decision to context
        ctx = context.WithValue(ctx, "authorized", true)
        c.Next(ctx)
    }
}
```

**Features:**
- Integrates OPA policy evaluation
- Fail-closed security (deny by default)
- 200ms timeout enforcement
- User context extraction from JWT/headers
- Path-based policy lookup with inheritance

#### **Schema Validation Middleware** (`validation.go`)
```go
package middleware

type ValidationMiddleware struct {
    schemas map[string]*jsonschema.Schema
}

func (m *ValidationMiddleware) Handler() app.HandlerFunc {
    return func(ctx context.Context, c *app.RequestContext) {
        route := c.FullPath()
        schema, exists := m.schemas[route]

        if exists {
            var body interface{}
            if err := c.BindJSON(&body); err != nil {
                c.JSON(400, map[string]string{"error": "invalid JSON"})
                c.Abort()
                return
            }

            if err := schema.Validate(body); err != nil {
                c.JSON(400, map[string]string{
                    "error": "validation failed",
                    "details": err.Error(),
                })
                c.Abort()
                return
            }

            // Re-attach validated body to context
            ctx = context.WithValue(ctx, "validated_body", body)
        }

        c.Next(ctx)
    }
}
```

**Schemas:**
```json
// schemas/create_file_request.json
{
  "type": "object",
  "required": ["directory_path", "name", "content_type"],
  "properties": {
    "directory_path": {"type": "string", "pattern": "^/.*"},
    "name": {
      "type": "string",
      "minLength": 1,
      "maxLength": 255,
      "pattern": "^[^/\\\\\\x00-\\x1f\\x7f]+$"
    },
    "content_type": {"type": "string"},
    "size": {"type": "integer", "maximum": 104857600}
  }
}
```

#### **Event Emission Middleware** (`events.go`)
```go
package middleware

type EventEmitter struct {
    db *gorm.DB
}

func (m *EventEmitter) Handler() app.HandlerFunc {
    return func(ctx context.Context, c *app.RequestContext) {
        // Store original response writer
        recorder := &responseRecorder{ResponseWriter: c.Response}
        c.Response = recorder

        c.Next(ctx)

        // After handler executes
        if recorder.statusCode >= 200 && recorder.statusCode < 300 {
            // Emit event based on route and method
            eventType := mapRouteToEvent(c.FullPath(), c.Method())
            aggregateID := extractAggregateID(recorder.body)
            requestID := ctx.Value("requestID").(string)

            m.emitEvent(ctx, eventType, aggregateID, requestID, recorder.body)
        }
    }
}
```

**Event Mapping:**
- `POST /api/v1/directories` → `directory.created`
- `DELETE /api/v1/directories/*` → `directory.deleted`
- `POST /api/v1/files` → `file.created`
- `PUT /api/v1/files/*` → `file.updated`
- `DELETE /api/v1/files/*` → `file.deleted`

#### **Observability Middleware** (`observability.go`)
```go
package middleware

func TracingMiddleware() app.HandlerFunc {
    return func(ctx context.Context, c *app.RequestContext) {
        start := time.Now()
        path := c.FullPath()
        method := string(c.Method())

        // Start distributed trace
        ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", method, path))
        defer span.End()

        c.Next(ctx)

        // Record metrics
        duration := time.Since(start)
        metrics.RequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
        metrics.RequestCount.WithLabelValues(method, path, strconv.Itoa(c.Response.StatusCode())).Inc()

        // Log request
        log.Info().
            Str("method", method).
            Str("path", path).
            Int("status", c.Response.StatusCode()).
            Dur("duration", duration).
            Msg("request completed")
    }
}
```

---

### 2. Repository Layer (`pkg/repository/`)

#### **Interfaces** (`interfaces.go`)
```go
package repository

type DirectoryRepository interface {
    Create(ctx context.Context, dir *models.Directory) error
    FindByID(ctx context.Context, id string) (*models.Directory, error)
    FindByPath(ctx context.Context, path string) (*models.Directory, error)
    FindByParentID(ctx context.Context, parentID string) ([]*models.Directory, error)
    Update(ctx context.Context, dir *models.Directory) error
    Delete(ctx context.Context, id string) error
    SoftDelete(ctx context.Context, id string) error
    LockPaths(ctx context.Context, tx Transaction, paths []string) error
}

type FileRepository interface {
    Create(ctx context.Context, file *models.File) error
    FindByID(ctx context.Context, id string) (*models.File, error)
    FindByDirectoryAndName(ctx context.Context, dirID, name string) (*models.File, error)
    Update(ctx context.Context, file *models.File) error
    Delete(ctx context.Context, id string) error
    CreateVersion(ctx context.Context, version *models.FileVersion) error
}

type Transaction interface {
    Commit() error
    Rollback() error
}

type UnitOfWork interface {
    BeginTransaction(ctx context.Context) (Transaction, error)
    Directories() DirectoryRepository
    Files() FileRepository
}
```

#### **GORM Implementation** (`gorm/directory_repo.go`)
```go
package gorm

type GormDirectoryRepository struct {
    db *gorm.DB
}

func (r *GormDirectoryRepository) Create(ctx context.Context, dir *models.Directory) error {
    return r.db.WithContext(ctx).Create(dir).Error
}

func (r *GormDirectoryRepository) FindByPath(ctx context.Context, path string) (*models.Directory, error) {
    var dir models.Directory
    err := r.db.WithContext(ctx).
        Where("path = ? AND deleted_at IS NULL", path).
        First(&dir).Error
    if err == gorm.ErrRecordNotFound {
        return nil, ErrNotFound
    }
    return &dir, err
}

func (r *GormDirectoryRepository) LockPaths(ctx context.Context, tx Transaction, paths []string) error {
    gormTx := tx.(*GormTransaction).tx
    for _, p := range paths {
        var dir models.Directory
        err := gormTx.WithContext(ctx).
            Where("path = ? AND deleted_at IS NULL", p).
            Clauses(clause.Locking{Strength: "UPDATE"}).
            First(&dir).Error
        if err != nil && err != gorm.ErrRecordNotFound {
            return err
        }
    }
    return nil
}
```

---

### 3. Domain Layer (`pkg/domain/`)

#### **Pure Business Logic** (`directory_service.go`)
```go
package domain

type DirectoryService struct {
    repo repository.DirectoryRepository
    uow  repository.UnitOfWork
}

func (s *DirectoryService) CreateDirectory(ctx context.Context, req CreateDirectoryRequest) (*Directory, error) {
    // NO validation here - done by middleware
    // NO event emission - done by middleware
    // NO database access - through repository

    // Calculate full path (pure business logic)
    fullPath := path.Join(req.ParentPath, req.Name)

    // Check depth limit (business rule)
    depth := strings.Count(fullPath, "/")
    if depth > MaxDirectoryDepth {
        return nil, ErrDepthLimitExceeded
    }

    // Start transaction
    tx, err := s.uow.BeginTransaction(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback()

    // Get parent directory (through repo)
    parent, err := s.repo.FindByPath(ctx, req.ParentPath)
    if err != nil {
        return nil, ErrParentNotFound
    }

    // Acquire tree lock (business rule - prevents race conditions)
    pathComponents := s.getPathComponents(fullPath)
    if err := s.repo.LockPaths(ctx, tx, pathComponents); err != nil {
        return nil, err
    }

    // Create directory entity
    dir := &models.Directory{
        ID:          uuid.New().String(),
        Name:        req.Name,
        Path:        fullPath,
        PathHash:    calculatePathHash(fullPath),
        ParentID:    &parent.ID,
        Version:     1,
        OPAPolicyID: req.OPAPolicyID,
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }

    // Persist through repository
    if err := s.repo.Create(ctx, dir); err != nil {
        return nil, err
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return nil, err
    }

    return dir, nil
}
```

**Benefits:**
- ✅ 100% unit testable with mocks
- ✅ No infrastructure dependencies
- ✅ Clear business rules
- ✅ Easy to reason about

---

### 4. Application Layer (Refactored Handlers)

#### **Simplified Handlers** (`services/vfs/handlers/directory.go`)
```go
package handlers

type DirectoryHandler struct {
    domainService *domain.DirectoryService
}

func (h *DirectoryHandler) CreateDirectory(ctx context.Context, c *app.RequestContext) {
    // Get validated request from context (set by validation middleware)
    req := ctx.Value("validated_body").(domain.CreateDirectoryRequest)

    // Get request ID from context (set by middleware)
    requestID := ctx.Value("requestID").(string)
    ctx = context.WithValue(ctx, "request_id", requestID)

    // Call domain service
    dir, err := h.domainService.CreateDirectory(ctx, req)
    if err != nil {
        // Map domain errors to HTTP status codes
        statusCode := mapErrorToStatus(err)
        c.JSON(statusCode, map[string]string{"error": err.Error()})
        return
    }

    // Format response
    response := formatDirectoryResponse(dir)
    c.JSON(201, response)

    // Event emission happens in middleware automatically
}
```

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1)
**Goal:** Set up infrastructure without breaking existing functionality

1. Create `pkg/middleware/` package structure
2. Create `pkg/repository/` package with interfaces
3. Create `pkg/domain/` package structure
4. Add dependency injection container (e.g., `wire` or `fx`)
5. Write comprehensive tests for each layer

**Deliverables:**
- [ ] Middleware package skeleton
- [ ] Repository interfaces defined
- [ ] Domain models separated from persistence models
- [ ] Unit test framework for domain layer

### Phase 2: Validation Layer (Week 2)
**Goal:** Extract and centralize validation

1. Create JSON schemas for all request types
2. Implement `ValidationMiddleware`
3. Remove validation from service methods
4. Add validation tests

**Migration Strategy:**
- Run both old and new validation in parallel
- Log differences
- Switch to new validation after confidence

**Deliverables:**
- [ ] `pkg/middleware/validation.go` implemented
- [ ] JSON schemas in `schemas/` directory
- [ ] All validation tests passing
- [ ] Documentation on schema format

### Phase 3: Authorization Layer (Week 2)
**Goal:** Integrate OPA as middleware

1. Create `AuthorizationMiddleware`
2. Define action mapping (route + method → action)
3. Integrate with existing OPA service
4. Add authorization tests

**Features:**
- Extract user context from JWT
- Path-based policy lookup with inheritance
- Fail-closed on errors
- 200ms timeout enforcement

**Deliverables:**
- [ ] `pkg/middleware/authorization.go` implemented
- [ ] OPA integration tests
- [ ] Authorization bypass tests (negative testing)
- [ ] Documentation on policy format

### Phase 4: Repository Layer (Week 3)
**Goal:** Abstract data access

1. Implement GORM repositories
2. Refactor services to use repositories
3. Create mock repositories for testing
4. Add unit tests for domain logic

**Pattern:**
```go
// Before
func (s *FileService) CreateFile(...) {
    s.db.Create(&file) // Direct GORM usage
}

// After
func (s *FileService) CreateFile(...) {
    s.repo.Create(ctx, &file) // Through interface
}
```

**Deliverables:**
- [ ] `pkg/repository/gorm/` implementations
- [ ] All services refactored to use repositories
- [ ] Mock repositories created
- [ ] Domain layer fully unit tested

### Phase 5: Event Emission Layer (Week 4)
**Goal:** Centralize event handling

1. Create `EventEmitterMiddleware`
2. Remove `emitEvent` from services
3. Implement response interception
4. Add event emission tests

**Event Flow:**
```
Handler → Domain Service → Repository → Response → Middleware → Event Emission
```

**Deliverables:**
- [ ] `pkg/middleware/events.go` implemented
- [ ] Event emission removed from services
- [ ] Event emission tests passing
- [ ] Exactly-once semantics preserved

### Phase 6: Observability Layer (Week 4)
**Goal:** Unified observability

1. Create `ObservabilityMiddleware`
2. Integrate distributed tracing
3. Add structured logging
4. Implement metrics collection

**Deliverables:**
- [ ] `pkg/middleware/observability.go` implemented
- [ ] Prometheus metrics endpoint
- [ ] Distributed tracing working
- [ ] Structured logs with request context

### Phase 7: Testing & Documentation (Week 5)
**Goal:** Comprehensive testing and docs

1. Write integration tests for middleware chain
2. Add performance benchmarks
3. Document architecture
4. Create migration guide

**Test Coverage Goals:**
- Domain layer: 90%+ (unit tests)
- Middleware: 80%+ (unit + integration)
- Repositories: 70%+ (integration tests)
- E2E: All critical paths covered

**Deliverables:**
- [ ] Architecture documentation
- [ ] Test coverage reports
- [ ] Migration guide for developers
- [ ] Performance benchmark results

---

## Success Criteria

### Functional Requirements
- ✅ All existing tests pass
- ✅ No regressions in functionality
- ✅ Authorization integrated into request flow
- ✅ Event emission still exactly-once
- ✅ Response times within 10% of baseline

### Code Quality
- ✅ Domain layer has zero infrastructure dependencies
- ✅ Services can be unit tested without database
- ✅ Middleware is composable and reusable
- ✅ Clear separation between layers
- ✅ All layers have > 70% test coverage

### Developer Experience
- ✅ Easy to add new middleware
- ✅ Easy to mock repositories for testing
- ✅ Clear where to add new features
- ✅ Reduced time to implement new endpoints
- ✅ Better error messages and debugging

---

## Risk Assessment

### High Risk
**Database Transaction Boundaries**
- **Risk:** Repository abstraction might complicate transaction management
- **Mitigation:** Use Unit of Work pattern, comprehensive transaction tests

**Performance Overhead**
- **Risk:** Additional middleware layers add latency
- **Mitigation:** Benchmark each middleware, optimize hot paths, cache validation schemas

### Medium Risk
**Breaking Changes**
- **Risk:** Refactoring might introduce bugs
- **Mitigation:** Run old and new code in parallel, gradual rollout, extensive testing

**Learning Curve**
- **Risk:** Team needs to learn new patterns
- **Mitigation:** Comprehensive documentation, pair programming, code reviews

### Low Risk
**Event Emission Ordering**
- **Risk:** Middleware might emit events in wrong order
- **Mitigation:** Response recorder pattern, integration tests

---

## Alternative Approaches Considered

### 1. **Keep Current Architecture**
- ❌ Validation remains scattered
- ❌ Testing requires full infrastructure
- ❌ Authorization never gets integrated
- ❌ Technical debt grows

### 2. **Full DDD with CQRS**
- ✅ Ultimate separation of concerns
- ❌ Overkill for this project size
- ❌ Requires event sourcing infrastructure
- ❌ Team expertise gap

### 3. **Hexagonal Architecture**
- ✅ Similar benefits to proposed approach
- ❌ More complex port/adapter model
- ❌ Harder to explain to team
- ✅ Could be future evolution

**Recommendation:** Proceed with proposed layered architecture as a pragmatic middle ground.

---

## Conclusion

This layered architecture refactoring will transform the codebase from a tightly-coupled service-oriented design to a clean, testable, and extensible architecture. The benefits include:

1. **Better Testing:** Domain logic fully unit testable
2. **Cleaner Code:** Each layer has single responsibility
3. **Easier Onboarding:** Clear structure, obvious where code belongs
4. **Extensibility:** Add new features via middleware
5. **Authorization:** Finally integrate OPA policies properly
6. **Observability:** Unified tracing and metrics

The implementation can be done incrementally over 5 weeks with minimal risk to existing functionality.

---

## Appendix: Directory Structure

```
pkg/
├── middleware/          # NEW: Cross-cutting concerns
│   ├── authorization.go
│   ├── validation.go
│   ├── events.go
│   └── observability.go
├── repository/          # NEW: Data access abstraction
│   ├── interfaces.go
│   ├── errors.go
│   └── gorm/           # GORM implementations
│       ├── directory_repo.go
│       ├── file_repo.go
│       ├── transaction.go
│       └── unit_of_work.go
├── domain/              # REFACTORED: Pure business logic
│   ├── directory/
│   │   ├── service.go
│   │   ├── models.go
│   │   └── errors.go
│   └── file/
│       ├── service.go
│       ├── models.go
│       └── errors.go
├── services/            # EXISTING: Becomes "Application" layer
│   ├── directory_service.go  # Thin wrapper over domain
│   ├── file_service.go
│   └── opa_service.go
└── idempotency/         # EXISTING: Already middleware
    └── middleware.go

schemas/                 # NEW: JSON validation schemas
├── create_directory_request.json
├── create_file_request.json
├── update_file_request.json
└── move_file_request.json

services/vfs/
└── handlers/            # NEW: HTTP handlers separate from main.go
    ├── directory.go
    ├── file.go
    └── errors.go
```

---

**Document Version:** 1.0
**Last Updated:** 2025-10-03
**Author:** Claude (Sonnet 4.5)
**Status:** Proposal

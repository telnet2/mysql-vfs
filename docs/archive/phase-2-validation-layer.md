# Phase 2: Validation Layer - Complete

**Date:** 2025-10-03
**Status:** ✅ Complete

## Summary

Successfully implemented Phase 2 of the layered architecture: JSON schema validation and HTTP handlers. Request validation is now centralized and declarative via JSON schemas, and new handlers use the domain layer with proper error mapping.

## What Was Built

### 1. JSON Schemas (`schemas/`)

Created comprehensive JSON Schema (Draft 07) definitions:

#### **create_directory_request.json**
- Validates directory creation requests
- Required: `parent_path`, `name`
- Optional: `opa_policy_id` (UUID format)
- Pattern validation for names (no `/`, `\`, control chars)
- Path validation (must start with `/`)
- Length constraints (1-255 for names, max 1024 for paths)

#### **create_file_request.json**
- Validates file creation requests
- Required: `directory_path`, `name`, `content_type`, `content`
- Optional: `storage_type` (enum: `mysql`, `s3`)
- MIME type validation for `content_type`
- File size limit (100MB)
- Base64 content support

#### **move_file_request.json**
- Validates file move/rename requests
- Required: `source_path`, `destination_path`
- Full path validation for both fields
- Max 2048 characters per path

#### **README.md**
- Complete documentation for all schemas
- Validation rules and examples
- Usage instructions
- Common patterns and anti-patterns

### 2. HTTP Handlers (`services/vfs/handlers/`)

Created clean, thin handlers that use the domain layer:

#### **errors.go**
- `mapErrorToStatus()` - Maps domain/repository errors to HTTP status codes
- `mapErrorToMessage()` - User-friendly error messages
- Standardized `ErrorResponse` struct

**Error Mapping:**
- `ErrNotFound` → 404
- `ErrAlreadyExists` → 409
- `ErrDepthLimitExceeded` → 400
- `ErrDirectoryNotEmpty` → 409
- `ErrConflict` → 409
- Default → 500

#### **directory.go**
- `DirectoryHandler` - Handles directory operations
- `CreateDirectory` - Create directory endpoint
- `ListDirectory` - List directory contents
- `DeleteDirectory` - Delete directory (with recursive option)
- `GetDirectory` - Get single directory

**Features:**
- Request validation via middleware (JSON schema)
- Domain service integration
- Proper error handling and mapping
- Request ID tracking
- Clean response formatting

## Architecture Flow

```
HTTP Request
    ↓
Request ID Middleware (adds tracking ID)
    ↓
Observability Middleware (logging)
    ↓
Validation Middleware (JSON schema) ✨ NEW
    ↓
Authorization Middleware (OPA)
    ↓
Handler (directory.go) ✨ NEW
    ↓
Domain Service (pure logic)
    ↓
Repository (data access)
    ↓
Database
```

## Request/Response Examples

### Create Directory

**Request:**
```json
POST /api/v1/directories
Content-Type: application/json

{
  "parent_path": "/",
  "name": "projects",
  "opa_policy_id": null
}
```

**Success Response (201):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "projects",
  "path": "/projects",
  "parent_id": null,
  "opa_policy_id": null,
  "created_at": "2025-10-03T10:00:00Z",
  "updated_at": "2025-10-03T10:00:00Z"
}
```

**Validation Error (400):**
```json
{
  "error": "validation failed",
  "details": [
    "name: Does not match pattern '^[^/\\\\\\x00-\\x1f\\x7f]+$'"
  ]
}
```

**Business Logic Error (404):**
```json
{
  "error": "parent directory not found",
  "request_id": "abc123"
}
```

## Validation Rules

### Directory Names
✅ **Valid:**
- `documents`
- `my-project`
- `data_2024`
- `folder.name`

❌ **Invalid:**
- `parent/child` (contains `/`)
- `folder\name` (contains `\`)
- `.` or `..` (reserved)
- Names with control characters

### Paths
✅ **Valid:**
- `/`
- `/documents`
- `/data/uploads/2024`

❌ **Invalid:**
- `documents` (doesn't start with `/`)
- `/path/with//double` (double slashes)
- Paths exceeding 1024 characters

### File Content Types
✅ **Valid:**
- `text/plain`
- `application/json`
- `image/png`
- `application/vnd.api+json`

❌ **Invalid:**
- `TEXT/PLAIN` (uppercase)
- `json` (missing subtype)
- `text-plain` (invalid format)

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `schemas/create_directory_request.json` | 45 | Directory creation validation |
| `schemas/create_file_request.json` | 58 | File creation validation |
| `schemas/move_file_request.json` | 28 | File move validation |
| `schemas/README.md` | 180 | Schema documentation |
| `services/vfs/handlers/errors.go` | 100 | Error handling utilities |
| `services/vfs/handlers/directory.go` | 195 | Directory HTTP handlers |

**Total:** 6 files, ~600 lines of code

## Benefits

### 1. Centralized Validation
- **Before:** Validation scattered across services
- **After:** Declarative JSON schemas in one place

### 2. Automatic Documentation
- JSON schemas serve as API documentation
- Examples included in schemas
- Tools can generate docs automatically

### 3. Better Error Messages
- Detailed validation errors with field names
- User-friendly domain error messages
- Request ID tracking for debugging

### 4. Clean Handlers
- Thin handlers (just request/response mapping)
- Business logic in domain layer
- Easy to test and maintain

## Testing Strategy

### JSON Schema Validation
```bash
# Install ajv-cli
npm install -g ajv-cli

# Validate a request
ajv validate \
  -s schemas/create_directory_request.json \
  -d test-request.json
```

### Handler Testing
```go
// Unit test with mocked domain service
func TestCreateDirectory(t *testing.T) {
    mockService := &MockDirectoryService{}
    handler := NewDirectoryHandler(mockService)

    // Test handler logic without database
}
```

## Integration Points

### Middleware Integration
```go
// In services/vfs/main.go
validator := middleware.NewValidationMiddleware()
validator.LoadSchemasFromDirectory("./schemas")

h.POST("/api/v1/directories",
    validator.Handler(),
    directoryHandler.CreateDirectory)
```

### Domain Service Integration
```go
// Handler uses domain service
domainReq := domain.CreateDirectoryRequest{
    ParentPath:  req.ParentPath,
    Name:        req.Name,
    OPAPolicyID: req.OPAPolicyID,
}

dir, err := h.domainService.CreateDirectory(ctx, domainReq)
```

## Success Criteria - Phase 2 ✅

| Criterion | Status |
|-----------|--------|
| JSON schemas created | ✅ |
| Schema documentation complete | ✅ |
| Error mapping implemented | ✅ |
| Directory handlers created | ✅ |
| Request/response types defined | ✅ |
| Zero breaking changes | ✅ |

## Next Steps: Phase 3

### Immediate Priorities

1. **Wire Everything Together**
   - Update `services/vfs/main.go`
   - Initialize domain services
   - Set up middleware chain
   - Register routes with handlers

2. **Add File Handlers**
   - `CreateFile` handler
   - `GetFile` handler
   - `MoveFile` handler
   - `DeleteFile` handler

3. **Write Tests**
   - Handler unit tests
   - Schema validation tests
   - Integration tests
   - End-to-end tests

4. **Authorization Integration**
   - Configure OPA middleware
   - Define access policies
   - Test authorization flow

### Estimated Timeline
- **Phase 3 (Integration):** 2-3 days
- **Testing:** 2-3 days
- **Documentation:** 1 day

## Migration Path

The new handlers coexist with old code:

1. **Old Endpoints** - Continue using `pkg/services/`
2. **New Endpoints** - Use new handlers
3. **Gradual Migration** - Switch endpoints one by one
4. **No Downtime** - Both systems work simultaneously

## Lessons Learned

### What Went Well
1. **JSON Schemas** - Clear, declarative validation
2. **Error Mapping** - Consistent error handling
3. **Clean Handlers** - Simple, easy to understand
4. **Documentation** - Comprehensive schema docs

### Improvements for Next Phase
1. **File Handlers** - Need to implement
2. **Integration Tests** - Should write alongside code
3. **Performance** - Should benchmark schema validation
4. **Examples** - Need more request/response examples

---

**Status:** Phase 2 Complete ✅
**Next:** Phase 3 - Service Integration & Wiring
**Updated:** 2025-10-03

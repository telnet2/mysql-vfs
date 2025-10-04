# Directory-Level Content Validation Design

**Feature:** Schema-based content validation for files
**Author:** Claude (Sonnet 4.5)
**Date:** 2025-10-03
**Status:** Design

---

## Overview

Enable directories to enforce content validation rules on uploaded files using JSON schemas. Schemas are stored in `/system/schemas/` (admin-only) and associated with directories via a `schema_id` field.

---

## Use Cases

### 1. Structured Data Directories
```
/data/users/          → schema: user.json
/data/products/       → schema: product.json
/config/              → schema: config.json
```

**Benefits:**
- Ensure all user data follows the same structure
- Prevent malformed configuration files
- Enforce API contracts for data exchange

### 2. API Data Exchange
```
/api/uploads/orders/  → schema: order-v1.json
/api/uploads/events/  → schema: event-v1.json
```

**Benefits:**
- Validate incoming API payloads
- Version schemas (order-v1, order-v2)
- Automatic data quality enforcement

### 3. Data Pipelines
```
/pipeline/input/      → schema: input-data.json
/pipeline/output/     → schema: output-data.json
```

**Benefits:**
- Ensure pipeline input quality
- Validate transformations
- Catch errors early

---

## Architecture

### High-Level Flow

```
User uploads file to /data/users/john.json
    ↓
FileService.CreateFile()
    ↓
Look up directory /data/users/
    ↓
Check if directory.schema_id is set
    ↓
If set, fetch schema from /system/schemas/{schema_id}
    ↓
Validate file content against schema
    ↓
If valid: Create file
If invalid: Return validation errors
```

### Database Schema

```sql
-- Add schema_id to directories table
ALTER TABLE directories
ADD COLUMN schema_id VARCHAR(255) NULL,
ADD COLUMN enforce_schema BOOLEAN DEFAULT false;

-- Create schemas table
CREATE TABLE content_schemas (
  id VARCHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL UNIQUE,
  description TEXT,
  schema_content JSON NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  created_by VARCHAR(255),

  INDEX idx_name (name)
);
```

### Domain Model

```go
// Directory with schema
type Directory struct {
    // ... existing fields
    SchemaID      *string `json:"schema_id,omitempty"`
    EnforceSchema bool    `json:"enforce_schema"`
}

// ContentSchema represents a validation schema
type ContentSchema struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`
    Description   string    `json:"description"`
    SchemaContent string    `json:"schema_content"` // JSON schema
    Version       int       `json:"version"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
    CreatedBy     string    `json:"created_by"`
}
```

---

## API Design

### 1. Schema Management

#### Create Schema
```http
POST /api/v1/system/schemas
Authorization: Bearer {admin-token}

{
  "name": "user-profile",
  "description": "User profile data schema",
  "schema_content": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "required": ["email", "name"],
    "properties": {
      "email": {"type": "string", "format": "email"},
      "name": {"type": "string", "minLength": 1},
      "age": {"type": "integer", "minimum": 0}
    }
  }
}

Response (201):
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "user-profile",
  "version": 1,
  "created_at": "2025-10-03T10:00:00Z"
}
```

#### List Schemas
```http
GET /api/v1/system/schemas
Authorization: Bearer {admin-token}

Response (200):
{
  "schemas": [
    {
      "id": "550e8400-...",
      "name": "user-profile",
      "description": "User profile data schema",
      "version": 1,
      "created_at": "2025-10-03T10:00:00Z"
    }
  ]
}
```

#### Get Schema
```http
GET /api/v1/system/schemas/user-profile
Authorization: Bearer {admin-token}

Response (200):
{
  "id": "550e8400-...",
  "name": "user-profile",
  "description": "User profile data schema",
  "schema_content": { ... },
  "version": 1
}
```

### 2. Directory Schema Association

#### Assign Schema to Directory
```http
PATCH /api/v1/directories/data/users
Authorization: Bearer {admin-token}

{
  "schema_id": "user-profile",
  "enforce_schema": true
}

Response (200):
{
  "id": "abc123",
  "path": "/data/users",
  "schema_id": "user-profile",
  "enforce_schema": true
}
```

### 3. File Upload with Validation

#### Upload Valid File
```http
POST /api/v1/files
Content-Type: application/json

{
  "directory_path": "/data/users",
  "name": "john.json",
  "content_type": "application/json",
  "content": "{\"email\":\"john@example.com\",\"name\":\"John Doe\",\"age\":30}"
}

Response (201):
{
  "id": "file123",
  "name": "john.json",
  "path": "/data/users/john.json",
  "validated_against_schema": "user-profile"
}
```

#### Upload Invalid File
```http
POST /api/v1/files
Content-Type: application/json

{
  "directory_path": "/data/users",
  "name": "invalid.json",
  "content_type": "application/json",
  "content": "{\"email\":\"not-an-email\",\"age\":-5}"
}

Response (400):
{
  "error": "content validation failed",
  "schema": "user-profile",
  "details": [
    "email: Does not match format 'email'",
    "name: Required property is missing",
    "age: Must be >= 0"
  ]
}
```

---

## Implementation

### 1. Add Schema Field to Directory Model

```go
// pkg/models/directory.go
type Directory struct {
    ID            string     `gorm:"primaryKey;type:varchar(36)"`
    Name          string     `gorm:"type:varchar(255);not null"`
    Path          string     `gorm:"type:varchar(1024);uniqueIndex;not null"`
    PathHash      string     `gorm:"type:varchar(64);uniqueIndex;not null"`
    ParentID      *string    `gorm:"type:varchar(36);index"`
    Version       int64      `gorm:"not null;default:1"`
    OPAPolicyID   *string    `gorm:"type:varchar(36)"`
    SchemaID      *string    `gorm:"type:varchar(255)"`      // NEW
    EnforceSchema bool       `gorm:"default:false"`          // NEW
    CreatedAt     time.Time  `gorm:"not null"`
    UpdatedAt     time.Time  `gorm:"not null"`
    DeletedAt     *time.Time `gorm:"index"`
}
```

### 2. Create ContentSchema Model

```go
// pkg/models/content_schema.go
package models

import "time"

type ContentSchema struct {
    ID            string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
    Name          string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"name"`
    Description   string    `gorm:"type:text" json:"description"`
    SchemaContent string    `gorm:"type:json;not null" json:"schema_content"`
    Version       int       `gorm:"not null;default:1" json:"version"`
    CreatedAt     time.Time `gorm:"not null" json:"created_at"`
    UpdatedAt     time.Time `gorm:"not null" json:"updated_at"`
    CreatedBy     string    `gorm:"type:varchar(255)" json:"created_by"`
}

func (ContentSchema) TableName() string {
    return "content_schemas"
}
```

### 3. Create Schema Repository

```go
// pkg/repository/interfaces.go
type SchemaRepository interface {
    Create(ctx context.Context, schema *models.ContentSchema) error
    FindByID(ctx context.Context, id string) (*models.ContentSchema, error)
    FindByName(ctx context.Context, name string) (*models.ContentSchema, error)
    List(ctx context.Context, limit int, cursor string) ([]*models.ContentSchema, string, error)
    Update(ctx context.Context, schema *models.ContentSchema) error
    Delete(ctx context.Context, id string) error
}
```

### 4. Add Content Validator to Domain Layer

```go
// pkg/domain/content_validator.go
package domain

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/xeipuuv/gojsonschema"
    "github.com/telnet2/mysql-vfs/pkg/repository"
)

type ContentValidator struct {
    schemaRepo repository.SchemaRepository
}

func NewContentValidator(schemaRepo repository.SchemaRepository) *ContentValidator {
    return &ContentValidator{schemaRepo: schemaRepo}
}

// ValidateContent validates file content against a schema
func (v *ContentValidator) ValidateContent(ctx context.Context, schemaName, content, contentType string) error {
    // Only validate JSON content
    if contentType != "application/json" {
        return nil // Skip validation for non-JSON files
    }

    // Fetch schema
    schema, err := v.schemaRepo.FindByName(ctx, schemaName)
    if err != nil {
        return fmt.Errorf("failed to fetch schema: %w", err)
    }

    // Parse schema
    schemaLoader := gojsonschema.NewStringLoader(schema.SchemaContent)
    jsonSchema, err := gojsonschema.NewSchema(schemaLoader)
    if err != nil {
        return fmt.Errorf("invalid schema: %w", err)
    }

    // Parse content
    var contentData interface{}
    if err := json.Unmarshal([]byte(content), &contentData); err != nil {
        return fmt.Errorf("invalid JSON content: %w", err)
    }

    // Validate
    documentLoader := gojsonschema.NewGoLoader(contentData)
    result, err := jsonSchema.Validate(documentLoader)
    if err != nil {
        return fmt.Errorf("validation error: %w", err)
    }

    if !result.Valid() {
        errors := make([]string, 0, len(result.Errors()))
        for _, desc := range result.Errors() {
            errors = append(errors, desc.String())
        }
        return &ContentValidationError{
            SchemaName: schemaName,
            Errors:     errors,
        }
    }

    return nil
}

// ContentValidationError represents content validation failure
type ContentValidationError struct {
    SchemaName string
    Errors     []string
}

func (e *ContentValidationError) Error() string {
    return fmt.Sprintf("content validation failed against schema '%s'", e.SchemaName)
}
```

### 5. Integrate into FileService

```go
// pkg/domain/file_service.go
type FileService struct {
    uow       repository.UnitOfWork
    validator *ContentValidator
}

func (s *FileService) CreateFile(ctx context.Context, req CreateFileRequest) (*models.File, error) {
    // ... existing code

    // Get directory
    dir, err := s.uow.Directories().FindByPath(ctx, req.DirectoryPath)
    if err != nil {
        return nil, err
    }

    // Validate content if schema is enforced
    if dir.EnforceSchema && dir.SchemaID != nil {
        if err := s.validator.ValidateContent(ctx, *dir.SchemaID, req.Content, req.ContentType); err != nil {
            return nil, err
        }
    }

    // ... continue with file creation
}
```

---

## Security Model

### Access Control

```
/system/                     → OPA policy: admin-only
/system/schemas/             → OPA policy: admin-only
/system/schemas/user.json    → OPA policy: admin-only

/data/users/                 → OPA policy: can inherit or custom
```

**Rules:**
1. Only admins can create/update/delete schemas
2. Only admins can assign schemas to directories
3. Regular users can read schemas (for debugging)
4. Schema enforcement happens automatically on file upload

### OPA Policy Example

```rego
# /system/schemas/ - admin only
package vfs.authz

allow {
    input.user.roles[_] == "admin"
    startswith(input.resource.path, "/system/schemas/")
}

# Regular users can read schemas for debugging
allow {
    input.action == "read"
    startswith(input.resource.path, "/system/schemas/")
}
```

---

## Migration Strategy

### Phase 1: Database Migration
```sql
-- Add columns to directories
ALTER TABLE directories
ADD COLUMN schema_id VARCHAR(255) NULL,
ADD COLUMN enforce_schema BOOLEAN DEFAULT false;

-- Create schemas table
CREATE TABLE content_schemas (
  id VARCHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL UNIQUE,
  description TEXT,
  schema_content JSON NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  created_by VARCHAR(255),
  INDEX idx_name (name)
);
```

### Phase 2: Code Implementation
1. Add ContentSchema model
2. Create SchemaRepository
3. Add ContentValidator
4. Update FileService
5. Create schema handlers
6. Add schema management endpoints

### Phase 3: Bootstrapping
```bash
# Create /system/ directory
POST /api/v1/directories
{
  "parent_path": "/",
  "name": "system",
  "opa_policy_id": "admin-only-policy"
}

# Create /system/schemas/ directory
POST /api/v1/directories
{
  "parent_path": "/system",
  "name": "schemas",
  "opa_policy_id": "admin-only-policy"
}
```

---

## Example Workflows

### Workflow 1: Setup User Data Validation

```bash
# 1. Admin creates user schema
POST /api/v1/system/schemas
{
  "name": "user-profile",
  "schema_content": {
    "type": "object",
    "required": ["email", "name"],
    "properties": {
      "email": {"type": "string", "format": "email"},
      "name": {"type": "string"},
      "age": {"type": "integer", "minimum": 0}
    }
  }
}

# 2. Admin creates /data/users directory
POST /api/v1/directories
{
  "parent_path": "/data",
  "name": "users"
}

# 3. Admin assigns schema to directory
PATCH /api/v1/directories/data/users
{
  "schema_id": "user-profile",
  "enforce_schema": true
}

# 4. User uploads valid file (succeeds)
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "john.json",
  "content": "{\"email\":\"john@example.com\",\"name\":\"John\",\"age\":30}"
}

# 5. User uploads invalid file (fails)
POST /api/v1/files
{
  "directory_path": "/data/users",
  "name": "invalid.json",
  "content": "{\"name\":\"Jane\"}"  # Missing required 'email'
}
# Returns: 400 Bad Request with validation errors
```

### Workflow 2: Schema Versioning

```bash
# Create v1 schema
POST /api/v1/system/schemas
{
  "name": "order-v1",
  "schema_content": { "type": "object", "required": ["id"] }
}

# Create v2 schema (with new fields)
POST /api/v1/system/schemas
{
  "name": "order-v2",
  "schema_content": {
    "type": "object",
    "required": ["id", "customer_id"]  # Added customer_id
  }
}

# Migrate directory from v1 to v2
PATCH /api/v1/directories/orders
{
  "schema_id": "order-v2"
}
```

---

## Benefits

### 1. Data Quality
- ✅ Ensure consistent data structure
- ✅ Catch errors at upload time
- ✅ Prevent malformed data

### 2. Self-Documenting
- ✅ Schemas serve as documentation
- ✅ Clear contracts for data exchange
- ✅ Version tracking

### 3. Developer Experience
- ✅ Clear validation errors
- ✅ Schema inheritance possible
- ✅ Easy to test with tools

### 4. Operational
- ✅ Reduce downstream errors
- ✅ Simplify data pipelines
- ✅ Enable automated validation

---

## Future Enhancements

### 1. Schema Inheritance
```
/data/                 → schema: base-data
/data/users/           → schema: user (extends base-data)
```

### 2. Multiple Schemas per Directory
```
/api/uploads/          → accepts: [order-v1, order-v2]
```

### 3. Conditional Validation
```
Only validate .json files
Skip validation for .txt files
```

### 4. Schema Templates
```
Common schemas: email, phone, address, etc.
```

### 5. Validation Modes
```
- strict: Reject invalid content
- warn: Allow but log warnings
- disabled: No validation
```

---

## Testing Strategy

### Unit Tests
```go
func TestContentValidator_ValidateContent(t *testing.T) {
    // Test valid content
    // Test invalid content
    // Test missing schema
    // Test non-JSON content
}
```

### Integration Tests
```go
func TestFileService_CreateFileWithValidation(t *testing.T) {
    // Test file upload to directory with schema
    // Test validation success
    // Test validation failure
}
```

### E2E Tests
```bash
# Create schema
# Assign to directory
# Upload valid file (expect 201)
# Upload invalid file (expect 400)
```

---

## Documentation

### User Guide
- How to create schemas
- How to assign schemas to directories
- How to debug validation errors

### Admin Guide
- Schema management best practices
- Security considerations
- Performance impact

### API Reference
- Schema endpoints
- Request/response examples
- Error codes

---

**Status:** Design Complete
**Next:** Implementation
**Estimated Effort:** 2-3 days

# Metadata and On-Behalf-Of Actor Implementation Plan

**Date**: 2025-10-06
**Status**: Planning
**Related Docs**:
- `docs/AUTHORIZATION.md`
- `docs/SYSTEM_FILES.md`
- `pkg/etc/schemas/file.metadata.schema.json`
- `pkg/etc/schemas/directory.metadata.schema.json`

---

## Problem Statement

### Current Issues

1. **No Metadata Population**: Files and directories created via API do not populate the `metadata` JSON field, even though:
   - Database schema has `metadata` column on `directories`, `files`, and `file_versions` tables
   - JSON schemas define required fields: `owner` (required), `creator` (required)
   - Bootstrap process correctly populates metadata for system files

2. **No On-Behalf-Of Semantics**: The system cannot distinguish between:
   - **Actor**: Who physically performed the operation (e.g., `automation-service`)
   - **Principal**: On whose behalf the operation was performed (e.g., `alice@example.com`)

3. **Missing User Context Extraction**: Current code has placeholder:
   ```go
   func (s *FileService) getUserContext(ctx context.Context) events.UserContext {
       // TODO: Extract from actual auth context
       // For now, return a default user
       return events.UserContext{
           UserID: "system",
           Groups: []string{"system-admin"},
       }
   }
   ```

### Schema Requirements

**file.metadata.schema.json** and **directory.metadata.schema.json**:
```json
{
  "required": ["owner", "creator"],
  "properties": {
    "owner": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9-]*[a-z0-9]$",
      "minLength": 2,
      "maxLength": 64,
      "description": "Owner user or group ID"
    },
    "creator": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9-]*[a-z0-9]$",
      "minLength": 2,
      "maxLength": 64,
      "description": "Original creator user ID"
    },
    "system": {
      "type": "boolean",
      "description": "System-managed resource flag"
    },
    "readonly": {
      "type": "boolean",
      "description": "Immutable resource flag"
    },
    "custom": {
      "type": "object",
      "description": "User-defined metadata (free-form JSON)"
    }
  }
}
```

---

## Industry Patterns for On-Behalf-Of

### RFC 8693 - OAuth 2.0 Token Exchange

**Actor Claim (`act`)**: Identifies who is currently acting
```json
{
  "sub": "alice@example.com",      // The principal (owner)
  "act": {                         // The actor (who's acting on behalf)
    "sub": "automation-service",
    "client_id": "jenkins-ci"
  }
}
```

**Nested delegation** for chains:
```json
{
  "sub": "end-user",
  "act": {
    "sub": "api-gateway",
    "act": {
      "sub": "backend-service"      // Deepest = earliest actor
    }
  }
}
```

### Kubernetes Impersonation

HTTP headers:
```http
Impersonate-User: alice@example.com
Impersonate-Group: developers
Impersonate-Extra-reason: debugging
```

Authorization: Actor must have `impersonate` verb permission

### AWS IAM

**Source Identity** (actor tracking):
```bash
aws sts assume-role \
  --role-arn arn:aws:iam::123456789012:role/MyRole \
  --source-identity jenkins-automation
```
- Persists across role chains
- Logged in CloudTrail for audit

---

## Proposed Solution

### 1. On-Behalf-Of Actor Authentication

#### Security Model

**CRITICAL**: Headers are **requests**, not trusted facts. The system MUST:
1. ✅ Authenticate the actor (from token, not header)
2. ✅ Authorize delegation (check impersonate permission)
3. ✅ Audit all attempts (success + failure)
4. ✅ Validate operation (as principal, not actor)

**Attack scenario without validation**:
```http
# Attacker injects header to create files as admin
Authorization: Bearer attacker-token
X-VFS-On-Behalf-Of: admin@example.com  # ← ATTACKER CONTROLS THIS!
```
**Result without validation**: Privilege escalation ❌
**Result with validation**: 403 Forbidden (logged) ✅

#### Option A: Authorization-Protected Headers (Recommended)

**HTTP Headers**:
```http
Authorization: Bearer <token>              # REQUIRED: Establishes actor identity
X-VFS-On-Behalf-Of: alice@example.com      # VALIDATED: Requires impersonate permission
X-VFS-Delegation-Reason: scheduled-backup  # Optional: Audit trail
```

**Request Context Structure**:
```go
// pkg/middleware/auth_context.go
type AuthContext struct {
    // Actor: Who is making the request (authenticated user/service)
    ActorUserID string
    ActorGroups []string

    // Principal: On whose behalf (if different from Actor)
    // Empty if actor is acting for themselves
    PrincipalUserID string

    // Audit trail
    DelegationReason string
    RequestID        string
    Timestamp        time.Time
}

func (a *AuthContext) GetOwner() string {
    // Owner is the principal if delegation, otherwise the actor
    if a.PrincipalUserID != "" {
        return a.PrincipalUserID
    }
    return a.ActorUserID
}

func (a *AuthContext) GetCreator() string {
    // Creator is always the actor
    return a.ActorUserID
}

func (a *AuthContext) IsDelegated() bool {
    return a.PrincipalUserID != "" && a.PrincipalUserID != a.ActorUserID
}
```

**Secure Middleware Extraction**:
```go
// pkg/middleware/auth_context.go
func ExtractAuthContext(r *http.Request) (*AuthContext, error) {
    // STEP 1: AUTHENTICATE (establish actor identity from TRUSTED source)
    // This comes from cryptographic verification (JWT signature, API key hash, mTLS cert)
    // NOT from user-controllable headers!
    actorUserID, groups, err := authenticateRequest(r)
    if err != nil {
        return nil, fmt.Errorf("authentication failed: %w", err)
    }

    authCtx := &AuthContext{
        ActorUserID: actorUserID,
        ActorGroups: groups,
        RequestID:   getRequestID(r),
        Timestamp:   time.Now(),
    }

    // STEP 2: CHECK FOR DELEGATION HEADER
    principalUserID := r.Header.Get("X-VFS-On-Behalf-Of")

    if principalUserID == "" {
        // No delegation - actor is acting for themselves
        return authCtx, nil
    }

    // STEP 3: CRITICAL SECURITY CHECK
    // Verify actor has permission to impersonate principal
    // Header is just a REQUEST - we must validate it!
    if err := validateImpersonation(actorUserID, principalUserID, groups); err != nil {
        // LOG SECURITY EVENT - potential attack attempt
        logSecurityEvent(r, "impersonation_denied", map[string]interface{}{
            "actor":      actorUserID,
            "principal":  principalUserID,
            "reason":     err.Error(),
            "remote_ip":  r.RemoteAddr,
            "user_agent": r.UserAgent(),
        })
        return nil, fmt.Errorf("impersonation denied: %w", err)
    }

    // STEP 4: ONLY NOW DO WE TRUST THE HEADER
    authCtx.PrincipalUserID = principalUserID
    authCtx.DelegationReason = r.Header.Get("X-VFS-Delegation-Reason")

    // LOG SUCCESSFUL DELEGATION for audit trail
    logSecurityEvent(r, "impersonation_granted", map[string]interface{}{
        "actor":     actorUserID,
        "principal": principalUserID,
        "reason":    authCtx.DelegationReason,
    })

    return authCtx, nil
}

// validateImpersonation enforces delegation authorization
func validateImpersonation(actor, principal string, groups []string) error {
    // Prevent self-impersonation (no-op, but flag suspicious behavior)
    if actor == principal {
        return fmt.Errorf("self-impersonation not allowed")
    }

    // METHOD 1: Group-based authorization (simple but effective)
    hasImpersonatePermission := false
    for _, group := range groups {
        if group == "service-accounts" || group == "system-admin" {
            hasImpersonatePermission = true
            break
        }
    }

    if !hasImpersonatePermission {
        return fmt.Errorf("user '%s' not in authorized groups for impersonation", actor)
    }

    // METHOD 2: Policy-based authorization (more flexible)
    // Check Rego policy for impersonate permission
    decision, err := evaluateRegoPolicy("impersonate", map[string]interface{}{
        "actor":     actor,
        "principal": principal,
        "groups":    groups,
        "action":    "impersonate",
    })
    if err != nil {
        return fmt.Errorf("policy evaluation failed: %w", err)
    }
    if !decision.Allow {
        return fmt.Errorf("policy denied: %s", decision.Reason)
    }

    // METHOD 3: Explicit allow-list (most restrictive)
    // Optional: Check database/config for specific actor->principal mappings
    // Example: backup-service can only impersonate backup-user
    if err := checkDelegationAllowList(actor, principal); err != nil {
        return err
    }

    return nil
}

// logSecurityEvent logs authentication/authorization events for audit
func logSecurityEvent(r *http.Request, eventType string, details map[string]interface{}) {
    entry := map[string]interface{}{
        "timestamp":  time.Now().Format(time.RFC3339),
        "event_type": eventType,
        "request_id": getRequestID(r),
        "path":       r.URL.Path,
        "method":     r.Method,
    }

    for k, v := range details {
        entry[k] = v
    }

    logJSON, _ := json.Marshal(entry)
    log.Printf("SECURITY: %s", string(logJSON))
}
```

**Context Storage**:
```go
// Store in request context
ctx := context.WithValue(r.Context(), "authContext", authCtx)
```

#### Authorization Integration

**Rego Policy** (`/.rego`):
```rego
package vfs.authz

# ========== IMPERSONATION AUTHORIZATION ==========

# Define who can impersonate (act on behalf of others)
can_impersonate {
    input.user.groups[_] == "service-accounts"
}

can_impersonate {
    input.user.groups[_] == "system-admin"
}

# Explicitly deny regular users from impersonating
# (defense in depth - middleware also checks)
deny_impersonate["Regular users cannot impersonate"] {
    input.principal != ""
    input.principal != input.user.user_id
    not can_impersonate
}

# ========== OPERATION AUTHORIZATION ==========

# For delegated operations:
# 1. Actor must have impersonate permission
# 2. Operation is authorized as the principal (not actor)
allow {
    input.principal != ""
    input.principal != input.user.user_id
    can_impersonate

    # Authorize the operation as if principal made the request
    authorize_operation(input.principal, input.action, input.resource)
}

# For direct operations (no delegation):
# Authorize as the actor
allow {
    input.principal == ""
    authorize_operation(input.user.user_id, input.action, input.resource)
}

# For self-impersonation (no-op but we allow it):
allow {
    input.principal == input.user.user_id
    authorize_operation(input.user.user_id, input.action, input.resource)
}

# ========== HELPER RULES ==========

# Check if user can perform operation on resource
authorize_operation(user_id, action, resource) {
    # Example: Check ownership
    resource.metadata.owner == user_id
    action == "read"
}

authorize_operation(user_id, action, resource) {
    # Example: Check if user is in authorized groups
    user_groups := get_user_groups(user_id)
    user_groups[_] == "admin"
}

# Get user's groups (would query /.group file in real implementation)
get_user_groups(user_id) = groups {
    # Stub - would actually load from /.group
    groups := ["developers"]
}
```

**Security Properties**:
- ✅ Impersonation requires explicit permission (`can_impersonate`)
- ✅ Operations authorized as principal (not actor)
- ✅ Denials are explicit and logged
- ✅ Self-impersonation handled safely

---

### 2. Metadata Population

#### Metadata Structure

**Standard metadata** (for all user-created resources):
```json
{
  "owner": "alice@example.com",
  "creator": "automation-service",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z"
}
```

**System metadata** (for bootstrap resources like `/etc`):
```json
{
  "owner": "system-admin",
  "creator": "system-admin",
  "system": true,
  "readonly": true
}
```

**With custom fields** (user-defined):
```json
{
  "owner": "alice@example.com",
  "creator": "automation-service",
  "system": false,
  "custom": {
    "project": "data-pipeline",
    "environment": "production",
    "cost-center": "engineering"
  }
}
```

#### Implementation in Domain Services

**Directory Creation** (`pkg/domain/directory_service.go`):
```go
func (s *DirectoryService) tryCreateDirectory(ctx context.Context, parentPath, fullPath, name string) (*models.Directory, error) {
    // Extract auth context
    authCtx := s.getAuthContext(ctx)

    // ... existing authorization/validation logic ...

    // Build metadata
    metadata := s.buildMetadata(authCtx, nil)
    metadataJSON, err := json.Marshal(metadata)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal metadata: %w", err)
    }
    metadataStr := string(metadataJSON)

    // Create directory with metadata
    dir = &models.Directory{
        ID:        uuid.New().String(),
        Name:      name,
        Path:      fullPath,
        PathHash:  pathHash,
        Version:   1,
        Metadata:  &metadataStr,  // ← NEW
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    // ... rest of creation logic ...
}

func (s *DirectoryService) buildMetadata(authCtx *AuthContext, custom map[string]interface{}) map[string]interface{} {
    metadata := map[string]interface{}{
        "owner":      authCtx.GetOwner(),
        "creator":    authCtx.GetCreator(),
        "system":     false,
        "created_at": time.Now().Format(time.RFC3339),
    }

    // Add delegation info if present
    if authCtx.IsDelegated() {
        metadata["delegated"] = true
        if authCtx.DelegationReason != "" {
            metadata["delegation_reason"] = authCtx.DelegationReason
        }
    }

    // Add custom fields if provided
    if custom != nil {
        metadata["custom"] = custom
    }

    return metadata
}
```

**File Creation** (`pkg/domain/file_service.go`):
```go
func (s *FileService) CreateFile(ctx context.Context, directoryPath, name, contentType string, size int64, content io.Reader) (*models.File, error) {
    // Extract auth context
    authCtx := s.getAuthContext(ctx)

    // ... existing validation/storage logic ...

    err = s.db.Transaction(func(tx *gorm.DB) error {
        // ... directory lookup, validation ...

        // Build metadata
        metadata := s.buildMetadata(authCtx, nil)
        metadataJSON, err := json.Marshal(metadata)
        if err != nil {
            return fmt.Errorf("failed to marshal metadata: %w", err)
        }
        metadataStr := string(metadataJSON)

        // Create file record with metadata
        file = &models.File{
            ID:             uuid.New().String(),
            DirectoryID:    dir.ID,
            Name:           name,
            ContentType:    contentType,
            SizeBytes:      size,
            StorageType:    storageType,
            JSONContent:    jsonContent,
            TextContent:    textContent,
            S3Key:          s3Key,
            ChecksumSHA256: checksum,
            Version:        1,
            Metadata:       &metadataStr,  // ← NEW
            CreatedAt:      time.Now(),
            UpdatedAt:      time.Now(),
        }

        // ... create file record ...

        // Create initial version with metadata
        version := &models.FileVersion{
            ID:             uuid.New().String(),
            FileID:         file.ID,
            VersionNumber:  1,
            ContentType:    contentType,
            SizeBytes:      size,
            StorageType:    storageType,
            JSONContent:    jsonContent,
            TextContent:    textContent,
            S3Key:          s3Key,
            ChecksumSHA256: checksum,
            Metadata:       &metadataStr,  // ← NEW
            CreatedAt:      time.Now(),
        }

        // ... create version record ...
    })

    // ... rest of logic ...
}
```

**File Update** (`pkg/domain/file_service.go`):
```go
func (s *FileService) UpdateFile(ctx context.Context, filePath, contentType string, size int64, content io.Reader, expectedVersion int64) (*models.File, error) {
    // Extract auth context
    authCtx := s.getAuthContext(ctx)

    // ... existing logic ...

    err = s.db.Transaction(func(tx *gorm.DB) error {
        // ... lock file, validate version ...

        // Preserve existing metadata, update modified_at
        var existingMetadata map[string]interface{}
        if file.Metadata != nil {
            json.Unmarshal([]byte(*file.Metadata), &existingMetadata)
        } else {
            existingMetadata = make(map[string]interface{})
        }

        // Add update tracking
        existingMetadata["updated_at"] = time.Now().Format(time.RFC3339)
        existingMetadata["updated_by"] = authCtx.GetCreator()

        metadataJSON, _ := json.Marshal(existingMetadata)
        metadataStr := string(metadataJSON)

        // Update file
        file.Metadata = &metadataStr  // ← UPDATED

        // ... update file ...

        // Create new version with metadata
        version := &models.FileVersion{
            // ... fields ...
            Metadata:       &metadataStr,  // ← NEW
        }

        // ... create version ...
    })

    // ... rest of logic ...
}
```

#### Helper Functions

**Extract Auth Context** (`pkg/domain/directory_service.go` and `pkg/domain/file_service.go`):
```go
func (s *DirectoryService) getAuthContext(ctx context.Context) *AuthContext {
    authCtx := ctx.Value("authContext")
    if authCtx == nil {
        // Fallback for non-HTTP calls (tests, internal operations)
        return &AuthContext{
            ActorUserID: "system",
            ActorGroups: []string{"system-admin"},
        }
    }
    return authCtx.(*AuthContext)
}
```

---

### 3. API Endpoints Enhancement

#### Add Metadata Query Parameter

**Create File with Custom Metadata**:
```http
POST /api/v1/files?path=/data/report.json&metadata={"project":"pipeline","env":"prod"}
Content-Type: application/json
X-VFS-On-Behalf-Of: alice@example.com

{...file content...}
```

**Implementation**:
```go
func (h *FileHandler) CreateFile(w http.ResponseWriter, r *http.Request) {
    // ... existing path/content parsing ...

    // Parse custom metadata
    var customMetadata map[string]interface{}
    if metadataParam := r.URL.Query().Get("metadata"); metadataParam != "" {
        if err := json.Unmarshal([]byte(metadataParam), &customMetadata); err != nil {
            http.Error(w, "invalid metadata JSON", http.StatusBadRequest)
            return
        }
    }

    // Store in context for domain service
    ctx := context.WithValue(r.Context(), "customMetadata", customMetadata)

    // Call service
    file, err := h.fileService.CreateFile(ctx, dirPath, name, contentType, size, content)
    // ...
}
```

---

### 4. CLI Support

#### Add Flags for On-Behalf-Of

**Import command**:
```bash
# Normal operation
vfs-cli import local.txt /data/

# On-behalf-of operation
vfs-cli import local.txt /data/ --on-behalf-of=alice@example.com --reason="scheduled-backup"

# With custom metadata
vfs-cli import local.txt /data/ --metadata='{"project":"pipeline","env":"prod"}'
```

**Implementation** (`cli/cmd/import.go`):
```go
var (
    onBehalfOf string
    delegationReason string
    customMetadata string
)

var importCmd = &cobra.Command{
    Use:   "import <local-path> <vfs-path>",
    Short: "Import a local file to VFS",
    Run: func(cmd *cobra.Command, args []string) {
        // ... existing logic ...

        // Add delegation headers
        if onBehalfOf != "" {
            client.SetHeader("X-VFS-On-Behalf-Of", onBehalfOf)
        }
        if delegationReason != "" {
            client.SetHeader("X-VFS-Delegation-Reason", delegationReason)
        }

        // Add metadata query param
        if customMetadata != "" {
            // Validate JSON
            var metadata map[string]interface{}
            if err := json.Unmarshal([]byte(customMetadata), &metadata); err != nil {
                fmt.Fprintf(os.Stderr, "Invalid metadata JSON: %v\n", err)
                os.Exit(1)
            }
            // Add to request
            req.SetQueryParam("metadata", customMetadata)
        }

        // ... perform upload ...
    },
}

func init() {
    importCmd.Flags().StringVar(&onBehalfOf, "on-behalf-of", "", "Import on behalf of another user")
    importCmd.Flags().StringVar(&delegationReason, "reason", "", "Delegation reason for audit trail")
    importCmd.Flags().StringVar(&customMetadata, "metadata", "", "Custom metadata as JSON")
    rootCmd.AddCommand(importCmd)
}
```

---

### 5. Audit Logging

#### Log Delegation Operations

**Audit Log Entry**:
```json
{
  "timestamp": "2025-10-06T10:00:00Z",
  "request_id": "req-123",
  "operation": "file.create",
  "resource_path": "/data/report.json",
  "actor": {
    "user_id": "automation-service",
    "groups": ["service-accounts"]
  },
  "principal": {
    "user_id": "alice@example.com"
  },
  "delegation": {
    "reason": "scheduled-backup",
    "validated": true
  },
  "outcome": "success",
  "metadata": {
    "owner": "alice@example.com",
    "creator": "automation-service"
  }
}
```

**Implementation** (`pkg/middleware/audit_logger.go`):
```go
func LogOperation(ctx context.Context, operation string, resourcePath string, outcome string, err error) {
    authCtx := ctx.Value("authContext").(*AuthContext)

    entry := map[string]interface{}{
        "timestamp":     time.Now().Format(time.RFC3339),
        "request_id":    authCtx.RequestID,
        "operation":     operation,
        "resource_path": resourcePath,
        "outcome":       outcome,
        "actor": map[string]interface{}{
            "user_id": authCtx.ActorUserID,
            "groups":  authCtx.ActorGroups,
        },
    }

    // Add delegation info if present
    if authCtx.IsDelegated() {
        entry["principal"] = map[string]interface{}{
            "user_id": authCtx.PrincipalUserID,
        }
        entry["delegation"] = map[string]interface{}{
            "reason":    authCtx.DelegationReason,
            "validated": true,
        }
    }

    // Add error if present
    if err != nil {
        entry["error"] = err.Error()
    }

    // Log as JSON
    logJSON, _ := json.Marshal(entry)
    log.Printf("AUDIT: %s", string(logJSON))
}
```

---

### 6. Testing Strategy

#### Unit Tests

**Test metadata population** (`pkg/domain/file_service_test.go`):
```go
func TestCreateFile_PopulatesMetadata(t *testing.T) {
    // Setup
    authCtx := &AuthContext{
        ActorUserID: "alice@example.com",
        ActorGroups: []string{"developers"},
    }
    ctx := context.WithValue(context.Background(), "authContext", authCtx)

    // Create file
    file, err := fileService.CreateFile(ctx, "/", "test.txt", "text/plain", 5, strings.NewReader("hello"))
    require.NoError(t, err)

    // Verify metadata
    require.NotNil(t, file.Metadata)

    var metadata map[string]interface{}
    err = json.Unmarshal([]byte(*file.Metadata), &metadata)
    require.NoError(t, err)

    assert.Equal(t, "alice@example.com", metadata["owner"])
    assert.Equal(t, "alice@example.com", metadata["creator"])
    assert.Equal(t, false, metadata["system"])
}

func TestCreateFile_OnBehalfOf_PopulatesMetadata(t *testing.T) {
    // Setup delegation
    authCtx := &AuthContext{
        ActorUserID:      "automation-service",
        ActorGroups:      []string{"service-accounts"},
        PrincipalUserID:  "alice@example.com",
        DelegationReason: "scheduled-backup",
    }
    ctx := context.WithValue(context.Background(), "authContext", authCtx)

    // Create file
    file, err := fileService.CreateFile(ctx, "/", "test.txt", "text/plain", 5, strings.NewReader("hello"))
    require.NoError(t, err)

    // Verify metadata
    var metadata map[string]interface{}
    json.Unmarshal([]byte(*file.Metadata), &metadata)

    assert.Equal(t, "alice@example.com", metadata["owner"])
    assert.Equal(t, "automation-service", metadata["creator"])
    assert.Equal(t, true, metadata["delegated"])
    assert.Equal(t, "scheduled-backup", metadata["delegation_reason"])
}
```

#### Integration Tests

**Test delegation authorization** (`citest/delegation_test.go`):
```go
var _ = Describe("On-Behalf-Of Delegation", Ordered, func() {
    It("should allow service accounts to create files on behalf of users", func() {
        // Authenticate as service account
        client.SetHeader("Authorization", "Bearer service-account-token")
        client.SetHeader("X-VFS-On-Behalf-Of", "alice@example.com")

        // Create file
        resp, err := client.POST("/api/v1/files?path=/data/test.txt", "hello")
        Expect(err).NotTo(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(201))

        // Verify metadata
        var file models.File
        db.Where("name = ?", "test.txt").First(&file)

        var metadata map[string]interface{}
        json.Unmarshal([]byte(*file.Metadata), &metadata)
        Expect(metadata["owner"]).To(Equal("alice@example.com"))
        Expect(metadata["creator"]).To(Equal("service-account"))
    })

    It("should reject delegation for users without impersonate permission", func() {
        // Authenticate as regular user
        client.SetHeader("Authorization", "Bearer alice-token")
        client.SetHeader("X-VFS-On-Behalf-Of", "bob@example.com")

        // Attempt to create file
        resp, err := client.POST("/api/v1/files?path=/data/test.txt", "hello")
        Expect(err).NotTo(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(403))
        Expect(resp.Body).To(ContainSubstring("delegation not allowed"))
    })
})
```

---

## Implementation Phases

### Phase 1: Auth Context Infrastructure (1-2 days)

1. Create `pkg/middleware/auth_context.go`
   - Define `AuthContext` struct
   - Implement `ExtractAuthContext()` middleware
   - Add delegation validation logic

2. Update HTTP handlers to use middleware
   - `services/vfs/main.go`: Add middleware to router
   - All handlers receive enriched context

3. Update domain services
   - Replace placeholder `getUserContext()` with `getAuthContext()`
   - Extract `AuthContext` from `context.Context`

**Deliverable**: Auth context flows from HTTP request → middleware → domain services

### Phase 2: Metadata Population (1-2 days)

1. Add metadata helper functions
   - `buildMetadata()` in directory_service.go
   - `buildMetadata()` in file_service.go

2. Update creation methods
   - `CreateDirectory()`: Add metadata field
   - `CreateFile()`: Add metadata field
   - `CreateFileVersion()`: Add metadata field

3. Update file update method
   - `UpdateFile()`: Preserve metadata, add `updated_by`

**Deliverable**: All user-created resources have populated metadata

### Phase 3: Authorization Integration (1 day)

1. Update `.rego` policy
   - Add `allow_impersonation` rules
   - Add delegation authorization checks

2. Add delegation validation
   - Check actor has impersonate permission
   - Integrate with existing authorization middleware

**Deliverable**: Delegation requires explicit permission

### Phase 4: API & CLI Support (1 day)

1. Add API query parameters
   - `?metadata={}` for custom metadata
   - Document in OpenAPI spec

2. Add CLI flags
   - `--on-behalf-of` flag
   - `--reason` flag
   - `--metadata` flag

**Deliverable**: Users can set metadata and delegate via API/CLI

### Phase 5: Audit Logging (1 day)

1. Create `pkg/middleware/audit_logger.go`
   - Implement `LogOperation()`
   - Log delegation operations with full context

2. Integrate into handlers
   - Log all create/update/delete operations
   - Include actor, principal, and delegation reason

**Deliverable**: Comprehensive audit trail for all operations

### Phase 6: Testing (1-2 days)

1. Unit tests
   - Test metadata population
   - Test delegation context extraction
   - Test authorization checks

2. Integration tests
   - Test E2E delegation flow
   - Test authorization enforcement
   - Test metadata persistence

**Deliverable**: Full test coverage for new functionality

### Phase 7: Documentation (1 day)

1. Write design document
   - `docs/ON_BEHALF_OF.md`: Delegation design and patterns
   - `docs/METADATA.md`: Metadata structure and usage

2. Write how-to guides
   - How to use delegation in API
   - How to use delegation in CLI
   - How to query metadata

3. Update existing docs
   - Update `docs/AUTHORIZATION.md` with delegation rules
   - Update `docs/USER_GUIDE.md` with examples

**Deliverable**: Complete documentation for delegation and metadata

---

## Open Questions

1. **Authentication Method**: What authentication system should we use?
   - JWT tokens?
   - API keys?
   - mTLS certificates?
   - Multiple methods?

2. **User Identity Format**: What format for user IDs?
   - Email addresses? (`alice@example.com`)
   - Short names? (`alice`)
   - UUIDs? (`123e4567-e89b-12d3-a456-426614174000`)

3. **Group Management**: How are groups managed?
   - Via `/.group` file? (current system)
   - External LDAP/AD?
   - Both?

4. **Delegation Scope**: Should delegation be scoped?
   - All operations?
   - Specific operations only?
   - Time-limited?
   - Resource-limited?

5. **Metadata Validation**: Should we validate custom metadata?
   - Allow any JSON?
   - Require schema registration?
   - Size limits?

6. **Backward Compatibility**: How to handle existing resources without metadata?
   - Backfill with default metadata?
   - Leave NULL and treat as legacy?
   - Lazy population on first update?

---

## Success Criteria

- [ ] All user-created files have `owner` and `creator` metadata
- [ ] All user-created directories have `owner` and `creator` metadata
- [ ] Service accounts can create resources on behalf of users
- [ ] Delegation requires explicit `impersonate` permission
- [ ] All delegation operations are logged in audit trail
- [ ] CLI supports `--on-behalf-of` flag
- [ ] API supports `X-VFS-On-Behalf-Of` header
- [ ] Custom metadata can be set via API/CLI
- [ ] Authorization checks work for both actor and principal
- [ ] Full test coverage (unit + integration)
- [ ] Complete documentation

---

## Future Enhancements

1. **Metadata Search**: Query files by metadata properties
   - `GET /api/v1/files?metadata.project=pipeline`

2. **Metadata Indexing**: Add database indexes for common metadata fields
   - Index on `metadata->>'owner'`
   - Index on `metadata->>'creator'`

3. **Delegation Audit Dashboard**: UI for viewing delegation operations
   - Who acted on whose behalf?
   - What resources were created?
   - Delegation patterns and anomalies

4. **Metadata Validation**: Register schemas for custom metadata
   - Store in `/etc/metadata-schemas/`
   - Validate on create/update

5. **Time-Limited Delegation**: Delegation tokens with expiration
   - Issue delegation token valid for 1 hour
   - Automatically expire old delegations

6. **Resource-Scoped Delegation**: Limit delegation to specific paths
   - Alice can delegate `/data/reports/*` to automation-service
   - But not `/data/secrets/*`

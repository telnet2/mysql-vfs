# System Admin Refactoring

**Date:** 2025-10-04
**Type:** Terminology Update & Consistency Fix

---

## Summary

Renamed "super user" terminology to "system admin" throughout the codebase and fixed role consistency issues.

### Key Changes

1. **Terminology Update**: `SUPER_USER_*` → `SYSTEM_ADMIN_*`
2. **Role Consistency**: Default system admin role changed from `"super-admin"` → `"admin"`
3. **New Helper Function**: `IsSystemAdmin()` for consistent admin checks across codebase

---

## Motivation

### Problem 1: Inconsistent Role Checking
- System admin was assigned role `"super-admin"`
- Special file permissions checked for role `"admin"`
- **Result**: System admin couldn't create special files unless `SUPER_USER_ROLE=admin` was manually configured

### Problem 2: Unclear Terminology
- "Super user" is informal and less professional
- "System admin" is more formal and industry-standard

---

## Changes Made

### 1. Configuration (pkg/config/config.go)

**Before:**
```go
SuperUserToken string // default: ""
SuperUserID    string // default: "super-admin"
SuperUserRole  string // default: "super-admin"
```

**After:**
```go
SystemAdminToken string // default: ""
SystemAdminID    string // default: "system-admin"
SystemAdminRole  string // default: "admin"  ← Changed!
```

**Environment Variables:**
- `SUPER_USER_TOKEN` → `SYSTEM_ADMIN_TOKEN`
- `SUPER_USER_ID` → `SYSTEM_ADMIN_ID` (default: "system-admin")
- `SUPER_USER_ROLE` → `SYSTEM_ADMIN_ROLE` (default: "admin")

### 2. Authentication Provider (pkg/middleware/auth_providers.go)

**Before:**
```go
// NewHybridAuthExtractor wraps any auth extractor with super user check
func NewHybridAuthExtractor(cfg config.AuthConfig, baseExtractor AuthExtractor) AuthExtractor {
    if cfg.SuperUserToken != "" && tokenString == cfg.SuperUserToken {
        return AuthContext{
            UserID: cfg.SuperUserID,
            Role:   cfg.SuperUserRole,
            Groups: []string{"super-admins"},
            Metadata: map[string]interface{}{
                "auth_type": "super_user",
            },
        }, nil
    }
    return baseExtractor(tokenString)
}
```

**After:**
```go
// NewHybridAuthExtractor wraps any auth extractor with system admin check
func NewHybridAuthExtractor(cfg config.AuthConfig, baseExtractor AuthExtractor) AuthExtractor {
    if cfg.SystemAdminToken != "" && tokenString == cfg.SystemAdminToken {
        return AuthContext{
            UserID: cfg.SystemAdminID,
            Role:   cfg.SystemAdminRole,
            Groups: []string{"system-admins"},
            Metadata: map[string]interface{}{
                "auth_type": "system_admin",
            },
        }, nil
    }
    return baseExtractor(tokenString)
}
```

### 3. New Helper Function (pkg/domain/special_files.go)

**Added:**
```go
// IsSystemAdmin checks if a user has system admin or admin privileges
// This should be used consistently across the codebase for admin checks
func IsSystemAdmin(userRole string) bool {
    return userRole == "admin" || userRole == "system-admin"
}
```

**Purpose:**
- Centralizes admin role checking logic
- Recognizes both "admin" and "system-admin" roles
- Prevents inconsistencies across services

### 4. File Service Updates (pkg/domain/file_service.go)

**Before:**
```go
if RequiresAdmin(req.Name) && req.UserRole != "admin" {
    return nil, ErrPermissionDenied
}
```

**After:**
```go
if RequiresAdmin(req.Name) && !IsSystemAdmin(req.UserRole) {
    return nil, ErrPermissionDenied
}
```

**Applied to:**
- `createSpecialFile()` - Creating special files
- `UpdateFile()` - Updating special files

### 5. Middleware Auth Helper (pkg/middleware/auth.go)

**Before:**
```go
func IsAdmin(ctx context.Context) bool {
    role, ok := GetUserRole(ctx)
    return ok && role == "admin"
}
```

**After:**
```go
func IsAdmin(ctx context.Context) bool {
    role, ok := GetUserRole(ctx)
    return ok && (role == "admin" || role == "system-admin")
}
```

### 6. Documentation Updates

Updated files:
- `docs/5_AUTHENTICATION.md` - All references to super user → system admin
- `.env.example` - Added system admin configuration section

### 7. Tests

**Added test:**
```go
func TestIsSystemAdmin(t *testing.T) {
    // Tests both "admin" and "system-admin" roles
    // Ensures consistency across the codebase
}
```

---

## Migration Guide

### For Existing Deployments

**Option 1: Update Environment Variables (Recommended)**
```bash
# Old variables (deprecated)
# SUPER_USER_TOKEN=...
# SUPER_USER_ID=...
# SUPER_USER_ROLE=...

# New variables
SYSTEM_ADMIN_TOKEN=your-secure-token
SYSTEM_ADMIN_ID=system-admin
SYSTEM_ADMIN_ROLE=admin
```

**Option 2: Keep Old Config (Backward Compatible)**

The old environment variables will continue to work temporarily, but emit deprecation warnings in logs.

### For New Deployments

```bash
# Generate secure token
SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)

# Set configuration
export SYSTEM_ADMIN_TOKEN
export SYSTEM_ADMIN_ID=system-admin
export SYSTEM_ADMIN_ROLE=admin
export AUTH_PROVIDER=file
```

---

## Benefits

1. **Consistency**: System admin role now matches admin permission checks
2. **Clarity**: "system-admin" is more professional terminology
3. **Flexibility**: Both "admin" and "system-admin" roles work for admin operations
4. **Maintainability**: Centralized `IsSystemAdmin()` helper prevents future inconsistencies

---

## Breaking Changes

### Environment Variables
- `SUPER_USER_TOKEN` → `SYSTEM_ADMIN_TOKEN`
- `SUPER_USER_ID` → `SYSTEM_ADMIN_ID`
- `SUPER_USER_ROLE` → `SYSTEM_ADMIN_ROLE`

### Default Values
- Default role: `"super-admin"` → `"admin"`

### Group Names
- Auto-assigned group: `"super-admins"` → `"system-admins"`

### Metadata Keys
- Auth metadata: `"auth_type": "super_user"` → `"auth_type": "system_admin"`

---

## Testing

All tests pass:
```bash
✅ go test ./pkg/domain -v
✅ go test ./pkg/middleware -v
✅ go test ./citest -v
```

New test coverage:
- `TestIsSystemAdmin` - Validates both "admin" and "system-admin" roles

---

## Files Changed

**Code:**
- `pkg/config/config.go`
- `pkg/middleware/auth_providers.go`
- `pkg/middleware/auth.go`
- `pkg/domain/special_files.go`
- `pkg/domain/file_service.go`
- `pkg/domain/special_files_test.go`

**Configuration:**
- `.env.example`

**Documentation:**
- `docs/5_AUTHENTICATION.md`
- `CHANGELOG_SYSTEM_ADMIN.md` (this file)

---

## Next Steps

1. Update deployment configurations to use new environment variables
2. Update CI/CD pipelines with new variable names
3. Update secret management (Vault, AWS Secrets Manager) with new keys
4. Consider deprecating old environment variables in next major version

---

## Questions?

- **Q: Do I need to update my .user files?**
  A: No, `.user` files can still use `"role": "admin"` as before.

- **Q: What if I set SYSTEM_ADMIN_ROLE to something else?**
  A: Make sure your `.rego` policies recognize that role. The `IsSystemAdmin()` function only recognizes "admin" and "system-admin" by default.

- **Q: Can I have multiple system admins?**
  A: No, only one system admin token. But you can have multiple admin users in `.user` files.

- **Q: Should I use "admin" or "system-admin" as the role?**
  A: Use "admin" for normal admin users in `.user` files. "system-admin" is reserved for the token-based system admin.

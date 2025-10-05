# Role-Only Authentication Refactor

**Date:** 2025-10-04
**Type:** Major Refactor - Groups Removal & System Admin Bypass

---

## Summary

Removed group-based authorization entirely and implemented system-admin bypass for all authorization checks. The system now uses role-only authentication.

### Key Changes

1. **Removed Groups**: Groups field removed from all auth contexts and user management
2. **System Admin Bypass**: System admins now bypass ALL rego policy evaluation
3. **Role Separation**: `system-admin` and `admin` are now completely separate roles

---

## Motivation

### Problems Addressed

1. **Unused Groups Feature**: The system doesn't manage user-group relationships
2. **Unclear Authorization Model**: Mixing groups and roles created confusion
3. **System Admin Limitations**: System admin should have unrestricted access, not go through rego policies
4. **Admin vs System Admin**: Need clear separation - admin follows rego rules, system-admin doesn't

---

## Changes Made

### 1. Removed Groups from AuthContext (pkg/middleware/auth.go)

**Before:**
```go
type AuthContext struct {
    UserID   string
    Role     string
    Groups   []string
    Metadata map[string]interface{}
}
```

**After:**
```go
type AuthContext struct {
    UserID   string
    Role     string
    Metadata map[string]interface{}
}
```

**Removed:**
- `UserGroupsKey` context key
- `GetUserGroups()` helper function
- All group-related context storage

### 2. Removed Groups from UserCredential (pkg/domain/special_files.go)

**Before:**
```go
type UserCredential struct {
    UserID       string   `json:"user_id"`
    PasswordHash string   `json:"password_hash"`
    Token        string   `json:"token,omitempty"`
    Role         string   `json:"role"`
    Groups       []string `json:"groups,omitempty"`
}
```

**After:**
```go
type UserCredential struct {
    UserID       string `json:"user_id"`
    PasswordHash string `json:"password_hash"`
    Token        string `json:"token,omitempty"`
    Role         string `json:"role"`
}
```

**.user file format now:**
```json
{
  "users": [
    {
      "user_id": "admin",
      "token": "admin-token",
      "role": "admin"
    }
  ]
}
```

### 3. System Admin Bypasses Rego Authorization (pkg/middleware/authorization.go)

**Added bypass logic:**
```go
// System admin bypasses all rego authorization
if userCtx.Role == "system-admin" {
    ctx = context.WithValue(ctx, "authorized", true)
    ctx = context.WithValue(ctx, "user_context", userCtx)
    c.Next(ctx)
    return
}
```

**Flow:**
1. Extract user context from auth middleware
2. **If role is "system-admin" → Skip rego entirely, grant access**
3. Otherwise → Load and evaluate rego policy as normal

### 4. Updated UserContext for Rego (pkg/middleware/authorization.go)

**Before:**
```go
type UserContext struct {
    UserID   string
    Username string
    Roles    []string
    Groups   []string
}
```

**After:**
```go
type UserContext struct {
    UserID   string
    Username string
    Role     string
}
```

**Rego input changed from:**
```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "roles": ["admin"],
    "groups": ["engineering"]
  }
}
```

**To:**
```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "role": "admin"
  }
}
```

### 5. Updated JWT Claims (pkg/middleware/auth_providers.go)

**Before:**
```go
type JWTClaims struct {
    UserID string   `json:"user_id"`
    Role   string   `json:"role"`
    Groups []string `json:"groups"`
    jwt.RegisteredClaims
}
```

**After:**
```go
type JWTClaims struct {
    UserID string `json:"user_id"`
    Role   string `json:"role"`
    jwt.RegisteredClaims
}
```

### 6. Updated Proxy Token Format (pkg/middleware/auth_providers.go)

**Before:**
```
Token format: "userID:role:groups:timestamp:signature"
Parts: 5
```

**After:**
```
Token format: "userID:role:timestamp:signature"
Parts: 4
```

### 7. Changed SYSTEM_ADMIN_ROLE Default (pkg/config/config.go)

**Before:**
```go
SystemAdminRole: getEnv("SYSTEM_ADMIN_ROLE", "admin")
```

**After:**
```go
SystemAdminRole: getEnv("SYSTEM_ADMIN_ROLE", "system-admin")
```

### 8. Updated IsSystemAdmin Logic (pkg/domain/special_files.go)

**Before:**
```go
func IsSystemAdmin(userRole string) bool {
    return userRole == "admin" || userRole == "system-admin"
}
```

**After:**
```go
func IsSystemAdmin(userRole string) bool {
    return userRole == "system-admin"
}
```

### 9. Separated IsAdmin and IsSystemAdmin in Middleware (pkg/middleware/auth.go)

**Added:**
```go
// IsAdmin checks if the user in context is an admin
func IsAdmin(ctx context.Context) bool {
    role, ok := GetUserRole(ctx)
    return ok && role == "admin"
}

// IsSystemAdmin checks if the user in context is a system admin
func IsSystemAdmin(ctx context.Context) bool {
    role, ok := GetUserRole(ctx)
    return ok && role == "system-admin"
}
```

---

## Authorization Model

### System Admin (role: "system-admin")
- **Token-based**: Matches `SYSTEM_ADMIN_TOKEN` environment variable
- **Bypasses ALL authorization**: Never evaluates rego policies
- **Use cases**:
  - Bootstrap (create initial .user file)
  - Emergency access (corrupted policies/files)
  - System maintenance
- **Special file access**: Granted automatically
- **Cannot be defined in .user files**

### Admin (role: "admin")
- **Defined in .user files**: Regular user with admin role
- **Subject to rego policies**: Must pass authorization checks
- **Special file access**: Only if rego policies allow OR if file doesn't require admin
- **Can be restricted**: Rego policies can deny admin users

### Regular Users (role: "user", "readonly", etc.)
- **Defined in .user files**
- **Subject to rego policies**: Must pass authorization checks
- **Limited access**: Based on rego policy rules

---

## Breaking Changes

### Environment Variables
- `SYSTEM_ADMIN_ROLE` default changed from `"admin"` to `"system-admin"`

### .user File Format
**Old format (deprecated):**
```json
{
  "users": [
    {
      "user_id": "alice",
      "token": "token123",
      "role": "admin",
      "groups": ["admins", "developers"]
    }
  ]
}
```

**New format:**
```json
{
  "users": [
    {
      "user_id": "alice",
      "token": "token123",
      "role": "admin"
    }
  ]
}
```

### Rego Policy Input
**Old input:**
```rego
allow {
    input.user.role == "admin"
    "engineering" in input.user.groups  # No longer available
}
```

**New input:**
```rego
allow {
    input.user.role == "admin"
    # Only user.id, user.username, and user.role available
}
```

### JWT Token Format
**Old JWT payload:**
```json
{
  "user_id": "alice",
  "role": "admin",
  "groups": ["engineering"]
}
```

**New JWT payload:**
```json
{
  "user_id": "alice",
  "role": "admin"
}
```

### Proxy Token Format
**Old:** `userID:role:groups:timestamp:signature` (5 parts)
**New:** `userID:role:timestamp:signature` (4 parts)

---

## Migration Guide

### Step 1: Update .user Files

Remove `groups` field from all user entries:

```bash
# Before
{
  "users": [
    {
      "user_id": "alice",
      "token": "...",
      "role": "admin",
      "groups": ["admins"]  ← Remove this
    }
  ]
}

# After
{
  "users": [
    {
      "user_id": "alice",
      "token": "...",
      "role": "admin"
    }
  ]
}
```

### Step 2: Update Rego Policies

Remove all group-based authorization:

```rego
# Before
allow {
    input.user.role == "admin"
    "engineering" in input.user.groups  ← Remove
}

# After
allow {
    input.user.role == "admin"
}
```

### Step 3: Update Environment Variables

If you set `SYSTEM_ADMIN_ROLE`, update it:

```bash
# Before (if customized)
SYSTEM_ADMIN_ROLE=admin

# After (use default or set explicitly)
SYSTEM_ADMIN_ROLE=system-admin
```

### Step 4: Update JWT Integration

If you issue JWTs externally, remove `groups` claim:

```go
// Before
claims := jwt.MapClaims{
    "user_id": "alice",
    "role": "admin",
    "groups": []string{"engineering"},  // Remove
}

// After
claims := jwt.MapClaims{
    "user_id": "alice",
    "role": "admin",
}
```

### Step 5: Update Proxy Integration

If you use reverse proxy auth, update token format:

```nginx
# Before
set $message "$user_id:$role:$groups:$timestamp";

# After
set $message "$user_id:$role:$timestamp";
```

---

## Role Comparison

| Aspect | system-admin | admin | user |
|--------|-------------|-------|------|
| **Defined in** | Environment var | .user file | .user file |
| **Rego evaluation** | ❌ Bypassed | ✅ Required | ✅ Required |
| **Can be restricted** | ❌ No | ✅ Yes | ✅ Yes |
| **Special files** | ✅ Always | Depends on rego | Depends on rego |
| **Bootstrap access** | ✅ Yes | ❌ No | ❌ No |
| **Emergency access** | ✅ Yes | ❌ No | ❌ No |

---

## Testing

All tests pass:

```bash
✅ go test ./pkg/domain -v
✅ go test ./pkg/middleware -v
✅ go build ./...
```

Test updates:
- `TestIsSystemAdmin` - Updated to only check for "system-admin"
- `TestUserLoader_LoadUser` - Removed groups assertions
- All authorization tests - Updated input structure

---

## Files Changed

**Core:**
- `pkg/config/config.go` - Changed default SYSTEM_ADMIN_ROLE
- `pkg/middleware/auth.go` - Removed Groups from AuthContext
- `pkg/middleware/auth_providers.go` - Removed Groups from all extractors
- `pkg/middleware/authorization.go` - Added system-admin bypass, removed Groups
- `pkg/domain/special_files.go` - Removed Groups from UserCredential, updated IsSystemAdmin
- `pkg/domain/user_loader_test.go` - Removed Groups from test data

**Configuration:**
- `.env.example` - Updated SYSTEM_ADMIN_ROLE default

**Documentation:**
- `ROLE_ONLY_REFACTOR.md` (this file)

---

## Important Notes

### System Admin Best Practices

1. **Use Sparingly**: Only for bootstrap and emergencies
2. **Secure Token**: Use long random tokens (32+ chars)
3. **Rotate Regularly**: Change token periodically
4. **Audit Access**: Log all system-admin operations
5. **Use Admin for Daily Operations**: Regular admins should use `admin` role with rego policies

### Admin Role Best Practices

1. **Use Rego Policies**: Define clear admin permissions
2. **Principle of Least Privilege**: Grant only necessary permissions
3. **Separate Concerns**: Different admins for different areas
4. **Regular Review**: Audit admin access regularly

### Example Rego Policies

**Admin can do everything:**
```rego
package vfs.authz

allow {
    input.user.role == "admin"
}
```

**Admin can only read/write, not delete:**
```rego
package vfs.authz

allow {
    input.user.role == "admin"
    input.action != "delete"
}
```

**Admin restricted to specific paths:**
```rego
package vfs.authz

allow {
    input.user.role == "admin"
    startswith(input.resource.path, "/admin")
}
```

---

## Next Steps

1. Update production .user files to remove groups
2. Update all rego policies to remove group references
3. Update JWT issuing services to remove groups claim
4. Update reverse proxy configurations (if using proxy auth)
5. Update documentation for end users
6. Communicate changes to team

---

## Questions?

**Q: Can I still use groups?**
A: No, groups have been completely removed from the system.

**Q: How do I manage team-based access now?**
A: Use rego policies with path-based rules or create different roles.

**Q: What happens to existing .user files with groups?**
A: The `groups` field is ignored (backward compatible for reads, but should be removed).

**Q: Can system-admin be restricted?**
A: No, system-admin bypasses all authorization. Use `admin` role if you need restrictions.

**Q: Should I use `admin` or `system-admin` for daily operations?**
A: Use `admin` defined in .user files. Reserve `system-admin` for emergencies.

**Q: How do I differentiate between teams without groups?**
A: Use different roles (e.g., "engineering", "finance") or path-based rules in rego.

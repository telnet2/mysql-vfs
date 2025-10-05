# 6. Authorization with OPA Policies

**Policy-based access control using `.rego` files**

**Implementation:** `pkg/middleware/authorization.go`, `pkg/domain/policy_loader.go`

[← Back: Authentication](5_AUTHENTICATION.md) | [Index](0_README.md) | [Next: Configuration →](7_CONFIGURATION.md)

---

## Overview

MySQL VFS v2 uses **Open Policy Agent (OPA)** for authorization. Policies are stored as **`.rego` files** directly in the VFS, making access control:

- **File-based** - Policies are files, not database records
- **Version-controlled** - Track policy changes like code
- **Inheritable** - Child directories inherit parent policies
- **Flexible** - Full Rego language support

---

## How It Works

### 1. Policy Files

Create a `.rego` file in any directory:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer <token>" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\nallow { input.user.groups[_] == \"admin\" }"
  }'
```

### 2. Policy Evaluation

When a request arrives:

```
1. Authorization middleware extracts user context (from JWT/headers)
2. PolicyLoader finds .rego file (checks /data/.rego, then parent dirs)
3. OPA evaluates policy with input
4. Returns allow/deny
```

### 3. Input to Policy

The `groups` field in the user object contains the user's group memberships. Users are assigned to groups via the `.user` file. See the [Group Management](22_GROUP_MANAGEMENT.md) documentation for more details.

```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "groups": ["admin", "engineering"]
  },
  "resource": {
    "path": "/data/users/alice.json",
    "type": "file",
    "owners": ["engineering"]
  },
  "action": "read"
}
```

---

## Policy Examples

### Example 1: Admin-Only

Only users in the admin group can access this directory:

```rego
package vfs.authz

# Allow all actions for admin group
allow {
    input.user.groups[_] == "admin"
}
```

### Example 2: Read-Only for Users

User group can read, admin group can do anything:

```rego
package vfs.authz

# Admin group can do anything
allow {
    input.user.groups[_] == "admin"
}

# User group can only read
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}
```

### Example 3: Own Files Only

Users can only access files with their user ID in the path:

```rego
package vfs.authz

# Admin group can do anything
allow {
    input.user.groups[_] == "admin"
}

# User group can access their own files
allow {
    input.user.groups[_] == "user"
    contains(input.resource.path, input.user.id)
}
```

### Example 4: Group-Based Access

Only members of specific groups can access:

```rego
package vfs.authz

# Admin group always allowed
allow {
    input.user.groups[_] == "admin"
}

# Members of "engineering" group can access
allow {
    input.user.groups[_] == "engineering"
}
```

### Example 5: Time-Based Access

Allow access only during business hours:

```rego
package vfs.authz

import future.keywords.if

# Admin group always allowed
allow if {
    input.user.groups[_] == "admin"
}

# User group only during 9am-5pm UTC
allow if {
    input.user.groups[_] == "user"
    hour := time.clock(time.now_ns())[0]
    hour >= 9
    hour < 17
}
```

### Example 6: Action-Specific Rules

Different rules for different actions:

```rego
package vfs.authz

# Admin group can do anything
allow {
    input.user.groups[_] == "admin"
}

# Anyone can read
allow {
    input.action == "read"
}

# Only engineering group can write
allow {
    input.action == "write"
    input.user.groups[_] == "engineering"
}

# Only admin group can delete
allow {
    input.action == "delete"
    input.user.groups[_] == "admin"
}
```

---

## Policy Inheritance

Policies are **inherited from parent directories**:

```
/
├── .rego              ← Default: admins only
└── data/
    ├── .rego          ← Override: users can read
    └── users/
        ├── alice.json  ← Uses /data/.rego (users can read)
        └── admin/
            ├── .rego   ← Override: admins only
            └── secrets.json  ← Uses /data/users/admin/.rego
```

**Lookup Algorithm:**
1. Check `/data/users/admin/.rego`
2. If not found, check `/data/users/.rego`
3. If not found, check `/data/.rego`
4. If not found, check `/.rego`
5. If not found, **deny by default**

---

## Policy Actions

The `input.action` field contains the operation being performed:

| HTTP Method | Action |
|-------------|--------|
| GET | `read` |
| POST | `write` |
| PUT | `write` |
| DELETE | `delete` |
| PATCH | `write` |

**Example:**
```rego
# Read-only policy
allow {
    input.action == "read"
}
```

---

## Advanced Patterns

### Pattern 1: Multi-Tier Access

```rego
package vfs.authz

# Tier 1: Admin group can do anything
allow {
    input.user.groups[_] == "admin"
}

# Tier 2: Manager group can read/write
allow {
    input.user.groups[_] == "manager"
    input.action in ["read", "write"]
}

# Tier 3: User group can only read
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}
```

### Pattern 2: Resource-Specific Rules

```rego
package vfs.authz

# Special files only accessible by admin group
allow {
    input.user.groups[_] == "admin"
    startswith(input.resource.path, "/data/.")
}

# Regular files accessible by user group
allow {
    input.user.groups[_] == "user"
    not startswith(input.resource.path, "/data/.")
}
```

### Pattern 3: Conditional Write Access

```rego
package vfs.authz

# Admin group can always write
allow {
    input.user.groups[_] == "admin"
    input.action == "write"
}

# User group can write only if file doesn't exist yet
allow {
    input.user.groups[_] == "user"
    input.action == "write"
    input.resource.exists == false
}
```

---

## Testing Policies

### Local Testing with OPA CLI

```bash
# Install OPA
brew install opa

# Test policy
opa eval \
  --data policy.rego \
  --input input.json \
  'data.vfs.authz.allow'
```

**policy.rego:**
```rego
package vfs.authz

allow {
    input.user.groups[_] == "admin"
}
```

**input.json:**
```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "groups": ["admin"]
  },
  "resource": {
    "path": "/data/file.json"
  },
  "action": "read"
}
```

**Result:**
```
true
```

---

## Policy Caching

Policies are cached to improve performance:

- **TTL:** 5 minutes (configurable via `POLICY_CACHE_TTL_SECONDS`)
- **Invalidation:** Automatic when `.rego` file is updated
- **Per-Directory:** Each directory's policy is cached separately
- **Implementation:** `pkg/domain/policy_loader.go` using `GenericLoader` pattern (lines 1-200)

**Configure cache:**
```bash
export POLICY_CACHE_TTL_SECONDS=300  # 5 minutes
```

**Cache Implementation:** Policies use the generic special file loader (`pkg/domain/special_file_loader.go`) which provides:
- Thread-safe caching with TTL
- Automatic invalidation on file changes
- Directory-level inheritance
- Efficient lookup with minimal DB queries

---

## Security Best Practices

### 1. Default Deny

Always start with deny-by-default:

```rego
package vfs.authz

# Default: deny everything
default allow = false

# Explicit allow rules
allow {
    input.user.groups[_] == "admin"
}
```

### 2. Validate Input

Check that required fields exist:

```rego
allow {
    # Ensure user context exists
    input.user.id != ""
    count(input.user.groups) > 0

    # Your policy logic
    input.user.groups[_] == "admin"
}
```

### 3. Principle of Least Privilege

Grant minimal permissions:

```rego
# ❌ Bad: Too permissive
allow {
    input.user.groups[_] == "user"
}

# ✅ Good: Specific permissions
allow {
    input.user.groups[_] == "user"
    input.action == "read"
    input.resource.type == "file"
}
```

### 4. Audit Special File Access

Log access to `.rego`, `.jsonschema`, `.quota` files:

```rego
allow {
    # Regular access rules
    input.user.groups[_] == "admin"

    # Log if accessing special file
    log_if_special_file
}

log_if_special_file {
    startswith(input.resource.path, ".")
    # Trigger audit log (future enhancement)
}
```

---

## Integration with Authentication

OPA policies use the authentication context from the auth middleware:

```
JWT Token → Auth Middleware → AuthContext → OPA Input
```

**Example Flow:**
```
1. User sends JWT: { "user_id": "alice", "groups": ["admin", "engineering"] }
2. Auth middleware validates JWT
3. Creates AuthContext: { UserID: "alice", Groups: ["admin", "engineering"] }
4. Authorization middleware passes to OPA as input.user
5. Policy evaluates with full user context
```

---

## Troubleshooting

### Policy Not Working

```bash
# Check policy exists
curl http://localhost:8080/api/v1/files/data/.rego

# Check policy syntax
opa check policy.rego

# Test policy locally
opa eval --data policy.rego --input test-input.json 'data.vfs.authz.allow'
```

### Always Denied

```bash
# Check input being passed to policy
# Add debug logging in policy:

package vfs.authz

allow {
    # Debug: print input
    trace(sprintf("User: %v", [input.user]))
    trace(sprintf("Resource: %v", [input.resource]))
    trace(sprintf("Action: %v", [input.action]))

    # Your policy logic
    input.user.groups[_] == "admin"
}
```

### Cache Not Refreshing

```bash
# Reduce cache TTL for testing
export POLICY_CACHE_TTL_SECONDS=10

# Restart VFS
docker-compose restart vfs-service
```

---

## Migration from Database Policies

If you previously stored policies in a database, migrate to `.rego` files:

```bash
# For each policy in database:
# 1. Convert to Rego format
# 2. Upload as .rego file

curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer <admin-token>" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content": "<rego-policy-content>"
  }'
```

---

## Next Steps

- **[Configuration](7_CONFIGURATION.md)** - Configure policy cache settings
- **[Testing](11_TESTING.md)** - Write tests for your policies
- **[OPA Documentation](https://www.openpolicyagent.org/docs/)** - Learn more about Rego

---

[← Back: Authentication](5_AUTHENTICATION.md) | [Index](0_README.md) | [Next: Configuration →](7_CONFIGURATION.md)

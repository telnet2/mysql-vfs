# Security Guide

**Authentication, Authorization, and Access Control**

---

## Table of Contents

- [Authentication](#authentication)
- [Authorization with OPA](#authorization-with-opa)
- [Groups & Ownership](#groups--ownership)
- [Bootstrap & Setup](#bootstrap--setup)
- [Security Best Practices](#security-best-practices)

---

## Authentication

MySQL VFS supports multiple authentication methods. Choose based on your environment:

| Method | Use Case | Security | Implementation |
|--------|----------|----------|----------------|
| System Admin Token | Bootstrap, emergency access | ⚠️ Bypasses all policies | `pkg/middleware/auth.go` |
| File-Based (.user) | Production, self-contained | ✅ Good with strong tokens | `pkg/domain/user_loader.go` |
| JWT | External auth service | ✅ Industry standard | `pkg/middleware/auth.go` |
| Headers | Development only | ❌ UNSAFE | `pkg/middleware/authorization.go` |

---

### 1. System Admin Token

**Purpose:** Bootstrap and emergency access

**Setup:**
```bash
export SYSTEM_ADMIN_TOKEN="your-secure-random-token-32plus-chars"
export SYSTEM_ADMIN_ID="system-admin"
```

**Usage:**
```bash
curl http://localhost:8080/api/v1/files/... \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"
```

**Behavior:**
- Bypasses ALL `.rego` policies
- Has unrestricted access
- Should be kept secret

**Generate Secure Token:**
```bash
openssl rand -hex 32
```

**When to Use:**
- Initial setup (create directories, users, policies)
- Emergency access (policy misconfiguration)
- Administrative tasks

**Security:**
- ⚠️ Never commit to git
- ⚠️ Rotate regularly
- ⚠️ Restrict access to this token
- ✅ Use environment variables only

**Implementation:**
- Check: `pkg/middleware/authorization.go:89-102`
- System admin group bypasses OPA evaluation

---

### 2. File-Based Authentication (.user)

**Purpose:** Self-contained user management

**How It Works:**

1. Create `.user` file with user credentials
2. Users authenticate with their tokens
3. System validates token and extracts groups
4. Groups used for authorization

**Setup:**

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"alice\",\"token\":\"alice-secure-token-32chars\",\"groups\":[\"admin\"]},{\"user_id\":\"bob\",\"token\":\"bob-secure-token-32chars\",\"groups\":[\"user\",\"engineering\"]}]}"
  }'
```

**Format:**
```json
{
  "users": [
    {
      "user_id": "alice",
      "token": "alice-secure-token-32chars",
      "password_hash": "$2a$10$...",
      "groups": ["admin", "engineering"]
    }
  ]
}
```

**Fields:**
- `user_id` (required) - Unique identifier
- `token` (optional) - Static bearer token
- `password_hash` (optional) - bcrypt hash (future password auth)
- `groups` (required) - Array of group names

**Usage:**
```bash
# Alice authenticates with her token
curl http://localhost:8080/api/v1/files/data/file.json \
  -H "Authorization: Bearer alice-secure-token-32chars"
```

**Generate Tokens:**
```bash
# Generate secure random token
openssl rand -hex 32

# Generate bcrypt hash (for future password auth)
htpasswd -bnBC 10 "" password | tr -d ':\n' | sed 's/$2y/$2a/'
```

**Security:**
- ✅ Self-contained (no external dependencies)
- ✅ Cached (5-minute TTL)
- ✅ Version controlled (via VFS)
- ⚠️ Tokens must be kept secret
- ⚠️ Distribute tokens securely (not via email)

**Caching:**
- `.user` file cached for 5 minutes
- Changes take effect after cache expiration
- Cache invalidated automatically on file update

**Implementation:** `pkg/domain/user_loader.go`

---

### 3. JWT Authentication

**Purpose:** External authentication service integration

**How It Works:**

1. External service (Auth0, Okta, etc.) issues JWT
2. Client sends JWT in Authorization header
3. MySQL VFS validates JWT signature
4. Extracts user_id and groups from claims
5. Uses groups for authorization

**Setup:**

```bash
# Configure JWT settings
export AUTH_JWT_SECRET="your-jwt-secret-key-min-32-chars"
export AUTH_JWT_ISSUER="https://auth.example.com"
export AUTH_JWT_AUDIENCE="mysql-vfs"
```

**JWT Payload:**
```json
{
  "user_id": "alice",
  "groups": ["admin", "engineering"],
  "iss": "https://auth.example.com",
  "aud": "mysql-vfs",
  "exp": 1735689600,
  "iat": 1735688700
}
```

**Usage:**
```bash
curl http://localhost:8080/api/v1/files/... \
  -H "Authorization: Bearer <jwt-token>"
```

**External Auth Service Example:**

Your auth service issues tokens:
```go
// See implementation in your auth service
token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
    "user_id": "alice",
    "groups":  []string{"admin", "engineering"},
    "iss":     "https://auth.example.com",
    "exp":     time.Now().Add(15 * time.Minute).Unix(),
})
tokenString, _ := token.SignedString([]byte(jwtSecret))
```

**Security:**
- ✅ Industry standard
- ✅ Short-lived tokens (recommend 15-60 min)
- ✅ Cannot be forged without secret key
- ✅ Stateless (no session storage)

**Implementation:** `pkg/middleware/auth.go:50-97`

---

### 4. Headers (Development Only)

**Purpose:** Local development and testing

**How It Works:**

Client sends headers, VFS trusts them without validation.

**Usage:**
```bash
curl http://localhost:8080/api/v1/files/... \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: admin,engineering"
```

**Security:**
- ❌ COMPLETELY UNSAFE
- ❌ Anyone can send any headers
- ❌ No cryptographic validation
- ✅ Convenient for development

**WARNING:**
- ⚠️ NEVER use in production
- ⚠️ NEVER expose to internet
- ⚠️ Only for localhost development

**Enable:**
```bash
# Default: headers trusted in development
# To disable in production, use JWT or .user auth
```

**Implementation:** `pkg/middleware/authorization.go:182-224`

---

## Authorization with OPA

### How It Works

Authorization uses **Open Policy Agent (OPA)** with Rego policies stored in `.rego` files.

**Flow:**

```
1. Request arrives
2. Auth middleware extracts user (groups)
3. Authz middleware loads .rego policy
4. Builds OPA input (user, resource, action)
5. OPA evaluates policy
6. Allow or deny
```

**Implementation:** `pkg/middleware/authorization.go`

---

### Policy Files (.rego)

**Location:** Any directory (e.g., `/.rego`, `/data/.rego`)

**Format:** Rego (OPA policy language)

**Example:**
```rego
package vfs.authz

# Admin group has full access
allow {
    input.user.groups[_] == "admin"
}

# User group can read
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}
```

**Creating Policies:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\nallow {\n    input.user.groups[_] == \"admin\"\n}"
  }'
```

---

### OPA Input Structure

Every policy receives this input:

```json
{
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

**Fields:**

**user:**
- `id` - User identifier
- `username` - Display name
- `groups` - Array of group names

**resource:**
- `path` - File or directory path
- `type` - "file" or "directory"
- `owners` - Owner groups (from `.owner` file)

**action:**
- `read` - GET operations
- `write` - POST, PUT operations
- `delete` - DELETE operations

**Implementation:** `pkg/middleware/authorization.go:148-160`

---

### Policy Examples

**1. Admin-Only Directory:**
```rego
package vfs.authz

allow {
    input.user.groups[_] == "admin"
}
```

**2. Read-Only for Users:**
```rego
package vfs.authz

# Admin can do anything
allow {
    input.user.groups[_] == "admin"
}

# User can only read
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}
```

**3. Group-Based Access:**
```rego
package vfs.authz

# Admin has full access
allow {
    input.user.groups[_] == "admin"
}

# Engineering group can access /engineering/*
allow {
    input.user.groups[_] == "engineering"
    startswith(input.resource.path, "/engineering")
}
```

**4. Owner-Based Access:**
```rego
package vfs.authz

# Admin has full access
allow {
    input.user.groups[_] == "admin"
}

# Everyone can read
allow {
    input.action == "read"
}

# Only owners can write
allow {
    input.action == "write"
    is_owner
}

# Helper: Check if user is in owner group
is_owner {
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

**5. Time-Based Access:**
```rego
package vfs.authz

import future.keywords.if

# Admin always allowed
allow if {
    input.user.groups[_] == "admin"
}

# Users only during business hours (9am-5pm UTC)
allow if {
    input.user.groups[_] == "user"
    hour := time.clock(time.now_ns())[0]
    hour >= 9
    hour < 17
}
```

**6. Action-Specific Rules:**
```rego
package vfs.authz

# Admin can do anything
allow {
    input.user.groups[_] == "admin"
}

# Anyone can read
allow {
    input.action == "read"
}

# Engineering can write
allow {
    input.action == "write"
    input.user.groups[_] == "engineering"
}

# Only admin can delete
allow {
    input.action == "delete"
    input.user.groups[_] == "admin"
}
```

---

### Policy Inheritance

Policies inherit from parent directories:

```
/
├── .rego (admin-only)
└── data/
    ├── .rego (users can read) ← overrides parent
    └── public/
        └── (no .rego)         ← inherits from /data
```

**Lookup Algorithm:**

For request to `/data/public/file.json`:
1. Check `/data/public/.rego`
2. If not found, check `/data/.rego` ✅ Found
3. If not found, check `/.rego`
4. If not found, use default policy (deny all)

**Implementation:** `pkg/domain/policy_loader.go:38-89`

**Default Policy:**

When no `.rego` found, uses built-in default:
- Admin group → full access
- System-admin group → full access (bypasses OPA)
- User group → read-only
- User group + owner → read+write+delete

See: `pkg/middleware/default_policy.go`

---

### Testing Policies

**1. Test Locally with OPA CLI:**

```bash
# Install OPA
brew install opa  # or download from openpolicyagent.org

# Test policy
opa eval --data policy.rego --input input.json 'data.vfs.authz.allow'
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

**2. Test in VFS:**

```bash
# Create policy
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"directory_path":"/test","name":".rego","content":"..."}'

# Test with user token
curl http://localhost:8080/api/v1/files/test/file.json \
  -H "Authorization: Bearer $USER_TOKEN"
```

**3. Debug Policies:**

Add trace statements:
```rego
package vfs.authz

allow {
    trace(sprintf("User: %v", [input.user]))
    trace(sprintf("Resource: %v", [input.resource]))
    trace(sprintf("Action: %v", [input.action]))

    input.user.groups[_] == "admin"
}
```

View traces in logs (if debug mode enabled).

---

## Groups & Ownership

### Group-Based Access Control

**How It Works:**

1. Users belong to one or more groups
2. Policies check group membership
3. Flexible: `["admin", "engineering", "oncall"]`

**Define Groups (via .user file):**
```json
{
  "users": [
    {
      "user_id": "alice",
      "groups": ["admin", "engineering"]
    },
    {
      "user_id": "bob",
      "groups": ["user", "engineering"]
    }
  ]
}
```

**Policy Pattern:**
```rego
# Check if user is in admin group
allow {
    input.user.groups[_] == "admin"
}

# Check if user is in ANY of multiple groups
allow {
    allowed := ["admin", "manager"]
    input.user.groups[_] == allowed[_]
}
```

---

### Owner-Based Access

**How It Works:**

1. Directories have owner groups (via `.owner` file)
2. Users in owner groups can write/delete
3. Ownership inherits from parent directories

**Define Ownership:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "directory_path": "/projects/alpha",
    "name": ".owner",
    "content": "{\"owner_groups\":[\"engineering\",\"product\"]}"
  }'
```

**Policy with Ownership:**
```rego
package vfs.authz

# Admin has full access
allow {
    input.user.groups[_] == "admin"
}

# Everyone can read
allow {
    input.action == "read"
}

# Owners can write
allow {
    input.action == "write"
    is_owner
}

# Owners can delete
allow {
    input.action == "delete"
    is_owner
}

# Helper: Check if user is in owner group
is_owner {
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

**Result:**
- Admin group → full access
- Engineering group → read+write+delete (owner)
- Product group → read+write+delete (owner)
- User group → read-only

**Inheritance:**
```
/projects/
├── .owner (["engineering"])
└── alpha/
    └── file.json  ← owned by "engineering" (inherited)
```

**Implementation:** `pkg/domain/owner_loader.go`

---

## Bootstrap & Setup

### Initial Setup

**1. Set System Admin Token:**
```bash
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
echo "Save this token: $SYSTEM_ADMIN_TOKEN"
```

**2. Start MySQL VFS:**
```bash
docker-compose up -d
```

**3. Create Root Policy:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".rego",
    "content": "package vfs.authz\n\n# Admin group has full access\nallow {\n    input.user.groups[_] == \"admin\"\n}\n\n# System admin bypasses this policy\nallow {\n    input.user.groups[_] == \"system-admin\"\n}\n\n# User group has read-only\nallow {\n    input.user.groups[_] == \"user\"\n    input.action == \"read\"\n}"
  }'
```

**4. Create Users:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"'$(openssl rand -hex 32)'\",\"groups\":[\"admin\"]},{\"user_id\":\"alice\",\"token\":\"'$(openssl rand -hex 32)'\",\"groups\":[\"user\"]}]}"
  }'
```

**5. Test Access:**
```bash
# Get admin token from .user file
ADMIN_TOKEN=$(curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  | jq -r '.users[0].token')

# Test admin access
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

### Production Setup

**1. Use Environment Variables for Secrets:**
```bash
# Never commit these
export SYSTEM_ADMIN_TOKEN="..."
export AUTH_JWT_SECRET="..."
export S3_SECRET_KEY="..."
```

**2. Enable HTTPS:**
```bash
# Use reverse proxy (nginx, caddy)
# Or configure TLS in VFS (future)
```

**3. Rotate Tokens Regularly:**
```bash
# Generate new system admin token
NEW_TOKEN=$(openssl rand -hex 32)

# Update environment
export SYSTEM_ADMIN_TOKEN="$NEW_TOKEN"

# Restart service
docker-compose restart vfs
```

**4. Review Policies:**
```bash
# Get all policies
curl http://localhost:8080/api/v1/files/.rego \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Test with OPA CLI
opa check policy.rego
```

---

## Security Best Practices

### 1. Token Security

✅ **DO:**
- Use `openssl rand -hex 32` to generate tokens
- Store in environment variables
- Rotate regularly (quarterly)
- Use HTTPS in production
- Distribute securely (not via email)

❌ **DON'T:**
- Commit tokens to git
- Use predictable tokens
- Share system admin token
- Use header auth in production

### 2. Policy Security

✅ **DO:**
- Start with deny-by-default
- Test policies with OPA CLI
- Review policy changes via PR
- Use principle of least privilege
- Document complex policies

❌ **DON'T:**
- Use overly permissive rules
- Skip testing
- Assume policies work without testing

**Example - Secure Policy:**
```rego
package vfs.authz

# Default deny
default allow = false

# Explicit allow rules
allow {
    input.user.groups[_] == "admin"
    # Admin can do anything
}

allow {
    input.user.groups[_] == "user"
    input.action == "read"
    # Users can only read
}
```

### 3. Group Management

✅ **DO:**
- Use descriptive group names
- Document group purposes
- Review group membership regularly
- Use groups for logical roles

❌ **DON'T:**
- Give users unnecessary groups
- Use groups as roles (use actual groups)
- Create one group per user

### 4. Audit & Monitoring

✅ **DO:**
- Enable event webhooks for audit logging
- Monitor failed authorization attempts
- Review access patterns
- Set up alerts for suspicious activity

❌ **DON'T:**
- Disable logging
- Ignore failed auth events

**Audit Setup:**
```json
{
  "handlers": [
    {
      "events": ["authorization.*.failed"],
      "handler": {
        "type": "webhook",
        "url": "https://security-monitoring.internal/auth-failures"
      }
    }
  ]
}
```

### 5. Principle of Least Privilege

✅ **DO:**
- Give minimum required access
- Use read-only where possible
- Restrict delete operations
- Use time-based access for temporary needs

❌ **DON'T:**
- Give everyone admin
- Use system admin token for regular operations

---

## Troubleshooting

### Access Denied

**Symptom:** 403 Forbidden

**Check:**

1. User's groups:
```bash
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

2. Policy:
```bash
curl http://localhost:8080/api/v1/files/data/.rego \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

3. Test policy locally:
```bash
opa eval --data policy.rego --input input.json 'data.vfs.authz.allow'
```

### Policy Not Working

**Symptom:** Policy changes not taking effect

**Causes:**
- Cache (up to 5 minutes)
- Syntax error in policy
- Wrong directory for policy file

**Solutions:**
```bash
# Check policy syntax
opa check policy.rego

# Wait for cache expiry (5 min) or restart
docker-compose restart vfs

# Check policy is in correct directory
curl http://localhost:8080/api/v1/directories/data
```

### Token Issues

**Symptom:** 401 Unauthorized

**Check:**
```bash
# Verify token exists in .user file
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"

# Test with system admin token
curl http://localhost:8080/api/v1/files/... \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"
```

---

## Next Steps

- **[USER_GUIDE.md](USER_GUIDE.md)** - Learn about features and API
- **[OPERATIONS.md](OPERATIONS.md)** - Deploy and configure
- **[DESIGN.md](DESIGN.md)** - Understand architecture

---

**Security Resources:**
- [OPA Best Practices](https://www.openpolicyagent.org/docs/latest/policy-performance/)
- [JWT Best Practices](https://tools.ietf.org/html/rfc8725)
- [bcrypt Guide](https://en.wikipedia.org/wiki/Bcrypt)

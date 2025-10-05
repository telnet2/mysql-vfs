# User Guide

**Complete reference for MySQL VFS features and API**

---

## Table of Contents

- [HTTP API](#http-api)
- [Special Files](#special-files)
- [Content Validation](#content-validation)
- [Events & Webhooks](#events--webhooks)
- [File Versioning](#file-versioning)
- [Group Management](#group-management)
- [Common Patterns](#common-patterns)

---

## HTTP API

### Base URL

```
http://localhost:8080/api/v1
```

### Authentication

All requests require authentication:

```bash
# Production: Bearer token
Authorization: Bearer <token>

# Development: Headers (UNSAFE - dev only!)
X-User-ID: alice
X-User-Groups: admin,engineering
```

See: `SECURITY.md` for authentication setup

---

## File Operations

### Create File

```bash
POST /api/v1/files
Content-Type: application/json

{
  "directory_path": "/data",
  "name": "example.json",
  "content_type": "application/json",
  "content": "{\"key\":\"value\"}"
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "directory_id": "...",
  "name": "example.json",
  "content_type": "application/json",
  "size_bytes": 15,
  "version": 1,
  "created_at": "2025-10-05T12:00:00Z",
  "updated_at": "2025-10-05T12:00:00Z"
}
```

**Validation:**
- Checks `.files` config for schema validation
- Triggers `file.create.*` events
- Requires write permission (via `.rego` policy)

### Get File

```bash
GET /api/v1/files/{path}
```

**Example:**
```bash
GET /api/v1/files/data/example.json
```

**Response:**
```json
{
  "key": "value"
}
```

**Query Parameters:**
- `version=2` - Get specific version
- `metadata=true` - Return file metadata instead of content

**With metadata:**
```bash
GET /api/v1/files/data/example.json?metadata=true
```

Returns file object with metadata (id, size, timestamps, etc.)

### Update File

```bash
PUT /api/v1/files/{path}
Content-Type: application/json

{
  "content": "{\"key\":\"new value\"}",
  "content_type": "application/json"
}
```

**Behavior:**
- Creates new version (old versions retained)
- Validates against `.files` schema
- Triggers `file.update.*` events

### Delete File

```bash
DELETE /api/v1/files/{path}
```

**Behavior:**
- Soft delete (sets `deleted_at`)
- Triggers `file.delete.*` events
- Can be recovered (if not purged)

### List Files in Directory

```bash
GET /api/v1/directories/{path}
```

**Example:**
```bash
GET /api/v1/directories/data
```

**Response:**
```json
{
  "entries": [
    {
      "id": "...",
      "name": "example.json",
      "type": "file",
      "size_bytes": 15,
      "content_type": "application/json",
      "updated_at": "2025-10-05T12:00:00Z"
    },
    {
      "id": "...",
      "name": "subdirectory",
      "type": "directory",
      "updated_at": "2025-10-05T11:00:00Z"
    }
  ]
}
```

**Query Parameters:**
- `recursive=true` - Include subdirectories
- `include_deleted=true` - Show soft-deleted files

---

## Directory Operations

### Create Directory

```bash
POST /api/v1/directories
Content-Type: application/json

{
  "parent_path": "/",
  "name": "data"
}
```

**Response:**
```json
{
  "id": "...",
  "name": "data",
  "path": "/data",
  "created_at": "2025-10-05T12:00:00Z"
}
```

**Behavior:**
- Creates parent directories if needed (optional)
- Inherits policies from parent
- Triggers `directory.create.*` events

### Get Directory

```bash
GET /api/v1/directories/{path}
```

Returns directory metadata + list of files/subdirectories.

### Delete Directory

```bash
DELETE /api/v1/directories/{path}
```

**Query Parameters:**
- `recursive=true` - Delete all contents (required if not empty)

**Behavior:**
- Fails if directory not empty (unless recursive)
- Triggers `directory.delete.*` events

---

## Special Files

Files starting with `.` have special meaning and control system behavior.

### .rego - Authorization Policies

**Purpose:** Control who can access what

**Format:** Rego (OPA policy language)

**Example:**
```rego
package vfs.authz

# Admin group has full access
allow {
    input.user.groups[_] == "admin"
}

# User group can read only
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}

# Engineering group can write to /engineering/*
allow {
    input.user.groups[_] == "engineering"
    startswith(input.resource.path, "/engineering")
}
```

**Input Structure:**
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

**Inheritance:**
- Child directories inherit parent policies
- Override by creating `.rego` in child directory
- See: `SECURITY.md` for policy examples

**Implementation:** `pkg/domain/policy_loader.go`

---

### .user - User Credentials

**Purpose:** Define users and their groups

**Format:** JSON

**Example:**
```json
{
  "users": [
    {
      "user_id": "alice",
      "token": "alice-secret-token",
      "password_hash": "$2a$10$...",
      "groups": ["admin", "engineering"]
    },
    {
      "user_id": "bob",
      "token": "bob-secret-token",
      "groups": ["user"]
    }
  ]
}
```

**Fields:**
- `user_id` (required) - Unique identifier
- `token` (optional) - Static bearer token
- `password_hash` (optional) - bcrypt hash for password auth
- `groups` (required) - Array of group names

**Usage:**
```bash
# Alice authenticates with token
curl http://localhost:8080/api/v1/files/data/file.json \
  -H "Authorization: Bearer alice-secret-token"
```

**Security:**
- Tokens should be long random strings (32+ chars)
- Use `openssl rand -hex 32` to generate
- Password auth is partial (hash storage exists, login endpoint planned)

**Implementation:** `pkg/domain/user_loader.go`

**See:** `SECURITY.md` for authentication setup

---

### .group - Group Definitions

**Purpose:** Define groups and their members

**Format:** JSON

**Example:**
```json
{
  "groups": [
    {
      "group_id": "admin",
      "members": ["alice", "charlie"]
    },
    {
      "group_id": "engineering",
      "members": ["alice", "bob", "diane"]
    }
  ]
}
```

**Note:**
- Currently managed via `.user` file (groups array)
- `.group` file is for future centralized group management
- See: Group Management section below

**Implementation:** `pkg/domain/group_loader.go`

---

### .owner - Directory Ownership

**Purpose:** Define who owns a directory (for owner-based access)

**Format:** JSON

**Example:**
```json
{
  "owner_groups": ["engineering", "product"]
}
```

**Usage in Policies:**
```rego
# Users in owner groups can write
allow {
    input.user.groups[_] == "user"
    input.action == "write"
    is_owner
}

is_owner {
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

**Inheritance:**
- Child directories inherit parent ownership
- Can be overridden by child `.owner` file

**Implementation:** `pkg/domain/owner_loader.go`

**See:** `SECURITY.md` for ownership patterns

---

### .files - Content Validation

**Purpose:** Enforce schemas on file uploads

**Format:** JSON

**Example:**
```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "properties": {
          "email": {
            "type": "string",
            "format": "email"
          },
          "name": {
            "type": "string",
            "minLength": 1
          }
        },
        "required": ["email", "name"]
      }
    },
    {
      "pattern": "admin-.*\\.json",
      "type": "regex",
      "schema": {
        "required": ["user_id", "groups"]
      }
    }
  ]
}
```

**Pattern Types:**
- `glob` - Shell-style patterns (`*.json`, `data/*.txt`)
- `regex` - Regular expressions (`admin-.*\.json`)

**Schema:**
- Standard JSON Schema (draft-07)
- Supports all JSON Schema features (required, format, minLength, etc.)

**Validation Flow:**
1. File upload
2. Match filename against patterns
3. If match found, validate content
4. Reject if validation fails

**Inheritance:**
- Rules inherit from parent directories
- Child rules merged with parent rules

**Implementation:** `pkg/domain/files_loader.go`

---

### .events - Lifecycle Event Handlers

**Purpose:** React to file/directory operations

**Format:** JSON

**Example:**
```json
{
  "handlers": [
    {
      "events": ["file.create.*", "file.update.*"],
      "handler": {
        "type": "webhook",
        "url": "https://api.example.com/webhook",
        "headers": {
          "X-API-Key": "secret"
        }
      }
    },
    {
      "events": ["file.delete.*"],
      "handler": {
        "type": "log",
        "level": "warn"
      }
    }
  ]
}
```

**Event Patterns:**
- `file.create.*` - All file creation events
- `file.*.succeeded` - All successful file operations
- `*.*.failed` - All failed operations

**Handler Types:**

1. **webhook** - HTTP POST to external URL
   ```json
   {
     "type": "webhook",
     "url": "https://...",
     "headers": {"X-API-Key": "..."}
   }
   ```

2. **log** - Write to logs
   ```json
   {
     "type": "log",
     "level": "info|warn|error"
   }
   ```

3. **metrics** - Increment counters
   ```json
   {
     "type": "metrics",
     "metric_name": "file_uploads"
   }
   ```

**See:** Events & Webhooks section below for details

**Implementation:** `pkg/domain/events_loader.go`, `pkg/events/handlers/`

---

## Content Validation

### How It Works

1. User uploads file
2. System loads `.files` config from directory (or parent)
3. Matches filename against patterns
4. If match found, validates content against JSON schema
5. Rejects upload if validation fails

### Example: User Registration

**Setup validation:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "directory_path": "/users",
    "name": ".files",
    "content": "{\"rules\":[{\"pattern\":\"*.json\",\"type\":\"glob\",\"schema\":{\"type\":\"object\",\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"},\"age\":{\"type\":\"integer\",\"minimum\":0}},\"required\":[\"email\"]}}]}"
  }'
```

**Valid upload (succeeds):**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "directory_path": "/users",
    "name": "alice.json",
    "content": "{\"email\":\"alice@example.com\",\"age\":30}"
  }'
```

**Invalid upload (rejected):**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "directory_path": "/users",
    "name": "bob.json",
    "content": "{\"name\":\"Bob\"}"
  }'
# Error: "email" is required
```

### Multiple Rules

You can have different schemas for different file patterns:

```json
{
  "rules": [
    {
      "pattern": "users/*.json",
      "schema": {"required": ["email"]}
    },
    {
      "pattern": "config/*.json",
      "schema": {"required": ["version", "settings"]}
    }
  ]
}
```

### Inheritance

Child directories inherit parent validation rules:

```
/
├── .files (require email)
└── data/
    ├── users/
    │   └── alice.json  ← must have email (inherited)
    └── config/
        ├── .files (require version)
        └── app.json     ← must have version (overrides parent)
```

---

## Events & Webhooks

### Event Lifecycle

Every operation triggers events at different stages:

```
file.create.starting     ← Before operation
  ↓
file.create.started      ← Operation in progress
  ↓
file.create.succeeded    ← Success
(or file.create.failed)  ← Failure
```

### Event Types

**File Events:**
- `file.create.{starting|started|succeeded|failed}`
- `file.update.{starting|started|succeeded|failed}`
- `file.delete.{starting|started|succeeded|failed}`

**Directory Events:**
- `directory.create.{starting|started|succeeded|failed}`
- `directory.delete.{starting|started|succeeded|failed}`

**Authorization Events:**
- `authorization.policy.checked.{succeeded|failed}`

**Validation Events:**
- `validation.schema.checked.{succeeded|failed}`

**Full List:** See `pkg/events/types.go`

### Event Payload

**Webhook receives:**
```json
{
  "event_type": "file.create.succeeded",
  "timestamp": "2025-10-05T12:00:00Z",
  "user": {
    "id": "alice",
    "groups": ["admin"]
  },
  "resource": {
    "path": "/data/example.json",
    "type": "file"
  },
  "metadata": {
    "file_id": "...",
    "size_bytes": 1024,
    "content_type": "application/json"
  }
}
```

### Configuring Event Handlers

**Create `.events` file:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".events",
    "content": "{\"handlers\":[{\"events\":[\"file.create.*\"],\"handler\":{\"type\":\"webhook\",\"url\":\"https://api.example.com/audit\"}}]}"
  }'
```

### Use Cases

**1. Audit Logging:**
```json
{
  "handlers": [
    {
      "events": ["file.*", "directory.*"],
      "handler": {
        "type": "webhook",
        "url": "https://audit.internal/events"
      }
    }
  ]
}
```

**2. Notifications:**
```json
{
  "handlers": [
    {
      "events": ["file.create.succeeded"],
      "handler": {
        "type": "webhook",
        "url": "https://slack.com/webhook/...",
        "headers": {
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

**3. Metrics:**
```json
{
  "handlers": [
    {
      "events": ["file.create.succeeded"],
      "handler": {
        "type": "metrics",
        "metric_name": "files_created"
      }
    }
  ]
}
```

**4. Failed Operations:**
```json
{
  "handlers": [
    {
      "events": ["*.*.failed"],
      "handler": {
        "type": "log",
        "level": "error"
      }
    }
  ]
}
```

### Event Inheritance

Event handlers inherit from parent directories and are merged:

```
/ (.events: webhook to audit system)
└── data/ (.events: webhook to slack)
    └── file.json creation triggers BOTH webhooks
```

---

## File Versioning

### How It Works

Every file update creates a new version. Old versions are retained.

**Version History:**
```
v1: Initial upload
v2: First update
v3: Second update (current)
```

### Get Specific Version

```bash
GET /api/v1/files/data/example.json?version=2
```

Returns content from version 2.

### List Versions

```bash
GET /api/v1/files/data/example.json/versions
```

**Response:**
```json
{
  "versions": [
    {
      "version": 1,
      "created_at": "2025-10-05T10:00:00Z",
      "size_bytes": 100
    },
    {
      "version": 2,
      "created_at": "2025-10-05T11:00:00Z",
      "size_bytes": 150
    },
    {
      "version": 3,
      "created_at": "2025-10-05T12:00:00Z",
      "size_bytes": 200
    }
  ]
}
```

### Retention

- All versions retained by default
- No automatic pruning (future enhancement)
- Manual cleanup via admin tools

**Implementation:** `pkg/persistence/db/mysql/file.go`

---

## Group Management

### Current Model

Groups are defined in `.user` files:

```json
{
  "users": [
    {
      "user_id": "alice",
      "groups": ["admin", "engineering", "oncall"]
    }
  ]
}
```

### Group Usage in Policies

**Check group membership:**
```rego
allow {
    input.user.groups[_] == "admin"
}
```

**Check if in any of multiple groups:**
```rego
allow {
    allowed_groups := ["admin", "engineering"]
    user_group := input.user.groups[_]
    user_group == allowed_groups[_]
}
```

**Owner-based access:**
```rego
allow {
    # User group matches owner group
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

### Future: Centralized Group Management

`.group` file for centralized group definitions:

```json
{
  "groups": [
    {
      "group_id": "engineering",
      "members": ["alice", "bob"],
      "description": "Engineering team"
    }
  ]
}
```

Status: Implemented but not widely used (groups typically in `.user` file)

---

## Common Patterns

### 1. Multi-Tenant Setup

**Directory structure:**
```
/
├── .rego (system admin only)
├── tenant-a/
│   ├── .user (tenant A users)
│   ├── .rego (tenant A access rules)
│   └── data/
└── tenant-b/
    ├── .user (tenant B users)
    ├── .rego (tenant B access rules)
    └── data/
```

**Root policy (/.rego):**
```rego
package vfs.authz

# System admin has access everywhere
allow {
    input.user.groups[_] == "system-admin"
}

# Users can only access their tenant directory
allow {
    tenant := split(input.resource.path, "/")[1]
    input.user.groups[_] == concat("-", ["tenant", tenant])
}
```

**Tenant A policy (/tenant-a/.rego):**
```rego
package vfs.authz

# Admin group has full access
allow {
    input.user.groups[_] == "tenant-a-admin"
}

# User group has read access
allow {
    input.user.groups[_] == "tenant-a-user"
    input.action == "read"
}
```

### 2. Audit Trail

**Setup event handler:**
```json
{
  "handlers": [
    {
      "events": ["file.*", "directory.*"],
      "handler": {
        "type": "webhook",
        "url": "https://audit-log.internal/events",
        "headers": {
          "X-API-Key": "audit-secret"
        }
      }
    }
  ]
}
```

**Audit service receives:**
- Who performed action (user.id)
- What was done (event_type)
- When it happened (timestamp)
- What resource (resource.path)

### 3. Data Validation Pipeline

**Enforce schemas at upload:**
```json
{
  "rules": [
    {
      "pattern": "raw/*.json",
      "schema": {"required": ["source_id", "data"]}
    },
    {
      "pattern": "processed/*.json",
      "schema": {
        "required": ["id", "validated", "transformed_data"]
      }
    }
  ]
}
```

**Processing flow:**
1. Upload raw data to `/data/raw/`
2. Webhook triggers processing
3. Processor validates and transforms
4. Saves to `/data/processed/` (different schema enforced)

### 4. Owner-Based Access

**Setup:**

1. Create `.owner` file:
```json
{
  "owner_groups": ["engineering"]
}
```

2. Create policy:
```rego
package vfs.authz

# Admin has full access
allow {
    input.user.groups[_] == "admin"
}

# All users can read
allow {
    input.action == "read"
}

# Only owners can write
allow {
    input.action == "write"
    is_owner
}

is_owner {
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

3. Users in "engineering" group can write, others can only read

### 5. Time-Based Access

**Business hours only:**
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

### 6. Progressive Validation

**Different schemas for different stages:**

```
/pipeline/
├── .files (lenient schema for intake)
└── stages/
    ├── raw/      (inherit lenient schema)
    ├── validated/
    │   └── .files (strict schema)
    └── final/
        └── .files (strictest schema)
```

Each stage enforces stricter validation.

---

## Error Handling

### Common Errors

**401 Unauthorized:**
```json
{
  "error": "missing authorization header"
}
```
→ Check authentication (token or headers)

**403 Forbidden:**
```json
{
  "error": "forbidden: access denied by policy"
}
```
→ Check `.rego` policy and user groups

**400 Bad Request:**
```json
{
  "error": "validation failed: email is required"
}
```
→ Check `.files` schema validation

**404 Not Found:**
```json
{
  "error": "file not found"
}
```
→ File or directory doesn't exist

### Debugging

**Check user context:**
```bash
# See who you are
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $TOKEN"
```

**Check policy:**
```bash
# See policy for directory
curl http://localhost:8080/api/v1/files/data/.rego \
  -H "Authorization: Bearer $TOKEN"
```

**Check validation rules:**
```bash
# See validation rules
curl http://localhost:8080/api/v1/files/data/.files \
  -H "Authorization: Bearer $TOKEN"
```

**Test policy locally:**
```bash
# Use OPA CLI
opa eval --data policy.rego --input input.json 'data.vfs.authz.allow'
```

---

## Next Steps

- **[SECURITY.md](SECURITY.md)** - Set up authentication and authorization
- **[OPERATIONS.md](OPERATIONS.md)** - Deploy and configure
- **[DESIGN.md](DESIGN.md)** - Understand architecture

---

**Need Help?**
- Check error messages carefully
- Review special files (`.rego`, `.files`, `.user`)
- Test policies with OPA CLI
- Check logs for detailed errors

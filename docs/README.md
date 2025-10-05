# MySQL VFS

**A file storage system with dynamic authorization, content validation, and audit trails**

**Version:** v2.1+

---

## What is MySQL VFS?

MySQL VFS is a file storage system that stores metadata in MySQL and content in S3-compatible storage. It provides:

- **Dynamic Authorization** - Change access rules without code deployment (via OPA policies)
- **Content Validation** - Enforce JSON schemas at upload time
- **Multi-Tenancy** - Directory-based isolation with owner-based access
- **Audit Trails** - Lifecycle events with webhook integration
- **Group-Based Access** - Flexible RBAC via user groups

---

## Key Features

### 1. Policy-Based Authorization

Create `.rego` files to control access:

```rego
# Admin group gets full access
allow {
    input.user.groups[_] == "admin"
}

# Users can read their own files
allow {
    input.user.groups[_] == "user"
    contains(input.resource.path, input.user.id)
}
```

**No code changes needed** - just update the `.rego` file.

### 2. Content Validation

Create `.files` config to enforce schemas:

```json
{
  "rules": [
    {
      "pattern": "*.json",
      "schema": {
        "type": "object",
        "required": ["email", "name"]
      }
    }
  ]
}
```

Invalid files are rejected at upload time.

### 3. Event System

Get notified on every operation:

```json
{
  "handlers": [
    {
      "events": ["file.create.*"],
      "handler": {
        "type": "webhook",
        "url": "https://your-api.com/webhook"
      }
    }
  ]
}
```

Perfect for audit trails, notifications, and integrations.

### 4. Multi-Tenancy

Each directory can have its own:
- Authorization policy (`.rego`)
- User credentials (`.user`)
- Ownership rules (`.owner`)
- Validation schemas (`.files`)

Policies inherit from parent directories.

---

## Architecture

```
Client
  ↓
HTTP API (REST)
  ↓
Middleware (Auth → Authz → Validation → Events)
  ↓
Domain Layer (Business Logic)
  ↓
Persistence (MySQL + S3)
```

**Storage Strategy:**
- Small files (< 1MB): MySQL
- Large files (≥ 1MB): S3

---

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Curl or HTTP client

### 1. Start Services

```bash
docker-compose up -d
```

This starts:
- MySQL VFS API (port 8080)
- MySQL (port 3306)
- MinIO S3 (port 9000)

### 2. Bootstrap System

Set system admin token:

```bash
export SYSTEM_ADMIN_TOKEN="your-secure-token-here"
```

The system admin can access everything without policy restrictions.

### 3. Create a Directory

```bash
curl -X POST http://localhost:8080/api/v1/directories \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "parent_path": "/",
    "name": "data"
  }'
```

### 4. Upload a File

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "hello.txt",
    "content_type": "text/plain",
    "content": "Hello, World!"
  }'
```

### 5. Create Users

Create a `.user` file for authentication:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"alice\",\"token\":\"alice-secret-token\",\"groups\":[\"admin\"]},{\"user_id\":\"bob\",\"token\":\"bob-secret-token\",\"groups\":[\"user\"]}]}"
  }'
```

Now Alice and Bob can authenticate with their tokens:

```bash
# Alice (admin group)
curl http://localhost:8080/api/v1/files/data/hello.txt \
  -H "Authorization: Bearer alice-secret-token"

# Bob (user group)
curl http://localhost:8080/api/v1/files/data/hello.txt \
  -H "Authorization: Bearer bob-secret-token"
```

### 6. Create Authorization Policy

Create a `.rego` file to control access:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\n# Admin group can do anything\nallow {\n    input.user.groups[_] == \"admin\"\n}\n\n# User group can only read\nallow {\n    input.user.groups[_] == \"user\"\n    input.action == \"read\"\n}"
  }'
```

Now:
- ✅ Alice can read and write (admin group)
- ✅ Bob can read (user group)
- ❌ Bob cannot write (denied by policy)

### 7. Add Content Validation

Create a `.files` config to enforce schemas:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer alice-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": ".files",
    "content_type": "application/json",
    "content": "{\"rules\":[{\"pattern\":\"*.json\",\"type\":\"glob\",\"schema\":{\"type\":\"object\",\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"}},\"required\":[\"email\"]}}]}"
  }'
```

Now JSON files must have an `email` field:

```bash
# ✅ Valid - will succeed
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer alice-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "user.json",
    "content_type": "application/json",
    "content": "{\"email\":\"alice@example.com\"}"
  }'

# ❌ Invalid - will be rejected
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer alice-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "invalid.json",
    "content_type": "application/json",
    "content": "{\"name\":\"Bob\"}"
  }'
```

---

## Use Cases

### 1. SaaS Multi-Tenant Storage

Each customer gets their own directory with isolated:
- User credentials (`.user`)
- Access policies (`.rego`)
- Validation rules (`.files`)

```
/
├── customer-a/
│   ├── .user      # Customer A's users
│   ├── .rego      # Customer A's policies
│   └── data/
└── customer-b/
    ├── .user      # Customer B's users
    ├── .rego      # Customer B's policies
    └── data/
```

### 2. Regulated Industries

Audit every operation via events:

```json
{
  "handlers": [
    {
      "events": ["file.*"],
      "handler": {
        "type": "webhook",
        "url": "https://audit-log.internal/events"
      }
    }
  ]
}
```

### 3. Dynamic Access Control

Change permissions without deployment:

```bash
# Update policy (no code deploy needed)
curl -X PUT http://localhost:8080/api/v1/files/.rego \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"content": "new policy..."}'
```

Policy takes effect within 5 minutes (cache TTL).

### 4. Content Management Systems

Enforce content schemas:

```json
{
  "rules": [
    {
      "pattern": "articles/*.json",
      "schema": {
        "required": ["title", "author", "published_date"]
      }
    }
  ]
}
```

---

## Core Concepts

### Special Files

Files starting with `.` have special meaning:

| File | Purpose | Format |
|------|---------|--------|
| `.rego` | Authorization policies | Rego (OPA) |
| `.user` | User credentials | JSON |
| `.group` | Group definitions | JSON |
| `.owner` | Directory ownership | JSON |
| `.files` | Content validation | JSON |
| `.events` | Event handlers | JSON |

See: `USER_GUIDE.md` for details

### Policy Inheritance

Policies inherit from parent directories:

```
/                    (.rego: admin-only)
└── data/           (.rego: users can read) ← overrides parent
    └── public/     (no .rego)              ← inherits from /data
```

### Groups

Users belong to one or more groups:

```json
{
  "user_id": "alice",
  "groups": ["admin", "engineering", "oncall"]
}
```

Policies check group membership:

```rego
allow {
    input.user.groups[_] == "admin"
}
```

### Events

Every operation triggers lifecycle events:

```
file.create.starting
  ↓
file.create.started
  ↓
file.create.succeeded (or file.create.failed)
```

Configure handlers in `.events` files.

---

## Documentation

- **[USER_GUIDE.md](USER_GUIDE.md)** - Complete API reference and features
- **[SECURITY.md](SECURITY.md)** - Authentication, authorization, and access control
- **[OPERATIONS.md](OPERATIONS.md)** - Deployment, configuration, and troubleshooting
- **[DESIGN.md](DESIGN.md)** - Architecture, design decisions, and implementation details

---

## API Overview

### File Operations

```bash
# Create file
POST /api/v1/files

# Get file
GET /api/v1/files/{path}

# Update file
PUT /api/v1/files/{path}

# Delete file
DELETE /api/v1/files/{path}

# List files in directory
GET /api/v1/directories/{path}
```

### Directory Operations

```bash
# Create directory
POST /api/v1/directories

# Get directory info
GET /api/v1/directories/{path}

# Delete directory
DELETE /api/v1/directories/{path}
```

### Authentication

All requests require authentication:

```bash
# Production: Bearer token
Authorization: Bearer <token>

# Development: Headers (unsafe!)
X-User-ID: alice
X-User-Groups: admin,engineering
```

See: `SECURITY.md` for authentication options

---

## Configuration

Environment variables:

```bash
# Database
DATABASE_URL=mysql://user:pass@localhost:3306/vfs

# Storage
S3_ENDPOINT=http://localhost:9000
S3_BUCKET=vfs-storage
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin

# Authentication
SYSTEM_ADMIN_TOKEN=your-secure-token
SYSTEM_ADMIN_ID=system-admin

# Cache
POLICY_CACHE_TTL_SECONDS=300
```

See: `OPERATIONS.md` for all configuration options

---

## Development

### Run Tests

```bash
go test ./...

# Integration tests
cd citest
ginkgo -v
```

### Run Locally

```bash
# Start dependencies
docker-compose up -d mysql minio

# Run server
go run cmd/server/main.go
```

---

## Performance

- **Authorization:** ~1-5ms per request (OPA evaluation)
- **Small files:** Single MySQL query
- **Large files:** MySQL + S3 (parallel)
- **Caching:** 5-minute TTL for policies, users, validation rules

**Horizontal Scaling:**
- Stateless design
- No in-memory sessions
- Cache per instance (or use Redis)

---

## Security

### Authentication Methods

1. **System Admin Token** - Bypass all policies (bootstrap)
2. **File-Based (.user)** - Static tokens per user
3. **JWT** - External auth service integration
4. **Headers** - Development only (unsafe!)

### Authorization

- **Default:** Deny all
- **Policies:** OPA (Open Policy Agent)
- **Bypass:** System admin group
- **Inheritance:** Child directories inherit parent policies

### Best Practices

✅ Use strong random tokens (32+ chars)
✅ Enable HTTPS in production
✅ Rotate system admin token regularly
✅ Use JWT for external auth
✅ Never use header auth in production
✅ Review policies before deployment

See: `SECURITY.md` for detailed security guide

---

## Troubleshooting

### Access Denied

```bash
# Check user's groups
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Check policy
curl http://localhost:8080/api/v1/files/data/.rego \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Test policy locally with OPA
opa eval --data policy.rego --input input.json 'data.vfs.authz.allow'
```

### File Upload Fails

```bash
# Check validation rules
curl http://localhost:8080/api/v1/files/data/.files \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Check file size limits
# Max: configurable via MAX_FILE_SIZE env var
```

### Policy Not Working

```bash
# Cache may be stale (up to 5 minutes)
# Wait or restart service

# Check policy syntax
opa check policy.rego
```

See: `OPERATIONS.md` for more troubleshooting tips

---

## Contributing

See: `CONTRIBUTING.md` (if exists)

File issues: GitHub repository

---

## License

[Your license here]

---

## Support

- **Documentation:** See docs/ folder
- **Issues:** File on GitHub
- **Security:** See SECURITY.md

---

**Quick Links:**
- [User Guide](USER_GUIDE.md) - Complete feature reference
- [Security Guide](SECURITY.md) - Auth and authorization
- [Operations Guide](OPERATIONS.md) - Deployment and config
- [Design Document](DESIGN.md) - Architecture and decisions

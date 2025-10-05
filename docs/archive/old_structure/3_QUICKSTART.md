# 3. Quick Start Guide

**Get MySQL VFS v2.1+ running in 5 minutes**

[← Back: Architecture](2_ARCHITECTURE.md) | [Index](0_README.md) | [Next: Special Files →](4_SPECIAL_FILES.md)

---

## Prerequisites

- Docker & Docker Compose
- curl or Postman
- (Optional) Go 1.21+ for local development

---

## Step 1: Start VFS

### Using Docker Compose

```bash
# Clone repository
git clone https://github.com/telnet2/mysql-vfs
cd mysql-vfs

# Start services (MySQL + MinIO + VFS)
docker-compose up -d

# Check health
curl http://localhost:8080/health
```

**Expected Response:**
```json
{
  "status": "ok",
  "checks": {
    "database": "ok",
    "migrations": "ok",
    "storage": "ok"
  }
}
```

---

## Step 2: Bootstrap System Admin

VFS v2.1+ uses **hybrid authentication** with a system admin for bootstrap:

```bash
# Generate a secure system admin token
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
echo "Your system admin token: $SYSTEM_ADMIN_TOKEN"

# Configure auth (we'll use file-based auth)
export AUTH_PROVIDER=file
export FILE_AUTH_DIRECTORY=/

# Or for quick testing, use header-based auth (DEV ONLY!)
export AUTH_PROVIDER=headers
export AUTH_ALLOW_ANONYMOUS=true
```

**⚠️ IMPORTANT:**
- System admin token should be stored securely (Vault, AWS Secrets Manager, etc.)
- Never commit system admin token to git
- Header auth is UNSAFE for production! See [Authentication Setup](8_AUTH_SETUP.md)

---

## Step 3: Create Root Directory

```bash
curl -X POST http://localhost:8080/api/v1/directories \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "parent_path": "/",
    "name": "data"
  }'
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "data",
  "path": "/data",
  "created_at": "2025-10-04T12:00:00Z"
}
```

---

## Step 4: Create .user File (File-Based Auth)

**Skip this if using header auth.** For production, create a `.user` file (implementation: `pkg/domain/user_loader.go`):

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"admin-secure-token-change-me\",\"groups\":[\"admin\"]},{\"user_id\":\"alice\",\"token\":\"alice-token\",\"groups\":[\"user\",\"engineering\"]}]}"
  }'
```

**Note:** Users are assigned to one or more groups. Authorization is controlled via group membership in OPA policies.

**From now on, use tokens from `.user` file:**
```bash
export ADMIN_TOKEN=admin-secure-token-change-me
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## Step 6: List Special Files

View available special files (implementation: `pkg/domain/files_loader.go`):

```bash
# Use ADMIN_TOKEN from .user file, or SYSTEM_ADMIN_TOKEN for bootstrap
curl http://localhost:8080/api/v1/files/data/.files \
  -H "Authorization: Bearer ${ADMIN_TOKEN:-$SYSTEM_ADMIN_TOKEN}"
```

**Available Special Files in v2.1+:**
- `.files` - List all files in directory (replaces `.jsonschema` for validation)
- `.user` - File-based authentication (see Step 4)
- `.events` - Lifecycle event handlers
- `.rego` - OPA authorization policies
- `.owner` - Resource ownership and protection

**Removed in v2.1+:** `.jsonschema`, `.quota`, `.lifecycle`, `.group`

---

## Step 7: Upload a Valid File

```bash
# Use alice's token from .user file, or ADMIN_TOKEN
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer ${ADMIN_TOKEN:-$SYSTEM_ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "alice.json",
    "content_type": "application/json",
    "content": "{\"email\":\"alice@example.com\",\"name\":\"Alice\"}"
  }'
```

**Response:**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "alice.json",
  "content_type": "application/json",
  "size_bytes": 45,
  "version": 1,
  "created_at": "2025-10-04T12:01:00Z"
}
```

✅ **File uploaded successfully!**

---

## Step 8: Read a File

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: user,engineering"
```

**Response:**
```json
{
  "email": "alice@example.com",
  "name": "Alice"
}
```

---

## Step 9: Create an Authorization Policy

Create a `.rego` file to control who can access `/data` (implementation: `pkg/domain/policy_loader.go`, `pkg/middleware/authorization.go`):

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -H "X-User-Groups: admin" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\n# Allow admin group to do anything\nallow {\n  input.user.groups[_] == \"admin\"\n}\n\n# Allow user group to read their own files\nallow {\n  input.user.groups[_] == \"user\"\n  input.action == \"read\"\n  contains(input.resource.path, input.user.id)\n}"
  }'
```

**What this policy does:**
- Users in the "admin" group can do anything
- Users in the "user" group can only read files containing their user ID in the path

---

## Step 10: Test Authorization

Alice can read her own file:

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: user,engineering"
# ✅ Success
```

But Bob cannot read Alice's file:

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: bob" \
  -H "X-User-Groups: user"
# ❌ 403 Forbidden
```

---

## Step 11: List Directory

```bash
curl "http://localhost:8080/api/v1/directories/data" \
  -H "X-User-ID: admin" \
  -H "X-User-Groups: admin"
```

**Response:**
```json
{
  "entries": [
    {
      "name": ".rego",
      "type": "file",
      "size_bytes": 200,
      "modified_at": "2025-10-04T12:02:00Z"
    },
    {
      "name": "alice.json",
      "type": "file",
      "size_bytes": 45,
      "modified_at": "2025-10-04T12:01:00Z"
    }
  ]
}
```

---

## 🎉 Congratulations!

You've successfully:
- ✅ Started MySQL VFS v2.1+
- ✅ Created directories and files
- ✅ Set up file-based authentication with `.user`
- ✅ Configured authorization with `.rego` policies
- ✅ Tested authentication and authorization

---

## Next Steps

### Production Setup

1. **Switch to JWT Authentication**
   - See [Authentication Setup](8_AUTH_SETUP.md)
   - Configure `AUTH_PROVIDER=jwt`
   - Set `AUTH_JWT_SECRET`

2. **Deploy to Production**
   - See [Deployment Guide](9_DEPLOYMENT.md)
   - Set up HTTPS
   - Configure database backups

3. **Advanced Features**
   - [Special Files](4_SPECIAL_FILES.md) - `.files`, `.user`, `.events`, `.rego`, `.owner`
   - [Authorization](6_AUTHORIZATION.md) - Complex OPA policies
   - [Lifecycle Events](15_LIFECYCLE_EVENTS.md) - Event handlers and webhooks
   - [Configuration](7_CONFIGURATION.md) - All environment variables

---

## Common Commands

### Create Directory
```bash
curl -X POST http://localhost:8080/api/v1/directories \
  -H "Content-Type: application/json" \
  -d '{"parent_path": "/", "name": "mydir"}'
```

### Upload File
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/mydir",
    "name": "file.json",
    "content_type": "application/json",
    "content": "{\"key\":\"value\"}"
  }'
```

### Read File
```bash
curl http://localhost:8080/api/v1/files/mydir/file.json
```

### Update File
```bash
curl -X PUT http://localhost:8080/api/v1/files/mydir/file.json \
  -H "Content-Type: application/json" \
  -d '{
    "content": "{\"key\":\"new-value\"}",
    "expected_version": 1
  }'
```

### Delete File
```bash
curl -X DELETE http://localhost:8080/api/v1/files/mydir/file.json
```

### Delete Directory
```bash
curl -X DELETE "http://localhost:8080/api/v1/directories/mydir?recursive=true"
```

---

## Troubleshooting

### Health Check Fails

```bash
# Check if MySQL is ready
docker-compose ps mysql

# Check VFS logs
docker-compose logs vfs-service
```

### File Upload Fails

```bash
# Check directory exists
curl http://localhost:8080/api/v1/directories/data

# Check authentication
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"
```

### Authorization Denied

```bash
# Check policy
curl http://localhost:8080/api/v1/files/data/.rego

# Verify headers
curl -v http://localhost:8080/api/v1/files/... \
  -H "X-User-ID: <your-user>" \
  -H "X-User-Groups: <your-groups>"
```

---

[← Back: Architecture](2_ARCHITECTURE.md) | [Index](0_README.md) | [Next: Special Files →](4_SPECIAL_FILES.md)

# 3. Quick Start Guide

**Get MySQL VFS v2 running in 5 minutes**

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

## Step 2: Bootstrap Super User

VFS v2 uses **hybrid authentication** with a super user for bootstrap:

```bash
# Generate a secure super user token
export SUPER_USER_TOKEN=$(openssl rand -hex 32)
echo "Your super user token: $SUPER_USER_TOKEN"

# Configure auth (we'll use file-based auth)
export AUTH_PROVIDER=file
export FILE_AUTH_DIRECTORY=/

# Or for quick testing, use header-based auth (DEV ONLY!)
export AUTH_PROVIDER=headers
export AUTH_ALLOW_ANONYMOUS=true
```

**⚠️ IMPORTANT:**
- Super user token should be stored securely (Vault, AWS Secrets Manager, etc.)
- Never commit super user token to git
- Header auth is UNSAFE for production! See [Authentication Setup](8_AUTH_SETUP.md)

---

## Step 3: Create Root Directory

```bash
curl -X POST http://localhost:8080/api/v1/directories \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
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

**Skip this if using header auth.** For production, create a `.user` file:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"admin-secure-token-change-me\",\"role\":\"admin\",\"groups\":[\"admins\"]},{\"user_id\":\"alice\",\"token\":\"alice-token\",\"role\":\"user\",\"groups\":[\"engineering\"]}]}"
  }'
```

**From now on, use tokens from `.user` file:**
```bash
export ADMIN_TOKEN=admin-secure-token-change-me
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## Step 6: Create a Schema File

Create a `.jsonschema` file to validate all JSON files in `/data`:

```bash
# Use ADMIN_TOKEN from .user file, or SUPER_USER_TOKEN for bootstrap
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer ${ADMIN_TOKEN:-$SUPER_USER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": ".jsonschema",
    "content_type": "application/json",
    "content": "{\"type\":\"object\",\"required\":[\"email\",\"name\"],\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"},\"name\":{\"type\":\"string\",\"minLength\":1}}}"
  }'
```

**What this does:**
- All JSON files uploaded to `/data` must have `email` and `name` fields
- `email` must be a valid email format
- `name` must be non-empty

---

## Step 7: Upload a Valid File

```bash
# Use alice's token from .user file, or ADMIN_TOKEN
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer ${ADMIN_TOKEN:-$SUPER_USER_TOKEN}" \
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

✅ **File uploaded successfully!** It was validated against the schema.

---

## Step 8: Try Uploading Invalid Data

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer ${ADMIN_TOKEN:-$SUPER_USER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "bob.json",
    "content_type": "application/json",
    "content": "{\"email\":\"invalid-email\"}"
  }'
```

**Response:**
```json
{
  "error": "content validation failed",
  "details": [
    "email: Does not match format 'email'",
    "name: Required property is missing"
  ]
}
```

❌ **Validation failed!** The file doesn't match the schema.

---

## Step 7: Read a File

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: alice" \
  -H "X-User-Role: user"
```

**Response:**
```json
{
  "email": "alice@example.com",
  "name": "Alice"
}
```

---

## Step 8: Create an Authorization Policy

Create a `.rego` file to control who can access `/data`:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -H "X-User-Role: admin" \
  -d '{
    "directory_path": "/data",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\n# Allow admins to do anything\nallow {\n  input.user.role == \"admin\"\n}\n\n# Allow users to read their own files\nallow {\n  input.user.role == \"user\"\n  input.action == \"read\"\n  contains(input.resource.path, input.user.user_id)\n}"
  }'
```

**What this policy does:**
- Admins can do anything
- Regular users can only read files containing their user ID in the path

---

## Step 9: Test Authorization

Alice can read her own file:

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: alice" \
  -H "X-User-Role: user"
# ✅ Success
```

But Bob cannot read Alice's file:

```bash
curl http://localhost:8080/api/v1/files/data/alice.json \
  -H "X-User-ID: bob" \
  -H "X-User-Role: user"
# ❌ 403 Forbidden
```

---

## Step 10: List Directory

```bash
curl "http://localhost:8080/api/v1/directories/data" \
  -H "X-User-ID: admin" \
  -H "X-User-Role: admin"
```

**Response:**
```json
{
  "entries": [
    {
      "name": ".jsonschema",
      "type": "file",
      "size_bytes": 150,
      "modified_at": "2025-10-04T12:00:00Z"
    },
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
- ✅ Started MySQL VFS v2
- ✅ Created directories and files
- ✅ Set up schema validation with `.jsonschema`
- ✅ Configured authorization with `.rego` policies
- ✅ Tested validation and authorization

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
   - [Special Files](4_SPECIAL_FILES.md) - Schema inheritance, quotas
   - [Authorization](6_AUTHORIZATION.md) - Complex OPA policies
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

# Check schema syntax (if using .jsonschema)
curl http://localhost:8080/api/v1/files/data/.jsonschema
```

### Authorization Denied

```bash
# Check policy
curl http://localhost:8080/api/v1/files/data/.rego

# Verify headers
curl -v http://localhost:8080/api/v1/files/... \
  -H "X-User-ID: <your-user>" \
  -H "X-User-Role: <your-role>"
```

---

[← Back: Architecture](2_ARCHITECTURE.md) | [Index](0_README.md) | [Next: Special Files →](4_SPECIAL_FILES.md)

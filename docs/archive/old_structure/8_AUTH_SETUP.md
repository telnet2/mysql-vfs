# MySQL VFS v2.1+ Authentication Setup

**How to set up system admin and file-based authentication**

[← Back: Configuration](7_CONFIGURATION.md) | [Index](0_README.md) | [Next: Deployment →](9_DEPLOYMENT.md)

---

## Overview

MySQL VFS v2.1+ uses **hybrid authentication** with two layers:

1. **System Admin (Environment-Based)** - Always checked first, used for bootstrap (implementation: `pkg/middleware/auth_providers.go`)
2. **File-Based Auth (.user files)** - Production auth stored in VFS itself (implementation: `pkg/domain/user_loader.go`)

This guide shows you how to bootstrap from scratch.

---

## Quick Start (5 Minutes)

### Step 1: Generate System Admin Token

```bash
# Generate a secure random token (64 characters)
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
echo "Save this token securely: $SYSTEM_ADMIN_TOKEN"

# Optional: customize system admin identity
export SYSTEM_ADMIN_ID=admin
export SYSTEM_ADMIN_ROLE=admin
```

### Step 2: Configure VFS

```bash
# Use file-based auth (production)
export AUTH_PROVIDER=file
export FILE_AUTH_DIRECTORY=/
export USER_CACHE_TTL_SECONDS=300

# Database
export DB_DSN='root:password@tcp(localhost:3306)/vfs?parseTime=true'

# Start VFS
./vfs
```

### Step 3: Create Root Directory

```bash
curl -X POST http://localhost:8080/api/v1/directories \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"parent_path": "/", "name": "root"}'
```

### Step 4: Create `.user` File

```bash
# Create admin and regular users
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"$(openssl rand -hex 16)\",\"role\":\"admin\",\"groups\":[\"admins\"]},{\"user_id\":\"alice\",\"token\":\"$(openssl rand -hex 16)\",\"role\":\"user\",\"groups\":[\"engineering\"]}]}"
  }'
```

**Note:** Groups are deprecated in v2.1+. The `groups` field is stored in `.user` for backward compatibility but not used for authorization. Use roles instead.

**Save the tokens!** Extract them from the created file:

```bash
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" | jq '.users'
```

### Step 5: Test User Auth

```bash
# Get admin token from previous step
export ADMIN_TOKEN=<token-from-user-file>

# Test it works
curl http://localhost:8080/api/v1/directories/ \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

✅ **Done!** System admin is still available for emergencies.

---

## .user File Format

```json
{
  "users": [
    {
      "user_id": "admin",
      "token": "static-token-here",           // Required if no password_hash
      "password_hash": "$2a$10$...",          // bcrypt hash (optional)
      "groups": ["admin", "engineering"]
    },
    {
      "user_id": "alice",
      "token": "alice-secret-token",
      "groups": ["user", "engineering"]
    }
  ]
}
```

**Fields:**
- `user_id` - Unique user identifier (required)
- `token` - Static bearer token for this user (required if no password_hash)
- `password_hash` - bcrypt hashed password (optional, for future password auth)
- `groups` - Array of group names (required)

---

## Security Best Practices

### System Admin Token

✅ **DO:**
- Generate with `openssl rand -hex 32` (64 chars)
- Store in secret management (Vault, AWS Secrets Manager, K8s Secrets)
- Rotate regularly (monthly/quarterly)
- Use only for bootstrap and emergencies

❌ **DON'T:**
- Commit to git
- Share with anyone
- Use simple/guessable values
- Use in production applications (use .user tokens instead)

### User Tokens

✅ **DO:**
- Generate unique random tokens per user
- Store securely (password manager, secrets vault)
- Rotate when compromised
- Use different tokens for different environments (dev/staging/prod)

❌ **DON'T:**
- Reuse tokens across users
- Embed in code
- Log tokens
- Send over unencrypted channels

### .user File Access

**Restrict access with `.rego` policy:**

```bash
# Create /.rego to protect /.user (implementation: pkg/middleware/authorization.go)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\ndefault allow = false\n\n# Only admins can access .user file\nallow {\n    input.resource.path == \"/.user\"\n    input.user.role == \"admin\"\n}\n\n# Admins can access everything else\nallow {\n    input.user.role == \"admin\"\n}\n"
  }'
```

---

## Production Setup

### Environment Variables (Production)

```bash
# System Admin (from secrets manager)
export SYSTEM_ADMIN_TOKEN=$(aws secretsmanager get-secret-value --secret-id vfs-system-admin --query SecretString --output text)
export SYSTEM_ADMIN_ID=admin
export SYSTEM_ADMIN_ROLE=admin

# File-based auth
export AUTH_PROVIDER=file
export FILE_AUTH_DIRECTORY=/
export USER_CACHE_TTL_SECONDS=300

# Database (from secrets)
export DB_DSN=$(aws secretsmanager get-secret-value --secret-id vfs-db-dsn --query SecretString --output text)

# Storage
export S3_BUCKET=production-vfs-files
export S3_REGION=us-east-1
```

### Kubernetes Deployment

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vfs-system-admin
type: Opaque
stringData:
  token: <generated-with-openssl-rand-hex-32>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vfs
spec:
  template:
    spec:
      containers:
      - name: vfs
        image: mysql-vfs:v2.1
        env:
        - name: SYSTEM_ADMIN_TOKEN
          valueFrom:
            secretKeyRef:
              name: vfs-system-admin
              key: token
        - name: AUTH_PROVIDER
          value: "file"
        - name: FILE_AUTH_DIRECTORY
          value: "/"
```

---

## Troubleshooting

### "User not found" after creating .user

**Cause:** Cache not invalidated or wrong directory

**Fix:**
```bash
# Check .user file exists
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"

# Verify FILE_AUTH_DIRECTORY matches
echo $FILE_AUTH_DIRECTORY  # Should be "/"
```

### "Invalid token" with valid .user token

**Cause:** System admin token being checked first, or cache issue

**Fix:**
```bash
# Ensure using token from .user file, not system admin token
# Wait for cache TTL (5 minutes default)
# Or restart VFS to clear cache
```

### Lost system admin token

**Solution 1:** Check secrets manager
```bash
aws secretsmanager get-secret-value --secret-id vfs-system-admin
```

**Solution 2:** Regenerate and update deployment
```bash
# Generate new token
NEW_TOKEN=$(openssl rand -hex 32)

# Update secret
aws secretsmanager update-secret \
  --secret-id vfs-system-admin \
  --secret-string "$NEW_TOKEN"

# Restart VFS pods
kubectl rollout restart deployment vfs
```

---

## Migration from Dev to Production

### Current: Header Auth (Dev)

```bash
AUTH_PROVIDER=headers
AUTH_ALLOW_ANONYMOUS=true
```

### Step 1: Set Up System Admin

```bash
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
export AUTH_PROVIDER=file
```

### Step 2: Bootstrap .user File

Use system admin token to create `/.user` with production users

### Step 3: Update Clients

Change all API clients to use tokens from `.user` file instead of headers

### Step 4: Keep System Admin for Emergencies

Keep `SYSTEM_ADMIN_TOKEN` configured but stored securely in secrets manager. Use only for emergencies and maintenance.

---

## FAQ

**Q: Can I have multiple system admins?**
A: No, only one system admin token. But you can have multiple admin users in `.user` file.

**Q: What if .user file gets corrupted?**
A: System admin token always works - use it to fix/recreate `.user` file.

**Q: Can I use passwords instead of tokens?**
A: `.user` supports `password_hash` (bcrypt), but password auth is not yet implemented. Use tokens for now.

**Q: How do I rotate user tokens?**
A: Edit `.user` file with system admin token or admin token, update the token field.

**Q: Are groups supported?**
A: Groups are deprecated in v2.1+. The `groups` field in `.user` is stored but not used. Use roles for authorization.

**Q: Should I disable system admin in production?**
A: Keep it enabled for emergencies, but store token in secure secrets manager (Vault, AWS Secrets Manager).

---

---

[← Back: Configuration](7_CONFIGURATION.md) | [Index](0_README.md) | [Next: Deployment →](9_DEPLOYMENT.md)

**Next Steps:**
- [Authorization Guide](6_AUTHORIZATION.md) - OPA policies for access control
- [Deployment Guide](9_DEPLOYMENT.md) - Production deployment
- [Special Files](4_SPECIAL_FILES.md) - `.user`, `.rego`, `.owner`, and more

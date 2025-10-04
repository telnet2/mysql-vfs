# MySQL VFS Bootstrap Guide

**How to set up initial super user and file-based authentication**

---

## Overview

MySQL VFS v2 uses **hybrid authentication** with two layers:

1. **Super User (Environment-Based)** - Always checked first, used for bootstrap
2. **File-Based Auth (.user files)** - Production auth stored in VFS itself

This guide shows you how to bootstrap from scratch.

---

## Quick Start (5 Minutes)

### Step 1: Generate Super User Token

```bash
# Generate a secure random token (64 characters)
export SUPER_USER_TOKEN=$(openssl rand -hex 32)
echo "Save this token securely: $SUPER_USER_TOKEN"

# Optional: customize super user identity
export SUPER_USER_ID=super-admin
export SUPER_USER_ROLE=super-admin
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
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"parent_path": "/", "name": "root"}'
```

### Step 4: Create `.user` File

```bash
# Create admin and regular users
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"$(openssl rand -hex 16)\",\"role\":\"admin\",\"groups\":[\"admins\"]},{\"user_id\":\"alice\",\"token\":\"$(openssl rand -hex 16)\",\"role\":\"user\",\"groups\":[\"engineering\"]}]}"
  }'
```

**Save the tokens!** Extract them from the created file:

```bash
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" | jq '.users'
```

### Step 5: Test User Auth

```bash
# Get admin token from previous step
export ADMIN_TOKEN=<token-from-user-file>

# Test it works
curl http://localhost:8080/api/v1/directories/ \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

✅ **Done!** Super user is still available for emergencies.

---

## .user File Format

```json
{
  "users": [
    {
      "user_id": "admin",
      "token": "static-token-here",           // Required if no password_hash
      "password_hash": "$2a$10$...",          // bcrypt hash (optional)
      "role": "admin",
      "groups": ["admins", "engineering"]
    },
    {
      "user_id": "alice",
      "token": "alice-secret-token",
      "role": "user",
      "groups": ["engineering"]
    }
  ]
}
```

**Fields:**
- `user_id` - Unique user identifier (required)
- `token` - Static bearer token for this user (required if no password_hash)
- `password_hash` - bcrypt hashed password (optional, for future password auth)
- `role` - User role (e.g., "admin", "user")
- `groups` - Array of group names

---

## Security Best Practices

### Super User Token

✅ **DO:**
- Generate with `openssl rand -hex 32` (64 chars)
- Store in secret management (Vault, AWS Secrets Manager, K8s Secrets)
- Rotate regularly (monthly/quarterly)
- Use only for bootstrap and emergencies

❌ **DON'T:**
- Commit to git
- Share with anyone
- Use simple/guessable values
- Leave in environment permanently

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
# Create /.rego to protect /.user
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\ndefault allow = false\n\n# Only super-admins can access .user file\nallow {\n    input.resource.path == \"/.user\"\n    input.user.role == \"super-admin\"\n}\n\n# Admins can access everything else\nallow {\n    input.user.role == \"admin\"\n}\n"
  }'
```

---

## Production Setup

### Environment Variables (Production)

```bash
# Super User (from secrets manager)
export SUPER_USER_TOKEN=$(aws secretsmanager get-secret-value --secret-id vfs-super-user --query SecretString --output text)
export SUPER_USER_ID=super-admin
export SUPER_USER_ROLE=super-admin

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
  name: vfs-super-user
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
        image: mysql-vfs:v2
        env:
        - name: SUPER_USER_TOKEN
          valueFrom:
            secretKeyRef:
              name: vfs-super-user
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
  -H "Authorization: Bearer $SUPER_USER_TOKEN"

# Verify FILE_AUTH_DIRECTORY matches
echo $FILE_AUTH_DIRECTORY  # Should be "/"
```

### "Invalid token" with valid .user token

**Cause:** Super user token being checked first, or cache issue

**Fix:**
```bash
# Ensure using token from .user file, not super user token
# Wait for cache TTL (5 minutes default)
# Or restart VFS to clear cache
```

### Lost super user token

**Solution 1:** Check secrets manager
```bash
aws secretsmanager get-secret-value --secret-id vfs-super-user
```

**Solution 2:** Regenerate and update deployment
```bash
# Generate new token
NEW_TOKEN=$(openssl rand -hex 32)

# Update secret
aws secretsmanager update-secret \
  --secret-id vfs-super-user \
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

### Step 1: Set Up Super User

```bash
export SUPER_USER_TOKEN=$(openssl rand -hex 32)
export AUTH_PROVIDER=file
```

### Step 2: Bootstrap .user File

Use super user token to create `/.user` with production users

### Step 3: Update Clients

Change all API clients to use tokens from `.user` file instead of headers

### Step 4: Disable Super User (Optional)

For maximum security, remove `SUPER_USER_TOKEN` from environment after setup (keeps emergency access available if needed).

---

## FAQ

**Q: Can I have multiple super users?**
A: No, only one super user token. But you can have multiple admin users in `.user` file.

**Q: What if .user file gets corrupted?**
A: Super user token always works - use it to fix/recreate `.user` file.

**Q: Can I use passwords instead of tokens?**
A: `.user` supports `password_hash` (bcrypt), but password auth is not yet implemented. Use tokens for now.

**Q: How do I rotate user tokens?**
A: Edit `.user` file with super user token or admin token, update the token field.

**Q: Should I disable super user in production?**
A: Keep it enabled for emergencies, but store token in secure secrets manager (Vault, AWS Secrets Manager).

---

**Next Steps:**
- [Authentication Guide](5_AUTHENTICATION.md) - Full auth architecture
- [Authorization Guide](6_AUTHORIZATION.md) - OPA policies for access control
- [Deployment Guide](9_DEPLOYMENT.md) - Production deployment

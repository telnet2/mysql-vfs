# VFS Authentication Configuration Examples

All auth configuration is centralized in environment variables. Choose the auth provider that fits your needs.

---

## 🔐 Production Configurations

### Option 1: JWT Authentication (Recommended)

**Use Case:** Standard web apps, mobile apps, microservices

```bash
# .env.production
AUTH_PROVIDER=jwt
AUTH_JWT_SECRET=your-secret-key-minimum-32-characters-long
AUTH_JWT_ISSUER=https://auth.yourcompany.com

# Optional
AUTH_ALLOW_ANONYMOUS=false

# Cache settings
SCHEMA_CACHE_TTL_SECONDS=300  # 5 minutes
POLICY_CACHE_TTL_SECONDS=300  # 5 minutes
QUOTA_CACHE_TTL_SECONDS=300   # 5 minutes
```

**Token Example:**
```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     https://vfs.example.com/api/v1/files/data/users/alice.json
```

**JWT Payload:**
```json
{
  "user_id": "alice",
  "role": "admin",
  "groups": ["engineering", "admins"],
  "iss": "https://auth.yourcompany.com",
  "exp": 1735689600
}
```

---

### Option 2: OAuth/OIDC Token Introspection

**Use Case:** Enterprise SSO (Okta, Auth0, Google Workspace)

```bash
# .env.production
AUTH_PROVIDER=oauth
AUTH_OAUTH_INTROSPECTION_URL=https://oauth.provider.com/oauth/introspect
AUTH_OAUTH_CLIENT_ID=vfs-service
AUTH_OAUTH_CLIENT_SECRET=your-oauth-client-secret

AUTH_ALLOW_ANONYMOUS=false
```

**Token Example:**
```bash
curl -H "Authorization: Bearer <oauth-access-token>" \
     https://vfs.example.com/api/v1/files/data/users/alice.json
```

---

### Option 3: Mutual TLS (mTLS)

**Use Case:** Banking, Government, High-Security Environments

```bash
# .env.production
AUTH_PROVIDER=mtls
AUTH_MTLS_CA_FILE=/etc/vfs/certs/ca.pem
AUTH_MTLS_CERT_FILE=/etc/vfs/certs/server.crt
AUTH_MTLS_KEY_FILE=/etc/vfs/certs/server.key

AUTH_ALLOW_ANONYMOUS=false
```

**Client Request:**
```bash
curl --cert client.crt --key client.key \
     https://vfs.example.com/api/v1/files/data/users/alice.json
```

---

### Option 4: Reverse Proxy with HMAC Signature

**Use Case:** Behind nginx/Traefik with custom auth

```bash
# .env.production
AUTH_PROVIDER=proxy
AUTH_PROXY_SHARED_SECRET=shared-secret-between-proxy-and-vfs-min-32-chars

AUTH_ALLOW_ANONYMOUS=false
```

**Nginx Configuration:**
```nginx
location /api/v1/ {
    # Authenticate user with your auth service
    auth_request /auth;

    # Generate HMAC token
    set $timestamp $msec;
    set $message "$auth_user_id:$auth_role:$auth_groups:$timestamp";
    set $signature hmac_sha256($message, "shared-secret");

    # Send as Authorization header
    proxy_set_header Authorization "Bearer $auth_user_id:$auth_role:$auth_groups:$timestamp:$signature";
    proxy_pass http://vfs-service:8080;
}
```

---

## 🧪 Development Configuration

### Header-Based Auth (UNSAFE - Development Only)

**Use Case:** Local development, testing

```bash
# .env.development
AUTH_PROVIDER=headers
AUTH_ALLOW_ANONYMOUS=true  # Allow requests without auth headers

# Cache settings (shorter for faster dev feedback)
SCHEMA_CACHE_TTL_SECONDS=10
POLICY_CACHE_TTL_SECONDS=10
QUOTA_CACHE_TTL_SECONDS=10
```

**Request Example:**
```bash
curl -H "X-User-ID: alice" \
     -H "X-User-Role: admin" \
     -H "X-User-Groups: engineering,admins" \
     http://localhost:8080/api/v1/files/data/users/alice.json
```

**⚠️ WARNING:** Never use `AUTH_PROVIDER=headers` in production!

---

## 🔄 Switching Auth Providers

You can switch auth providers by changing the `AUTH_PROVIDER` environment variable. No code changes required!

### Example: Development → Production

```bash
# Development (local)
export AUTH_PROVIDER=headers
export AUTH_ALLOW_ANONYMOUS=true

# Staging (test with JWT)
export AUTH_PROVIDER=jwt
export AUTH_JWT_SECRET=staging-secret-key-min-32-chars
export AUTH_JWT_ISSUER=https://auth.staging.example.com

# Production (JWT)
export AUTH_PROVIDER=jwt
export AUTH_JWT_SECRET=production-secret-key-min-32-chars
export AUTH_JWT_ISSUER=https://auth.example.com
```

---

## 🐳 Docker Compose Examples

### Development with Header Auth

```yaml
# docker-compose.dev.yml
services:
  vfs-service:
    image: vfs:latest
    environment:
      - AUTH_PROVIDER=headers
      - AUTH_ALLOW_ANONYMOUS=true
      - DB_DSN=root:password@tcp(mysql:3306)/vfs?parseTime=true
      - SCHEMA_CACHE_TTL_SECONDS=10
      - POLICY_CACHE_TTL_SECONDS=10
    ports:
      - "8080:8080"
```

### Production with JWT

```yaml
# docker-compose.prod.yml
services:
  vfs-service:
    image: vfs:latest
    environment:
      - AUTH_PROVIDER=jwt
      - AUTH_JWT_SECRET=${JWT_SECRET}  # from .env file or secrets
      - AUTH_JWT_ISSUER=https://auth.example.com
      - AUTH_ALLOW_ANONYMOUS=false
      - DB_DSN=${DATABASE_DSN}
      - SCHEMA_CACHE_TTL_SECONDS=300
      - POLICY_CACHE_TTL_SECONDS=300
    ports:
      - "8080:8080"
    secrets:
      - jwt_secret
      - database_dsn
```

---

## 🔒 Security Best Practices

### 1. JWT Secrets

- **Minimum 32 characters**
- Use strong random strings: `openssl rand -base64 32`
- Rotate regularly (store in secrets manager)
- Never commit to git

### 2. Token Expiration

Configure short-lived tokens in your JWT issuer:
```json
{
  "exp": 1735689600,  // 15 minutes from now
  "iat": 1735688700
}
```

### 3. HTTPS Only

Always use HTTPS in production:
```nginx
server {
    listen 443 ssl;
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://vfs-service:8080;
    }
}
```

### 4. Network Isolation

```yaml
# docker-compose.yml
services:
  vfs-service:
    networks:
      - internal
    # No public ports - only via reverse proxy

  nginx:
    networks:
      - internal
      - public
    ports:
      - "443:443"
```

---

## 📋 Complete Configuration Reference

```bash
# ============================================================================
# Authentication
# ============================================================================

# Provider: jwt, oauth, mtls, proxy, headers (dev only)
AUTH_PROVIDER=jwt

# JWT Configuration
AUTH_JWT_SECRET=your-secret-key-minimum-32-characters-long
AUTH_JWT_ISSUER=https://auth.yourcompany.com

# OAuth Configuration
AUTH_OAUTH_INTROSPECTION_URL=https://oauth.provider.com/oauth/introspect
AUTH_OAUTH_CLIENT_ID=vfs-service
AUTH_OAUTH_CLIENT_SECRET=your-oauth-client-secret

# Proxy Configuration
AUTH_PROXY_SHARED_SECRET=shared-secret-between-proxy-and-vfs

# mTLS Configuration
AUTH_MTLS_CA_FILE=/etc/vfs/certs/ca.pem
AUTH_MTLS_CERT_FILE=/etc/vfs/certs/server.crt
AUTH_MTLS_KEY_FILE=/etc/vfs/certs/server.key

# Allow anonymous access (dev only)
AUTH_ALLOW_ANONYMOUS=false

# ============================================================================
# Cache Configuration
# ============================================================================

SCHEMA_CACHE_TTL_SECONDS=300  # 5 minutes
POLICY_CACHE_TTL_SECONDS=300  # 5 minutes
QUOTA_CACHE_TTL_SECONDS=300   # 5 minutes

# ============================================================================
# Database
# ============================================================================

DB_DSN=user:password@tcp(localhost:3306)/vfs?parseTime=true
LOG_LEVEL=info

# ============================================================================
# Server
# ============================================================================

PORT=8080

# ============================================================================
# Storage
# ============================================================================

S3_BUCKET=vfs-files
S3_REGION=us-east-1
S3_ENDPOINT=https://s3.amazonaws.com

# ============================================================================
# Idempotency
# ============================================================================

IDEMPOTENCY_TTL_SECONDS=86400  # 24 hours
```

---

## 🧪 Testing Different Auth Providers

```bash
# Test with JWT
AUTH_PROVIDER=jwt \
AUTH_JWT_SECRET=test-secret-min-32-chars-long-key \
AUTH_JWT_ISSUER=https://test.example.com \
go run ./services/vfs/main.go

# Test with headers (dev mode)
AUTH_PROVIDER=headers \
AUTH_ALLOW_ANONYMOUS=true \
go run ./services/vfs/main.go

# Test with OAuth
AUTH_PROVIDER=oauth \
AUTH_OAUTH_INTROSPECTION_URL=https://oauth.test.com/introspect \
AUTH_OAUTH_CLIENT_ID=test \
AUTH_OAUTH_CLIENT_SECRET=secret \
go run ./services/vfs/main.go
```

---

**Key Benefits:**
- ✅ **Centralized** - All config in one place
- ✅ **Flexible** - Switch providers without code changes
- ✅ **Secure** - Production-grade options (JWT, OAuth, mTLS)
- ✅ **Dev-Friendly** - Simple header mode for local development
- ✅ **Environment-Based** - Different configs for dev/staging/prod

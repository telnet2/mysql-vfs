# MySQL VFS Configuration Guide

This guide covers all configuration options for MySQL VFS, including the new YAML-based configuration system.

## Table of Contents

- [Overview](#overview)
- [Configuration Methods](#configuration-methods)
- [Configuration Priority](#configuration-priority)
- [Configuration Files](#configuration-files)
- [Environment Variables](#environment-variables)
- [Migration Guide](#migration-guide)
- [Database Configuration](#database-configuration)
  - [Table Prefix](#table-prefix)
- [Complete Configuration Reference](#complete-configuration-reference)

---

## Overview

MySQL VFS supports multiple configuration methods to provide flexibility across different deployment environments:

1. **YAML Configuration Files** (Recommended) - Structured, version-controllable configuration
2. **Environment Variables** - For secrets and container orchestration
3. **Command-line Flags** - For one-off overrides

## Configuration Methods

### YAML Configuration Files

Create a `config.yaml` file in your project root or specify a custom path:

```bash
# Use default config file locations
./vfs-service

# Specify custom config file
./vfs-service --conf /path/to/config.yaml

# Use environment variable
export VFS_CONFIG_FILE=/path/to/config.yaml
./vfs-service
```

**Config file discovery order:**
1. Path from `--config` flag
2. Path from `VFS_CONFIG_FILE` environment variable
3. `./config.yaml`
4. `./config/config.yaml`
5. `/etc/vfs/config.yaml`
6. `$HOME/.vfs/config.yaml`

### Environment Variables

All configuration values can be set via environment variables for backward compatibility:

```bash
export DB_DSN="root:root@tcp(localhost:3306)/vfs"
export S3_BUCKET="my-bucket"
export AUTH_PROVIDER="jwt"
export JWT_SECRET="my-secret"
```

### Environment Variable Interpolation

Config files support environment variable expansion using `${VAR_NAME}` syntax:

```yaml
auth:
  jwt:
    secret: "${JWT_SECRET}"  # Will be replaced with env var value
```

This is the **recommended approach** for handling secrets in config files.

## Configuration Priority

When the same configuration is specified in multiple places, this is the priority order (highest to lowest):

1. **Command-line flags** (e.g., `--port 8080`)
2. **Environment variables** (e.g., `PORT=8080`)
3. **Config file values** (e.g., `server.port: 8080`)
4. **Default values** (hardcoded in code)

Example:
```yaml
# config.yaml
server:
  port: 8080
```

```bash
# Environment variable overrides config file
export PORT=9090

# Both exist, PORT=9090 wins
./vfs-service
```

## Configuration Files

### Quick Start

1. **Copy example config:**
   ```bash
   cp config.example.yaml config.yaml
   ```

2. **Edit for your environment:**
   ```bash
   vim config.yaml
   ```

3. **Set secrets via environment variables:**
   ```bash
   export JWT_SECRET="your-secret-key"
   export SYSTEM_ADMIN_TOKEN="your-admin-token"
   ```

4. **Run service:**
   ```bash
   ./vfs-service
   ```

### Environment-Specific Configs

Create different config files for each environment:

```bash
# Development
cp config.development.yaml config.yaml

# Production  
cp config.production.yaml config.yaml
```

Or use environment variable to switch:

```bash
# Development
export VFS_CONFIG_FILE=config.development.yaml
./vfs-service

# Production
export VFS_CONFIG_FILE=config.production.yaml
./vfs-service
```

### Docker Compose

Mount config file as volume:

```yaml
services:
  vfs-service:
    image: mysql-vfs:latest
    volumes:
      - ./config.yaml:/app/config.yaml
    environment:
      - VFS_CONFIG_FILE=/app/config.yaml
      - JWT_SECRET=${JWT_SECRET}
      - SYSTEM_ADMIN_TOKEN=${SYSTEM_ADMIN_TOKEN}
```

### Kubernetes

Use ConfigMap for config file and Secret for sensitive values:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vfs-config
data:
  config.yaml: |
    database:
      dsn: "${DB_DSN}"
    auth:
      provider: "jwt"
      jwt:
        secret: "${JWT_SECRET}"
---
apiVersion: v1
kind: Secret
metadata:
  name: vfs-secrets
type: Opaque
stringData:
  JWT_SECRET: "your-jwt-secret"
  DB_DSN: "root:password@tcp(mysql:3306)/vfs"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vfs-service
spec:
  template:
    spec:
      containers:
      - name: vfs
        image: mysql-vfs:latest
        volumeMounts:
        - name: config
          mountPath: /etc/vfs
        envFrom:
        - secretRef:
            name: vfs-secrets
      volumes:
      - name: config
        configMap:
          name: vfs-config
```

## Environment Variables

### Complete List

All environment variables with their corresponding config file paths:

| Environment Variable | Config Path | Description | Default |
|---------------------|-------------|-------------|---------|
| `DB_DSN` | `database.dsn` | MySQL connection string | See example |
| `TABLE_PREFIX` | `database.table_prefix` | Table name prefix | `vfs_` |
| `S3_ENDPOINT` | `storage.s3.endpoint` | S3 endpoint URL | - |
| `S3_BUCKET` | `storage.s3.bucket` | S3 bucket name | - |
| `AWS_REGION` | `storage.s3.region` | AWS region | `us-east-1` |
| `NATS_URL` | `messaging.nats.url` | NATS server URL | - |
| `PORT` | `server.port` | HTTP server port | `8080` |
| `LOG_LEVEL` | `logging.level` | Log level | `info` |
| `AUTH_PROVIDER` | `auth.provider` | Auth provider type | `headers` |
| `JWT_SECRET` | `auth.jwt.secret` | JWT signing secret | - |
| `AUTH_JWT_ISSUER` | `auth.jwt.issuer` | JWT issuer | `mysql-vfs` |
| `SYSTEM_ADMIN_TOKEN` | `auth.system_admin.token` | System admin token | - |
| `USER_CACHE_TTL_SECONDS` | `cache.user_ttl` | User cache TTL | `5m` |
| `SCHEMA_CACHE_TTL_SECONDS` | `cache.schema_ttl` | Schema cache TTL | `5m` |
| `POLICY_CACHE_TTL_SECONDS` | `cache.policy_ttl` | Policy cache TTL | `5m` |
| `QUOTA_CACHE_TTL_SECONDS` | `cache.quota_ttl` | Quota cache TTL | `5m` |
| `IDEMPOTENCY_TTL_SECONDS` | `idempotency.ttl` | Idempotency TTL | `24h` |
| `WORKER_COUNT` | `services.event_worker.worker_count` | Event worker count | `5` |
| `POLL_INTERVAL` | `services.event_worker.poll_interval` | Worker poll interval | `1s` |
| `BATCH_SIZE` | `services.event_worker.batch_size` | Worker batch size | `10` |
| `SCHEDULER_ID` | `services.scheduler.scheduler_id` | Scheduler ID | auto |
| `EVENT_BUFFER_SIZE` | `services.event_publisher.event_buffer_size` | Event buffer size | `1000` |
| `SSE_MAX_CONNECTIONS` | `services.event_publisher.max_connections` | Max SSE connections | `100` |

## Migration Guide

### From Environment Variables to Config File

**Before (environment variables only):**
```bash
export DB_DSN="root:root@tcp(localhost:3306)/vfs"
export S3_ENDPOINT="http://localhost:4566"
export S3_BUCKET="my-bucket"
export AUTH_PROVIDER="jwt"
export JWT_SECRET="my-secret"
./vfs-service
```

**After (config file + env vars for secrets):**

1. Create `config.yaml`:
```yaml
database:
  dsn: "root:root@tcp(localhost:3306)/vfs"
  table_prefix: "vfs_"

storage:
  s3:
    endpoint: "http://localhost:4566"
    bucket: "my-bucket"

auth:
  provider: "jwt"
  jwt:
    secret: "${JWT_SECRET}"
```

2. Keep only secrets as env vars:
```bash
export JWT_SECRET="my-secret"
./vfs-service
```

### Gradual Migration Strategy

You can migrate gradually - the system supports both approaches simultaneously:

**Phase 1:** Keep using environment variables (no changes needed)

**Phase 2:** Add config file for non-sensitive values, keep env vars for secrets

**Phase 3:** Move all env vars to config file with `${VAR}` interpolation

**Phase 4:** Remove deprecated env vars (in future major version)

## Database Configuration

### Table Prefix

MySQL VFS supports configurable table prefixes to allow multiple VFS instances to share the same MySQL database or to avoid naming conflicts.

**Configuration:**

```yaml
database:
  dsn: "root:root@tcp(localhost:3306)/vfs"
  table_prefix: "vfs_"  # Default is "vfs_"
```

Or via environment variable:
```bash
export TABLE_PREFIX="myapp_vfs_"
```

**How it works:**

All database tables will be created with the specified prefix:
- `directories` → `vfs_directories`
- `files` → `vfs_files`
- `file_versions` → `vfs_file_versions`
- etc.

**Use cases:**

1. **Multi-tenant databases** - Run multiple VFS instances in one database:
   ```yaml
   # Instance 1
   database:
     dsn: "root:root@tcp(localhost:3306)/shared_db"
     table_prefix: "tenant1_"

   # Instance 2
   database:
     dsn: "root:root@tcp(localhost:3306)/shared_db"
     table_prefix: "tenant2_"
   ```

2. **Avoid naming conflicts** - When sharing database with other applications:
   ```yaml
   database:
     dsn: "root:root@tcp(localhost:3306)/myapp_db"
     table_prefix: "vfs_"  # Prevents conflicts with app's own tables
   ```

3. **Testing environments** - Separate test data from production in same database:
   ```yaml
   # Production
   table_prefix: "prod_"

   # Testing
   table_prefix: "test_"
   ```

**Important notes:**

- The prefix is applied during database migrations (table creation)
- If you change the prefix on an existing database, you must either:
  - Manually rename all existing tables to match the new prefix, or
  - Drop and recreate the database (data loss!)
- The prefix applies to all tables created by MySQL VFS
- Default prefix is `vfs_` if not specified

**Example with no prefix:**

```yaml
database:
  dsn: "root:root@tcp(localhost:3306)/vfs"
  table_prefix: ""  # Empty string = no prefix
```

This creates tables without any prefix: `directories`, `files`, etc.

## Complete Configuration Reference

See `config.example.yaml` for a fully annotated configuration file with all options, defaults, and documentation.

### Minimum Required Configuration

```yaml
database:
  dsn: "root:root@tcp(localhost:3306)/vfs"
  table_prefix: "vfs_"  # Optional, default is "vfs_"

storage:
  s3:
    endpoint: "http://localhost:4566"
    bucket: "my-bucket"
    region: "us-east-1"

server:
  port: 8080

logging:
  level: "info"

auth:
  provider: "headers"  # Use "jwt" in production
```

### Development Configuration

```yaml
database:
  dsn: "root:root@tcp(localhost:3306)/vfs"
  table_prefix: "vfs_"

storage:
  s3:
    endpoint: "http://localhost:4566"
    bucket: "dev-bucket"
    region: "us-east-1"

logging:
  level: "debug"

auth:
  provider: "headers"
  allow_anonymous: true

cache:
  user_ttl: "1m"
  schema_ttl: "1m"
```

### Production Configuration

```yaml
database:
  dsn: "${DB_DSN}"  # From secrets manager
  table_prefix: "vfs_"

storage:
  s3:
    endpoint: "https://s3.amazonaws.com"
    bucket: "${S3_BUCKET}"
    region: "${AWS_REGION}"

logging:
  level: "info"

auth:
  provider: "jwt"
  allow_anonymous: false
  jwt:
    secret: "${JWT_SECRET}"
    issuer: "mysql-vfs-production"

services:
  event_worker:
    worker_count: 20
    batch_size: 50

  event_publisher:
    event_buffer_size: 5000
    max_connections: 500
    auth_enabled: true
```

## Validation

Validate your config file before deploying:

```bash
# Test loading the config file
./vfs-service --conf config.yaml

# The service will report if config validation fails
```

## Troubleshooting

### Config file not found

```
Error: failed to read config file: Config File "config" Not Found
```

**Solution:** Specify config file path explicitly:
```bash
./vfs-service --conf /path/to/config.yaml
```

Or set environment variable:
```bash
export VFS_CONFIG_FILE=/path/to/config.yaml
```

### Environment variable not interpolated

```yaml
auth:
  jwt:
    secret: "${JWT_SECRET}"
```

If this shows `"${JWT_SECRET}"` instead of the actual value, ensure the environment variable is set:

```bash
export JWT_SECRET="actual-secret-value"
```

### Validation errors

```
Error: config validation failed: auth config: JWT secret is required when using jwt provider
```

**Solution:** Set the required field or environment variable:
```bash
export JWT_SECRET="your-secret"
```

Or in config file (not recommended for production):
```yaml
auth:
  jwt:
    secret: "your-secret"
```

## Best Practices

1. **Use config files for structure** - Keep all non-sensitive configuration in version-controlled YAML files

2. **Use env vars for secrets** - Never commit secrets to version control, use `${VAR}` interpolation

3. **Environment-specific configs** - Maintain separate configs for dev/staging/prod

4. **Validate before deploy** - Always validate config files before production deployment

5. **Document custom values** - Add comments to explain non-obvious configuration choices

6. **Use secrets management** - In production, use Kubernetes Secrets, AWS Secrets Manager, Vault, etc.

7. **Test config changes** - Test configuration changes in non-production environments first

## See Also

- [config.example.yaml](../config.example.yaml) - Fully annotated example config
- [Security Guide](SECURITY.md) - Authentication and authorization configuration
- [Microservices Guide](MICROSERVICES.md) - Service-specific configuration
- [Operations Guide](OPERATIONS.md) - Deployment and operations

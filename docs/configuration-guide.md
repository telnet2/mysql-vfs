# VFS Configuration Guide

## Overview

The VFS system uses environment variables for configuration. All configuration can be customized via the `.env` file or by setting environment variables directly.

## Configuration Package

Starting from this version, we've introduced a centralized configuration package at `pkg/config` that manages all application settings.

## Environment Variables

### Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DSN` | `root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local` | MySQL connection string |

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |

### Idempotency Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `IDEMPOTENCY_TTL_SECONDS` | `86400` (24 hours) | Time-to-live for idempotency records in seconds |

**Why configurable?**
- Production: Use default 24-hour TTL for safety
- Testing: Use shorter TTL (e.g., 150ms) for realistic CI tests
- High-traffic systems: Adjust based on your request patterns

**Example configurations:**

```bash
# Production (default)
IDEMPOTENCY_TTL_SECONDS=86400  # 24 hours

# Testing
IDEMPOTENCY_TTL_SECONDS=1      # 1 second for fast tests

# High-frequency operations
IDEMPOTENCY_TTL_SECONDS=3600   # 1 hour
```

### Storage Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_ENDPOINT` | `http://localhost:4566` | S3-compatible endpoint (e.g., LocalStack, MinIO) |
| `S3_BUCKET` | `vfs-files` | S3 bucket name |
| `S3_REGION` | `us-east-1` | S3 region |
| `AWS_ACCESS_KEY_ID` | `test` | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | `test` | AWS secret key |

### Webhook Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `WEBHOOK_DAEMON_URL` | `http://localhost:9000` | Webhook daemon URL |

### Worker Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_CONCURRENCY` | `10` | Number of concurrent event workers |
| `POLL_INTERVAL` | `1s` | Event polling interval |

### Scheduler Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHEDULER_ID` | `scheduler-1` | Unique scheduler instance ID |

## Usage in Code

### Loading Configuration

```go
import "github.com/telnet2/mysql-vfs/pkg/config"

func main() {
    cfg := config.LoadFromEnv()

    // Access configuration
    fmt.Println("Server port:", cfg.ServerPort)
    fmt.Println("Idempotency TTL:", cfg.IdempotencyTTL)
}
```

### Using Custom TTL for Testing

```go
import "github.com/telnet2/mysql-vfs/pkg/idempotency"

// Production - uses default 24-hour TTL
service := idempotency.NewService(db)

// Testing - uses custom TTL
testService := idempotency.NewServiceWithTTL(db, 150*time.Millisecond)
```

## Configuration File

Create a `.env` file in the project root:

```bash
# Copy from example
cp .env.example .env

# Edit with your values
vim .env
```

See `.env.example` for a complete reference configuration.

## Docker Compose

When using Docker Compose, environment variables are automatically loaded from `.env`:

```yaml
services:
  vfs:
    image: vfs-service
    env_file:
      - .env
    # Or specify directly
    environment:
      - IDEMPOTENCY_TTL_SECONDS=3600
```

## Testing Configuration

For testing, you can override configuration programmatically:

```go
// In test setup
os.Setenv("IDEMPOTENCY_TTL_SECONDS", "1")
cfg := config.LoadFromEnv()
// cfg.IdempotencyTTL will be 1 second
```

Or use the `NewServiceWithTTL` constructor for granular control:

```go
testService := idempotency.NewServiceWithTTL(testDB, 150*time.Millisecond)
```

## Best Practices

1. **Never commit `.env` files** - Use `.env.example` as a template
2. **Use defaults for development** - Override only what's necessary
3. **Document custom values** - Add comments explaining non-standard settings
4. **Test with realistic TTLs** - Use sub-second TTLs (150-500ms) for CI tests
5. **Monitor in production** - Track idempotency cache hit rates and cleanup performance

## Migration from Hardcoded Values

If upgrading from an earlier version:

**Before:**
```go
// Hardcoded values
db.Connect("root:root@tcp(localhost:3306)/vfs")
service := idempotency.NewService(db) // Always 24 hours
```

**After:**
```go
// Configurable via environment
cfg := config.LoadFromEnv()
db.Connect(cfg.DatabaseDSN)
service := idempotency.NewServiceWithTTL(db, cfg.IdempotencyTTL)
```

## Troubleshooting

### Issue: Idempotency records expiring too quickly

**Solution:** Increase `IDEMPOTENCY_TTL_SECONDS`:
```bash
IDEMPOTENCY_TTL_SECONDS=172800  # 48 hours
```

### Issue: Database filling up with old idempotency records

**Solution:** Decrease TTL or increase cleanup frequency:
```bash
IDEMPOTENCY_TTL_SECONDS=3600  # 1 hour
```

The cleanup worker runs every hour by default and removes expired records.

### Issue: CI tests taking too long

**Solution:** Use shorter TTL for tests:
```go
// In test setup
testService := idempotency.NewServiceWithTTL(db, 150*time.Millisecond)
```

This allows realistic expiration testing without waiting 24 hours.

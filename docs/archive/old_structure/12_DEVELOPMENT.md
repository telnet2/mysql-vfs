# 12. Development Guide

**MySQL VFS v2.1+ Development Setup and Architecture**

[← Back: Testing](11_TESTING.md) | [Index](0_README.md)

---

## Project Structure

```
mysql-vfs/
├── pkg/                      # Core packages
│   ├── config/               # Configuration (pkg/config/config.go)
│   ├── middleware/           # Auth, validation, authorization
│   │   ├── auth.go           # Authentication middleware
│   │   ├── auth_providers.go # System admin, file-based, header auth
│   │   ├── authorization.go  # OPA integration
│   │   └── default_policy.go # Default authorization policy
│   ├── domain/               # Business logic (service layer)
│   │   ├── file_service.go           # File operations
│   │   ├── directory_service.go      # Directory operations
│   │   ├── files_loader.go           # .files special file loader
│   │   ├── user_loader.go            # .user special file loader
│   │   ├── events_loader.go          # .events special file loader
│   │   ├── policy_loader.go          # .rego special file loader
│   │   ├── owner_loader.go           # .owner special file loader
│   │   ├── group_loader.go           # (Deprecated) .group loader
│   │   ├── protection.go             # Resource protection logic
│   │   └── file_validator.go        # File validation
│   ├── models/               # Domain models
│   │   ├── file.go           # File model
│   │   ├── file_version.go   # File version model
│   │   └── directory.go      # Directory model
│   ├── events/               # Event system
│   │   ├── types.go          # Event types and structures
│   │   └── handlers/         # Event handlers
│   │       ├── webhook.go    # Webhook handler
│   │       ├── log.go        # Log handler
│   │       └── metrics.go    # Metrics handler
│   ├── persistence/          # Database persistence layer
│   └── setup/                # Service setup utilities
├── services/                 # Microservices
│   ├── vfs/                  # Main VFS service
│   │   ├── main.go           # Entry point
│   │   └── handlers/         # HTTP handlers
│   │       ├── directory.go  # Directory API
│   │       ├── file.go       # File API (removed, see handlers/)
│   │       ├── errors.go     # Error handling
│   │       └── auth.go       # Auth endpoints
│   ├── event-worker/         # Event processing worker
│   ├── scheduler/            # Cron scheduler
│   └── webhook-orchestrator/ # Webhook delivery service
├── citest/                   # Integration tests (103/104 passing)
├── scripts/                  # Build and deployment scripts
└── docs/                     # Documentation

## Development Setup

```bash
# Clone repo
git clone https://github.com/telnet2/mysql-vfs
cd mysql-vfs

# Start dependencies (MySQL + MinIO)
docker-compose up -d mysql minio

# Set up development environment
export AUTH_PROVIDER=headers
export AUTH_ALLOW_ANONYMOUS=true
export DB_DSN=root:root@tcp(localhost:3306)/vfs?parseTime=true
export S3_ENDPOINT=http://localhost:9000
export S3_BUCKET=vfs-dev
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin

# Run VFS locally
go run ./services/vfs/main.go

# Run all tests (Ginkgo)
ginkgo -r -p citest/

# Run specific test file
ginkgo -r --focus="File Operations" citest/
```

---

## Key Architecture Decisions

### Special Files System

Special files (`.files`, `.user`, `.events`, `.rego`, `.owner`) are stored as regular files but loaded on-demand with caching:

- **Loaders:** `pkg/domain/*_loader.go` - Load and parse special files
- **Caching:** TTL-based caching (default 5 minutes)
- **Inheritance:** Policies and events inherit from parent directories

### Authentication Flow

1. **System Admin Check** (`pkg/middleware/auth_providers.go`) - Check `SYSTEM_ADMIN_TOKEN`
2. **File-Based Auth** (`pkg/domain/user_loader.go`) - Load `.user` file, validate token
3. **Header Auth** (dev only) - Extract user from headers

### Authorization Flow

1. **Default Policy** (`pkg/middleware/default_policy.go`) - Owner-based and role-based access
2. **OPA Policies** (`pkg/domain/policy_loader.go`) - Load `.rego` files, evaluate with OPA
3. **Resource Protection** (`pkg/domain/protection.go`) - Enforce `.owner` file protections

### Event System

- **Event Types** (`pkg/events/types.go`) - `file.created`, `file.updated`, etc.
- **Handlers** (`pkg/events/handlers/`) - webhook, log, metrics
- **Configuration** (`pkg/domain/events_loader.go`) - Load from `.events` files
- **Veto Support** - Handlers can veto operations during validation/authorization stages

## Adding a New Feature

1. **Domain Layer** (`pkg/domain/`) - Add business logic and services
2. **Models** (`pkg/models/`) - Add data models (if needed)
3. **Handler** (`services/vfs/handlers/`) - Add HTTP endpoint
4. **Middleware** (`pkg/middleware/`) - Add cross-cutting concern (if needed)
5. **Tests** (`citest/`, `pkg/domain/*_test.go`) - Add E2E and unit tests

### Example: Adding a New Special File

1. Create loader in `pkg/domain/my_special_file_loader.go`
2. Define structure and parsing logic
3. Add caching with TTL
4. Wire up in service initialization (`services/vfs/main.go`)
5. Add tests in `pkg/domain/my_special_file_loader_test.go`

---

## Code Style

- Follow Go best practices
- Use `gofmt` and `golangci-lint`
- Write tests for new features (aim for 80%+ coverage on critical paths)
- Document public APIs with godoc comments
- Use structured logging (zerolog)
- Handle errors explicitly (no silent failures)

## Common Development Tasks

### Running Locally

```bash
# Start services
docker-compose up -d

# Run VFS service
go run ./services/vfs/main.go

# Run event worker
go run ./services/event-worker/main.go

# Run scheduler
go run ./services/scheduler/main.go
```

### Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Use delve debugger
dlv debug ./services/vfs/main.go
```

### Database Migrations

Database schema is managed through migrations (previously in `pkg/db/migrate.go`, now removed).
Schema is automatically created on service startup.

### Testing

```bash
# Run all tests
ginkgo -r -p citest/

# Run with coverage
ginkgo -r -p --cover --coverprofile=coverage.out citest/

# Run specific test
ginkgo -r --focus="should create file" citest/

# Run with race detector
ginkgo -r -p --race citest/
```

## Key Files Reference

- `services/vfs/main.go` - VFS service entry point
- `pkg/config/config.go` - Configuration loading
- `pkg/middleware/auth.go` - Authentication middleware (line 45: token validation)
- `pkg/middleware/authorization.go` - OPA authorization (line 80: policy evaluation)
- `pkg/domain/file_service.go` - File operations (line 120: CreateFile)
- `pkg/domain/directory_service.go` - Directory operations (line 60: CreateDirectory)
- `pkg/domain/user_loader.go` - Load users from `.user` files (line 35: LoadUsers)
- `pkg/domain/policy_loader.go` - Load policies from `.rego` files (line 50: LoadPolicy)
- `pkg/events/handlers/webhook.go` - Webhook event handler (line 90: Handle, line 150: veto logic)
- `pkg/events/types.go` - Event type definitions

## Resources

- [Special Files Guide](4_SPECIAL_FILES.md)
- [Authorization Guide](6_AUTHORIZATION.md)
- [Lifecycle Events](15_LIFECYCLE_EVENTS.md)
- [Testing Guide](11_TESTING.md)

---

[← Back: Testing](11_TESTING.md) | [Index](0_README.md)

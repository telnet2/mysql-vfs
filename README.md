# MySQL-based Distributed Virtual File System

A distributed virtual file system (VFS) backed by MySQL with strong consistency guarantees, idempotent operations, lifecycle event tracking, and file-based authentication.

## Project Status

**Current Version**: v2.1+
**Status**: Production Ready

✅ Complete layered architecture
✅ File-based authentication
✅ Owner-based access control
✅ Lifecycle event system with veto support
✅ Pattern-based file validation
✅ Resource protection system
✅ **Workflow system with directory-as-state architecture**

See [Implementation Status](docs/21_IMPLEMENTATION_STATUS.md) for detailed progress.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.25+ (for local development)
- Make (optional, for convenience commands)

### Start All Services

```bash
# Using make
make up

# Or directly with docker-compose
docker-compose up -d
```

### Bootstrap the System

```bash
# Generate system admin token
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)

# Add to .env file
echo "SYSTEM_ADMIN_TOKEN=$SYSTEM_ADMIN_TOKEN" >> .env

# Create default configuration files
cd scripts
go run bootstrap.go
```

See [Bootstrap Guide](docs/18_BOOTSTRAP.md) for detailed instructions.

### Check Service Health

```bash
# Check all services
make status

# Check VFS service health endpoint
curl http://localhost:8080/health
```

### View Logs

```bash
# All services
make logs

# Specific service
make logs-vfs
make logs-worker
```

### Stop Services

```bash
make down
```

## Architecture

### Core Principles

1. **Layered Architecture** - Clean separation of concerns
2. **Domain-Driven Design** - Business logic in domain layer
3. **Repository Pattern** - Data access abstraction
4. **Event-Driven** - Lifecycle events for observability and veto
5. **File-Based Auth** - Self-contained, no external user DB
6. **Workflow-Driven State** - Directory-as-state with Rego gates

### Package Structure

```
pkg/
├── domain/              # Business Logic Layer
│   ├── file_service.go         # File operations
│   ├── directory_service.go    # Directory operations
│   ├── *_loader.go             # Special file loaders
│   ├── file_validator.go       # Validation logic
│   └── protection.go           # Resource protection
│
├── persistence/         # Data Access Layer
│   ├── db/
│   │   ├── interfaces.go       # Repository contracts
│   │   ├── migrate.go          # Schema migrations
│   │   └── mysql/
│   │       ├── file.go         # File repo (GORM + S3)
│   │       ├── directory.go    # Directory repo
│   │       └── ...
│   └── storage/
│       └── s3.go               # S3 client
│
├── middleware/          # Cross-Cutting Concerns
│   ├── auth.go                 # Authentication
│   ├── authorization.go        # OPA-based authz
│   └── default_policy.go       # Fallback policy
│
├── events/              # Event System
│   ├── lifecycle_types.go      # Event types
│   ├── event_trigger.go        # Event dispatcher
│   └── handlers/               # Event handlers
│
├── models/              # Data Models (GORM)
├── setup/               # Bootstrap utilities
└── config/              # Configuration
```

**Source**: See `pkg/` directory and [Architecture](docs/2_ARCHITECTURE.md)

### Services

1. **VFS Service** (Port 8080) - `services/vfs/main.go`
   - Manages directories, files, and metadata
   - Provides REST API for CRUD operations
   - Handles file upload/download with S3 storage
   - Enforces OPA policies and idempotency
   - Emits lifecycle events

2. **Event Worker** - `services/event-worker/main.go`
   - Processes events from transactional outbox
   - Drives webhook delivery
   - Handles dead letter queue

3. **Webhook Orchestrator** - `services/webhook-orchestrator/main.go`
   - Dispatches webhook notifications
   - Implements circuit breaker pattern
   - Tracks delivery status and retries

4. **Scheduler** - `services/scheduler/main.go`
   - Executes cron jobs with lease-based locking
   - Provides heartbeat and recovery mechanisms

5. **CLI** (Interactive) - `cli/`
   - REPL interface for VFS operations
   - Supports piping and jq querying
   - See [CLI How-To](docs/CLI_HOWTO.md)

### Database

- **MySQL 8.0** with full schema migrations
- Schema: `pkg/models/` - GORM data models
- Migrations: `pkg/persistence/db/migrate.go`

### Object Storage

- **LocalStack** (S3-compatible) for file storage
- Production: Use AWS S3, GCS, or Azure Blob via gocloud.dev
- Storage client: `pkg/persistence/storage/s3.go`

## Key Features

### 1. File-Based Authentication

No external user database needed. Users are defined in `.user` files.

**User File Format** (`/.user`):
```json
{
  "users": [
    {
      "user_id": "alice",
      "token": "secure-random-token",
      "password_hash": "$2a$10$...",
      "role": "admin"
    }
  ]
}
```

**Features:**
- Static token authentication
- Bcrypt password hashing
- Hybrid auth (system admin + file-based)
- 5-minute TTL caching

**Implementation**: `pkg/domain/user_loader.go`
**Documentation**: [Authentication](docs/5_AUTHENTICATION.md)

### 2. Owner-Based Access Control

Users can only access directories they own.

**Owner File Format** (`.owner`):
```json
{
  "owner": "alice"
}
```

**Features:**
- Inheritance from parent directories
- Integration with rego policies
- Full control for owners

**Implementation**: `pkg/domain/owner_loader.go`
**Documentation**: [Owner-Based Access](docs/20_OWNER_BASED_ACCESS.md)

### 3. OPA Authorization Policies

Flexible, rego-based authorization system.

**Policy File** (`/.rego`):
```rego
package vfs.authz

allow {
    input.user.role == "admin"
}

allow {
    input.user.role == "user"
    input.action == "read"
}
```

**Features:**
- Hierarchical policies (inherit from parents)
- Fallback to default policy if missing
- Hot-reload with caching

**Implementation**: `pkg/domain/policy_loader.go`
**Default Policy**: `pkg/middleware/default_policy.go`
**Documentation**: [Authorization](docs/6_AUTHORIZATION.md)

### 4. Lifecycle Event System

Complete operation observability with authorization-first approach.

**Event Flow:**
```
1. Authorization → Policy → Permission → Role
2. Validation    → Schema → Quota → Content → Size
3. Execution     → Lock → Transaction → Storage → Commit
4. Completion    → Success/Failure
```

**Event Examples:**
- `file.create.authorization.started`
- `file.create.validation.schema.checking`
- `file.create.validation.schema.checked.succeeded`
- `file.create.execution.started`
- `file.create.completed`

**Wildcard Patterns:**
```json
{
  "handlers": [
    {
      "name": "audit-all-auth",
      "events": ["*.*.authorization.*"],
      "type": "log"
    }
  ]
}
```

**Key Features:**
- Authorization-first (prevents info disclosure)
- Substage granularity (identify exact bottlenecks)
- Veto-capable handlers (external policy enforcement)
- Synchronous + async dispatch

**Implementation**: `pkg/events/`
**Documentation**: [Lifecycle Events](docs/15_LIFECYCLE_EVENTS.md)

### 5. Pattern-Based File Validation

Multiple validation rules per directory with glob and regex support.

**Files Configuration** (`.files`):
```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email"]
      }
    },
    {
      "pattern": "admin-.*\\.json",
      "type": "regex",
      "schema": {
        "required": ["role", "permissions"]
      }
    }
  ],
  "default_action": "deny"
}
```

**Features:**
- Glob and regex patterns
- Per-pattern JSON schemas
- Whitelist/blacklist modes
- Pattern inheritance

**Implementation**: `pkg/domain/files_loader.go`
**Documentation**: [Files Spec](docs/13_FILES_SPEC.md)

### 6. Resource Protection

Hard-coded rules to protect critical system files, separate from `.rego` policies.

**Protected Files:**
- `/.rego` - Authorization policy (system-admin only)
- `/.group` - Group definitions (system-admin only, root only)
- `/.user` - User credentials (system-admin only, root only)

**Features:**
- Cannot be bypassed via misconfigured policies
- Pluggable protection interface
- Chainable rules

**Implementation**: `pkg/domain/protection.go`
**Documentation**: [Resource Protection](docs/19_RESOURCE_PROTECTION.md)

### 7. Webhook Event Handlers

Reliable webhook delivery with retries and circuit breaker.

**Webhook Configuration** (`.events`):
```json
{
  "handlers": [
    {
      "name": "external-webhook",
      "events": ["file.create.completed"],
      "type": "webhook",
      "synchronous": false,
      "veto_enabled": false,
      "config": {
        "url": "https://example.com/webhook",
        "secret": "hmac-secret",
        "timeout_ms": 5000,
        "retry": {
          "max_attempts": 3,
          "initial_delay_ms": 1000,
          "backoff": "exponential"
        },
        "circuit_breaker": {
          "enabled": true,
          "failure_threshold": 5,
          "recovery_timeout_ms": 30000
        }
      }
    }
  ]
}
```

**Features:**
- HMAC signature verification
- Exponential/linear backoff retries
- Circuit breaker (closed/open/half-open)
- Veto capability (abort operations)

**Implementation**: `pkg/events/handlers/webhook.go`
**Documentation**: [Webhooks](docs/17_WEBHOOKS.md)

### 8. Workflow System

**Directory-as-state architecture** where file location determines workflow state.

**Workflow Configuration** (`.workflow`):
```yaml
state_directories:
  draft: "draft"
  review: "review"
  published: "published"
initial_state: draft
states:
  draft:
    transitions:
      - to: review
        gates:
          - policy: |
              package vfs.workflow.gates
              default allow = input.user.groups[_] == "editors"
  review:
    transitions:
      - to: published
      - to: draft
  published:
    transitions: []
```

**Features:**
- **State as Directory**: File state is implicit in its directory location
- **Rego-Based Gates**: Policy-driven transition validation
- **System Admin Bypass**: `system-admin` group bypasses all workflow gates
- **Audit Trail**: All transitions logged in `workflow_audit` table
- **Event-Driven**: Supports `move_file` action handler for automatic transitions
- **Authorization Integration**: Workflow context available in OPA policies
- **REST API**: Query workflow info and trigger transitions via API

**API Endpoints:**
```bash
# Get workflow information
GET /api/v1/workflows/{filepath}/info

# Get valid transitions
GET /api/v1/workflows/{filepath}/transitions

# Transition to new state
POST /api/v1/workflows/{filepath}/next
{
  "target_state": "review",
  "preserve_structure": true
}
```

**Implementation**: 
- Core: `pkg/domain/workflow_*.go`
- API: `services/vfs/handlers/workflow.go`
- Authorization: `pkg/middleware/authorization.go`

**Documentation**: 
- [Workflow API](docs/WORKFLOW_API.md)
- [Workflow Authorization](docs/WORKFLOW_AUTHORIZATION.md)

## Development

### Build Locally

```bash
# Download dependencies
go mod download

# Build all services
go build ./...

# Build specific service
go build -o ./bin/vfs-service ./services/vfs
```

### Run Tests

```bash
# Unit tests
make test

# Integration tests
cd citest && ginkgo -v

# Specific test
go test -v ./pkg/domain -run TestUserLoader
```

**Test Status**: 103/104 passing (1 flaky concurrency test)
**Test Coverage**: Unit tests + Integration tests
**Documentation**: [Testing Guide](docs/11_TESTING.md)

### Database Management

```bash
# Connect to MySQL shell
make db-shell

# Migrations run automatically on VFS service startup
# Migration code: pkg/persistence/db/migrate.go
```

### Format Code

```bash
make fmt
make tidy
```

## Configuration

Environment variables are configured via `.env` file. See `.env.example` for available options.

### Key Variables

**Database & Storage:**
```bash
DB_DSN=user:pass@tcp(mysql:3306)/vfsdb
S3_ENDPOINT=http://localstack:4566
S3_BUCKET=cc-vfs-storage
AWS_S3_FORCE_PATH_STYLE=true
```

**Authentication:**
```bash
SYSTEM_ADMIN_TOKEN=your-secure-random-token
SYSTEM_ADMIN_ID=system-admin
SYSTEM_ADMIN_ROLE=system-admin
AUTH_PROVIDER=file
```

**Service:**
```bash
PORT=8080
LOG_LEVEL=info
```

**Caching:**
```bash
POLICY_CACHE_TTL_SECONDS=300
USER_CACHE_TTL_SECONDS=300
OWNER_CACHE_TTL_SECONDS=300
```

**Source**: `pkg/config/config.go`
**Documentation**: [Configuration](docs/7_CONFIGURATION.md)

## Special Files

Special files control VFS behavior at the directory level:

| File | Purpose | Admin Only | Implementation |
|------|---------|------------|----------------|
| `.rego` | Authorization policy | ✅ | `pkg/domain/policy_loader.go` |
| `.user` | User credentials | ✅ | `pkg/domain/user_loader.go` |
| `.group` | Group membership | ✅ | `pkg/domain/group_loader.go` |
| `.owner` | Directory ownership | ❌ | `pkg/domain/owner_loader.go` |
| `.files` | File validation rules | ❌ | `pkg/domain/files_loader.go` |
| `.events` | Event handlers | ❌ | `pkg/domain/events_loader.go` |

**Documentation**: [Special Files](docs/4_SPECIAL_FILES.md)

## Documentation

### Feature Guides (⭐ Comprehensive)

- [**Workflows**](docs/WORKFLOWS.md) - Complete workflow system guide
- [**Special Files Framework**](docs/SPECIAL_FILES_FRAMEWORK.md) - Framework architecture & validation
- [Workflow API](docs/WORKFLOW_API.md) - Workflow REST API reference
- [Workflow Authorization](docs/WORKFLOW_AUTHORIZATION.md) - Workflow & authorization integration

### User Guides

- [Overview](docs/1_OVERVIEW.md) - System overview and concepts
- [Architecture](docs/2_ARCHITECTURE.md) - System architecture
- [Quickstart](docs/3_QUICKSTART.md) - Getting started
- [Special Files](docs/4_SPECIAL_FILES.md) - All special file types
- [Authentication](docs/5_AUTHENTICATION.md) - User authentication
- [Authorization](docs/6_AUTHORIZATION.md) - OPA policies
- [Configuration](docs/7_CONFIGURATION.md) - Environment config
- [Deployment](docs/9_DEPLOYMENT.md) - Production deployment
- [API](docs/10_API.md) - REST API reference
- [CLI How-To](docs/CLI_HOWTO.md) - CLI usage guide

### Technical Guides

- [Testing](docs/11_TESTING.md) - Testing strategies
- [Development](docs/12_DEVELOPMENT.md) - Developer guide
- [Files Spec](docs/13_FILES_SPEC.md) - File validation spec
- [Events Spec](docs/14_EVENTS_SPEC.md) - Event system spec
- [Lifecycle Events](docs/15_LIFECYCLE_EVENTS.md) - Implementation guide
- [Event Examples](docs/16_LIFECYCLE_EXAMPLES.md) - Usage examples
- [Webhooks](docs/17_WEBHOOKS.md) - Webhook integration
- [Bootstrap](docs/18_BOOTSTRAP.md) - System bootstrap
- [Resource Protection](docs/19_RESOURCE_PROTECTION.md) - Protection system
- [Owner-Based Access](docs/20_OWNER_BASED_ACCESS.md) - Ownership model
- [Implementation Status](docs/21_IMPLEMENTATION_STATUS.md) - Current status

## Bootstrap Guide

To set up a new VFS instance:

1. **Generate System Admin Token:**
```bash
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
echo "SYSTEM_ADMIN_TOKEN=$SYSTEM_ADMIN_TOKEN" >> .env
```

2. **Create Default Files:**
```bash
cd scripts
go run bootstrap.go
# Follow the displayed curl commands
```

3. **Create First User:**
```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"...\",\"role\":\"admin\"}]}"
  }'
```

**See**: [Bootstrap Guide](docs/18_BOOTSTRAP.md) for detailed instructions

## Code Reference

### Key Implementation Files

**Domain Layer:**
- File service: `pkg/domain/file_service.go`
- Directory service: `pkg/domain/directory_service.go`
- Loaders: `pkg/domain/*_loader.go`
- Protection: `pkg/domain/protection.go`
- Validation: `pkg/domain/file_validator.go`

**Persistence Layer:**
- Repository interfaces: `pkg/persistence/db/interfaces.go`
- File repository: `pkg/persistence/db/mysql/file.go`
- Directory repository: `pkg/persistence/db/mysql/directory.go`
- Migrations: `pkg/persistence/db/migrate.go`
- S3 storage: `pkg/persistence/storage/s3.go`

**Middleware:**
- Authentication: `pkg/middleware/auth.go`
- Auth providers: `pkg/middleware/auth_providers.go`
- Authorization: `pkg/middleware/authorization.go`
- Default policy: `pkg/middleware/default_policy.go`

**Event System:**
- Event types: `pkg/events/lifecycle_types.go`
- Event trigger: `pkg/events/event_trigger.go`
- Webhook handler: `pkg/events/handlers/webhook.go`
- Log handler: `pkg/events/handlers/log.go`
- Metrics handler: `pkg/events/handlers/metrics.go`

**Data Models:**
- File model: `pkg/models/file.go`
- Directory model: `pkg/models/directory.go`
- File version: `pkg/models/file_version.go`
- Event outbox: `pkg/models/event.go`

**Services:**
- VFS service: `services/vfs/main.go`
- Event worker: `services/event-worker/main.go`
- Webhook orchestrator: `services/webhook-orchestrator/main.go`
- Scheduler: `services/scheduler/main.go`

**Bootstrap:**
- Setup package: `pkg/setup/setup.go`
- Bootstrap script: `scripts/bootstrap.go`

**Configuration:**
- Config: `pkg/config/config.go`

## Contributing

This is a reference implementation project. Key branches:

- `main` - Stable release
- `claude-v1` - Current development

### Version Tags

- `v1.0.0` - Initial release
- `v2.0.0` - Layered architecture
- `v2.1.0` - File-based auth + lifecycle events

## License

MIT License - See LICENSE file for details

## See Also

- [Implementation Status](docs/21_IMPLEMENTATION_STATUS.md) - Detailed status
- [Architecture](docs/2_ARCHITECTURE.md) - Architecture deep dive
- [API Reference](docs/10_API.md) - REST API documentation
- [Development Guide](docs/12_DEVELOPMENT.md) - Developer onboarding

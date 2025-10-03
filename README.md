# MySQL-based Distributed Virtual File System

A distributed virtual file system (VFS) backed by MySQL with strong consistency guarantees, idempotent operations, and horizontal scalability.

## Project Status

**Current Phase**: Phase 1 Complete ✓

See [docs/planning.md](docs/planning.md) for the complete architectural specification.

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

## Project Structure

```
.
├── cli/                      # CLI client (Phase 5)
├── deployments/              # Deployment configurations
│   └── webhook-configs/      # Webhook daemon configs
├── docs/                     # Documentation
│   ├── planning.md           # Architecture specification
│   └── phase-*.md            # Phase reports
├── idl/                      # Thrift IDL definitions
│   └── vfs.thrift            # VFS service interface
├── pkg/                      # Shared packages
│   ├── db/                   # Database connectivity & migrations
│   └── models/               # GORM data models
├── services/                 # Microservices
│   ├── vfs/                  # VFS service (Phase 1-2)
│   ├── webhook-orchestrator/ # Webhook dispatcher (Phase 3)
│   ├── event-worker/         # Event processor (Phase 3)
│   └── scheduler/            # Cron scheduler (Phase 4)
├── docker-compose.yml        # Service orchestration
├── Makefile                  # Common operations
└── go.mod                    # Go dependencies
```

## Architecture

### Services

1. **VFS Service** (Port 8080)
   - Manages directories, files, and metadata
   - Provides REST API for CRUD operations
   - Handles file upload/download with S3 storage
   - Enforces OPA policies and idempotency

2. **Webhook Orchestrator**
   - Dispatches webhook notifications
   - Implements circuit breaker pattern
   - Tracks delivery status and retries

3. **Event Worker**
   - Processes events from transactional outbox
   - Drives webhook delivery
   - Handles dead letter queue

4. **Scheduler**
   - Executes cron jobs with lease-based locking
   - Provides heartbeat and recovery mechanisms

5. **CLI** (Interactive)
   - REPL interface for VFS operations
   - Supports piping and jq querying

### Database

- **MySQL 8.0** with full schema migrations
- See `pkg/models/` for data model definitions

### Object Storage

- **LocalStack** (S3-compatible) for file storage
- Production: Use AWS S3, GCS, or Azure Blob via gocloud.dev

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

# Integration tests (Phase 6)
make test-integration
```

### Database Management

```bash
# Connect to MySQL shell
make db-shell

# Migrations run automatically on VFS service startup
```

### Format Code

```bash
make fmt
make tidy
```

## Configuration

Environment variables are configured via `.env` file. See `.env.example` for available options.

Key variables:
- `DB_DSN`: MySQL connection string
- `S3_ENDPOINT`: S3-compatible storage endpoint
- `S3_BUCKET`: Storage bucket name
- `PORT`: VFS service port (default: 8080)
- `LOG_LEVEL`: Logging verbosity (debug, info, warn, error)

## Implementation Roadmap

- [x] **Phase 1**: Foundation (Scaffold, IDL, models, Docker Compose)
- [ ] **Phase 2**: Core VFS Logic (APIs, S3, idempotency, OPA)
- [ ] **Phase 3**: Event & Webhook System
- [ ] **Phase 4**: Cron & Scheduling
- [ ] **Phase 5**: CLI Gateway
- [ ] **Phase 6**: Testing & Hardening
- [ ] **Phase 7**: Documentation & Polish

See `docs/planning.md` for detailed phase descriptions.

## Phase Reports

- [Phase 1 Report](docs/phase-1-report.md) - Foundation complete

## Contributing

This is a reference implementation project. Phase completion is tracked via Git tags:
- `phase-1-complete`
- `phase-2-complete`
- etc.

## License

MIT License - See LICENSE file for details

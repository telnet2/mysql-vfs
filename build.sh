#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Get the current working directory (where the script was executed from)
WORK_DIR="$(pwd)"

# Output directory is relative to where the script was executed
OUTPUT_DIR="${WORK_DIR}/output"
BIN_DIR="${OUTPUT_DIR}/bin"
CONF_DIR="${OUTPUT_DIR}/conf"

echo -e "${GREEN}=== MySQL VFS Build Script ===${NC}"
echo "Script location: ${SCRIPT_DIR}"
echo "Working directory: ${WORK_DIR}"
echo "Output directory: ${OUTPUT_DIR}"
echo ""

# Clean and create output directories
echo -e "${YELLOW}Cleaning output directory...${NC}"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${BIN_DIR}"
mkdir -p "${CONF_DIR}"

# Change to script directory for building
cd "${SCRIPT_DIR}"

# Build all services
echo -e "${GREEN}Building services...${NC}"

echo "  → Building vfs-service..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/vfs-service" ./services/vfs

echo "  → Building event-worker..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/event-worker" ./services/event-worker

echo "  → Building scheduler..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/scheduler" ./services/scheduler

echo "  → Building webhook-orchestrator..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/webhook-orchestrator" ./services/webhook-orchestrator

echo "  → Building event-publisher..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/event-publisher" ./services/event-publisher

echo "  → Building CLI tool..."
CGO_ENABLED=0 go build -o "${BIN_DIR}/vfs-cli" ./cli

echo ""
echo -e "${GREEN}Copying configuration files...${NC}"

# Copy example config files
if [ -f "config.example.yaml" ]; then
  echo "  → Copying config.example.yaml"
  cp config.example.yaml "${CONF_DIR}/"
fi

if [ -f "config.local.yaml" ]; then
  echo "  → Copying config.local.yaml"
  cp config.local.yaml "${CONF_DIR}/"
fi

# Copy webhook configs if they exist
if [ -d "deployments/webhook-configs" ]; then
  echo "  → Copying webhook configurations"
  cp -r deployments/webhook-configs "${CONF_DIR}/"
fi

# Create a sample production config
echo "  → Creating config.production.yaml template"
cat > "${CONF_DIR}/config.production.yaml" << 'EOF'
# MySQL VFS Production Configuration
# Copy this file and customize for your environment

database:
  dsn: "${DB_DSN}"  # Set via environment variable
  table_prefix: "vfs_"

storage:
  s3:
    endpoint: "${S3_ENDPOINT}"
    bucket: "${S3_BUCKET}"
    region: "${AWS_REGION:-us-east-1}"

messaging:
  nats:
    url: "${NATS_URL}"

server:
  port: 8080

logging:
  level: "info"

auth:
  provider: "jwt"
  allow_anonymous: false
  jwt:
    secret: "${JWT_SECRET}"
    issuer: "mysql-vfs-production"
    ttl: "24h"
  system_admin:
    token: "${SYSTEM_ADMIN_TOKEN}"
    id: "system-admin"

cache:
  user_ttl: "5m"
  schema_ttl: "5m"
  policy_ttl: "5m"
  quota_ttl: "5m"

idempotency:
  ttl: "24h"

services:
  event_worker:
    worker_count: 10
    poll_interval: "1s"
    batch_size: 10

  scheduler:
    scheduler_id: "${HOSTNAME:-scheduler-1}"
    poll_interval: "10s"
    lease_duration: "5m"
    heartbeat_interval: "30s"

  webhook_orchestrator:
    daemon_url: "http://localhost:9000"
    worker_count: 5
    poll_interval: "1s"
    batch_size: 10

  event_publisher:
    port: 8083
    event_buffer_size: 1000
    max_connections: 100
    auth_enabled: true
EOF

# Copy bootstrap script
echo "  → Copying bootstrap.sh"
if [ -f "${SCRIPT_DIR}/bootstrap.sh" ]; then
  cp "${SCRIPT_DIR}/bootstrap.sh" "${OUTPUT_DIR}/bootstrap.sh"
  chmod +x "${OUTPUT_DIR}/bootstrap.sh"
else
  echo -e "${RED}Warning: bootstrap.sh not found in ${SCRIPT_DIR}${NC}"
fi

# Create a README in output directory
echo "  → Creating README.md"
cat > "${OUTPUT_DIR}/README.md" << 'EOF'
# MySQL VFS Binaries

This directory contains the built binaries and configuration files for MySQL VFS.

## Directory Structure

```
output/
├── bin/                    # Compiled binaries
│   ├── vfs-service        # Main VFS API service
│   ├── event-worker       # Background event processor
│   ├── scheduler          # Cron job scheduler
│   ├── webhook-orchestrator # Webhook manager
│   ├── event-publisher    # SSE event publisher
│   └── vfs-cli           # CLI tool
├── conf/                  # Configuration files
│   ├── config.example.yaml
│   ├── config.local.yaml
│   └── config.production.yaml
├── bootstrap.sh           # Service startup script
└── README.md             # This file
```

## Quick Start

### 1. Configure

Copy and edit the configuration file:

```bash
cd output
cp conf/config.example.yaml conf/config.yaml
vim conf/config.yaml
```

Or use environment variables to override config values.

### 2. Start All Services

```bash
./bootstrap.sh
```

This will start all microservices. Press Ctrl+C to stop them all.

### 3. Start Specific Services

Use environment variables to enable/disable services:

```bash
# Only start VFS service and event worker (no NATS)
ENABLE_NATS=false \
ENABLE_SCHEDULER=false \
ENABLE_WEBHOOK_ORCHESTRATOR=false \
ENABLE_EVENT_PUBLISHER=false \
./bootstrap.sh

# Start all services except event publisher
ENABLE_EVENT_PUBLISHER=false \
./bootstrap.sh
```

### 4. Start Services Individually

```bash
# Start VFS service only
./bin/vfs-service --conf conf/config.yaml

# Start event worker only
./bin/event-worker --conf conf/config.yaml

# Start scheduler only
./bin/scheduler --conf conf/config.yaml
```

## Service Control Environment Variables

- `ENABLE_VFS_SERVICE` - Enable/disable VFS API service (default: true)
- `ENABLE_EVENT_WORKER` - Enable/disable event worker (default: true)
- `ENABLE_SCHEDULER` - Enable/disable cron scheduler (default: true)
- `ENABLE_WEBHOOK_ORCHESTRATOR` - Enable/disable webhook orchestrator (default: true)
- `ENABLE_NATS` - Enable/disable NATS integration (default: true)
- `ENABLE_EVENT_PUBLISHER` - Enable/disable SSE event publisher (default: true)

**Note:** Event Publisher requires NATS. If `ENABLE_NATS=false`, Event Publisher will be automatically disabled.

## Configuration

### Using Config File

Set `VFS_CONFIG_FILE` to specify a custom config file:

```bash
export VFS_CONFIG_FILE=/path/to/custom-config.yaml
./bootstrap.sh
```

### Using Environment Variables

All config values can be overridden via environment variables:

```bash
export DB_DSN="root:root@tcp(localhost:3306)/vfs"
export S3_BUCKET="my-bucket"
export JWT_SECRET="my-secret"
./bootstrap.sh
```

## CLI Tool

The CLI provides both command mode and interactive shell mode:

### Command Mode

```bash
./bin/vfs-cli --help
./bin/vfs-cli upload /local/file.txt /remote/path/
./bin/vfs-cli download /remote/file.txt /local/path/
./bin/vfs-cli ls /path/
```

### Interactive Shell Mode

```bash
./bin/vfs-cli shell
# Provides an interactive REPL for VFS operations
```

## Service Dependencies

The services have these dependencies:

1. **vfs-service** - Requires MySQL and S3
2. **event-worker** - Requires MySQL and vfs-service
3. **scheduler** - Requires MySQL and vfs-service
4. **webhook-orchestrator** - Requires MySQL and vfs-service
5. **event-publisher** - Requires NATS

Start them in order or use `bootstrap.sh` which handles the startup sequence.

## Health Checks

Check if services are running:

```bash
# VFS Service
curl http://localhost:8080/health

# Event Publisher
curl http://localhost:8083/health
```

## Logs

Services log to stdout/stderr. Redirect to files if needed:

```bash
./bin/vfs-service --conf conf/config.yaml > vfs-service.log 2>&1 &
```

## Systemd Integration

To run as systemd services, see the `deployments/systemd/` directory in the source repository.

## Docker

To run with Docker, see `docker-compose.yml` in the source repository.
EOF

# Create version info file
echo "  → Creating version.txt"
cat > "${OUTPUT_DIR}/version.txt" << EOF
Build Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
Git Commit: $(git rev-parse HEAD 2>/dev/null || echo "unknown")
Git Branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
Built From: ${SCRIPT_DIR}
EOF

echo ""
echo -e "${GREEN}=== Build Complete ===${NC}"
echo ""
echo "Output directory: ${OUTPUT_DIR}"
echo ""
echo "Binaries:"
ls -lh "${BIN_DIR}"
echo ""
echo "Configuration files:"
ls -lh "${CONF_DIR}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "  1. cd ${OUTPUT_DIR}"
echo "  2. cp conf/config.example.yaml conf/config.yaml"
echo "  3. Edit conf/config.yaml for your environment"
echo "  4. ./bootstrap.sh"
echo ""
echo -e "${GREEN}Or start individual services:${NC}"
echo "  ${OUTPUT_DIR}/bin/vfs-service --conf ${OUTPUT_DIR}/conf/config.yaml"
echo ""

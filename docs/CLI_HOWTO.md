# VFS CLI - Complete Guide

## Overview

The VFS CLI provides two interfaces for interacting with the MySQL-based Virtual File System:

1. **vfs-cli-prompt** (Recommended) - Rich REPL with auto-completion, dual-mode shell support
2. **vfs-cli** (Legacy) - Basic REPL interface

## Quick Start

### Using Docker (Recommended)

```bash
# 1. Start VFS services
docker compose up -d

# 2. Initialize S3 storage
make s3-init

# 3. Run the CLI
docker compose run --rm cli
```

### Local Development

```bash
# 1. Build the CLI
cd cli
go build -o vfs-cli-prompt main_prompt.go

# 2. Set VFS service URL
export VFS_SERVICE_URL=http://localhost:18080

# 3. Run
./vfs-cli-prompt
```

## Installation

### Docker Build

```bash
# Build the CLI image
docker compose build cli

# Run with docker compose
docker compose run --rm cli

# Run with docker directly
docker run --rm -it \
  --network mysql-vfs-claude_cc-vfs-network \
  -e VFS_SERVICE_URL=http://vfs-service:8080 \
  vfs-cli
```

### Local Build

```bash
# Navigate to CLI directory
cd cli

# Install dependencies
go mod download

# Build new prompt-based CLI
go build -o vfs-cli-prompt main_prompt.go

# Or build legacy CLI
go build -o vfs-cli main.go

# Run
./vfs-cli-prompt
```

### Makefile Targets

```bash
make cli-prompt      # Build and run prompt CLI locally
make cli-build       # Build prompt CLI only
make up-cli          # Start services + CLI with docker-compose
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VFS_SERVICE_URL` | `http://localhost:8080` | VFS service endpoint |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |

### Docker Compose

The CLI service configuration in `docker-compose.yml`:

```yaml
cli:
  build:
    context: .
    dockerfile: cli/Dockerfile
  depends_on:
    vfs-service:
      condition: service_healthy
  environment:
    VFS_SERVICE_URL: http://vfs-service:8080
    LOG_LEVEL: info
  stdin_open: true
  tty: true
  networks:
    - cc-vfs-network
  profiles:
    - cli
```

**Key settings:**
- `stdin_open: true` and `tty: true` - Required for interactive CLI
- `profiles: [cli]` - CLI only starts when explicitly requested
- Network must match VFS service network

### S3/LocalStack Configuration

For file uploads to work, ensure S3 is properly configured:

```bash
# Initialize S3 bucket
make s3-init

# Verify bucket exists
docker compose exec localstack awslocal s3 ls
```

**Required environment variables for vfs-service:**
```yaml
S3_ENDPOINT: http://localstack:4566
S3_BUCKET: cc-vfs-storage
AWS_ACCESS_KEY_ID: test
AWS_SECRET_ACCESS_KEY: test
AWS_REGION: us-east-1
AWS_S3_FORCE_PATH_STYLE: "true"  # Important for LocalStack
```

## Using vfs-cli-prompt (New Interface)

### Features

✅ Rich auto-completion with Tab key
✅ Dual-mode: VFS commands + Local shell commands
✅ Path-aware completions for both VFS and local filesystem
✅ Command history with arrow keys (↑/↓)
✅ Live prompt showing current mode and directory
✅ Supports zsh for local shell commands

### Interface Overview

```
=== VFS CLI (go-prompt) ===
VFS Service: http://vfs-service:8080
Local Dir: /app

Commands:
  VFS: ls, cd, pwd, mkdir, rmdir, cat, import, rm, mv, jq
  Local: $<cmd> (e.g., $ls, $cat, $pwd)
  Mode: $ (local mode), / (VFS mode)
  Quit: exit, quit, Ctrl+D

✓ Connected to VFS service

vfs:/>
```

### Mode Switching

**VFS Mode (default):**
```bash
vfs:/> ls              # List VFS directory
vfs:/> cd data         # Change VFS directory
vfs:/> pwd             # Show VFS working directory
```

**Switch to Local Mode:**
```bash
vfs:/> $               # Switch to local shell mode
local:/app>            # Now in local mode
```

**Switch back to VFS Mode:**
```bash
local:/app> /          # Switch to VFS mode
vfs:/>                 # Back to VFS mode
```

**One-off Local Commands (from VFS mode):**
```bash
vfs:/> $ls -la         # Run local ls command
vfs:/> $pwd            # Show local directory
vfs:/> $cat file.txt   # Read local file
```

### VFS Commands

#### Directory Operations

```bash
# List directory
vfs:/> ls
vfs:/> ls /data
vfs:/> ls -r /data     # Recursive listing

# Change directory
vfs:/> cd /data
vfs:/data> cd subdir
vfs:/data/subdir> cd ..
vfs:/data> cd /        # Go to root

# Print working directory
vfs:/> pwd
```

#### Directory Management

```bash
# Create directory
vfs:/> mkdir projects
vfs:/> cd projects

# Remove directory
vfs:/> rmdir projects

# Remove directory recursively
vfs:/> rmdir -r projects
```

#### File Operations

```bash
# Import local file to VFS
vfs:/> import /local/path/file.txt /vfs/destination.txt
vfs:/> import ~/document.pdf /docs/document.pdf

# Display file contents
vfs:/> cat /data/file.txt

# Move/rename file
vfs:/> mv /data/old.txt /data/new.txt
vfs:/> mv /data/file.txt /backup/file.txt

# Delete file
vfs:/> rm /data/file.txt
```

#### JSON Operations

```bash
# Query JSON file with jq
vfs:/> jq /data/config.json '.'
vfs:/> jq /users.json '.users[] | select(.active)'
vfs:/> jq /config.json '.database.host'
```

### Local Shell Commands

**In Local Mode:**
```bash
local:/app> ls -la              # Local directory listing
local:/app> cd /tmp             # Change local directory
local:/app> pwd                 # Local working directory
local:/app> cat file.txt        # Read local file
local:/app> find . -name "*.go" # Find files
local:/app> grep -r "TODO" .    # Search in files
local:/app> tree                # Directory tree
```

**Common local commands available:**
- `ls`, `cd`, `pwd` - Navigation
- `cat`, `head`, `tail` - File viewing
- `find`, `grep` - Searching
- `tree` - Directory visualization
- `wc`, `file`, `du`, `df` - File utilities
- Any zsh command

### Tab Completion

**Command Completion:**
```bash
vfs:/> l<TAB>          # Shows: ls
vfs:/> mk<TAB>         # Shows: mkdir
```

**VFS Path Completion:**
```bash
vfs:/> ls /da<TAB>     # Completes to: /data/
vfs:/> cat /data/co<TAB>  # Shows files starting with 'co'
```

**Local Path Completion:**
```bash
local:/app> ls cl<TAB>     # Completes to: cli/
local:/app> cat main<TAB>  # Completes to: main.go
$cat /tmp/te<TAB>          # Completes local paths
```

**Features:**
- Completes commands, VFS paths, and local paths
- Shows file types (file/directory) and sizes
- Filters hidden files (unless you type `.`)
- Works with absolute and relative paths

### Workflow Examples

#### Upload and Process Files

```bash
# Switch to local mode to find file
vfs:/> $
local:/app> ls Documents/
local:/app> pwd
/home/user

# Switch back to VFS mode
local:/app> /

# Import the file
vfs:/> mkdir uploads
vfs:/> cd uploads
vfs:/uploads> import /home/user/Documents/data.json data.json

# Process the JSON
vfs:/uploads> jq data.json '.results[]'
```

#### Working with Multiple Files

```bash
# Use local commands to list files
vfs:/> $ls ~/Downloads/*.pdf
vfs:/> $find ~/Documents -name "*.txt"

# Import multiple files
vfs:/> mkdir documents
vfs:/> import ~/Documents/report.txt /documents/report.txt
vfs:/> import ~/Documents/notes.txt /documents/notes.txt

# Verify
vfs:/> ls documents/
```

#### Combining VFS and Local Operations

```bash
# Search local files
vfs:/> $grep -l "TODO" *.go

# Read local file
vfs:/> $cat main.go

# Import to VFS
vfs:/> import main.go /backup/main.go

# Verify in VFS
vfs:/> cat /backup/main.go
```

## Using vfs-cli (Legacy Interface)

### Basic REPL

```bash
=== VFS CLI ===
Connecting to: http://vfs-service:8080
Connected successfully!
Type 'help' for available commands or 'exit' to quit

/>
```

### Available Commands

```bash
# Same commands as prompt version, but no tab completion
/> ls
/> cd data
/> mkdir test
/> import local.txt /remote.txt
/> cat /remote.txt
/> rm /remote.txt
/> help
/> exit
```

### Piping Support

```bash
# Chain commands together
/> cat /data/logs.json | jq '.entries[]' | grep error

# Query and filter JSON
/> jq /users.json '.users[] | select(.role == "admin")'
```

## Troubleshooting

### Connection Issues

**Error:** `Warning: Cannot connect to VFS service`

**Solutions:**
```bash
# Check VFS service is running
docker compose ps vfs-service

# Check service health
curl http://localhost:18080/health

# Check logs
docker compose logs vfs-service

# Restart service
docker compose restart vfs-service
```

### S3 Upload Errors

**Error:** `failed to upload to S3: dial tcp: lookup cc-vfs-storage.localstack`

**Solutions:**
```bash
# 1. Ensure S3 bucket exists
make s3-init

# 2. Verify environment variable is set
docker compose exec vfs-service env | grep S3

# 3. Ensure AWS_S3_FORCE_PATH_STYLE is set to "true"
# Edit docker-compose.yml and add:
# AWS_S3_FORCE_PATH_STYLE: "true"

# 4. Restart VFS service
docker compose restart vfs-service
```

### Path Errors

**Error:** `invalid path: ../etc/passwd`

**Solution:** Path traversal with `..` is not allowed for security. Use absolute paths:
```bash
vfs:/data/subdir> cd /data    # Correct
vfs:/data/subdir> cd ..       # Not allowed
```

### File Size Errors

**Error:** `file size exceeds maximum allowed (104857600 bytes)`

**Solution:** Maximum file size is 100MB. Split larger files:
```bash
# Split large file locally
$split -b 50M largefile.dat part_

# Import parts separately
vfs:/> import part_aa /data/part_aa
vfs:/> import part_ab /data/part_ab
```

### Tab Completion Not Working

**Issue:** Tab key doesn't complete paths

**Solutions:**
1. Ensure you're using `vfs-cli-prompt`, not `vfs-cli`
2. Check you're not in a nested terminal (Docker in Docker)
3. Verify terminal supports ANSI codes: `echo $TERM`

### Local Commands Not Working

**Error:** `zsh: command not found`

**Solution:** Ensure zsh is installed:
```bash
# In Docker, it should be pre-installed
docker compose run --rm cli which zsh

# Local installation
# macOS: brew install zsh
# Ubuntu: apt-get install zsh
# Alpine: apk add zsh
```

## Tips and Best Practices

### 1. Use Tab Completion Extensively

```bash
# Instead of typing full paths
vfs:/> cat /very/long/path/to/file.txt

# Use tab completion
vfs:/> cat /v<TAB>/l<TAB>/p<TAB>/f<TAB>
```

### 2. Leverage Dual Mode

```bash
# Find files locally first
vfs:/> $
local:/> find . -name "*.json"
local:/> /

# Then import them to VFS
vfs:/> import ./found-file.json /data/imported.json
```

### 3. Verify Before Importing

```bash
# Check file size locally
vfs:/> $ls -lh ~/large-file.dat
vfs:/> $du -h ~/large-file.dat

# Only import if < 100MB
```

### 4. Use Relative Paths in VFS

```bash
# Navigate to target directory first
vfs:/> cd /data/uploads

# Use relative paths
vfs:/data/uploads> import ~/file.txt file.txt
vfs:/data/uploads> ls
```

### 5. Preview JSON Before Querying

```bash
# See structure first
vfs:/> cat /data/config.json | head

# Then query specific fields
vfs:/> jq /data/config.json '.database'
```

## Advanced Usage

### Batch Operations

```bash
# Create directory structure
vfs:/> mkdir projects
vfs:/> cd projects
vfs:/projects> mkdir src
vfs:/projects> mkdir docs
vfs:/projects> mkdir tests

# Import multiple files
vfs:/projects> import ~/src/main.go src/main.go
vfs:/projects> import ~/README.md docs/README.md
```

### Working with Environment Variables

```bash
# Set custom VFS URL
export VFS_SERVICE_URL=http://production-vfs:8080

# Run CLI
./vfs-cli-prompt

# Or with docker
docker compose run --rm \
  -e VFS_SERVICE_URL=http://production-vfs:8080 \
  cli
```

### Using with Scripts

```bash
# Non-interactive commands (legacy CLI only)
echo "ls /" | docker compose run --rm cli ./vfs-cli

# Or with heredoc
docker compose run --rm cli ./vfs-cli <<EOF
cd /data
ls
cat file.txt
EOF
```

## Keyboard Shortcuts (vfs-cli-prompt)

| Key | Action |
|-----|--------|
| `Tab` | Auto-complete command or path |
| `↑` / `↓` | Navigate command history |
| `Ctrl+A` | Move cursor to beginning of line |
| `Ctrl+E` | Move cursor to end of line |
| `Ctrl+W` | Delete word before cursor |
| `Ctrl+U` | Clear line before cursor |
| `Ctrl+K` | Clear line after cursor |
| `Ctrl+D` | Exit CLI |
| `Ctrl+C` | Cancel current input |

## Docker Compose Commands

```bash
# Start all services
docker compose up -d

# Start with CLI
docker compose --profile cli up -d

# Run CLI (ephemeral)
docker compose run --rm cli

# Run old CLI
docker compose run --rm cli ./vfs-cli

# Run shell in container
docker compose run --rm cli /bin/bash

# Build CLI image
docker compose build cli

# View CLI logs (if running as service)
docker compose logs -f cli

# Stop all services
docker compose down

# Clean up everything
docker compose down -v
```

## Summary

| Feature | vfs-cli-prompt | vfs-cli (legacy) |
|---------|----------------|------------------|
| Tab Completion | ✅ | ❌ |
| Path-aware Completion | ✅ | ❌ |
| Local Shell Mode | ✅ | ❌ |
| Command History | ✅ | ✅ |
| Piping Support | ❌ | ✅ |
| Interactive Mode | ✅ | ✅ |
| Script-friendly | ❌ | ✅ |

**Recommendation:** Use `vfs-cli-prompt` for interactive work and `vfs-cli` for scripting.

## Next Steps

- See [cli/README.md](cli/README.md) for implementation details
- Check [docs/](docs/) for VFS API documentation
- Review [docker-compose.yml](docker-compose.yml) for service configuration
- Run `make help` to see all available make targets

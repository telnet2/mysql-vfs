# VFS CLI - Complete Guide

## Overview

The VFS CLI provides a rich interactive interface for interacting with the MySQL-based Virtual File System with auto-completion, dual-mode shell support, and comprehensive file operations.

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
go build -o vfs-cli main.go

# 2. Set VFS service URL
export VFS_SERVICE_URL=http://localhost:18080

# 3. Run
./vfs-cli
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

# Build the CLI
go build -o vfs-cli main.go

# Run
./vfs-cli
```

### Makefile Targets

```bash
make cli-build       # Build CLI only
make cli             # Build and run CLI locally
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

## Using the VFS CLI

### Features

✅ Rich auto-completion with Tab key
✅ Dual-mode: VFS commands + Local shell commands
✅ Path-aware completions for both VFS and local filesystem
✅ Command history with arrow keys (↑/↓)
✅ Live prompt showing current mode and directory
✅ Supports shell commands for local operations

### Interface Overview

```
=== VFS CLI ===
Connecting to: http://vfs-service:8080
Connected successfully!
Type 'help' for available commands or 'exit' to quit
Type '$' to toggle between VFS mode and shell mode, or '$<command>' or '$ <command>' to run a single shell command
Press TAB to autocomplete commands
Press CTRL+C twice to exit

/>
```

### Mode Switching

**VFS Mode (default):**
```bash
/> ls              # List VFS directory
/> cd data         # Change VFS directory
/> pwd             # Show VFS working directory
```

**Switch to Local Mode:**
```bash
/> $               # Switch to local shell mode
shell:/app$        # Now in local mode
```

**Switch back to VFS Mode:**
```bash
shell:/app$ $      # Switch back to VFS mode
/>                 # Back to VFS mode
```

**One-off Local Commands (from VFS mode):**
```bash
/> $ls -la         # Run local ls command
/> $pwd            # Show local directory
/> $cat file.txt   # Read local file
```

### VFS Commands

#### Directory Operations

```bash
# List directory
/> ls
/> ls /data
/> ls -r /data     # Recursive listing
/> ls -l /data     # Long listing with details

# Change directory
/> cd /data
/data> cd subdir
/data/subdir> cd ..
/data> cd /        # Go to root

# Print working directory
/> pwd
```

#### Directory Management

```bash
# Create directory
/> mkdir projects
/> cd projects

# Remove directory
/> rmdir projects

# Remove directory recursively
/> rmdir -r projects
```

#### File Operations

```bash
# Import local file to VFS
/> import /local/path/file.txt /vfs/destination.txt
/> import ~/document.pdf /docs/document.pdf

# Display file contents
/> cat /data/file.txt

# Copy files
/> cp /source/file.txt /dest/file.txt
/> cp *.json /backup/     # Copy with glob patterns

# Move/rename file
/> mv /data/old.txt /data/new.txt
/> mv /data/file.txt /backup/file.txt

# Delete file
/> rm /data/file.txt
```

#### Search Operations

```bash
# Find files and directories
/> find /data -name "*.json"    # Find JSON files
/> find / -type d               # Find directories
/> find /data -size +1000       # Find files > 1000 bytes

# Search file contents
/> grep "error" /logs/app.log   # Search for "error" in file
/> grep "user_id" /data/        # Search recursively in directory

# Advanced JSON search with JSONPath (fast SQL queries)
/> search --json-path '$.name' --value 'John'                    # Find JSON files with name=John
/> search --json-path '$.users[0].email' --value 'user@test.com' # Nested object search
/> search --meta-json-path '$.owner' --value 'admin'             # Search by metadata

# Powerful JQ expression search (advanced processing)
/> search --jq-expression '.users[] | select(.age > 21).name' --value 'Alice'  # Complex filtering
/> search --jq-expression '.items | map(.price) | add' --value '100'           # Mathematical operations
/> search --meta-jq-expression '.tags | contains(["important"])' --value 'true' # Array operations
```

#### JSON Operations

```bash
# Query JSON file with jq
/> jq /data/config.json '.'
/> jq /users.json '.users[] | select(.active)'
/> jq /config.json '.database.host'

# Search across multiple JSON files
/> search --json-path '$.status' --value 'active'              # Find active records
/> search --jq-expression '.price | select(. > 100)' --value '150'  # Find expensive items
/> search --meta-json-path '$.schema' --value 'user-v1'        # Find by schema version
```

#### Version Control

```bash
# Show file version history
/> version /data/file.txt
```

### Local Shell Commands

**In Local Mode:**
```bash
shell:/app$ ls -la              # Local directory listing
shell:/app$ cd /tmp             # Change local directory
shell:/app$ pwd                 # Local working directory
shell:/app$ cat file.txt        # Read local file
shell:/app$ find . -name "*.go" # Find files
shell:/app$ grep -r "TODO" .    # Search in files
shell:/app$ tree                # Directory tree
```

**Common local commands available:**
- `ls`, `cd`, `pwd` - Navigation
- `cat`, `head`, `tail` - File viewing
- `find`, `grep` - Searching
- `tree` - Directory visualization
- `wc`, `file`, `du`, `df` - File utilities
- Any shell command

### Tab Completion

**Command Completion:**
```bash
/> l<TAB>          # Shows: ls
/> mk<TAB>         # Shows: mkdir
/> gr<TAB>         # Shows: grep
```

**VFS Path Completion:**
```bash
/> ls /da<TAB>     # Completes to: /data/
/> cat /data/co<TAB>  # Shows files starting with 'co'
/> find /da<TAB>   # Completes paths for find command
```

**Local Path Completion:**
```bash
shell:/app$ ls cl<TAB>     # Completes to: cli/
shell:/app$ cat main<TAB>  # Completes to: main.go
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
/> $
shell:/app$ ls Documents/
shell:/app$ pwd
/home/user

# Switch back to VFS mode
shell:/app$ $

# Import the file
/> mkdir uploads
/> cd uploads
/uploads> import /home/user/Documents/data.json data.json

# Process the JSON
/uploads> jq data.json '.results[]'
```

#### Working with Multiple Files

```bash
# Use local commands to list files
/> $ls ~/Downloads/*.pdf
/> $find ~/Documents -name "*.txt"

# Import multiple files
/> mkdir documents
/> import ~/Documents/report.txt /documents/report.txt
/> import ~/Documents/notes.txt /documents/notes.txt

# Verify
/> ls documents/
```

#### Combining VFS and Local Operations

```bash
# Search local files
/> $grep -l "TODO" *.go

# Read local file
/> $cat main.go

# Import to VFS
/> import main.go /backup/main.go

# Verify in VFS
/> cat /backup/main.go
```

#### Search and Copy Operations

```bash
# Find all JSON files in VFS
/> find /data -name "*.json"

# Search for specific content
/> grep "error" /logs/

# Advanced JSON search across files
/> search --json-path '$.status' --value 'completed'    # Find completed tasks
/> search --meta-json-path '$.owner' --value 'john'     # Find John's files

# Copy found files to backup
/> cp /data/*.json /backup/
```

#### JSON Data Analysis Workflow

```bash
# Import JSON datasets
/> mkdir datasets
/> import ~/data/users.json /datasets/users.json
/> import ~/data/orders.json /datasets/orders.json

# Analyze data with search
/> search --json-path '$.role' --value 'admin'           # Find admin users
/> search --jq-expression '.orders | length' --value '5' # Find users with 5+ orders
/> search --meta-json-path '$.version' --value 'v2.0'    # Find v2.0 datasets

# Query specific fields
/> jq /datasets/users.json '.[] | select(.active == true).name'
/> search --jq-expression '.[] | select(.active == true).name' --value 'John'
```

## Command Reference

### All Available Commands

```bash
# Directory operations
ls [-r] [-l] [path]                # List directory contents
cd [path]                         # Change directory
pwd                               # Print working directory
mkdir <name>                      # Create directory
rmdir [-r] <path>                 # Remove directory

# File operations
cat <path>                        # Display file contents
import <local> [vfs]               # Import local file to VFS
cp <src> <dst>                    # Copy files (supports globs)
mv <src> <dst>                    # Move/rename files (supports globs)
rm <path>                         # Remove files (supports globs)

# Search operations
grep <pattern> <path>             # Search for pattern in files
find <path> [options]             # Find files and directories
search [options]                  # Advanced JSON search with JSONPath/JQ

# JSON operations
jq <path> [expression]            # Query JSON files

# Version control
version <path>                    # Show file version history

# Special files
edit <path>                       # Edit file with $EDITOR
create-sample-files <dir>         # Create sample .files configs
create-trigger <dir> <url>        # Create webhook triggers

# Authentication
login <user> <pass>               # Authenticate with VFS
logout                            # Clear authentication

# Help
help                              # Show available commands
```

### Find Command Options

```bash
find <path> -name <pattern>        # Files matching name pattern
find <path> -type f|d              # Files (f) or directories (d)
find <path> -size <n>              # Files of exact size n bytes
find <path> -size +<n>             # Files larger than n bytes
find <path> -size -<n>             # Files smaller than n bytes
```

### Search Command Options

The `search` command provides powerful JSON querying with two approaches:

#### JSONPath Queries (Fast SQL-based)
```bash
# Content search
search --json-path '$.name' --value 'John'                    # Simple field
search --json-path '$.user.email' --value 'user@test.com'     # Nested objects
search --json-path '$.items[0].price' --value '29.99'         # Array elements

# Metadata search
search --meta-json-path '$.owner' --value 'admin'             # Simple metadata
search --meta-json-path '$.permissions.write' --value 'true'  # Nested metadata
```

#### JQ Expression Queries (Powerful processing)
```bash
# Content filtering and transformation
search --jq-expression '.users[] | select(.age > 21).name' --value 'Alice'    # Filter by age
search --jq-expression '.items | map(.price) | add' --value '100'             # Sum prices
search --jq-expression '.data | keys | length' --value '5'                    # Count keys

# Metadata operations
search --meta-jq-expression '.tags | contains(["important"])' --value 'true'  # Tag filtering
search --meta-jq-expression '.permissions | has("admin")' --value 'true'      # Permission check
```

#### Search Options
```bash
--json-path <path>           # JSONPath for file content (fast)
--jq-expression <expr>       # JQ expression for file content (powerful)
--meta-json-path <path>      # JSONPath for metadata (fast)
--meta-jq-expression <expr>  # JQ expression for metadata (powerful)
--value <value>              # Expected result value
--type f|d                   # Search files (f) or directories (d)
--limit <n>                  # Maximum results (default: 100)
```

#### Performance Notes
- **JSONPath**: ⚡ Fast SQL queries, best for simple field access
- **JQ Expressions**: 🐌 Slower but powerful, processes files individually
- Use JSONPath for frequent searches, JQ for complex one-off queries

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
1. Check you're not in a nested terminal (Docker in Docker)
2. Verify terminal supports ANSI codes: `echo $TERM`
3. Ensure you're running the CLI interactively

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
/> $
shell:/> find . -name "*.json"
shell:/> $
# Then import them to VFS
/> import ./found-file.json /data/imported.json
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
# Non-interactive commands
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

The VFS CLI provides a comprehensive interface for managing files in the Virtual File System with:

- **Full command set**: Directory operations, file management, search, JSON processing
- **Rich completion**: Tab completion for commands and paths
- **Dual mode**: VFS operations and local shell commands
- **Piping support**: Chain commands together for complex operations
- **Interactive and scriptable**: Works for both manual use and automation

**Key Commands:**
- File operations: `ls`, `cat`, `cp`, `mv`, `rm`, `import`
- Search: `grep`, `find`, `search` (advanced JSON search)
- JSON processing: `jq`, `search` (JSONPath/JQ queries)
- Directory management: `cd`, `pwd`, `mkdir`, `rmdir`

## Next Steps

- See [cli/README.md](cli/README.md) for implementation details
- Check [docs/](docs/) for VFS API documentation
- Review [docker-compose.yml](docker-compose.yml) for service configuration
- Run `make help` to see all available make targets
- Check [SECURITY.md](SECURITY.md) for authentication and authorization details

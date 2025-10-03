# VFS CLI

Interactive command-line interface for the MySQL-based Virtual File System.

## Features

- **REPL Interface**: Interactive shell with command history
- **Session Management**: Maintains current directory and authentication state
- **Full File Operations**: Create, read, update, delete files and directories
- **Streaming**: Efficient streaming for large files (up to 100MB)
- **Piping Support**: Chain commands together with Unix-style pipes
- **JSON Querying**: Query JSON files using jq expressions
- **Idempotency**: All mutations include request IDs for safe retries

## Installation

### Using Docker

```bash
# Start VFS service and CLI
make up-cli

# Or manually
docker-compose --profile cli up -d
docker-compose exec cli /bin/bash
```

### Building Locally

```bash
cd cli
go build -o vfs-cli
./vfs-cli
```

## Configuration

Set the VFS service URL via environment variable:

```bash
export VFS_SERVICE_URL=http://localhost:8080
./vfs-cli
```

## Available Commands

### Directory Navigation

```bash
# List directory contents
ls [path]

# List recursively
ls -r [path]

# Change directory
cd <path>

# Print working directory
pwd
```

### Directory Management

```bash
# Create directory
mkdir <name>

# Remove directory
rmdir <path>

# Remove directory recursively
rmdir -r <path>
```

### File Operations

```bash
# Import local file to VFS
import <local_path> <vfs_path>

# Display file contents
cat <path>

# Move/rename file
mv <source> <destination>

# Delete file
rm <path>
```

### JSON Querying

```bash
# Query JSON file with jq expression
jq <path> <expression>

# Examples
jq /data/users.json '.users[] | select(.active)'
jq /config.json '.database.host'
```

### Piping

Chain commands together for powerful workflows:

```bash
# Stream file through jq and grep
cat /data/logs.json | jq '.entries[]' | grep error

# Query and filter JSON
jq /users.json '.users[] | select(.role == "admin")' | grep john
```

### Utility

```bash
# Show help
help

# Exit CLI
exit
```

## Usage Examples

### Basic File Management

```bash
/> mkdir projects
Created directory: /projects

/> cd projects
/projects> import ~/document.txt /projects/doc.txt
Imported: /projects/doc.txt (ID: abc123, Version: 1)

/projects> ls
doc.txt  (1024 bytes)

/projects> cat doc.txt
This is the content of the document...

/projects> mv doc.txt readme.txt
Moved: /projects/doc.txt -> /projects/readme.txt (ID: abc123)

/projects> rm readme.txt
Deleted file: /projects/readme.txt
```

### Working with JSON

```bash
/> import ~/config.json /config.json
Imported: /config.json (ID: def456, Version: 1)

/> jq /config.json '.database'
{
  "host": "localhost",
  "port": 3306,
  "name": "vfs"
}

/> jq /config.json '.database.host'
"localhost"
```

### Advanced Piping

```bash
# Find all active users in a large JSON file
/> cat /data/users.json | jq '.users[] | select(.active)' | grep admin

# Process logs
/> cat /logs/app.log | grep ERROR | tail -10
```

### Recursive Operations

```bash
# List all files recursively
/> ls -r /projects
/projects/
  docs/
    readme.md  (2048 bytes)
    guide.md  (4096 bytes)
  src/
    main.go  (1024 bytes)

# Remove directory tree
/> rmdir -r /projects
Deleted directory: /projects
```

## Implementation Details

### Client Architecture

- **HTTP Client**: Standard net/http client with 30s timeout
- **Request IDs**: Auto-generated UUIDv4 for all mutations
- **Streaming**: io.Pipe for efficient data transfer
- **Path Resolution**: Relative paths resolved against current directory

### Command Execution

Each command implements the `Command` interface:

```go
type Command interface {
    Execute(ctx *Context, args []string) error
    Help() string
}
```

Context provides:
- VFS HTTP client
- Session state (current directory, auth token)
- I/O streams (stdin, stdout, stderr)

### Piping Implementation

Piping uses Go's `io.Pipe()` to create concurrent command chains:

1. Parse pipeline into command segments
2. Create pipes between consecutive commands
3. Execute all commands concurrently in goroutines
4. Wait for completion and collect errors

### File Size Limits

- Maximum file size: 100MB (enforced client-side)
- Progress indication for files >10MB
- Validation before upload to prevent waste

### Error Handling

- Network errors: Connection refused, timeouts
- API errors: 4xx/5xx status codes with messages
- Validation errors: Path traversal, invalid arguments
- Idempotency errors: Duplicate request IDs

## Development

### Adding New Commands

1. Implement `Command` interface in `commands/commands.go`
2. Register command in `main.go` command map
3. Add help text and examples
4. Test with real VFS service

### Testing

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Test against live service
VFS_SERVICE_URL=http://localhost:8080 go test ./...
```

## Troubleshooting

### Connection Errors

```
Warning: Cannot connect to VFS service: dial tcp: connection refused
```

**Solution**: Ensure VFS service is running:
```bash
docker-compose up vfs-service
```

### Path Errors

```
Error: invalid path: ../etc/passwd
```

**Solution**: Path traversal with `..` is not allowed for security.

### File Size Errors

```
Error: file size (150000000 bytes) exceeds maximum allowed (104857600 bytes)
```

**Solution**: File must be <100MB. Split into smaller files.

### jq Command Not Found

```
Error: jq command failed: exec: "jq": executable file not found
```

**Solution**: Install jq:
- macOS: `brew install jq`
- Ubuntu: `apt-get install jq`
- Docker: Already included in image

## Future Enhancements

- [ ] Command history with up/down arrows
- [ ] Tab completion for paths
- [ ] Better progress bars with ETA
- [ ] Batch operations
- [ ] File synchronization
- [ ] Offline mode with queue

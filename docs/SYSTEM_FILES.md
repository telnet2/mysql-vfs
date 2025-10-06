# System Files and `/etc` Directory

## Overview

The VFS maintains a read-only `/etc` directory that contains system-wide configuration schemas and metadata definitions. This directory is embedded in the Go binary and automatically seeded during database initialization.

## Architecture

### Read-Only System Directory

The `/etc` directory has special properties:

1. **Immutable**: Cannot be modified by any user, including system-admin
2. **Embedded**: Files are compiled into the binary using `go:embed`
3. **Auto-Seeded**: Always overwritten during service startup
4. **Persistent**: Stored in MySQL but regenerated from embedded files

### Why `/etc`?

- **Consistency**: All VFS instances have identical system schemas
- **Versioning**: Schema updates are tied to binary version
- **Protection**: System schemas cannot be accidentally modified
- **Discovery**: Users can read schemas to understand file formats

## Metadata Fields

All directories, files, and file versions have a `metadata` JSON field.

### Metadata Schema

```json
{
  "owner": "string",      // Owner (user or group ID)
  "creator": "string",    // Original creator (user ID)
  "system": boolean,      // System-managed flag (optional)
  "readonly": boolean,    // Immutable flag (optional)
  "custom": {}            // User-defined metadata (optional)
}
```

### Database Schema

```sql
-- Added to directories table
ALTER TABLE directories ADD COLUMN metadata JSON DEFAULT NULL;

-- Added to files table
ALTER TABLE files ADD COLUMN metadata JSON DEFAULT NULL;

-- Added to file_versions table
ALTER TABLE file_versions ADD COLUMN metadata JSON DEFAULT NULL;
```

### System File Metadata

All files in `/etc` have this metadata:

```json
{
  "owner": "system-admin",
  "creator": "system-admin",
  "system": true
}
```

The `/etc` directory itself has:

```json
{
  "owner": "system-admin",
  "creator": "system-admin",
  "system": true,
  "readonly": true
}
```

## System Schema Files

The `/etc` directory contains JSON Schema definitions for special files:

### `/etc/owner.schema.json`

Validates `.owner` files that define directory ownership:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["owners"],
  "properties": {
    "owners": {
      "type": "array",
      "items": {
        "type": "string",
        "pattern": "^[a-z0-9-]+$"
      },
      "minItems": 1,
      "description": "List of group IDs that own this directory"
    }
  }
}
```

**Example `.owner` file:**
```json
{
  "owners": ["engineering", "data-team"]
}
```

### `/etc/files.schema.json`

Validates `.files` files that define allowed file patterns:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["patterns"],
  "properties": {
    "patterns": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["pattern"],
        "properties": {
          "pattern": {
            "type": "string",
            "description": "Glob pattern for allowed files"
          },
          "description": {
            "type": "string"
          },
          "content_types": {
            "type": "array",
            "items": {"type": "string"}
          },
          "max_size_bytes": {
            "type": "integer",
            "minimum": 0,
            "maximum": 104857600
          },
          "schema_ref": {
            "type": "string",
            "description": "Path to JSON schema for validation"
          }
        }
      }
    }
  }
}
```

**Example `.files` file:**
```json
{
  "patterns": [
    {
      "pattern": "*.json",
      "content_types": ["application/json"],
      "max_size_bytes": 10485760,
      "description": "JSON data files"
    }
  ]
}
```

### `/etc/events.schema.json`

Validates `.events` files that define lifecycle event handlers:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["handlers"],
  "properties": {
    "handlers": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "events", "type", "config"],
        "properties": {
          "name": {
            "type": "string",
            "pattern": "^[a-z0-9-]+$"
          },
          "events": {
            "type": "array",
            "items": {
              "type": "string",
              "pattern": "^(file|directory)\\.(create|update|delete)\\.(authorization|validation|execution|completion)\\.(started|succeeded|failed)$"
            },
            "minItems": 1
          },
          "type": {
            "type": "string",
            "enum": ["webhook", "log", "metrics"]
          },
          "config": {
            "type": "object"
          }
        }
      }
    }
  }
}
```

**Example `.events` file:**
```json
{
  "handlers": [
    {
      "name": "webhook-trigger",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/webhook",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

### `/etc/file.metadata.schema.json`

Validates file metadata structure:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["owner", "creator"],
  "properties": {
    "owner": {
      "type": "string",
      "pattern": "^[a-z0-9-]+$",
      "description": "Owner user or group ID"
    },
    "creator": {
      "type": "string",
      "pattern": "^[a-z0-9-]+$",
      "description": "Original creator user ID"
    },
    "system": {
      "type": "boolean",
      "description": "System-managed file flag"
    },
    "readonly": {
      "type": "boolean",
      "description": "Immutable file flag"
    },
    "custom": {
      "type": "object",
      "description": "User-defined metadata"
    }
  }
}
```

### `/etc/directory.metadata.schema.json`

Validates directory metadata structure (same as file metadata):

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["owner", "creator"],
  "properties": {
    "owner": {
      "type": "string",
      "pattern": "^[a-z0-9-]+$",
      "description": "Owner user or group ID"
    },
    "creator": {
      "type": "string",
      "pattern": "^[a-z0-9-]+$",
      "description": "Original creator user ID"
    },
    "system": {
      "type": "boolean",
      "description": "System directory flag"
    },
    "readonly": {
      "type": "boolean",
      "description": "Immutable directory flag"
    },
    "custom": {
      "type": "object",
      "description": "User-defined metadata"
    }
  }
}
```

## Protection Mechanism

### File Service Layer

In `pkg/domain/file_service.go`:

```go
// Check if path is under /etc
func isSystemProtectedPath(path string) bool {
    return strings.HasPrefix(path, "/etc/") || path == "/etc"
}

// In CreateFile, UpdateFile, DeleteFile:
if isSystemProtectedPath(directoryPath) {
    return nil, ErrProtectedSystemDirectory
}
```

### Directory Service Layer

In `pkg/domain/directory_service.go`:

```go
// In CreateDirectory:
if isSystemProtectedPath(parentPath) || isSystemProtectedPath(fullPath) {
    return nil, ErrProtectedSystemDirectory
}

// In DeleteDirectory:
if isSystemProtectedPath(path) {
    return ErrProtectedSystemDirectory
}
```

### Error Type

```go
var ErrProtectedSystemDirectory = errors.New("cannot modify system-protected /etc directory")
```

## Bootstrap Process

### Seeding Flow

1. **Migration Phase** (`pkg/persistence/db/migrate.go`):
   ```go
   func AutoMigrate(db *gorm.DB) error {
       // ... existing migrations ...

       // Bootstrap /etc directory
       if err := bootstrapSystemFiles(db); err != nil {
           return err
       }

       return nil
   }
   ```

2. **System Files Bootstrap**:
   ```go
   func bootstrapSystemFiles(db *gorm.DB) error {
       // 1. Delete existing /etc directory and files
       // 2. Create /etc directory with system metadata
       // 3. Seed all schemas from embedded FS
       // 4. Set system metadata on all files
   }
   ```

3. **Always Overwrite**: Unlike other bootstrap files (`.rego`, `.group`), `/etc` is ALWAYS recreated from embedded files on every startup.

### Embedded Files

In `pkg/setup/setup.go`:

```go
package setup

import (
    _ "embed"
    "embed"
)

//go:embed seed/*.schema.json
var seedFS embed.FS

// GetSchemaContent reads embedded schema file
func GetSchemaContent(filename string) ([]byte, error) {
    return seedFS.ReadFile("seed/" + filename)
}
```

## Implementation Checklist

### Phase 1: Database Schema
- [ ] Add `metadata` JSON field to `directories` table
- [ ] Add `metadata` JSON field to `files` table
- [ ] Add `metadata` JSON field to `file_versions` table
- [ ] Update GORM models with Metadata field
- [ ] Create migration for existing databases

### Phase 2: Schema Files
- [ ] Create `pkg/setup/seed/` directory
- [ ] Write `owner.schema.json`
- [ ] Write `files.schema.json`
- [ ] Write `events.schema.json`
- [ ] Write `file.metadata.schema.json`
- [ ] Write `directory.metadata.schema.json`
- [ ] Add `go:embed` directive

### Phase 3: Protection Logic
- [ ] Add `ErrProtectedSystemDirectory` error
- [ ] Add protection checks in `file_service.go`
- [ ] Add protection checks in `directory_service.go`
- [ ] Update authorization middleware if needed

### Phase 4: Bootstrap
- [ ] Create `bootstrapSystemFiles()` function
- [ ] Implement `/etc` directory creation with metadata
- [ ] Implement schema file seeding
- [ ] Update `AutoMigrate()` to call bootstrap
- [ ] Ensure idempotent behavior

### Phase 5: Testing
- [ ] Unit tests for metadata validation
- [ ] Tests for `/etc` protection
- [ ] Tests for bootstrap behavior
- [ ] Integration tests for seeding

## Usage Examples

### Reading System Schemas

```bash
# List system schemas
vfs-cli ls /etc

# View owner file schema
vfs-cli cat /etc/owner.schema.json

# View file metadata schema
vfs-cli cat /etc/file.metadata.schema.json
```

### Attempting to Modify (Should Fail)

```bash
# These should all fail with permission denied
vfs-cli rm /etc/owner.schema.json
vfs-cli import ./custom.json /etc/
vfs-cli mkdir /etc/custom
```

### Setting Metadata on User Files

```bash
# Create file with metadata (future feature)
vfs-cli import ./data.json /projects/ --metadata '{"owner":"data-team","creator":"john.doe"}'
```

## Migration Guide

### Existing Databases

When upgrading to this version:

1. **Automatic Migration**: Migration adds `metadata` column to existing tables
2. **Null Values**: Existing rows will have `NULL` metadata (acceptable)
3. **System Files**: `/etc` directory is automatically created
4. **No Action Required**: Upgrade is seamless

### API Changes

No breaking API changes. New features:

- `metadata` field available in all directory/file responses
- `/etc` directory appears in root listings
- Protection errors for `/etc` modifications

## See Also

- [Special Files](./SPECIAL_FILES.md) - Special file types (`.rego`, `.owner`, etc.)
- [Authorization](./AUTHORIZATION.md) - Authorization system using `.rego`
- [Lifecycle Events](./LIFECYCLE_EVENTS.md) - Event system using `.events`
- [File Validation](./VALIDATION.md) - Schema validation using `.files`

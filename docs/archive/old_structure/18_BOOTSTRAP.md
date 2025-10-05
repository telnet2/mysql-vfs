# MySQL VFS Bootstrap Guide

This guide explains how to bootstrap your MySQL VFS instance with default configuration files.

## What is Bootstrap?

Bootstrap creates the essential configuration files in your VFS root directory:

- **`/.rego`** - Default authorization policy (<pkg>middleware.default_policy.go</pkg>)
- **`/.group`** - Default group definitions (<pkg>setup.setup.go</pkg>)

These files are created as actual VFS files that you can view, edit, and manage like any other file.

## Core Components

### Bootstrap Script

**Location**: `scripts/bootstrap.go`

A helper script that displays the default configuration files that should be created.

**Usage**:
```bash
cd scripts
go run bootstrap.go
```

### Setup Package

**Location**: `pkg/setup/setup.go`

Provides the bootstrap logic and default configurations:

- `DefaultRegoPolicy` (line 11): Default authorization policy template
- `DefaultGroupConfig` (line 52): Default group configuration
- `NewBootstrapper()` (line 72): Creates a bootstrapper instance
- `Bootstrap()` (line 80): Main bootstrap function
- `BootstrapWithServices()` (line 140): Bootstrap using service layer

## When to Run Bootstrap

Run bootstrap:
- **After initial database migration** - First time setup
- **After accidentally deleting /.rego or /.group** - Restore defaults
- **When starting fresh** - Reset to default configuration

## Running Bootstrap

### Option 1: Using the Bootstrap Script

```bash
cd scripts
go run bootstrap.go
```

This displays the default file contents and curl commands to create them.

### Option 2: Manual API Calls

Create the files using the VFS API with system-admin token:

```bash
# Set your system admin token
export SYSTEM_ADMIN_TOKEN="your-secure-token-here"

# Create /.rego file
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".rego",
    "content_type": "text/plain",
    "content": "package vfs.authz\n\n# Admin group: full access to all actions\nallow {\n    input.user.groups[_] == \"admin\"\n}\n\n# System admin group: full access\nallow {\n    input.user.groups[_] == \"system-admin\"\n}\n\n# User group: read-only access\nallow {\n    input.user.groups[_] == \"user\"\n    input.action == \"read\"\n}"
  }'

# Create /.group file
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".group",
    "content_type": "application/json",
    "content": "{\"groups\":[{\"group_id\":\"admin\",\"members\":[]},{\"group_id\":\"user\",\"members\":[]}]}"
  }'
```

### Option 3: Programmatically

Use the setup package in your Go application:

```go
import (
    "context"
    "github.com/telnet2/mysql-vfs/pkg/setup"
    "github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
)

func main() {
    ctx := context.Background()

    // Initialize repositories
    dirRepo := mysql.NewGormDirectoryRepository(db)
    fileRepo := mysql.NewGormFileRepository(db, storage)

    // Run bootstrap
    err := setup.BootstrapWithServices(ctx, dirRepo, fileRepo)
    if err != nil {
        log.Printf("Bootstrap warning: %v", err)
    }
}
```

## What Gets Created

### `/.rego` - Default Authorization Policy

**Source**: `pkg/setup/setup.go` (line 11-49) and `pkg/middleware/default_policy.go` (line 13-51)

```rego
package vfs.authz

# Default policy: admin group has full access, user group has read-only access, owners have write access

# Admin group: full access to all actions
allow {
    input.user.groups[_] == "admin"
}

# System admin group: full access to all actions
allow {
    input.user.groups[_] == "system-admin"
}

# User group: read-only access
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}

# Owners: Users in owner groups can write
allow {
    input.user.groups[_] == "user"
    input.action == "write"
    is_owner
}

# Owners: Users in owner groups can delete
allow {
    input.user.groups[_] == "user"
    input.action == "delete"
    is_owner
}

# Helper rule: Check if user is in any owner group
is_owner {
    user_group := input.user.groups[_]
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
```

**Default Behavior:**
- `admin` group → full access (read, write, delete)
- `system-admin` group → full access (read, write, delete)
- `user` group → read-only access
- `user` group + owner group → read+write+delete access in owned directories

**Related Components:**
- Policy loader: `pkg/domain/policy_loader.go`
- Authorization middleware: `pkg/middleware/authorization.go`

### `/.group` - Default Groups

**Source**: `pkg/setup/setup.go` (line 52-63)

```json
{
  "groups": [
    {
      "group_id": "admin",
      "members": []
    },
    {
      "group_id": "user",
      "members": []
    }
  ]
}
```

**Default Groups:**
- `admin` - Initially empty, add admin users here
- `user` - Initially empty, add regular users here

**Related Components:**
- Group loader: `pkg/domain/group_loader.go`
- Special file definitions: `pkg/domain/special_files.go`

## Next Steps After Bootstrap

### 1. Create Users

Create a `/.user` file to define users:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content_type": "application/json",
    "content": "{\"users\": [{\"user_id\": \"alice\", \"password_hash\": \"$2a$10$...\", \"groups\": [\"admin\"]}]}"
  }'
```

**Related Components:**
- User loader: `pkg/domain/user_loader.go`
- User credentials spec: `pkg/domain/special_files.go` (UserCredential struct)

### 2. Add Users to Groups

Edit the `/.group` file to assign users to groups:

```bash
# Get current .group file ID
FILE_ID=$(curl -s http://localhost:8080/api/v1/files?directory_path=/&name=.group \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" | jq -r '.files[0].id')

# Update .group file
curl -X PUT http://localhost:8080/api/v1/files/$FILE_ID \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "{\"groups\":[{\"group_id\":\"admin\",\"members\":[\"alice\",\"bob\"]},{\"group_id\":\"user\",\"members\":[\"charlie\",\"david\"]}]}"
  }'
```

### 3. Customize the Authorization Policy (Optional)

Edit the `/.rego` file to customize authorization rules:

```bash
# Get current .rego file ID
FILE_ID=$(curl -s http://localhost:8080/api/v1/files?directory_path=/&name=.rego \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" | jq -r '.files[0].id')

# Update .rego file
curl -X PUT http://localhost:8080/api/v1/files/$FILE_ID \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  --data-binary @custom-policy.rego
```

### 4. Set Directory Ownership (Optional)

Create `.owner` files in directories to control ownership:

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/projects/alpha",
    "name": ".owner",
    "content_type": "application/json",
    "content": "{\"owner\": \"alice\"}"
  }'
```

**Related Components:**
- Owner loader: `pkg/domain/owner_loader.go`
- Owner config spec: `pkg/domain/special_files.go` (OwnerConfig struct)

## Idempotency

Bootstrap is **idempotent** - it's safe to run multiple times:

- ✅ If `/.rego` exists → **Skip** (keeps your customizations)
- ✅ If `/.group` exists → **Skip** (keeps your group definitions)
- ✅ Only creates missing files

**Implementation**: See `pkg/setup/setup.go` lines 100-116 and 120-136

## Protection Rules

Bootstrap files are protected by default protection rules:

**Source**: `pkg/domain/protection.go`

- `/.rego` - Only system-admin can modify/delete (line 58)
- `/.group` - Only system-admin can modify/delete, cannot exist in subdirectories (line 63-68)
- `/.user` - Only system-admin can modify/delete, cannot exist in subdirectories (line 66-68)

## Troubleshooting

### Bootstrap Fails with "Permission Denied"

**Solution:** Use a system-admin token:

```bash
# Generate a secure token
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)

# Add to your environment or .env file
echo "SYSTEM_ADMIN_TOKEN=$SYSTEM_ADMIN_TOKEN" >> .env
```

**Configuration**: See `pkg/config/config.go` for SYSTEM_ADMIN_TOKEN setup

### Files Already Exist

**Expected behavior.** Bootstrap skips existing files to preserve your customizations.

To reset:
1. Delete the files manually (requires system-admin token)
2. Run bootstrap again

```bash
# Delete existing .rego
curl -X DELETE http://localhost:8080/api/v1/files/$FILE_ID \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN"

# Run bootstrap again
cd scripts && go run bootstrap.go
```

### Default Policy Not Working

**Check:**
1. Bootstrap ran successfully: `SELECT * FROM files WHERE name = '.rego' AND directory_path = '/'`
2. Policy is valid Rego: `opa check /.rego`
3. Authorization middleware is configured in `services/vfs/main.go`

**Related Components:**
- Authorization middleware: `pkg/middleware/authorization.go`
- Policy loader: `pkg/domain/policy_loader.go`

## Default Policy Reference

### Groups

| Group | Access | Defined In |
|-------|--------|------------|
| `system-admin` | Bypasses all authorization | `pkg/middleware/authorization.go` |
| `admin` | Full access everywhere | `pkg/setup/setup.go` (line 18) |
| `user` | Read-only (unless owner) | `pkg/setup/setup.go` (line 28) |

### Ownership

Users in the `user` group can write/delete in directories they own:
- Ownership is defined in `.owner` files (<pkg>domain.owner_loader.go</pkg>)
- User must be in an owner group for the directory
- Ownership inherits from parent directories

**Implementation**: See `pkg/domain/owner_loader.go` for inheritance logic (line 38-89)

### Examples

**Example 1: Admin group has full access**
```
User: alice (groups: ["admin"])
Action: write to /data/file.json
Result: ✅ Allowed (admin group rule at line 18)
```

**Example 2: User group can read**
```
User: bob (groups: ["user"])
Action: read /data/file.json
Result: ✅ Allowed (user group read rule at line 28)
```

**Example 3: User cannot write (not owner)**
```
User: bob (groups: ["user"])
Action: write to /data/file.json
Directory owners: ["alice-group"]
Result: ❌ Denied (not in owner group)
```

**Example 4: User can write (is owner)**
```
User: charlie (groups: ["user", "charlie-group"])
Action: write to /projects/alpha/file.json
Directory owners: ["charlie-group"]
Result: ✅ Allowed (owner rule at line 35)
```

## Code References

### Core Implementation Files

- **Bootstrap logic**: `pkg/setup/setup.go`
  - Default policies: lines 11-63
  - Bootstrap function: lines 80-98
  - Service integration: lines 140-200

- **Default policy fallback**: `pkg/middleware/default_policy.go`
  - Used when no .rego file exists
  - Lines 13-51

- **Protection rules**: `pkg/domain/protection.go`
  - Protects bootstrap files
  - Lines 42-102

- **Loaders**:
  - Policy: `pkg/domain/policy_loader.go`
  - Groups: `pkg/domain/group_loader.go`
  - Users: `pkg/domain/user_loader.go`
  - Owners: `pkg/domain/owner_loader.go`

### Related Documentation

- [Authorization Guide](6_AUTHORIZATION.md)
- [Special Files](4_SPECIAL_FILES.md)
- [Authentication](5_AUTHENTICATION.md)
- [Resource Protection](19_RESOURCE_PROTECTION.md)

## See Also

- Bootstrap script: `scripts/bootstrap.go`
- Environment configuration: `pkg/config/config.go`
- VFS main service: `services/vfs/main.go`

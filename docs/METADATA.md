# Metadata Guide

**Version**: 1.0
**Status**: Production Ready
**Last Updated**: 2025-10-06

---

## Overview

All files and directories in the VFS system have **metadata** - structured JSON data tracking ownership, creation, and custom attributes. Metadata enables:

- **Ownership tracking** (who owns what)
- **Audit trails** (who created, who updated)
- **Delegation tracking** (on-behalf-of operations)
- **Custom attributes** (user-defined tags)

---

## Metadata Structure

### Required Fields

All resources have these fields:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `owner` | string | Effective owner (principal if delegated) | `"alice"` |
| `creator` | string | Actual creator (actor who performed creation) | `"service-account"` |
| `system` | boolean | System-managed resource flag | `false` |
| `created_at` | string | ISO 8601 timestamp of creation | `"2025-10-06T10:00:00Z"` |

### Optional Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `readonly` | boolean | Immutable resource flag | `true` |
| `updated_at` | string | ISO 8601 timestamp of last update | `"2025-10-06T11:00:00Z"` |
| `updated_by` | string | Actor who performed last update | `"admin-user"` |
| `delegated` | boolean | Created via delegation | `true` |
| `delegation_reason` | string | Audit trail for delegation | `"automated-backup"` |
| `custom` | object | User-defined metadata | `{"project":"web","env":"prod"}` |

---

## Examples

### Regular File (No Delegation)

User creates file directly:

```json
{
  "owner": "alice",
  "creator": "alice",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z"
}
```

### Delegated File

Service account creates file on behalf of user:

```json
{
  "owner": "alice",
  "creator": "service-account",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z",
  "delegated": true,
  "delegation_reason": "automated-backup"
}
```

### System File

Bootstrap-created file in `/etc`:

```json
{
  "owner": "system-admin",
  "creator": "system-admin",
  "system": true,
  "readonly": true,
  "created_at": "2025-10-06T09:00:00Z"
}
```

### File with Custom Metadata

User adds custom tags:

```json
{
  "owner": "alice",
  "creator": "alice",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z",
  "custom": {
    "project": "data-pipeline",
    "environment": "production",
    "cost-center": "engineering",
    "version": "1.2.3"
  }
}
```

### Updated File

File updated by different user:

```json
{
  "owner": "alice",
  "creator": "alice",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z",
  "updated_at": "2025-10-06T11:30:00Z",
  "updated_by": "bob"
}
```

---

## Metadata Schemas

Metadata structure is defined in JSON Schema files:

### File Metadata Schema

**Location**: `/etc/schemas/file.metadata.schema.json`

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "File Metadata Schema",
  "type": "object",
  "required": ["owner", "creator"],
  "properties": {
    "owner": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9-]*[a-z0-9]$",
      "minLength": 2,
      "maxLength": 64
    },
    "creator": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9-]*[a-z0-9]$",
      "minLength": 2,
      "maxLength": 64
    },
    "system": {
      "type": "boolean"
    },
    "readonly": {
      "type": "boolean"
    },
    "custom": {
      "type": "object"
    }
  }
}
```

### Directory Metadata Schema

**Location**: `/etc/schemas/directory.metadata.schema.json`

Same structure as file metadata.

---

## Viewing Metadata

### CLI

```bash
# List file with metadata
vfs-cli ls -l /data/report.txt

# View file details including metadata
vfs-cli cat /data/report.txt --json | jq '.metadata'
```

### API

```bash
# Get file with metadata
curl "${VFS_URL}/api/v1/files/data/report.txt" \
  -H "Authorization: Bearer ${TOKEN}"
```

**Response**:
```json
{
  "id": "file-123",
  "name": "report.txt",
  "path": "/data/report.txt",
  "size_bytes": 1024,
  "content_type": "text/plain",
  "metadata": {
    "owner": "alice",
    "creator": "service-account",
    "system": false,
    "created_at": "2025-10-06T10:00:00Z",
    "delegated": true,
    "delegation_reason": "automated-report"
  },
  "created_at": "2025-10-06T10:00:00Z",
  "updated_at": "2025-10-06T10:00:00Z"
}
```

---

## Setting Custom Metadata (Future)

### API (Planned)

```bash
# Create file with custom metadata
curl -X POST "${VFS_URL}/api/v1/files?path=/data/report.txt&metadata={\"project\":\"web\"}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: text/plain" \
  -d "Report content..."
```

### CLI (Planned)

```bash
# Import with custom metadata
vfs-cli import local.txt /data/ \
  --metadata='{"project":"web","env":"prod"}'

# Set metadata on existing file
vfs-cli metadata set /data/report.txt \
  --key=project --value=web
```

---

## Querying by Metadata (Future)

### Find Files by Owner

```bash
# API
curl "${VFS_URL}/api/v1/files?metadata.owner=alice" \
  -H "Authorization: Bearer ${TOKEN}"

# CLI
vfs-cli find / --owner=alice
```

### Find Files by Custom Tag

```bash
# Find all production files
vfs-cli find / --metadata=environment:production

# Find by project
vfs-cli find / --metadata=project:data-pipeline
```

---

## Metadata in Workflows

### Backup Workflow

```bash
# 1. Service account creates backup
vfs-cli import backup-$(date +%Y%m%d).tar.gz /backups/alice/ \
  --on-behalf-of=alice \
  --reason="daily-backup"

# Metadata:
# {
#   "owner": "alice",
#   "creator": "backup-service",
#   "delegated": true,
#   "delegation_reason": "daily-backup"
# }

# 2. Query backups for alice
vfs-cli ls /backups/alice/

# 3. Restore (alice owns, can read)
vfs-cli cat /backups/alice/backup-20251006.tar.gz > restore.tar.gz
```

### CI/CD Workflow

```bash
# 1. Jenkins deploys artifact
curl -X POST "${VFS_URL}/api/v1/files?path=/prod/app/release.zip" \
  -H "Authorization: Bearer ${JENKINS_TOKEN}" \
  -H "X-VFS-On-Behalf-Of: deployment-bot" \
  -H "X-VFS-Delegation-Reason: release-v1.2.3" \
  --data-binary @release.zip

# Metadata:
# {
#   "owner": "deployment-bot",
#   "creator": "jenkins",
#   "delegated": true,
#   "delegation_reason": "release-v1.2.3"
# }

# 2. Application reads artifact (authorized as deployment-bot)
curl "${VFS_URL}/api/v1/files/prod/app/release.zip" \
  -H "Authorization: Bearer ${APP_TOKEN}"
```

---

## Metadata Best Practices

### 1. Use Delegation for Automation

Always use delegation when services create resources for users:

```bash
# Good: Clear ownership
vfs-cli import data.csv /users/alice/ \
  --on-behalf-of=alice \
  --reason="data-import-job"

# Bad: Service owns the file
vfs-cli import data.csv /users/alice/
# (owner would be "import-service", not "alice")
```

### 2. Provide Clear Delegation Reasons

```bash
# Good: Specific reason
--reason="nightly-backup-job"
--reason="release-v1.2.3-deployment"
--reason="workspace-initialization"

# Bad: Vague reason
--reason="backup"
--reason="update"
--reason="stuff"
```

### 3. Use Custom Metadata for Classification

```json
{
  "owner": "alice",
  "creator": "alice",
  "custom": {
    "classification": "confidential",
    "retention": "7years",
    "compliance": "gdpr,sox",
    "project": "customer-data",
    "cost-center": "CC-1234"
  }
}
```

### 4. Track Update History

Metadata automatically tracks updates:

```json
{
  "owner": "alice",
  "creator": "alice",
  "created_at": "2025-10-06T10:00:00Z",
  "updated_at": "2025-10-06T11:00:00Z",
  "updated_by": "bob"
}
```

### 5. Review System Files

System files have special metadata:

```bash
# View system file metadata
vfs-cli cat /etc/schemas/file.metadata.schema.json --json | jq '.metadata'

# Output:
# {
#   "owner": "system-admin",
#   "creator": "system-admin",
#   "system": true,
#   "readonly": true
# }
```

---

## Metadata Storage

### Database Schema

Metadata is stored as JSON in the database:

```sql
-- files table
CREATE TABLE files (
  id VARCHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  directory_id VARCHAR(36),
  metadata JSON,  -- ← Metadata column
  created_at TIMESTAMP,
  updated_at TIMESTAMP,
  ...
);

-- directories table
CREATE TABLE directories (
  id VARCHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  path VARCHAR(4096) NOT NULL,
  metadata JSON,  -- ← Metadata column
  created_at TIMESTAMP,
  updated_at TIMESTAMP,
  ...
);
```

### Querying Metadata in SQL

```sql
-- Find files by owner
SELECT * FROM files
WHERE JSON_EXTRACT(metadata, '$.owner') = 'alice';

-- Find delegated files
SELECT * FROM files
WHERE JSON_EXTRACT(metadata, '$.delegated') = true;

-- Find files by custom tag
SELECT * FROM files
WHERE JSON_EXTRACT(metadata, '$.custom.project') = 'web';
```

---

## Metadata Lifecycle

### Creation

```
User Request → Auth Middleware → Delegation Middleware → Domain Service
                                                              ↓
                                              buildMetadata(authCtx, custom)
                                                              ↓
                                                      JSON → Database
```

### Update

```
Update Request → Extract existing metadata → Add update fields → Save
                                                  ↓
                                   {
                                     "updated_at": "...",
                                     "updated_by": "..."
                                   }
```

### Retrieval

```
Database → JSON → Parse → Include in API response
```

---

## Troubleshooting

### Missing Metadata

**Symptom**: File has no metadata

**Cause**: File created before metadata feature

**Solution**: Metadata is optional (nullable). Old files will have NULL metadata.

### Invalid Metadata Format

**Symptom**: Error when creating file

**Cause**: Custom metadata doesn't match schema

**Solution**: Validate JSON structure:
```bash
# Valid
--metadata='{"key":"value"}'

# Invalid (not JSON)
--metadata='key=value'
```

### Owner vs Creator Confusion

**Q**: Why is owner different from creator?

**A**: Delegation!
- `owner` = principal (on whose behalf)
- `creator` = actor (who performed action)

Example:
```json
{
  "owner": "alice",        // Alice owns the file
  "creator": "backup-service"  // Backup service created it
}
```

---

## Migration

### Backfilling Metadata

For files created before metadata feature:

```sql
-- Add default metadata to files without it
UPDATE files
SET metadata = JSON_OBJECT(
  'owner', 'unknown',
  'creator', 'system',
  'system', false
)
WHERE metadata IS NULL;
```

### Schema Validation

Validate metadata structure:

```bash
# Get schema
vfs-cli cat /etc/schemas/file.metadata.schema.json > schema.json

# Validate metadata
echo '{"owner":"alice","creator":"alice"}' | \
  ajv validate -s schema.json -d -
```

---

## Related Documentation

- [On-Behalf-Of Delegation](./ON_BEHALF_OF.md) - Delegation guide
- [Authorization](./AUTHORIZATION.md) - Authorization system
- [System Files](./SYSTEM_FILES.md) - System files and schemas
- [API Reference](./API_REFERENCE.md) - Complete API documentation

---

## Future Enhancements

### Metadata Search API

```bash
# Find files by metadata
GET /api/v1/files/search?metadata.owner=alice
GET /api/v1/files/search?metadata.custom.project=web
```

### Metadata Indexing

```sql
-- Add indexes for common queries
CREATE INDEX idx_files_metadata_owner
  ON files((JSON_EXTRACT(metadata, '$.owner')));

CREATE INDEX idx_files_metadata_delegated
  ON files((JSON_EXTRACT(metadata, '$.delegated')));
```

### Metadata Validation

Validate custom metadata against user-defined schemas:

```bash
# Register metadata schema
vfs-cli metadata register-schema /schemas/project.json

# Create file with validated metadata
vfs-cli import data.csv /data/ \
  --metadata='{"project":"web"}' \
  --validate-schema=/schemas/project.json
```

---

## Support

For questions or issues:
- GitHub Issues: https://github.com/telnet2/mysql-vfs/issues
- Documentation: https://github.com/telnet2/mysql-vfs/docs

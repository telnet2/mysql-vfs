# 10. API Reference

**MySQL VFS v2.1+ REST API**

[← Back: Deployment](9_DEPLOYMENT.md) | [Index](0_README.md) | [Next: Testing →](11_TESTING.md)

---

## Base URL

```
http://localhost:8080/api/v1
```

**Implementation:** `services/vfs/handlers/`

---

## Directories

**Implementation:** `services/vfs/handlers/directory.go`, `pkg/domain/directory_service.go`

### Create Directory
```http
POST /api/v1/directories
```

**Request:**
```json
{
  "parent_path": "/",
  "name": "data"
}
```

**Response (201):**
```json
{
  "id": "uuid",
  "name": "data",
  "path": "/data",
  "created_at": "2025-10-04T12:00:00Z"
}
```

### List Directory
```http
GET /api/v1/directories/{path}?limit=100&cursor=
```

**Response (200):**
```json
{
  "entries": [
    {"name": "file.json", "type": "file", "size_bytes": 100, "modified_at": "..."},
    {"name": "subdir", "type": "directory", "size_bytes": 0, "modified_at": "..."}
  ],
  "next_cursor": "abc123"
}
```

### Delete Directory
```http
DELETE /api/v1/directories/{path}?recursive=true
```

---

## Files

**Implementation:** `services/vfs/handlers/file.go`, `pkg/domain/file_service.go`

### Create File
```http
POST /api/v1/files
```

**Request:**
```json
{
  "directory_path": "/data",
  "name": "file.json",
  "content_type": "application/json",
  "content": "{\"key\":\"value\"}"
}
```

**Response (201):**
```json
{
  "id": "uuid",
  "name": "file.json",
  "version": 1,
  "checksum": "sha256...",
  "created_at": "2025-10-04T12:00:00Z"
}
```

### Read File
```http
GET /api/v1/files/{path}
```

**Response (200):**
```
{file content}
```

### Update File
```http
PUT /api/v1/files/{path}
```

**Request:**
```json
{
  "content": "{\"key\":\"new-value\"}",
  "expected_version": 1
}
```

### Delete File
```http
DELETE /api/v1/files/{path}
```

### Move File
```http
POST /api/v1/files/move
```

**Request:**
```json
{
  "source_path": "/data/old.json",
  "destination_path": "/data/new.json"
}
```

---

## Authentication

**Implementation:** `pkg/middleware/auth.go`, `pkg/middleware/auth_providers.go`

All requests (except `/health`, `/ready`) require authentication.

**Bearer Token (Production):**
```
Authorization: Bearer <token-from-user-file>
```

**System Admin Token (Bootstrap):**
```
Authorization: Bearer $SYSTEM_ADMIN_TOKEN
```

**Header Auth (Dev mode only):**
```
X-User-ID: alice
X-User-Groups: admin,engineering
```

**Note:** Authorization uses group-based access control. Users can belong to multiple groups.

---

## Special Files

Access special files using the Files API:

- `GET /api/v1/files/.files` - List files in directory
- `GET /api/v1/files/.user` - User authentication data (implementation: `pkg/domain/user_loader.go`)
- `GET /api/v1/files/.events` - Event handlers (implementation: `pkg/domain/events_loader.go`)
- `GET /api/v1/files/.rego` - OPA policies (implementation: `pkg/domain/policy_loader.go`)
- `GET /api/v1/files/.owner` - Resource ownership (implementation: `pkg/domain/owner_loader.go`)

**Removed in v2.1+:** `.jsonschema`, `.quota`, `.lifecycle`, `.group`

---

## Error Responses

**Implementation:** `services/vfs/handlers/errors.go`

```json
{
  "error": "error message",
  "details": ["validation error 1", "validation error 2"]
}
```

**Status Codes:**
- 400: Bad Request
- 401: Unauthorized (invalid or missing token)
- 403: Forbidden (OPA policy denied, or operation vetoed by event handler)
- 404: Not Found
- 409: Conflict (version mismatch, optimistic locking failure)
- 500: Internal Server Error

---

[← Back: Deployment](9_DEPLOYMENT.md) | [Index](0_README.md) | [Next: Testing →](11_TESTING.md)

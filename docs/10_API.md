# 10. API Reference

[← Back: Deployment](9_DEPLOYMENT.md) | [Index](0_README.md) | [Next: Testing →](11_TESTING.md)

---

## Base URL

```
http://localhost:8080/api/v1
```

---

## Directories

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

All requests (except `/health`, `/ready`) require authentication.

**Header:**
```
Authorization: Bearer <jwt-token>
```

Or (dev mode only):
```
X-User-ID: alice
X-User-Role: admin
X-User-Groups: engineering,admins
```

---

## Error Responses

```json
{
  "error": "error message",
  "details": ["validation error 1", "validation error 2"]
}
```

**Status Codes:**
- 400: Bad Request
- 401: Unauthorized
- 403: Forbidden
- 404: Not Found
- 409: Conflict (version mismatch)
- 500: Internal Server Error

---

[← Back: Deployment](9_DEPLOYMENT.md) | [Index](0_README.md) | [Next: Testing →](11_TESTING.md)

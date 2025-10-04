# .events Special File Specification

## Overview

The `.events` special file defines event handlers for file and directory operations in VFS.

**Features:**
- Multiple handlers per event
- Webhook, log, metrics, and extensible handler types
- Per-handler retry, circuit breaker, filtering
- Inheritance from parent directories

## Event Types

### File Events
- `file.created` - File uploaded/created
- `file.updated` - File content updated
- `file.deleted` - File deleted
- `file.moved` - File moved/renamed

### Directory Events
- `directory.created` - Directory created
- `directory.deleted` - Directory deleted (recursive or empty)

## Format

```json
{
  "handlers": [
    {
      "name": "notify-external-service",
      "events": ["file.created", "file.updated"],
      "type": "webhook",
      "enabled": true,
      "filter": {
        "pattern": "*.json",
        "type": "glob",
        "min_size_bytes": 0,
        "max_size_bytes": 10485760
      },
      "config": {
        "url": "https://example.com/webhook",
        "method": "POST",
        "headers": {
          "X-Custom-Header": "value"
        },
        "secret": "hmac-secret-for-signature",
        "timeout_ms": 5000,
        "retry": {
          "max_attempts": 3,
          "initial_delay_ms": 1000,
          "max_delay_ms": 60000,
          "backoff": "exponential"
        },
        "circuit_breaker": {
          "enabled": true,
          "failure_threshold": 5,
          "recovery_timeout_ms": 30000
        }
      }
    },
    {
      "name": "audit-log",
      "events": ["file.deleted", "directory.deleted"],
      "type": "log",
      "enabled": true,
      "config": {
        "level": "warn",
        "message": "{{event.type}}: {{resource.path}} by {{user.user_id}}"
      }
    }
  ]
}
```

## Handler Fields

### Common Fields

- `name` (string, required) - Unique handler name
- `events` (array, required) - List of event types to handle
- `type` (string, required) - Handler type: "webhook", "log", "metrics"
- `enabled` (boolean, optional) - Enable/disable handler (default: true)
- `filter` (object, optional) - Event filtering criteria

### Filter Object

```json
{
  "pattern": "*.json",
  "type": "glob",
  "min_size_bytes": 1024,
  "max_size_bytes": 10485760,
  "content_types": ["application/json", "text/plain"]
}
```

- `pattern` - Filename pattern (glob or regex)
- `type` - Pattern type: "glob" or "regex"
- `min_size_bytes` - Minimum file size
- `max_size_bytes` - Maximum file size
- `content_types` - Allowed content types

## Handler Types

### 1. Webhook Handler

```json
{
  "name": "external-webhook",
  "events": ["file.created"],
  "type": "webhook",
  "config": {
    "url": "https://example.com/webhook",
    "method": "POST",
    "headers": {
      "X-API-Key": "secret"
    },
    "secret": "hmac-secret",
    "timeout_ms": 5000,
    "retry": {
      "max_attempts": 3,
      "initial_delay_ms": 1000,
      "max_delay_ms": 60000,
      "backoff": "exponential"
    },
    "circuit_breaker": {
      "enabled": true,
      "failure_threshold": 5,
      "recovery_timeout_ms": 30000
    }
  }
}
```

**Webhook Payload:**
```json
{
  "event": {
    "id": "evt_abc123",
    "type": "file.created",
    "timestamp": "2025-10-04T12:00:00Z",
    "directory_path": "/data/users"
  },
  "resource": {
    "type": "file",
    "id": "file_xyz789",
    "name": "alice.json",
    "path": "/data/users/alice.json",
    "size_bytes": 1024,
    "content_type": "application/json",
    "version": 1,
    "checksum_sha256": "abc..."
  },
  "user": {
    "user_id": "alice",
    "role": "admin",
    "groups": ["engineering", "admins"]
  },
  "metadata": {
    "request_id": "req_123",
    "ip_address": "192.168.1.1",
    "user_agent": "vfs-cli/1.0"
  }
}
```

**HMAC Signature:**
- Header: `X-VFS-Signature: sha256=<hex>`
- Computed: `HMAC-SHA256(payload_json, secret)`

**Retry Logic:**
- Exponential backoff: 1s, 2s, 4s, 8s, ...
- Linear backoff: 1s, 2s, 3s, 4s, ...
- Max attempts: Configurable (default: 3)
- Max delay: Configurable (default: 60s)

**Circuit Breaker:**
- State: Closed → Open → Half-Open → Closed
- Failure threshold: Number of consecutive failures to open circuit
- Recovery timeout: Time before trying half-open state

### 2. Log Handler

```json
{
  "name": "audit-logger",
  "events": ["file.deleted", "directory.deleted"],
  "type": "log",
  "config": {
    "level": "warn",
    "message": "{{event.type}}: {{resource.path}} by {{user.user_id}}"
  }
}
```

**Log Levels:** `debug`, `info`, `warn`, `error`

**Template Variables:**
- `{{event.type}}` - Event type
- `{{event.timestamp}}` - ISO 8601 timestamp
- `{{resource.type}}` - "file" or "directory"
- `{{resource.path}}` - Full path
- `{{resource.name}}` - Resource name
- `{{user.user_id}}` - User ID
- `{{user.role}}` - User role

### 3. Metrics Handler

```json
{
  "name": "metrics-collector",
  "events": ["file.created", "file.updated"],
  "type": "metrics",
  "config": {
    "metric_name": "vfs.files.operations",
    "tags": {
      "event_type": "{{event.type}}",
      "directory": "{{resource.directory}}",
      "user_role": "{{user.role}}"
    },
    "value_field": "resource.size_bytes"
  }
}
```

**Metrics are emitted to logs** (can be scraped by Prometheus, Datadog, etc.)

## Event Payloads

### File Event Payload

```json
{
  "event": {
    "id": "evt_abc123",
    "type": "file.created",
    "timestamp": "2025-10-04T12:00:00Z",
    "directory_path": "/data/users"
  },
  "resource": {
    "type": "file",
    "id": "file_xyz789",
    "name": "alice.json",
    "path": "/data/users/alice.json",
    "size_bytes": 1024,
    "content_type": "application/json",
    "version": 1,
    "checksum_sha256": "abc...",
    "created_at": "2025-10-04T12:00:00Z",
    "updated_at": "2025-10-04T12:00:00Z"
  },
  "user": {
    "user_id": "alice",
    "role": "admin",
    "groups": ["engineering", "admins"]
  },
  "metadata": {
    "request_id": "req_123",
    "ip_address": "192.168.1.1"
  }
}
```

### Directory Event Payload

```json
{
  "event": {
    "id": "evt_def456",
    "type": "directory.created",
    "timestamp": "2025-10-04T12:00:00Z",
    "directory_path": "/data"
  },
  "resource": {
    "type": "directory",
    "id": "dir_789",
    "name": "users",
    "path": "/data/users",
    "created_at": "2025-10-04T12:00:00Z"
  },
  "user": {
    "user_id": "admin",
    "role": "admin",
    "groups": ["admins"]
  },
  "metadata": {
    "request_id": "req_456"
  }
}
```

## Examples

### Example 1: Multiple Webhooks

```json
{
  "handlers": [
    {
      "name": "primary-webhook",
      "events": ["file.created", "file.updated"],
      "type": "webhook",
      "filter": {
        "pattern": "*.json",
        "type": "glob"
      },
      "config": {
        "url": "https://primary.example.com/webhook",
        "secret": "primary-secret"
      }
    },
    {
      "name": "backup-webhook",
      "events": ["file.created", "file.updated"],
      "type": "webhook",
      "filter": {
        "pattern": "*.json",
        "type": "glob"
      },
      "config": {
        "url": "https://backup.example.com/webhook",
        "secret": "backup-secret"
      }
    }
  ]
}
```

**Result:** Both webhooks are called for JSON file operations

### Example 2: Conditional Handlers

```json
{
  "handlers": [
    {
      "name": "large-file-webhook",
      "events": ["file.created"],
      "type": "webhook",
      "filter": {
        "min_size_bytes": 10485760
      },
      "config": {
        "url": "https://example.com/large-files"
      }
    },
    {
      "name": "small-file-log",
      "events": ["file.created"],
      "type": "log",
      "filter": {
        "max_size_bytes": 10485759
      },
      "config": {
        "level": "info",
        "message": "Small file created: {{resource.path}}"
      }
    }
  ]
}
```

**Result:** Large files (>10MB) trigger webhook, small files just log

### Example 3: Admin Operations Audit

```json
{
  "handlers": [
    {
      "name": "admin-audit",
      "events": ["file.deleted", "directory.deleted"],
      "type": "webhook",
      "config": {
        "url": "https://audit.example.com/admin-actions",
        "secret": "audit-secret",
        "headers": {
          "X-Audit-System": "VFS"
        }
      }
    },
    {
      "name": "deletion-log",
      "events": ["file.deleted", "directory.deleted"],
      "type": "log",
      "config": {
        "level": "warn",
        "message": "DELETION: {{resource.type}} {{resource.path}} by {{user.user_id}}"
      }
    }
  ]
}
```

## Inheritance

`.events` files **inherit from parent directories** with **merging**:

```
/
├── .events (handlers: ["global-webhook"])
└── data/
    ├── .events (handlers: ["data-webhook"])
    └── users/
        └── alice.json (triggers: global-webhook + data-webhook)
```

**Merging Rules:**
1. Handlers from parent and child are combined
2. Handlers with same `name` in child override parent
3. Disabled handlers (`enabled: false`) in child remove parent handlers

## Error Handling

### Webhook Failures

**Transient errors** (network, timeout, 5xx):
- Retry with backoff
- Open circuit breaker after threshold

**Permanent errors** (4xx, invalid config):
- Log error, don't retry
- Disable handler if misconfigured

### Handler Failures

Handlers run **asynchronously** - failures don't block VFS operations.

**Error logged:**
```json
{
  "level": "error",
  "message": "Event handler failed",
  "handler_name": "external-webhook",
  "event_type": "file.created",
  "error": "connection refused",
  "file_path": "/data/users/alice.json"
}
```

## Performance

- **Async execution** - Events handled in background
- **Batching** - Multiple events can be batched (future)
- **Rate limiting** - Per-handler rate limits (future)
- **Filtering** - Events filtered before handler execution

## Security

- **HMAC signatures** - Verify webhook authenticity
- **Secret rotation** - Update secrets without downtime
- **TLS required** - Webhooks must use HTTPS
- **IP allowlist** - Restrict webhook destinations (future)

## Migration from .webhook

**Old (.webhook):**
```json
{
  "url": "https://example.com/webhook",
  "events": ["file.created", "file.updated"],
  "secret": "secret"
}
```

**New (.events):**
```json
{
  "handlers": [
    {
      "name": "default-webhook",
      "events": ["file.created", "file.updated"],
      "type": "webhook",
      "config": {
        "url": "https://example.com/webhook",
        "secret": "secret"
      }
    }
  ]
}
```

**Benefit:** Can now have multiple webhooks, logs, metrics!

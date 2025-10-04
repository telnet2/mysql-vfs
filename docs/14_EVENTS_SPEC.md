# .events Special File Specification - Lifecycle Events

## Overview

The `.events` special file defines event handlers for the complete lifecycle of file and directory operations in VFS.

**Version:** 2.0 (Lifecycle Events)
**Status:** Design Complete

### Key Features

- **Full Lifecycle Tracking** - Authorization → Validation → Execution → Completion
- **Substage Events** - Granular tracking of individual checks (policy, schema, quota, etc.)
- **Authorization-First** - Security before validation (prevents information disclosure)
- **Veto Capability** - Handlers can abort operations via synchronous webhooks
- **Wildcard Patterns** - Simplified configuration (`*.*.authorization.*`)
- **Multiple Handler Types** - Webhook, log, metrics (extensible)
- **Inheritance & Merging** - Parent directory handlers combine with child handlers
- **Sync + Async Dispatch** - Critical checks synchronous, observability async

---

## Operation Lifecycle

Every operation follows this flow:

```
┌─────────────────────────────────────────────────────────┐
│ 1. AUTHORIZATION (Security First!)                     │
│    • Policy checking                                    │
│    • Permission verification                            │
│    • Role validation                                    │
│    ↓ Can VETO operation                                 │
├─────────────────────────────────────────────────────────┤
│ 2. VALIDATION (After Authorization)                     │
│    • Schema validation                                  │
│    • Quota checking                                     │
│    • Content scanning                                   │
│    • Size limits                                        │
│    ↓ Can VETO operation                                 │
├─────────────────────────────────────────────────────────┤
│ 3. EXECUTION (Actual Operation)                         │
│    • Lock acquisition                                   │
│    • Transaction start                                  │
│    • Storage write                                      │
│    • Transaction commit                                 │
├─────────────────────────────────────────────────────────┤
│ 4. COMPLETION                                           │
│    • Success or failure                                 │
│    • Rollback if needed                                 │
│    • Post-processing (cache, index, etc.)               │
└─────────────────────────────────────────────────────────┘
```

**Why Authorization First?**
- ✅ Security-first approach
- ✅ Prevents information disclosure via validation errors
- ✅ Fail fast on permission errors (higher failure rate)
- ✅ Don't waste resources on validation if user lacks access

---

## Event Naming Convention

Events use hierarchical naming: `{category}.{operation}.{stage}.{outcome}`

### Structure

```
file.create.authorization.policy.checked.succeeded
 │    │       │              │       │        │
 │    │       │              │       │        └─ Outcome (optional)
 │    │       │              │       └────────── Substage action
 │    │       │              └────────────────── Substage
 │    │       └───────────────────────────────── Lifecycle stage
 │    └───────────────────────────────────────── Operation
 └────────────────────────────────────────────── Category
```

### Categories

- `file` - File operations
- `directory` - Directory operations
- `auth` - Authentication events (future)
- `policy` - Policy changes (future)

### Operations

**File Operations:**
- `create` - Creating a new file
- `read` - Reading file content
- `update` - Updating file content
- `delete` - Deleting a file
- `move` - Moving/renaming a file
- `copy` - Copying a file

**Directory Operations:**
- `create` - Creating directory
- `list` - Listing directory contents
- `delete` - Deleting directory

### Lifecycle Stages

#### Authorization Stages
- `authorization.started`
- `authorization.policy.checking`
- `authorization.policy.checked.{succeeded|failed}`
- `authorization.permission.checking`
- `authorization.permission.checked.{succeeded|failed}`
- `authorization.role.checking`
- `authorization.role.checked.{succeeded|failed}`
- `authorization.{succeeded|failed}`

#### Validation Stages
- `validation.started`
- `validation.schema.checking`
- `validation.schema.checked.{succeeded|failed}`
- `validation.quota.checking`
- `validation.quota.checked.{succeeded|failed}`
- `validation.content.checking`
- `validation.content.checked.{succeeded|failed}`
- `validation.size.checking`
- `validation.size.checked.{succeeded|failed}`
- `validation.{succeeded|failed}`

#### Execution Stages
- `execution.started`
- `execution.lock.acquiring`
- `execution.lock.acquired.{succeeded|failed}`
- `execution.transaction.starting`
- `execution.transaction.started.succeeded`
- `execution.storage.writing`
- `execution.storage.written.{succeeded|failed}`
- `execution.transaction.committing`
- `execution.transaction.committed.{succeeded|failed}`
- `execution.{succeeded|failed}`

#### Completion Stages
- `rollback.started`
- `rollback.completed`
- `completed`
- `failed`

### Event Examples

```
file.create.started
file.create.authorization.started
file.create.authorization.policy.checking
file.create.authorization.policy.checked.succeeded
file.create.authorization.succeeded
file.create.validation.started
file.create.validation.schema.checking
file.create.validation.schema.checked.failed
file.create.validation.failed
file.create.failed

directory.create.started
directory.create.authorization.succeeded
directory.create.validation.succeeded
directory.create.execution.started
directory.create.execution.storage.written.succeeded
directory.create.completed
```

---

## Wildcard Pattern Matching

Event patterns support wildcards for matching multiple events:

### Wildcard Syntax

| Pattern | Description | Matches |
|---------|-------------|---------|
| `file.create.*` | All stages of file.create | authorization, validation, execution, completion |
| `file.*.authorization.*` | Authorization for all file ops | create, update, delete authorization events |
| `*.*.validation.failed` | All validation failures | Any operation's validation failures |
| `*.*.completed` | All successful operations | All completions across categories |
| `file.{create,update}.*` | Multiple operations | File create and update, all stages |
| `*.*.authorization.policy.*` | All policy checks | Policy checking across all operations |

### Pattern Examples

```json
{
  "handlers": [
    {
      "name": "audit-all-authorization",
      "events": ["*.*.authorization.*"],
      "type": "log"
    },
    {
      "name": "monitor-validation-failures",
      "events": ["*.*.validation.failed"],
      "type": "metrics"
    },
    {
      "name": "track-completions",
      "events": ["*.*.completed"],
      "type": "metrics"
    }
  ]
}
```

---

## Handler Configuration

### Common Handler Fields

```json
{
  "name": "handler-name",
  "events": ["event.pattern.*"],
  "type": "webhook|log|metrics",
  "enabled": true,
  "synchronous": false,
  "veto_enabled": false,
  "timeout_ms": 5000,
  "filter": { ... },
  "config": { ... }
}
```

**Field Descriptions:**

- **`name`** (string, required) - Unique handler identifier
- **`events`** (array, required) - Event patterns to handle (supports wildcards)
- **`type`** (string, required) - Handler type: `webhook`, `log`, or `metrics`
- **`enabled`** (boolean, optional, default: `true`) - Enable/disable handler
- **`synchronous`** (boolean, optional, default: `false`) - Whether handler blocks operation
- **`veto_enabled`** (boolean, optional, default: `false`) - Whether handler can abort operation
- **`timeout_ms`** (number, optional) - Timeout for synchronous handlers
- **`filter`** (object, optional) - Event filtering criteria
- **`config`** (object, required) - Handler-specific configuration

### Synchronous vs Asynchronous

**Synchronous Handlers (`synchronous: true`):**
- Block operation until handler completes
- Used for authorization and validation events
- Can veto operations (with `veto_enabled: true`)
- Typically 2-5ms overhead
- Timeout required to prevent hanging

**Asynchronous Handlers (`synchronous: false`):**
- Run in background, don't block operation
- Used for metrics, logging, completion events
- Cannot veto operations
- No overhead on critical path
- Failures don't affect operation

### Veto Capability

Handlers with `veto_enabled: true` can abort operations:

```json
{
  "name": "external-authorization",
  "events": ["file.*.authorization.started"],
  "type": "webhook",
  "synchronous": true,
  "veto_enabled": true,
  "timeout_ms": 2000,
  "config": {
    "url": "https://auth.example.com/check",
    "on_timeout": "abort",
    "on_error": "abort"
  }
}
```

**Veto Response:**

Webhook returns HTTP status:
- **2xx** - Continue operation
- **403, 401** - Abort operation (authorization denied)
- **4xx** - Abort operation (client error)
- **5xx** - Retry or abort (based on config)
- **Timeout** - Action based on `on_timeout` config

**Use Cases:**
- External authorization services
- ML-based content scanning
- Compliance checks
- Dynamic rate limiting

---

## Handler Types

### 1. Webhook Handler

Sends HTTP requests to external services with retry and circuit breaker.

```json
{
  "name": "external-webhook",
  "events": ["file.create.completed"],
  "type": "webhook",
  "synchronous": false,
  "config": {
    "url": "https://example.com/webhook",
    "method": "POST",
    "headers": {
      "X-API-Key": "secret-key"
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

**Webhook Configuration:**
- `url` - Webhook endpoint (HTTPS required)
- `method` - HTTP method (default: POST)
- `headers` - Custom HTTP headers
- `secret` - HMAC secret for signature
- `timeout_ms` - Request timeout
- `retry` - Retry configuration
- `circuit_breaker` - Circuit breaker configuration
- `on_timeout` - Action on timeout: `continue`, `abort`
- `on_error` - Action on error: `continue`, `abort`

**HMAC Signature:**
```
Header: X-VFS-Signature: sha256=<hex_digest>
Computed: HMAC-SHA256(payload_json, secret)
```

**Retry Backoff:**
- **Exponential:** 1s, 2s, 4s, 8s, 16s, ...
- **Linear:** 1s, 2s, 3s, 4s, 5s, ...

**Circuit Breaker States:**
- **Closed** - Normal operation
- **Open** - Failing, reject requests immediately
- **Half-Open** - Testing if service recovered

### 2. Log Handler

Structured logging with template support.

```json
{
  "name": "audit-logger",
  "events": ["*.*.authorization.*", "*.*.validation.failed"],
  "type": "log",
  "synchronous": false,
  "config": {
    "level": "info",
    "message": "{{event.category}}.{{event.operation}}.{{event.stage}}: user={{user.user_id}} outcome={{event.outcome}} duration={{stage.duration_ms}}ms"
  }
}
```

**Log Levels:** `debug`, `info`, `warn`, `error`

**Template Variables:**
- `{{event.category}}` - Event category (file, directory)
- `{{event.operation}}` - Operation (create, update, delete)
- `{{event.stage}}` - Lifecycle stage
- `{{event.outcome}}` - Outcome (succeeded, failed)
- `{{event.type}}` - Full event type string
- `{{resource.path}}` - Resource path
- `{{resource.name}}` - Resource name
- `{{resource.size_bytes}}` - File size
- `{{user.user_id}}` - User ID
- `{{user.role}}` - User role
- `{{user.groups}}` - User groups (array)
- `{{stage.duration_ms}}` - Stage duration in milliseconds
- `{{error.message}}` - Error message (if failed)

### 3. Metrics Handler

Emit metrics in Prometheus/StatsD compatible format.

```json
{
  "name": "performance-metrics",
  "events": ["*.*.*.checked.*"],
  "type": "metrics",
  "synchronous": false,
  "config": {
    "metric_name": "vfs.stage.duration",
    "tags": {
      "operation": "{{event.operation}}",
      "stage": "{{event.stage}}",
      "outcome": "{{event.outcome}}"
    },
    "value_field": "stage.duration_ms"
  }
}
```

**Metrics Configuration:**
- `metric_name` - Name of the metric
- `tags` - Key-value tags (support templates)
- `value_field` - Field to use as metric value (default: 1 for counting)

**Output Format:**
```
METRIC vfs.stage.duration{operation="create",stage="validation.schema",outcome="succeeded"} 120
```

---

## Event Payloads

### Base Event Structure

All events include:

```json
{
  "event": {
    "id": "evt_abc123",
    "category": "file",
    "operation": "create",
    "stage": "authorization.policy.checked",
    "outcome": "succeeded",
    "timestamp": "2025-10-04T12:00:00Z",
    "directory_path": "/data/users",
    "operation_id": "op_xyz789",
    "correlation_id": "corr_456"
  },
  "resource": { ... },
  "user": { ... },
  "metadata": { ... },
  "stage_info": { ... }
}
```

### File Event Payload

```json
{
  "event": {
    "id": "evt_001",
    "category": "file",
    "operation": "create",
    "stage": "validation.schema.checked",
    "outcome": "succeeded",
    "timestamp": "2025-10-04T12:00:00Z",
    "directory_path": "/data/users",
    "operation_id": "op_123"
  },
  "resource": {
    "type": "file",
    "id": "file_456",
    "name": "alice.json",
    "path": "/data/users/alice.json",
    "size_bytes": 1024,
    "content_type": "application/json",
    "version": 1,
    "checksum_sha256": "abc...",
    "created_at": "2025-10-04T12:00:00Z"
  },
  "user": {
    "user_id": "alice",
    "role": "admin",
    "groups": ["engineering", "admins"]
  },
  "metadata": {
    "request_id": "req_789",
    "ip_address": "192.168.1.1",
    "user_agent": "vfs-cli/1.0"
  },
  "stage_info": {
    "stage": "validation.schema",
    "duration_ms": 120,
    "started_at": "2025-10-04T12:00:00.000Z",
    "completed_at": "2025-10-04T12:00:00.120Z"
  }
}
```

### Authorization Event Payload

```json
{
  "event": { ... },
  "resource": { ... },
  "user": { ... },
  "authorization": {
    "policy_name": "file-write-policy",
    "policy_version": "v1.0",
    "decision": "allow",
    "reason": "User has admin role",
    "evaluation_time_ms": 15
  }
}
```

### Validation Event Payload

```json
{
  "event": { ... },
  "resource": { ... },
  "user": { ... },
  "validation": {
    "type": "schema",
    "rules": ["email_required", "name_pattern"],
    "violations": [
      {
        "field": "email",
        "message": "Email is required",
        "rule": "email_required"
      }
    ],
    "validation_time_ms": 45
  }
}
```

---

## Complete Examples

### Example 1: External Authorization with Veto

```json
{
  "handlers": [
    {
      "name": "corporate-policy-check",
      "events": ["file.*.authorization.started"],
      "type": "webhook",
      "synchronous": true,
      "veto_enabled": true,
      "timeout_ms": 2000,
      "config": {
        "url": "https://policy.corp.com/authorize",
        "secret": "policy-hmac-secret",
        "headers": {
          "X-Service": "VFS",
          "X-Environment": "production"
        },
        "on_timeout": "abort",
        "on_error": "abort"
      }
    }
  ]
}
```

**Behavior:**
- Blocks ALL file operations during authorization
- Sends request to corporate policy service
- If webhook returns 403 → Operation aborted
- If timeout (>2s) → Operation aborted
- If webhook returns 200 → Operation continues

### Example 2: ML Content Scanning

```json
{
  "handlers": [
    {
      "name": "malware-scanner",
      "events": ["file.*.validation.content.checking"],
      "type": "webhook",
      "synchronous": true,
      "veto_enabled": true,
      "timeout_ms": 5000,
      "filter": {
        "pattern": "*.{exe,pdf,zip,jpg,png}",
        "type": "glob",
        "min_size_bytes": 1
      },
      "config": {
        "url": "https://scanner.example.com/scan",
        "secret": "scanner-secret",
        "on_timeout": "continue",
        "on_error": "continue"
      }
    }
  ]
}
```

**Behavior:**
- Scans executables, PDFs, images during validation
- Synchronous - blocks until scan completes
- If malicious → Scanner returns 403 → File rejected
- If timeout → Continue (fail-open for availability)

### Example 3: Comprehensive Observability

```json
{
  "handlers": [
    {
      "name": "authorization-audit",
      "events": ["*.*.authorization.*"],
      "type": "log",
      "synchronous": false,
      "config": {
        "level": "info",
        "message": "AUTH: {{event.operation}} user={{user.user_id}} stage={{event.stage}} outcome={{event.outcome}}"
      }
    },
    {
      "name": "validation-failures",
      "events": ["*.*.validation.failed"],
      "type": "webhook",
      "synchronous": false,
      "config": {
        "url": "https://alerts.example.com/validation-failed",
        "secret": "alert-secret"
      }
    },
    {
      "name": "stage-performance",
      "events": [
        "*.*.authorization.*.checked.*",
        "*.*.validation.*.checked.*",
        "*.*.execution.*.written.*"
      ],
      "type": "metrics",
      "synchronous": false,
      "config": {
        "metric_name": "vfs.stage.duration",
        "tags": {
          "operation": "{{event.operation}}",
          "stage": "{{event.stage}}",
          "outcome": "{{event.outcome}}"
        },
        "value_field": "stage.duration_ms"
      }
    },
    {
      "name": "operation-summary",
      "events": ["*.*.completed", "*.*.failed"],
      "type": "log",
      "synchronous": false,
      "config": {
        "level": "info",
        "message": "OPERATION {{event.outcome}}: {{event.category}}.{{event.operation}} user={{user.user_id}} path={{resource.path}}"
      }
    }
  ]
}
```

**Behavior:**
- Audit all authorization events (async log)
- Alert on validation failures (async webhook)
- Track performance of all substages (async metrics)
- Log final outcome of all operations (async log)

### Example 4: Performance Monitoring

```json
{
  "handlers": [
    {
      "name": "slow-validation-alert",
      "events": ["*.*.validation.*.checked.succeeded"],
      "type": "webhook",
      "synchronous": false,
      "config": {
        "url": "https://alerts.example.com/slow-validation",
        "secret": "alert-secret"
      }
    },
    {
      "name": "track-all-stages",
      "events": ["*.*.*"],
      "type": "metrics",
      "synchronous": false,
      "config": {
        "metric_name": "vfs.events.total",
        "tags": {
          "category": "{{event.category}}",
          "operation": "{{event.operation}}",
          "stage": "{{event.stage}}"
        }
      }
    }
  ]
}
```

---

## Filter Object

Filters restrict which events trigger handlers (in addition to event pattern matching):

```json
{
  "filter": {
    "pattern": "*.json",
    "type": "glob",
    "min_size_bytes": 1024,
    "max_size_bytes": 10485760,
    "content_types": ["application/json", "text/plain"]
  }
}
```

**Filter Fields:**
- `pattern` - Filename pattern
- `type` - Pattern type: `glob` or `regex`
- `min_size_bytes` - Minimum file size
- `max_size_bytes` - Maximum file size
- `content_types` - Allowed content types

---

## Inheritance

`.events` files inherit from parent directories with merging:

```
/
├── .events (handlers: ["global-auth", "global-metrics"])
└── data/
    ├── .events (handlers: ["data-webhook", "global-auth": disabled])
    └── users/
        └── alice.json
```

**Merging Rules:**
1. Parent and child handlers are combined
2. Child handler with same `name` overrides parent
3. Child handler with `enabled: false` removes parent handler

**Example:**

Parent (`/`):
```json
{
  "handlers": [
    {"name": "global-auth", "events": ["*.*.authorization.*"], "type": "log"},
    {"name": "global-metrics", "events": ["*.*.completed"], "type": "metrics"}
  ]
}
```

Child (`/data`):
```json
{
  "handlers": [
    {"name": "data-webhook", "events": ["file.create.completed"], "type": "webhook"},
    {"name": "global-auth", "enabled": false}
  ]
}
```

**Result for `/data/users/alice.json`:**
- `global-metrics` (from parent)
- `data-webhook` (from child)
- `global-auth` DISABLED (child overrides)

---

## Performance Considerations

### Synchronous Handler Overhead

| Handler Type | Typical Latency | When to Use |
|-------------|-----------------|-------------|
| Authorization webhook | 10-50ms | Critical security checks |
| Validation webhook | 50-200ms | Content scanning, compliance |
| Policy check (local) | 1-5ms | Always enabled |
| Schema validation (local) | 5-20ms | Always enabled |

**Recommendation:** Keep synchronous handlers <100ms total overhead.

### Async Handler Impact

Async handlers have ~0ms impact on operation latency:
- Metrics collection
- Audit logging
- Completion notifications

### Best Practices

1. **Use wildcards** to reduce handler count
2. **Async for observability** (logs, metrics)
3. **Sync only for critical checks** (auth, malware scanning)
4. **Set timeouts** on all synchronous handlers
5. **Enable circuit breakers** for external services

---

## Migration from Basic Events

**Old (Phase 3 - Basic Events):**
```json
{
  "handlers": [
    {
      "name": "notify",
      "events": ["file.created", "file.updated"],
      "type": "webhook"
    }
  ]
}
```

**New (Phase 5 - Lifecycle Events):**
```json
{
  "handlers": [
    {
      "name": "notify",
      "events": ["file.{create,update}.completed"],
      "type": "webhook",
      "synchronous": false
    }
  ]
}
```

**Event Name Mapping:**

| Old Event | New Event |
|-----------|-----------|
| `file.created` | `file.create.completed` |
| `file.updated` | `file.update.completed` |
| `file.deleted` | `file.delete.completed` |
| `file.moved` | `file.move.completed` |
| `directory.created` | `directory.create.completed` |
| `directory.deleted` | `directory.delete.completed` |

**New Capabilities:**
- Monitor authorization: `file.create.authorization.*`
- Track validation: `file.create.validation.*`
- Watch specific checks: `file.create.validation.schema.checked.*`
- Alert on failures: `*.*.*.failed`

---

## Security

### HMAC Signatures

All webhook payloads include HMAC signature:

```
X-VFS-Signature: sha256=<hex_digest>
```

**Verification (Python):**
```python
import hmac
import hashlib

def verify_signature(payload, signature, secret):
    expected = hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

### TLS Required

All webhook URLs must use HTTPS in production.

### Secret Rotation

Update secrets without downtime:
1. Add new handler with new secret
2. Verify new handler works
3. Remove old handler

---

## Error Handling

### Webhook Failures

**Transient Errors** (network, timeout, 5xx):
- Retry with backoff
- Open circuit breaker after threshold

**Permanent Errors** (4xx, invalid config):
- Log error, don't retry
- Disable handler if misconfigured

### Veto Failures

If veto-enabled handler fails:
- Timeout → Action based on `on_timeout`
- Error → Action based on `on_error`
- Default: `abort` (fail-secure)

### Handler Errors Don't Block Async Operations

Async handler failures are logged but don't affect operation success.

---

## Summary

### Key Concepts

1. **Lifecycle Events** - Track full operation flow
2. **Authorization First** - Security before validation
3. **Substage Granularity** - Identify exact bottlenecks
4. **Veto Capability** - External systems can block operations
5. **Wildcard Patterns** - Simplified configuration
6. **Sync + Async** - Performance with control

### Quick Reference

```json
{
  "handlers": [
    {
      "name": "external-auth",
      "events": ["file.*.authorization.started"],
      "type": "webhook",
      "synchronous": true,
      "veto_enabled": true,
      "timeout_ms": 2000,
      "config": {...}
    },
    {
      "name": "audit-all",
      "events": ["*.*.*"],
      "type": "log",
      "synchronous": false,
      "config": {...}
    },
    {
      "name": "performance",
      "events": ["*.*.*.checked.*"],
      "type": "metrics",
      "synchronous": false,
      "config": {...}
    }
  ]
}
```

---

**Version History:**
- v2.0 (2025-10-04) - Lifecycle events, authorization-first, veto capability
- v1.0 (2025-10-03) - Basic outcome events (file.created, etc.)

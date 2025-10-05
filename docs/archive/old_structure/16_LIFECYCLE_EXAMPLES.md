# 16. Lifecycle Events - User Guide with Examples

**Status:** ✅ Complete (Production Ready)

[← Back: Lifecycle Events](15_LIFECYCLE_EVENTS.md) | [Index](0_README.md) | [Next: Webhooks →](17_WEBHOOKS.md)

---

## Introduction

This guide provides practical examples of using MySQL VFS v2.1+ lifecycle events for common use cases. Each example includes complete `.events` configuration and expected behavior.

**Implementation:** Events are configured via `.events` special files (loader: `pkg/domain/events_loader.go`)

## Use Case 1: Audit Logging

### Requirement
Log all file operations for compliance

### Solution

Create `/audit/.events`:

```json
{
  "handlers": [
    {
      "name": "audit-all-operations",
      "type": "log",
      "pattern": "file.*.completion.*",
      "config": {
        "level": "info",
        "message": "[AUDIT] {{event.operation}} {{resource.path}} by {{user.user_id}} - {{event.outcome}}"
      }
    }
  ]
}
```

### Output
```
[INFO] audit-all-operations: [AUDIT] create /audit/report.pdf by alice - succeeded
[INFO] audit-all-operations: [AUDIT] update /audit/report.pdf by bob - succeeded
[INFO] audit-all-operations: [AUDIT] delete /audit/old-data.csv by alice - succeeded
```

### Advanced: Separate Log Files by Operation

```json
{
  "handlers": [
    {
      "name": "audit-creates",
      "type": "log",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "level": "info",
        "message": "CREATED: {{resource.path}} by {{user.user_id}} at {{event.timestamp}}"
      }
    },
    {
      "name": "audit-deletes",
      "type": "log",
      "pattern": "file.delete.completion.succeeded",
      "config": {
        "level": "warn",
        "message": "DELETED: {{resource.path}} by {{user.user_id}} at {{event.timestamp}}"
      }
    }
  ]
}
```

## Use Case 2: Metrics Collection

### Requirement
Track file operation metrics for monitoring dashboard

### Solution

Create `/.events` (applies to all directories):

```json
{
  "handlers": [
    {
      "name": "operation-counter",
      "type": "metrics",
      "pattern": "file.*.completion.succeeded",
      "config": {
        "metric_name": "vfs_operations_total",
        "tags": {
          "operation": "{{event.operation}}",
          "category": "{{event.category}}",
          "user": "{{user.user_id}}"
        }
      }
    },
    {
      "name": "operation-failures",
      "type": "metrics",
      "pattern": "file.*.completion.failed",
      "config": {
        "metric_name": "vfs_operations_failed",
        "tags": {
          "operation": "{{event.operation}}",
          "error_type": "{{error.type}}"
        }
      }
    },
    {
      "name": "file-size-gauge",
      "type": "metrics",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "metric_name": "vfs_file_size_bytes",
        "tags": {
          "path": "{{resource.path}}"
        },
        "value_field": "resource.size"
      }
    }
  ]
}
```

### Metrics Output
```
METRIC vfs_operations_total{operation="create",category="file",user="alice"} 1.000000
METRIC vfs_operations_total{operation="update",category="file",user="bob"} 1.000000
METRIC vfs_operations_failed{operation="delete",error_type="permission_denied"} 1.000000
METRIC vfs_file_size_bytes{path="/data/report.pdf"} 2048576.000000
```

## Use Case 3: Slack Notifications

### Requirement
Notify team in Slack when important files are created/modified

### Solution

Create `/important-files/.events`:

```json
{
  "handlers": [
    {
      "name": "slack-notification",
      "type": "webhook",
      "pattern": "file.{create,update}.completion.succeeded",
      "config": {
        "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        },
        "body_template": {
          "text": "File {{event.operation}}d: `{{resource.path}}` by {{user.user_id}}",
          "channel": "#file-updates"
        },
        "timeout_ms": 3000
      }
    }
  ]
}
```

### Slack Message
```
File created: `/important-files/quarterly-report.xlsx` by alice
```

## Use Case 4: Approval Workflow

### Requirement
Require approval from external system before allowing file deletions

### Solution

Create `/protected/.events`:

```json
{
  "handlers": [
    {
      "name": "deletion-approval",
      "type": "webhook",
      "pattern": "file.delete.validation.succeeded",
      "config": {
        "url": "https://approval.company.com/api/check-deletion",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer ${APPROVAL_TOKEN}",
          "Content-Type": "application/json"
        },
        "veto_enabled": true,
        "veto_on_4xx": true,
        "timeout_ms": 5000,
        "max_retries": 1,
        "on_error": "abort"
      }
    }
  ]
}
```

### Webhook Request Payload
```json
{
  "event": {
    "type": "file.delete.validation.succeeded",
    "operation_id": "op_123456"
  },
  "resource": {
    "path": "/protected/sensitive-data.csv",
    "id": "file_789"
  },
  "user": {
    "user_id": "bob"
  }
}
```

### Webhook Responses

**Approve Deletion** (200 OK):
```json
{
  "approved": true
}
```

**Deny Deletion** (403 Forbidden):
```json
{
  "veto": true,
  "message": "Deletion requires manager approval",
  "code": "APPROVAL_REQUIRED"
}
```

## Use Case 5: Size Quota Enforcement

### Requirement
Prevent files larger than 10MB from being uploaded to specific directory

### Solution

Create `/uploads/.events`:

```json
{
  "handlers": [
    {
      "name": "size-check",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://quota-service.company.com/check-size",
        "method": "POST",
        "veto_enabled": true,
        "veto_on_4xx": true,
        "timeout_ms": 2000
      }
    }
  ]
}
```

### Quota Service Response

If file > 10MB, return 403:
```json
{
  "veto": true,
  "message": "File size 15MB exceeds limit of 10MB",
  "code": "SIZE_LIMIT_EXCEEDED"
}
```

## Use Case 6: Multi-Environment Setup

### Requirement
Different event handling for dev, staging, and production

### Solution

**Development** (`/dev/.events`):
```json
{
  "handlers": [
    {
      "name": "dev-logger",
      "type": "log",
      "pattern": "*.*.*.*",
      "config": {
        "level": "debug",
        "message": "[DEV] {{event.type}} - {{resource.path}}"
      }
    }
  ]
}
```

**Staging** (`/staging/.events`):
```json
{
  "handlers": [
    {
      "name": "staging-logger",
      "type": "log",
      "pattern": "file.*.completion.*",
      "config": {
        "level": "info",
        "message": "[STAGING] {{event.operation}} {{resource.path}}"
      }
    },
    {
      "name": "staging-metrics",
      "type": "metrics",
      "pattern": "file.*.completion.succeeded",
      "config": {
        "metric_name": "staging_ops_total",
        "tags": {
          "operation": "{{event.operation}}"
        }
      }
    }
  ]
}
```

**Production** (`/production/.events`):
```json
{
  "handlers": [
    {
      "name": "prod-audit",
      "type": "log",
      "pattern": "file.*.completion.*",
      "config": {
        "level": "info",
        "message": "[PROD] {{event.operation}} {{resource.path}} by {{user.user_id}}"
      }
    },
    {
      "name": "prod-metrics",
      "type": "metrics",
      "pattern": "file.*.completion.*",
      "config": {
        "metric_name": "prod_ops",
        "tags": {
          "operation": "{{event.operation}}",
          "outcome": "{{event.outcome}}"
        }
      }
    },
    {
      "name": "prod-alerts",
      "type": "webhook",
      "pattern": "file.*.completion.failed",
      "config": {
        "url": "https://alerts.company.com/api/incident",
        "method": "POST",
        "timeout_ms": 3000
      }
    }
  ]
}
```

## Use Case 7: Data Loss Prevention

### Requirement
Scan files for sensitive data before allowing creation

### Solution

Create `/customer-data/.events`:

```json
{
  "handlers": [
    {
      "name": "dlp-scan",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://dlp-scanner.company.com/scan",
        "method": "POST",
        "headers": {
          "X-API-Key": "${DLP_API_KEY}"
        },
        "veto_enabled": true,
        "veto_on_4xx": true,
        "timeout_ms": 10000,
        "max_retries": 0,
        "on_error": "abort"
      }
    }
  ]
}
```

### DLP Scanner Response

**If PII Detected** (403 Forbidden):
```json
{
  "veto": true,
  "message": "File contains unencrypted SSN",
  "code": "PII_DETECTED",
  "details": {
    "violations": ["SSN", "Credit Card"]
  }
}
```

## Use Case 8: Event Chaining

### Requirement
Trigger multiple actions when file is created

### Solution

Create `/data/.events`:

```json
{
  "handlers": [
    {
      "name": "log-creation",
      "type": "log",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "level": "info",
        "message": "New file: {{resource.path}}"
      }
    },
    {
      "name": "update-metrics",
      "type": "metrics",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "metric_name": "files_created_total",
        "tags": {
          "directory": "{{resource.directory}}"
        }
      }
    },
    {
      "name": "notify-processing-service",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://processor.company.com/api/process",
        "method": "POST",
        "timeout_ms": 1000
      }
    },
    {
      "name": "update-index",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://search.company.com/api/index",
        "method": "POST",
        "timeout_ms": 2000
      }
    }
  ]
}
```

All handlers execute in parallel for the same event.

## Use Case 9: Gradual Rollout

### Requirement
Test new validation rules on subset of traffic

### Solution

**Option 1: Directory-based rollout**

Create `/beta-users/.events` with strict rules:
```json
{
  "handlers": [
    {
      "name": "strict-validation",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://validator.company.com/strict",
        "veto_enabled": true
      }
    }
  ]
}
```

Regular users use `/users/.events` with standard rules.

**Option 2: Conditional via webhook**

The webhook service can implement sampling:

```javascript
// Webhook service
app.post('/validate', (req, res) => {
  const userId = req.body.user.user_id;

  // 10% rollout based on user ID hash
  if (hashCode(userId) % 10 === 0) {
    // Apply new strict rules
    if (!validateStrict(req.body)) {
      return res.status(403).json({
        veto: true,
        message: "Strict validation failed"
      });
    }
  }

  res.json({ approved: true });
});
```

## Use Case 10: Error Recovery

### Requirement
Retry webhook on transient failures

### Solution

Create `/.events`:

```json
{
  "handlers": [
    {
      "name": "resilient-webhook",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://unreliable-service.com/api/notify",
        "method": "POST",
        "timeout_ms": 5000,
        "max_retries": 3,
        "retry_delay_ms": 1000,
        "retry_backoff": "exponential",
        "circuit_breaker": {
          "threshold": 5,
          "timeout_seconds": 60
        },
        "on_error": "allow"
      }
    }
  ]
}
```

### Retry Behavior

- **Attempt 1**: Immediate
- **Attempt 2**: After 1000ms
- **Attempt 3**: After 2000ms (exponential backoff)
- **Attempt 4**: After 4000ms

After 5 consecutive failures, circuit opens for 60 seconds.

## Common Patterns

### Pattern: Log Everything, Veto Selectively

```json
{
  "handlers": [
    {
      "name": "audit-log",
      "type": "log",
      "pattern": "*.*.*.*",
      "config": {
        "level": "info",
        "message": "{{event.type}}"
      }
    },
    {
      "name": "deletion-veto",
      "type": "webhook",
      "pattern": "file.delete.validation.succeeded",
      "config": {
        "url": "https://approval.company.com/check",
        "veto_enabled": true
      }
    }
  ]
}
```

### Pattern: Async Notifications, Sync Approvals

```json
{
  "handlers": [
    {
      "name": "async-slack",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://hooks.slack.com/...",
        "dispatch_mode": "async"
      }
    },
    {
      "name": "sync-approval",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://approval.company.com/check",
        "veto_enabled": true,
        "dispatch_mode": "sync"
      }
    }
  ]
}
```

### Pattern: Metrics + Alerts

```json
{
  "handlers": [
    {
      "name": "success-metrics",
      "type": "metrics",
      "pattern": "file.*.completion.succeeded",
      "config": {
        "metric_name": "ops_success_total"
      }
    },
    {
      "name": "failure-alert",
      "type": "webhook",
      "pattern": "file.*.completion.failed",
      "config": {
        "url": "https://pagerduty.com/api/incidents"
      }
    }
  ]
}
```

## Testing Your Configuration

### 1. Start with Logging

```json
{
  "handlers": [
    {
      "name": "debug-all",
      "type": "log",
      "pattern": "*.*.*.*",
      "config": {
        "level": "debug",
        "message": "EVENT: {{event.type}}"
      }
    }
  ]
}
```

Create a file and check logs to see all emitted events.

### 2. Test Webhook Locally

Use [webhook.site](https://webhook.site) for testing:

```json
{
  "handlers": [
    {
      "name": "test-webhook",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://webhook.site/your-unique-url"
      }
    }
  ]
}
```

### 3. Verify Veto Logic

```json
{
  "handlers": [
    {
      "name": "always-veto",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://httpstat.us/403",
        "veto_enabled": true,
        "veto_on_4xx": true
      }
    }
  ]
}
```

File creation should be blocked.

## Troubleshooting

### Events Not Triggering

1. Check pattern matches event type
2. Verify `.events` file is valid JSON
3. Check logs for errors
4. Ensure pattern uses correct wildcards

### Webhook Timeouts

1. Increase `timeout_ms`
2. Use async mode for non-critical webhooks
3. Reduce `max_retries`
4. Optimize webhook endpoint

### Veto Not Working

1. Ensure `veto_enabled: true`
2. Check webhook returns 4xx/5xx (if configured)
3. Verify pattern matches validation/authorization stage
4. Check `on_error` setting

## Current Event Format

**Note:** The examples above show the Phase 5 event format. MySQL VFS v2.1+ currently implements simpler event types:

**Current Events** (from `pkg/events/types.go`):
```go
const (
    EventFileCreated EventType = "file.created"
    EventFileUpdated EventType = "file.updated"
    EventFileDeleted EventType = "file.deleted"
    EventFileMoved   EventType = "file.moved"

    EventDirectoryCreated EventType = "directory.created"
    EventDirectoryDeleted EventType = "directory.deleted"
)
```

**Pattern Matching:**
```json
{
  "handlers": [
    {
      "pattern": "file.*",
      "pattern": "file.created",
      "pattern": "directory.*"
    }
  ]
}
```

The granular lifecycle stages (authorization.started, validation.succeeded, etc.) from the examples above represent the planned future implementation.

---

[← Back: Lifecycle Events](15_LIFECYCLE_EVENTS.md) | [Index](0_README.md) | [Next: Webhooks →](17_WEBHOOKS.md)

## See Also

- [Implementation Guide](15_LIFECYCLE_EVENTS.md)
- [Webhook Integration](17_WEBHOOKS.md)
- [API Reference](10_API.md)

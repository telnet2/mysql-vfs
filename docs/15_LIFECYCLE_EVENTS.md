# Lifecycle Events - Implementation Guide

## Overview

The MySQL VFS v2.1 lifecycle event system provides fine-grained observability and control over every operation. Events are emitted at different stages of an operation's lifecycle, allowing you to:

- **Observe** operations in real-time
- **Veto** operations before they complete
- **Audit** all filesystem activity
- **Integrate** with external systems via webhooks
- **Collect metrics** for monitoring

## Event Lifecycle Stages

Every operation goes through a series of lifecycle stages:

### 1. Authorization Stage
- **Purpose**: Check permissions and policies
- **Veto Allowed**: ✅ Yes
- **Examples**:
  - `file.create.authorization.started`
  - `file.create.authorization.succeeded`
  - `file.create.authorization.failed`

### 2. Validation Stage
- **Purpose**: Validate schemas, quotas, and content
- **Veto Allowed**: ✅ Yes
- **Examples**:
  - `file.create.validation.schema.checking`
  - `file.create.validation.quota.checking`
  - `file.create.validation.succeeded`

### 3. Execution Stage
- **Purpose**: Execute the actual operation
- **Veto Allowed**: ⚠️ Limited (before lock acquisition)
- **Examples**:
  - `file.create.execution.lock.acquiring`
  - `file.create.execution.storage.writing`
  - `file.create.execution.succeeded`

### 4. Completion Stage
- **Purpose**: Operation completed or failed
- **Veto Allowed**: ❌ No (operation already completed)
- **Examples**:
  - `file.create.completion.succeeded`
  - `file.create.completion.failed`

## Event Structure

Each event includes comprehensive context:

```go
type EventContext struct {
    // Event identification
    ID            string
    Type          string
    Timestamp     time.Time

    // Operation context
    OperationID   string
    Category      string  // "file" or "directory"
    Operation     string  // "create", "update", "delete", etc.
    Stage         string  // "authorization", "validation", etc.

    // Resource information
    ResourceType  string
    ResourceID    string
    ResourcePath  string

    // User context
    UserID        string
    RequestID     string

    // Metadata
    Metadata      map[string]interface{}
}
```

## Configuration

### 1. Enable Lifecycle Events

Lifecycle events are configured via the `.events` special file:

```json
{
  "handlers": [
    {
      "name": "audit-log",
      "type": "log",
      "pattern": "file.*.completion.*",
      "config": {
        "level": "info",
        "message": "{{event.type}} on {{resource.path}} by {{user.user_id}}"
      }
    },
    {
      "name": "webhook-approval",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://approval.company.com/api/check",
        "method": "POST",
        "veto_enabled": true,
        "veto_on_4xx": true
      }
    }
  ]
}
```

### 2. Wire Up Event Trigger

In your service initialization:

```go
import (
    "github.com/telnet2/mysql-vfs/pkg/domain"
    "github.com/telnet2/mysql-vfs/pkg/events"
    "github.com/telnet2/mysql-vfs/pkg/services"
)

// Create event trigger
eventTrigger := domain.NewLifecycleEventTrigger(
    eventRepo,
    filesLoader,
    domain.WithMaxConcurrentEvents(100),
)

// Create services with lifecycle support
fileService := services.NewFileServiceWithLifecycle(
    db,
    storage,
    filesLoader,
    eventTrigger,
)

dirService := services.NewDirectoryServiceWithLifecycle(
    db,
    eventTrigger,
)
```

## Handler Types

### Log Handler

Logs events with template support:

```json
{
  "name": "file-operations",
  "type": "log",
  "pattern": "file.*.*.*",
  "config": {
    "level": "info",
    "message": "{{event.type}}: {{resource.path}} ({{user.user_id}})"
  }
}
```

**Template Variables**:
- `{{event.type}}` - Full event type
- `{{event.category}}` - "file" or "directory"
- `{{event.operation}}` - "create", "update", etc.
- `{{resource.path}}` - Resource path
- `{{resource.id}}` - Resource ID
- `{{user.user_id}}` - User ID

### Metrics Handler

Emits metrics in Prometheus/StatsD format:

```json
{
  "name": "file-operations-counter",
  "type": "metrics",
  "pattern": "file.create.completion.succeeded",
  "config": {
    "metric_name": "vfs_file_operations_total",
    "tags": {
      "operation": "{{event.operation}}",
      "category": "{{event.category}}"
    },
    "value_field": ""
  }
}
```

**Metric Types**:
- **Counter** (value_field empty): Increments by 1
- **Gauge** (value_field specified): Uses extracted value

### Webhook Handler

Calls external HTTP endpoints:

```json
{
  "name": "approval-webhook",
  "type": "webhook",
  "pattern": "file.create.validation.succeeded",
  "config": {
    "url": "https://api.company.com/approve",
    "method": "POST",
    "headers": {
      "Authorization": "Bearer ${WEBHOOK_TOKEN}"
    },
    "veto_enabled": true,
    "veto_on_4xx": true,
    "veto_on_5xx": false,
    "timeout_ms": 5000,
    "max_retries": 3
  }
}
```

## Veto Mechanism

Handlers can veto operations by returning:

1. **HTTP Status Codes** (webhook):
   - 401, 403: Veto (if `veto_on_4xx: true`)
   - 500+: No veto (unless `veto_on_5xx: true`)

2. **JSON Response** (webhook):
```json
{
  "veto": true,
  "message": "File exceeds size limit",
  "code": "FILE_TOO_LARGE"
}
```

3. **Handler Response** (programmatic):
```go
return events.VetoResponse("Operation not allowed", "PERMISSION_DENIED")
```

## Pattern Matching

Event patterns support wildcards:

```
file.*                     # All file events
file.create.*             # All file creation events
file.*.validation.*       # All validation stages
*.*.completion.succeeded  # All successful completions
file.{create,update}.*    # File create or update events
```

## Best Practices

### 1. Use Specific Patterns

❌ **Bad**: `*` (matches everything, including system events)

✅ **Good**: `file.*.completion.*` (specific to file operations)

### 2. Enable Veto Only When Needed

```json
{
  "name": "approval-check",
  "pattern": "file.create.validation.succeeded",
  "config": {
    "veto_enabled": true  // Only enable for validation/authorization
  }
}
```

### 3. Set Appropriate Timeouts

```json
{
  "config": {
    "timeout_ms": 2000,  // Fast webhooks
    "max_retries": 1     // Limit retries for sync operations
  }
}
```

### 4. Use Async for Completion Events

```json
{
  "pattern": "*.*.completion.*",
  "config": {
    "dispatch_mode": "async"  // Don't block on completion
  }
}
```

### 5. Add Context with Metadata

```go
metadata := map[string]interface{}{
    "request_id": requestID,
    "source_ip":  clientIP,
    "user_agent": userAgent,
}

eventTrigger.EmitSync(ctx, eventType, resourceContext, metadata)
```

## Event Ordering

Events are emitted in strict order:

1. `authorization.started`
2. `authorization.succeeded` OR `authorization.failed`
3. `validation.started`
4. `validation.schema.checking`
5. `validation.schema.succeeded`
6. `validation.succeeded` OR `validation.failed`
7. `execution.started`
8. `execution.lock.acquiring`
9. `execution.storage.writing`
10. `execution.succeeded` OR `execution.failed`
11. `completion.succeeded` OR `completion.failed`

## Performance Considerations

### Synchronous vs Asynchronous

**Synchronous** (default for validation/authorization):
- Blocks operation until handler completes
- Required for veto support
- Adds latency to operation

**Asynchronous** (recommended for logging/metrics):
- Non-blocking
- No veto support
- Better performance

### Concurrency Control

```go
eventTrigger := domain.NewLifecycleEventTrigger(
    eventRepo,
    filesLoader,
    domain.WithMaxConcurrentEvents(50),  // Limit concurrent handlers
)
```

### Circuit Breaker

Webhook handler includes automatic circuit breaking:

```json
{
  "config": {
    "circuit_breaker": {
      "threshold": 5,        // Failures before opening
      "timeout_seconds": 60  // Time before retry
    }
  }
}
```

## Error Handling

### On Handler Error

```json
{
  "config": {
    "on_error": "allow"  // or "abort"
  }
}
```

- **`allow`**: Continue operation if handler fails
- **`abort`**: Veto operation on handler error

### Retry Logic

```json
{
  "config": {
    "max_retries": 3,
    "retry_delay_ms": 1000,
    "retry_backoff": "exponential"  // or "linear"
  }
}
```

## Testing

### Unit Tests

Test individual handlers:

```go
func TestMyHandler(t *testing.T) {
    handler := &MyHandler{}

    eventHandler := &events.EventHandler{
        Name: "test",
        Type: "custom",
        Config: map[string]interface{}{
            "key": "value",
        },
    }

    payload := struct{
        Event events.Event
    }{
        Event: events.Event{Type: "file.created"},
    }

    resp := handler.Handle(context.Background(), eventHandler, payload)

    assert.True(t, resp.Success)
    assert.False(t, resp.Veto)
}
```

### Integration Tests

Test full lifecycle:

```go
func TestFileCreateWithVeto(t *testing.T) {
    // Setup mock webhook that returns 403
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusForbidden)
    }))
    defer server.Close()

    // Create .events file with webhook veto
    eventsConfig := fmt.Sprintf(`{
        "handlers": [{
            "name": "veto-test",
            "type": "webhook",
            "pattern": "file.create.validation.succeeded",
            "config": {
                "url": "%s",
                "veto_enabled": true,
                "veto_on_4xx": true
            }
        }]
    }`, server.URL)

    // Attempt file creation - should be vetoed
    _, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 100, reader)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "operation vetoed")
}
```

## Monitoring

### Key Metrics

Monitor these metrics:

- `vfs_events_emitted_total` - Total events emitted
- `vfs_events_handled_total` - Events successfully handled
- `vfs_events_failed_total` - Failed event handlers
- `vfs_events_vetoed_total` - Operations vetoed
- `vfs_webhook_latency_ms` - Webhook response time
- `vfs_event_queue_depth` - Pending events (async mode)

### Health Checks

Check event system health:

```go
status := eventTrigger.Health()
// {
//   "healthy": true,
//   "pending_events": 5,
//   "failed_handlers": 0,
//   "circuit_breakers_open": 0
// }
```

## Troubleshooting

### Events Not Firing

1. Check `.events` file exists in directory
2. Verify pattern matches event type
3. Check logs for handler errors
4. Ensure EventTrigger is wired up

### Veto Not Working

1. Verify `veto_enabled: true` in config
2. Check handler returns veto response
3. Ensure handler is on validation/authorization stage
4. Check `on_error` setting

### High Latency

1. Use async mode for non-critical events
2. Reduce webhook timeout
3. Disable retries for fast operations
4. Check webhook endpoint performance

### Memory Leaks

1. Monitor event queue depth
2. Reduce `max_concurrent_events`
3. Use shorter TTLs for event storage
4. Enable circuit breakers

## Migration from Phase 3 Events

If migrating from the older Phase 3 event system:

### Before (Phase 3)
```json
{
  "webhooks": [
    {
      "url": "https://api.example.com/webhook",
      "events": ["file.created"]
    }
  ]
}
```

### After (Phase 5 Lifecycle)
```json
{
  "handlers": [
    {
      "name": "file-created-webhook",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://api.example.com/webhook",
        "method": "POST"
      }
    }
  ]
}
```

### Key Differences

1. **Granular Events**: `file.created` → `file.create.completion.succeeded`
2. **Pattern Matching**: More powerful wildcard support
3. **Veto Support**: Can block operations at validation stage
4. **Multiple Handlers**: Single event can trigger multiple handlers
5. **Handler Types**: log, metrics, webhook (extensible)

## See Also

- [Event Specification](14_EVENTS_SPEC.md)
- [Webhook Integration Guide](17_WEBHOOKS.md)
- [Lifecycle Examples](16_LIFECYCLE_EXAMPLES.md)

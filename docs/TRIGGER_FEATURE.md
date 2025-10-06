# VFS Trigger Feature

## Overview

The VFS Trigger feature allows you to configure HTTP POST webhooks that are automatically called when files are created in a directory. This enables real-time integrations, notifications, and workflow automation.

## Configuration File: `.trigger`

Place a `.trigger` file in any directory to define triggers for that directory.

### Basic Example

```json
{
  "triggers": [
    {
      "event": "file.create",
      "url": "https://example.com/webhook",
      "method": "POST",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN",
        "Content-Type": "application/json"
      },
      "include_content": true
    }
  ]
}
```

### Configuration Options

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `event` | string | Yes | Event type to trigger on. Currently supports: `file.create`, `file.update`, `file.delete` |
| `url` | string | Yes | HTTP endpoint to call |
| `method` | string | No | HTTP method (default: `POST`) |
| `headers` | object | No | Custom HTTP headers to include |
| `include_content` | boolean | No | Whether to include file content in the payload (default: `false`) |
| `pattern` | string | No | Glob pattern to filter files (e.g., `*.json`) |

## Payload Structure

When a trigger fires, the HTTP POST request includes:

```json
{
  "event": {
    "id": "evt_123",
    "type": "file.create.completion.succeeded",
    "timestamp": "2025-10-06T12:34:56Z",
    "request_id": "req_abc"
  },
  "actor": {
    "user_id": "user_123",
    "username": "john.doe",
    "email": "john@example.com"
  },
  "file": {
    "id": "file_xyz",
    "name": "document.json",
    "path": "/projects/document.json",
    "directory_id": "dir_123",
    "content_type": "application/json",
    "size_bytes": 1024,
    "version": 1,
    "created_at": "2025-10-06T12:34:56Z"
  },
  "content": "...base64 encoded content..."
}
```

The `content` field is only included if `include_content: true` in the trigger configuration.

## Example Use Cases

### 1. Slack Notification

```json
{
  "triggers": [
    {
      "event": "file.create",
      "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
      "method": "POST",
      "pattern": "*.pdf",
      "payload_template": {
        "text": "New PDF uploaded: {{file.name}} by {{actor.username}}"
      }
    }
  ]
}
```

### 2. Data Processing Pipeline

```json
{
  "triggers": [
    {
      "event": "file.create",
      "url": "https://api.example.com/process",
      "pattern": "data-*.json",
      "include_content": true,
      "headers": {
        "Authorization": "Bearer sk_live_1234567890",
        "X-Pipeline-ID": "json-validator"
      }
    }
  ]
}
```

### 3. Multi-Event Triggers

```json
{
  "triggers": [
    {
      "event": "file.create",
      "url": "https://api.example.com/created",
      "include_content": true
    },
    {
      "event": "file.update",
      "url": "https://api.example.com/updated",
      "include_content": false
    },
    {
      "event": "file.delete",
      "url": "https://api.example.com/deleted"
    }
  ]
}
```

## Implementation Details

### Trigger Resolution

Triggers are inherited from parent directories:
- When a file is created at `/projects/data/file.json`, the system checks for:
  1. `/projects/data/.trigger`
  2. `/projects/.trigger`
  3. `/.trigger`
- All matching triggers are executed in parallel

### Error Handling

- HTTP timeouts: 30 seconds
- Failed webhook calls are logged but don't block file operations
- Retry logic: 3 retries with exponential backoff
- Status codes 2xx are considered success

### Security Considerations

1. **Authentication**: Use headers for API keys/tokens
2. **Content Size**: Files larger than 10MB don't include content in webhook payload
3. **Rate Limiting**: Maximum 10 webhooks per file operation
4. **HTTPS Only**: Only HTTPS URLs are allowed in production

## CLI Commands

Create a trigger configuration:

```bash
vfs-cli create-trigger /projects
```

Test a trigger:

```bash
vfs-cli test-trigger /projects/.trigger
```

View trigger logs:

```bash
vfs-cli logs --type=trigger --path=/projects
```

## Integration with .events

The `.trigger` feature is a simplified interface on top of the `.events` system:
- `.trigger` files are automatically converted to `.events` handlers
- For advanced use cases (conditions, transformations), use `.events` directly
- `.trigger` focuses on simple webhook integrations

## Comparison

| Feature | `.trigger` | `.events` |
|---------|-----------|----------|
| Simplicity | ✓ Simple webhook config | Advanced event handling |
| File Operations | ✓ All CRUD ops | ✓ All lifecycle stages |
| Conditions | Pattern matching only | Full Rego policies |
| Transformations | None | Custom payload templates |
| Retries | ✓ Automatic | Manual configuration |

## Examples

See `/examples/triggers/` for complete examples:
- `slack-notification.json` - Slack integration
- `lambda-processor.json` - AWS Lambda trigger
- `webhook-logger.json` - Logging service
- `approval-workflow.json` - Multi-stage approval

## Future Enhancements

- [ ] Conditional triggers (based on file content)
- [ ] Batch triggers (collect multiple events)
- [ ] Circuit breaker for failing webhooks
- [ ] Webhook replay/debugging UI

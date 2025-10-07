# VFS Webhook Triggers - Complete Guide

## Overview

VFS supports webhook triggers that automatically send HTTP POST requests when files are created, updated, or deleted in a directory. This enables real-time integrations with external systems.

## Quick Start

### Create a Webhook Trigger

```bash
vfs-cli create-trigger /projects https://your-webhook-url.com/endpoint
```

This creates a `.events` file in `/projects` that will trigger an HTTP POST to your URL whenever a file is created in that directory.

### Test It

```bash
# Create a test file
vfs-cli import /tmp/test.txt /projects/

# Check your webhook endpoint - it should have received a POST request!
```

## Webhook Payload

When a file is created, your webhook receives this JSON payload:

```json
{
  "event": {
    "id": "evt_a1b2c3d4",
    "type": "file.create.completion.succeeded",
    "timestamp": "2025-10-06T12:34:56.789Z",
    "request_id": "req_xyz123"
  },
  "operation": {
    "operation_id": "op_456789",
    "category": "file",
    "operation": "create",
    "resource_path": "/projects/test.txt",
    "user_id": "user_123",
    "started_at": "2025-10-06T12:34:56.780Z",
    "completed_at": "2025-10-06T12:34:56.789Z",
    "current_stage": "completion",
    "status": "succeeded"
  },
  "actor": {
    "user_id": "user_123",
    "username": "john.doe",
    "email": "john@example.com",
    "roles": ["developer"]
  },
  "file": {
    "id": "file_abc123",
    "name": "test.txt",
    "path": "/projects/test.txt",
    "directory_id": "dir_proj",
    "content_type": "text/plain",
    "size_bytes": 1024,
    "version": 1,
    "created_at": "2025-10-06T12:34:56.789Z",
    "updated_at": "2025-10-06T12:34:56.789Z"
  },
  "metadata": {
    "environment": "production",
    "vfs_version": "1.0.0"
  }
}
```

## Event Types

### File Events (Completion)

These are the most commonly used events for webhooks:

| Event Type | When It Triggers | Use Case |
|------------|------------------|----------|
| `file.create.completion.succeeded` | After file is created successfully | Notifications, processing pipelines |
| `file.update.completion.succeeded` | After file is updated | Change detection, sync |
| `file.delete.completion.succeeded` | After file is deleted | Cleanup, archiving |
| `file.move.completion.succeeded` | After file is moved | Track relocations, update indexes |

### Lifecycle Events (Advanced)

All file operations go through multiple stages. Each stage emits events that can be used for advanced use cases:

**File Create Events:**

| Event Type | Stage | Can Veto? | Description |
|------------|-------|-----------|-------------|
| `file.create.authorization.started` | Authorization | ✓ Yes | Authorization check begins |
| `file.create.authorization.succeeded` | Authorization | ✓ Yes | User authorized to create file |
| `file.create.validation.schema.checking` | Validation | ✓ Yes | Schema validation in progress |
| `file.create.validation.schema.succeeded` | Validation | ✓ Yes | Schema validation passed |
| `file.create.validation.schema.failed` | Validation | ✗ No | Schema validation failed |
| `file.create.validation.succeeded` | Validation | ✓ Yes | All validations passed |
| `file.create.completion.succeeded` | Completion | ✗ No | File created successfully |
| `file.create.completion.failed` | Completion | ✗ No | File creation failed |

**File Update Events:**

| Event Type | Can Veto? | Description |
|------------|-----------|-------------|
| `file.update.authorization.started` | ✓ Yes | Authorization check begins |
| `file.update.authorization.succeeded` | ✓ Yes | User authorized to update file |
| `file.update.validation.schema.checking` | ✓ Yes | Schema validation in progress |
| `file.update.validation.schema.succeeded` | ✓ Yes | Schema validation passed |
| `file.update.validation.succeeded` | ✓ Yes | All validations passed |
| `file.update.completion.succeeded` | ✗ No | File updated successfully |
| `file.update.completion.failed` | ✗ No | File update failed |

**File Delete Events:**

| Event Type | Can Veto? | Description |
|------------|-----------|-------------|
| `file.delete.authorization.started` | ✓ Yes | Authorization check begins |
| `file.delete.authorization.succeeded` | ✓ Yes | User authorized to delete file |
| `file.delete.validation.succeeded` | ✓ Yes | All validations passed |
| `file.delete.completion.succeeded` | ✗ No | File deleted successfully |
| `file.delete.completion.failed` | ✗ No | File deletion failed |

**File Move Events:**

| Event Type | Can Veto? | Description |
|------------|-----------|-------------|
| `file.move.authorization.started` | ✓ Yes | Authorization check begins |
| `file.move.authorization.succeeded` | ✓ Yes | User authorized to move file |
| `file.move.validation.succeeded` | ✓ Yes | All validations passed |
| `file.move.completion.succeeded` | ✗ No | File moved successfully |
| `file.move.completion.failed` | ✗ No | File move failed |

### Directory Events

| Event Type | Description |
|------------|-------------|
| `directory.create.authorization.started` | Authorization check begins |
| `directory.create.authorization.succeeded` | User authorized to create directory |
| `directory.create.validation.succeeded` | All validations passed |
| `directory.create.completion.succeeded` | Directory created successfully |
| `directory.delete.authorization.started` | Authorization check begins |
| `directory.delete.authorization.succeeded` | User authorized to delete directory |
| `directory.delete.validation.succeeded` | All validations passed |
| `directory.delete.completion.succeeded` | Directory deleted successfully |

### Workflow Events

| Event Type | Description |
|------------|-------------|
| `workflow.transition.started` | File move between workflow states initiated |
| `workflow.transition.succeeded` | State transition successful |
| `workflow.transition.failed` | State transition blocked by gate policy |
| `workflow.deletion.blocked` | Attempt to delete file in workflow directory |
| `workflow.escape.blocked` | Attempt to move file outside workflow tree |
| `workflow.create.blocked` | Attempt to create file directly in state directory |
| `workflow.state_dir.protected` | Attempt to modify state directory structure |

### Wildcard Patterns

You can use wildcards to match multiple events:

| Pattern | Matches | Example |
|---------|---------|---------|
| `file.create.*` | Single token | `file.create.authorization`, `file.create.validation` |
| `file.create.>` | All remaining | `file.create.authorization.started`, `file.create.validation.schema.checking` |
| `*.completion.succeeded` | Any operation | `file.create.completion.succeeded`, `file.update.completion.succeeded` |
| `file.*.completion.*` | Multiple wildcards | All file completion events |
| `workflow.>` | All workflow events | Any workflow event |

## Advanced Configurations

### Custom Headers (Authentication)

Create `.events` manually for custom headers:

```json
{
  "handlers": [
    {
      "name": "authenticated-webhook",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/webhook",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer sk_live_1234567890",
          "X-API-Key": "your-api-key",
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

### Multiple Event Types

```json
{
  "handlers": [
    {
      "name": "multi-event-webhook",
      "events": [
        "file.create.completion.succeeded",
        "file.update.completion.succeeded",
        "file.delete.completion.succeeded"
      ],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/file-events",
        "method": "POST"
      }
    }
  ]
}
```

### Separate Webhooks for Different Events

```json
{
  "handlers": [
    {
      "name": "creation-webhook",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/created"
      }
    },
    {
      "name": "deletion-webhook",
      "events": ["file.delete.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/deleted"
      }
    }
  ]
}
```

## Real-World Examples

### 1. Slack Notification

```bash
# Get Slack webhook URL from https://api.slack.com/messaging/webhooks
vfs-cli create-trigger /uploads https://hooks.slack.com/services/YOUR/WEBHOOK/URL
```

Then customize the `.events` file:

```json
{
  "handlers": [
    {
      "name": "slack-notification",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

Your Slack channel will receive messages when files are uploaded to `/uploads`!

### 2. AWS Lambda Processing

```json
{
  "handlers": [
    {
      "name": "lambda-processor",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://your-lambda-url.execute-api.us-east-1.amazonaws.com/prod/process",
        "method": "POST",
        "headers": {
          "x-api-key": "your-lambda-api-key",
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

### 3. Email Notification (via Zapier/Make.com)

```json
{
  "handlers": [
    {
      "name": "email-notification",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://hooks.zapier.com/hooks/catch/YOUR/WEBHOOK/ID",
        "method": "POST"
      }
    }
  ]
}
```

### 4. File Processing Pipeline

```json
{
  "handlers": [
    {
      "name": "json-validator",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://api.example.com/validate-json",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer your-token",
          "X-Pipeline-Stage": "validation"
        }
      }
    }
  ]
}
```

## Webhook Implementation Tips

### Node.js Express Example

```javascript
const express = require('express');
const app = express();

app.use(express.json());

app.post('/webhook', (req, res) => {
  const { event, actor, file } = req.body;

  console.log(`File created: ${file.path}`);
  console.log(`By user: ${actor.username}`);
  console.log(`File ID: ${file.id}`);
  console.log(`Size: ${file.size_bytes} bytes`);

  // Process the file
  // ...

  // Return 200 to acknowledge receipt
  res.status(200).json({ received: true });
});

app.listen(3000, () => {
  console.log('Webhook server running on port 3000');
});
```

### Python Flask Example

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/webhook', methods=['POST'])
def webhook():
    data = request.json

    event = data.get('event', {})
    actor = data.get('actor', {})
    file_info = data.get('file', {})

    print(f"File created: {file_info.get('path')}")
    print(f"By user: {actor.get('username')}")

    # Process the file
    # ...

    return jsonify({'received': True}), 200

if __name__ == '__main__':
    app.run(port=3000)
```

### Go HTTP Server Example

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type WebhookPayload struct {
    Event struct {
        ID   string `json:"id"`
        Type string `json:"type"`
    } `json:"event"`
    Actor struct {
        UserID   string `json:"user_id"`
        Username string `json:"username"`
    } `json:"actor"`
    File struct {
        ID          string `json:"id"`
        Name        string `json:"name"`
        Path        string `json:"path"`
        SizeBytes   int64  `json:"size_bytes"`
        ContentType string `json:"content_type"`
    } `json:"file"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    var payload WebhookPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    fmt.Printf("File created: %s\n", payload.File.Path)
    fmt.Printf("By user: %s\n", payload.Actor.Username)

    // Process the file
    // ...

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]bool{"received": true})
}

func main() {
    http.HandleFunc("/webhook", webhookHandler)
    http.ListenAndServe(":3000", nil)
}
```

## Webhook Best Practices

### 1. Return 200 Quickly

```javascript
// Good: Return immediately, process async
app.post('/webhook', async (req, res) => {
  res.status(200).json({ received: true });

  // Process in background
  processFileAsync(req.body);
});
```

### 2. Implement Idempotency

```javascript
const processedEvents = new Set();

app.post('/webhook', (req, res) => {
  const eventId = req.body.event.id;

  if (processedEvents.has(eventId)) {
    return res.status(200).json({ already_processed: true });
  }

  processedEvents.add(eventId);
  // Process event...

  res.status(200).json({ received: true });
});
```

### 3. Validate Payloads

```javascript
app.post('/webhook', (req, res) => {
  const { event, file } = req.body;

  if (!event || !file) {
    return res.status(400).json({ error: 'Invalid payload' });
  }

  if (event.type !== 'file.create.completion.succeeded') {
    return res.status(400).json({ error: 'Unexpected event type' });
  }

  // Process...
  res.status(200).json({ received: true });
});
```

## Troubleshooting

### Webhook Not Firing?

1. **Check .events file exists:**
   ```bash
   vfs-cli ls /your/directory
   # Should show .events file
   ```

2. **Verify .events content:**
   ```bash
   vfs-cli cat /your/directory/.events
   # Should show valid JSON with webhook config
   ```

3. **Check webhook orchestrator logs:**
   ```bash
   docker logs cc-vfs-webhook-orchestrator
   ```

4. **Test with webhook.site:**
   ```bash
   # Go to https://webhook.site to get a test URL
   vfs-cli create-trigger /test https://webhook.site/YOUR-UNIQUE-ID
   vfs-cli import /tmp/test.txt /test/
   # Check webhook.site for the request
   ```

### Webhook Receiving Invalid Data?

- Ensure your endpoint expects `Content-Type: application/json`
- Check that you're parsing the request body as JSON
- Verify the payload structure matches the documentation

### Webhook Timing Out?

- Webhooks have a 30-second timeout
- Return HTTP 200 immediately, process asynchronously
- Use queue systems (SQS, RabbitMQ) for long-running tasks

## See Also

- [Events System Documentation](./EVENTS.md)
- [.events File Reference](./EVENTS_FILE_SPEC.md)
- [Lifecycle Events](./LIFECYCLE_EVENTS.md)
- [Webhook Orchestrator Architecture](./WEBHOOK_ORCHESTRATOR.md)

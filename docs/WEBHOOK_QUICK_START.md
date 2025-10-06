# Webhook Triggers - Quick Start

## 1-Minute Setup

### Step 1: Create a Webhook Trigger

```bash
vfs-cli create-trigger /your/directory https://your-webhook-url.com
```

### Step 2: Test It

```bash
vfs-cli import /tmp/test.txt /your/directory/
```

That's it! Your webhook will receive a POST request.

## What You Get

Every time a file is created in `/your/directory`, your URL receives:

```json
{
  "event": {
    "type": "file.create.completion.succeeded",
    "timestamp": "2025-10-06T12:34:56Z"
  },
  "actor": {
    "username": "john.doe",
    "email": "john@example.com"
  },
  "file": {
    "name": "test.txt",
    "path": "/your/directory/test.txt",
    "size_bytes": 1024,
    "content_type": "text/plain"
  }
}
```

## Common Use Cases

### Slack Notifications

```bash
vfs-cli create-trigger /uploads https://hooks.slack.com/services/YOUR/WEBHOOK/URL
```

### AWS Lambda

```bash
vfs-cli create-trigger /data https://your-lambda.execute-api.us-east-1.amazonaws.com/prod
```

### Custom Processing

```bash
vfs-cli create-trigger /projects https://api.yourcompany.com/process-file
```

## Advanced: Add Authentication

Edit the `.events` file:

```bash
vfs-cli cat /your/directory/.events
```

Add headers:

```json
{
  "handlers": [
    {
      "name": "webhook-trigger",
      "events": ["file.create.completion.succeeded"],
      "type": "webhook",
      "config": {
        "url": "https://your-webhook-url.com",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer YOUR_TOKEN",
          "Content-Type": "application/json"
        }
      }
    }
  ]
}
```

## Testing

Use [webhook.site](https://webhook.site) to test:

```bash
# 1. Go to https://webhook.site - copy your unique URL
# 2. Create trigger
vfs-cli create-trigger /test https://webhook.site/YOUR-UNIQUE-ID

# 3. Create a file
echo "test" > /tmp/test.txt
vfs-cli import /tmp/test.txt /test/

# 4. Check webhook.site - you'll see the request!
```

## Event Types

| Command | Event Type |
|---------|-----------|
| Default | `file.create.completion.succeeded` |
| For updates | `file.update.completion.succeeded` |
| For deletions | `file.delete.completion.succeeded` |

Specify event type:

```bash
vfs-cli create-trigger /dir https://url.com file.update.completion.succeeded
```

## Troubleshooting

**Webhook not firing?**
```bash
# Check .events exists
vfs-cli ls /your/directory
# Should show .events

# View content
vfs-cli cat /your/directory/.events
```

**Check logs:**
```bash
docker logs cc-vfs-webhook-orchestrator
```

## Next Steps

- [Full Webhook Guide](./WEBHOOK_TRIGGERS_GUIDE.md)
- [Events System](./EVENTS.md)
- [Payload Examples](./WEBHOOK_PAYLOAD_EXAMPLES.md)

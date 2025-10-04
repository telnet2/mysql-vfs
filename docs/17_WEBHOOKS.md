# Webhook Integration Guide

## Overview

Webhooks allow MySQL VFS to integrate with external systems by sending HTTP requests when events occur. Webhooks can also **veto** operations, giving external services the ability to approve or deny filesystem operations.

## Basic Webhook Configuration

### Minimal Configuration

```json
{
  "handlers": [
    {
      "name": "my-webhook",
      "type": "webhook",
      "pattern": "file.create.completion.succeeded",
      "config": {
        "url": "https://api.example.com/notify",
        "method": "POST"
      }
    }
  ]
}
```

### Full Configuration

```json
{
  "handlers": [
    {
      "name": "advanced-webhook",
      "type": "webhook",
      "pattern": "file.*.validation.succeeded",
      "config": {
        "url": "https://api.example.com/validate",
        "method": "POST",
        "headers": {
          "Authorization": "Bearer ${WEBHOOK_TOKEN}",
          "Content-Type": "application/json",
          "X-Custom-Header": "value"
        },
        "timeout_ms": 5000,
        "max_retries": 3,
        "retry_delay_ms": 1000,
        "retry_backoff": "exponential",
        "veto_enabled": true,
        "veto_on_4xx": true,
        "veto_on_5xx": false,
        "on_error": "abort",
        "hmac_secret": "${WEBHOOK_SECRET}",
        "circuit_breaker": {
          "threshold": 5,
          "timeout_seconds": 60
        }
      }
    }
  ]
}
```

## Request Payload

MySQL VFS sends a JSON payload with complete event context:

```json
{
  "event": {
    "id": "evt_abc123",
    "type": "file.create.validation.succeeded",
    "timestamp": "2025-01-15T10:30:00Z",
    "operation_id": "op_xyz789",
    "category": "file",
    "operation": "create",
    "stage": "validation",
    "outcome": "succeeded"
  },
  "resource": {
    "type": "file",
    "id": "file_123",
    "path": "/data/report.pdf",
    "directory": "/data",
    "name": "report.pdf",
    "size": 2048576,
    "mime_type": "application/pdf"
  },
  "user": {
    "user_id": "alice",
    "email": "alice@company.com",
    "roles": ["admin", "editor"]
  },
  "request": {
    "id": "req_456",
    "ip": "192.168.1.100",
    "user_agent": "Mozilla/5.0..."
  },
  "metadata": {
    "custom_field": "value"
  }
}
```

## Response Handling

### Success Response (200 OK)

Operation continues normally:

```json
{
  "status": "success"
}
```

### Veto Responses

#### HTTP Status Code Veto

Return 4xx or 5xx status codes to veto:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "error": "Operation not allowed"
}
```

Configure which status codes trigger veto:

```json
{
  "config": {
    "veto_on_4xx": true,   // 400-499 triggers veto
    "veto_on_5xx": false   // 500-599 does not veto
  }
}
```

#### JSON Response Veto

Return explicit veto in response body:

```json
{
  "veto": true,
  "message": "File exceeds size limit",
  "code": "FILE_TOO_LARGE"
}
```

The `veto` field overrides status code behavior.

## Veto Use Cases

### Use Case 1: File Size Limit

Block files larger than 10MB:

**.events configuration**:
```json
{
  "handlers": [
    {
      "name": "size-check",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://validator.company.com/check-size",
        "veto_enabled": true,
        "veto_on_4xx": true
      }
    }
  ]
}
```

**Webhook service**:
```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/check-size', methods=['POST'])
def check_size():
    data = request.json
    file_size = data['resource']['size']

    # 10MB limit
    MAX_SIZE = 10 * 1024 * 1024

    if file_size > MAX_SIZE:
        return jsonify({
            'veto': True,
            'message': f'File size {file_size} exceeds limit of {MAX_SIZE}',
            'code': 'SIZE_LIMIT_EXCEEDED'
        }), 403

    return jsonify({'approved': True}), 200
```

### Use Case 2: Content Scanning

Scan for malware before allowing upload:

**.events configuration**:
```json
{
  "handlers": [
    {
      "name": "malware-scan",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://scanner.company.com/scan",
        "veto_enabled": true,
        "veto_on_4xx": true,
        "timeout_ms": 30000,
        "on_error": "abort"
      }
    }
  ]
}
```

**Webhook service**:
```javascript
app.post('/scan', async (req, res) => {
  const { resource } = req.body;

  try {
    // Fetch file content from VFS
    const fileContent = await fetchFileContent(resource.path);

    // Scan for malware
    const scanResult = await antivirusEngine.scan(fileContent);

    if (scanResult.threatFound) {
      return res.status(403).json({
        veto: true,
        message: `Threat detected: ${scanResult.threatName}`,
        code: 'MALWARE_DETECTED',
        details: scanResult
      });
    }

    res.json({ clean: true });
  } catch (error) {
    // On error, abort operation (on_error: "abort")
    res.status(500).json({ error: error.message });
  }
});
```

### Use Case 3: Approval Workflow

Require manager approval for deletions:

**.events configuration**:
```json
{
  "handlers": [
    {
      "name": "deletion-approval",
      "type": "webhook",
      "pattern": "file.delete.validation.succeeded",
      "config": {
        "url": "https://workflow.company.com/api/approval",
        "veto_enabled": true,
        "veto_on_4xx": true,
        "timeout_ms": 60000
      }
    }
  ]
}
```

**Workflow service**:
```go
func approvalHandler(w http.ResponseWriter, r *http.Request) {
    var event WebhookEvent
    json.NewDecoder(r.Body).Decode(&event)

    userID := event.User.UserID
    resourcePath := event.Resource.Path

    // Check if user is manager
    if !isManager(userID) {
        // Request approval from manager
        approvalID := createApprovalRequest(userID, resourcePath, "delete")

        // Return veto - user must wait for approval
        w.WriteHeader(http.StatusForbidden)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "veto":   true,
            "message": "Manager approval required",
            "code":    "APPROVAL_PENDING",
            "approval_id": approvalID,
        })
        return
    }

    // Manager can delete immediately
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]bool{"approved": true})
}
```

### Use Case 4: Quota Enforcement

Enforce per-user storage quotas:

**.events configuration**:
```json
{
  "handlers": [
    {
      "name": "quota-check",
      "type": "webhook",
      "pattern": "file.create.validation.succeeded",
      "config": {
        "url": "https://quota.company.com/check",
        "veto_enabled": true,
        "veto_on_4xx": true
      }
    }
  ]
}
```

**Quota service**:
```ruby
post '/check' do
  data = JSON.parse(request.body.read)

  user_id = data['user']['user_id']
  file_size = data['resource']['size']

  # Get current usage
  current_usage = get_user_usage(user_id)
  quota_limit = get_user_quota(user_id)

  if current_usage + file_size > quota_limit:
    status 403
    json({
      veto: true,
      message: "Quota exceeded: #{current_usage + file_size} / #{quota_limit}",
      code: 'QUOTA_EXCEEDED',
      current_usage: current_usage,
      quota_limit: quota_limit
    })
  else
    json({ approved: true })
  end
end
```

### Use Case 5: Business Hours Enforcement

Only allow uploads during business hours:

**.events configuration**:
```json
{
  "handlers": [
    {
      "name": "business-hours",
      "type": "webhook",
      "pattern": "file.create.authorization.succeeded",
      "config": {
        "url": "https://policy.company.com/business-hours",
        "veto_enabled": true,
        "veto_on_4xx": true
      }
    }
  ]
}
```

**Policy service**:
```python
@app.route('/business-hours', methods=['POST'])
def check_business_hours():
    from datetime import datetime

    now = datetime.now()
    hour = now.hour
    weekday = now.weekday()

    # Business hours: Mon-Fri, 9am-5pm
    if weekday >= 5 or hour < 9 or hour >= 17:
        return jsonify({
            'veto': True,
            'message': 'Uploads only allowed during business hours (Mon-Fri 9am-5pm)',
            'code': 'OUTSIDE_BUSINESS_HOURS'
        }), 403

    return jsonify({'approved': True}), 200
```

## Security

### HMAC Signature Verification

MySQL VFS can sign webhook requests:

**.events configuration**:
```json
{
  "config": {
    "url": "https://api.example.com/webhook",
    "hmac_secret": "${WEBHOOK_SECRET}"
  }
}
```

**Request headers**:
```
X-VFS-Signature: sha256=abc123...
X-VFS-Timestamp: 1674000000
```

**Webhook service verification**:
```javascript
const crypto = require('crypto');

function verifySignature(req) {
  const signature = req.headers['x-vfs-signature'];
  const timestamp = req.headers['x-vfs-timestamp'];
  const body = JSON.stringify(req.body);

  // Prevent replay attacks (5 minute window)
  const now = Math.floor(Date.now() / 1000);
  if (Math.abs(now - timestamp) > 300) {
    throw new Error('Request too old');
  }

  // Verify HMAC
  const secret = process.env.WEBHOOK_SECRET;
  const payload = `${timestamp}.${body}`;
  const expectedSig = crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex');

  if (!crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(`sha256=${expectedSig}`))) {
    throw new Error('Invalid signature');
  }
}

app.post('/webhook', (req, res) => {
  try {
    verifySignature(req);
    // Process webhook...
  } catch (error) {
    res.status(401).json({ error: error.message });
  }
});
```

### IP Whitelisting

Restrict webhook sources to VFS server IPs:

```nginx
location /webhook {
    allow 10.0.0.0/8;     # VFS server subnet
    deny all;

    proxy_pass http://localhost:3000;
}
```

### TLS/HTTPS

Always use HTTPS for webhook URLs:

```json
{
  "config": {
    "url": "https://api.example.com/webhook"
  }
}
```

## Error Handling

### Retry Logic

Configure automatic retries:

```json
{
  "config": {
    "max_retries": 3,
    "retry_delay_ms": 1000,
    "retry_backoff": "exponential"
  }
}
```

**Retry schedule**:
- Attempt 1: Immediate
- Attempt 2: 1000ms delay
- Attempt 3: 2000ms delay
- Attempt 4: 4000ms delay

### On Error Behavior

Control what happens when webhook fails:

```json
{
  "config": {
    "on_error": "allow"   // or "abort"
  }
}
```

- **`allow`**: Continue operation if webhook fails
- **`abort`**: Veto operation on webhook failure

### Circuit Breaker

Automatically stop calling failing webhooks:

```json
{
  "config": {
    "circuit_breaker": {
      "threshold": 5,        // Open after 5 failures
      "timeout_seconds": 60  // Stay open for 60 seconds
    }
  }
}
```

**Circuit states**:
1. **Closed**: Normal operation
2. **Open**: All requests fast-fail (after threshold failures)
3. **Half-Open**: Test if service recovered (after timeout)

## Performance

### Timeouts

Set appropriate timeouts:

```json
{
  "config": {
    "timeout_ms": 5000   // 5 second timeout
  }
}
```

**Recommendations**:
- Authorization/Validation: 2000-5000ms
- Completion events: 1000-3000ms
- Async notifications: 10000ms+

### Async vs Sync

**Synchronous** (required for veto):
```json
{
  "pattern": "file.create.validation.succeeded",
  "config": {
    "dispatch_mode": "sync",
    "veto_enabled": true
  }
}
```

**Asynchronous** (better performance):
```json
{
  "pattern": "file.create.completion.succeeded",
  "config": {
    "dispatch_mode": "async"
  }
}
```

### Connection Pooling

MySQL VFS automatically pools HTTP connections. For high-traffic webhooks:

```go
// Webhook service should also use connection pooling
http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
```

## Monitoring

### Webhook Metrics

Monitor webhook performance:

```
vfs_webhook_requests_total{handler="my-webhook",status="200"}
vfs_webhook_requests_total{handler="my-webhook",status="403"}
vfs_webhook_latency_ms{handler="my-webhook",p95="1250"}
vfs_webhook_retries_total{handler="my-webhook"}
vfs_webhook_circuit_breaker_state{handler="my-webhook",state="open"}
```

### Logging

Enable debug logging:

```json
{
  "handlers": [
    {
      "name": "debug-webhook",
      "type": "log",
      "pattern": "webhook.*",
      "config": {
        "level": "debug",
        "message": "Webhook {{event.handler}}: {{event.status}} ({{event.latency}}ms)"
      }
    }
  ]
}
```

## Testing

### Local Testing with ngrok

```bash
# Start ngrok
ngrok http 3000

# Use ngrok URL in .events
{
  "config": {
    "url": "https://abc123.ngrok.io/webhook"
  }
}
```

### Mock Webhook Service

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/webhook', methods=['POST'])
def webhook():
    print('Received:', request.json)

    # Simulate different responses for testing
    if 'test-veto' in request.json.get('resource', {}).get('path', ''):
        return jsonify({
            'veto': True,
            'message': 'Test veto'
        }), 403

    return jsonify({'status': 'success'}), 200

if __name__ == '__main__':
    app.run(port=3000)
```

### Integration Tests

```go
func TestWebhookVeto(t *testing.T) {
    // Create mock webhook server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusForbidden)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "veto":   true,
            "message": "Test veto",
        })
    }))
    defer server.Close()

    // Create .events with webhook pointing to mock server
    eventsConfig := fmt.Sprintf(`{
        "handlers": [{
            "name": "test-veto",
            "type": "webhook",
            "pattern": "file.create.validation.succeeded",
            "config": {
                "url": "%s",
                "veto_enabled": true,
                "veto_on_4xx": true
            }
        }]
    }`, server.URL)

    // Create file - should be vetoed
    _, err := fileService.CreateFile(ctx, "/", "test.json", "application/json", 100, reader)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "Test veto")
}
```

## Troubleshooting

### Webhook Not Firing

1. Check pattern matches event type
2. Verify URL is accessible from VFS server
3. Check firewall/network rules
4. Enable debug logging

### Veto Not Working

1. Ensure `veto_enabled: true`
2. Check webhook returns proper status code
3. Verify pattern matches validation/authorization stage
4. Check webhook response format

### Timeouts

1. Increase `timeout_ms`
2. Optimize webhook endpoint
3. Use async mode if veto not needed
4. Enable circuit breaker

### High Latency

1. Reduce webhook processing time
2. Use async mode
3. Implement webhook caching
4. Scale webhook service

## Best Practices

1. **Always use HTTPS** for production webhooks
2. **Implement HMAC verification** to prevent spoofing
3. **Set appropriate timeouts** to avoid blocking operations
4. **Use circuit breakers** to handle webhook failures gracefully
5. **Monitor webhook performance** with metrics
6. **Test veto logic** thoroughly before deploying
7. **Use async mode** for non-critical notifications
8. **Implement idempotency** in webhook handlers
9. **Log all webhook calls** for audit trail
10. **Version your webhook API** to allow gradual rollouts

## See Also

- [Lifecycle Events Guide](15_LIFECYCLE_EVENTS.md)
- [Lifecycle Examples](16_LIFECYCLE_EXAMPLES.md)
- [Events Specification](14_EVENTS_SPEC.md)

# VFS Event Publisher Design Concept

## Overview

The VFS Event Publisher is a new microservice that provides real-time event streaming capabilities to clients through Server-Sent Events (SSE). It collects events from all VFS microservices, combines them into a unified stream, and broadcasts them to connected clients with guaranteed event delivery order.

## Architecture

### Current Architecture
```
VFS Service ─── Database ─── Event Worker ─── Webhook Orchestrator
    │                           │
    └───────────────────────────┼─────────────────── Scheduler
                                │
                                └─────────────────── Clients (via polling)
```

### Proposed Architecture
```
VFS Service ──┐
Event Worker ─┼── NATS ── Event Publisher ── SSE ── Clients
Webhook Orchestrator ──┘
Scheduler ────┘
```

## Core Components

### 1. Event Publisher Service
A new microservice (`services/event-publisher/`) that:
- Subscribes to NATS subjects for all VFS events
- Maintains an in-memory event buffer with configurable size
- Provides SSE endpoint for real-time event streaming
- Handles multiple concurrent client connections
- Implements connection limits and rate limiting

### 2. NATS Message Bus
Lightweight messaging system for inter-service communication:
- All services publish events to NATS subjects
- Event Publisher subscribes to collect all events
- Provides decoupling between event producers and consumers

### 3. SSE Streaming Endpoint
`/api/v1/publish-events` endpoint that:
- Accepts query parameters for event filtering
- Maintains persistent connections with clients
- Sends events as JSON payloads with proper SSE formatting
- Handles client disconnections and reconnections

## Event Flow

### Event Production
1. VFS operations generate events (file create, directory operations, etc.)
2. Events are stored in database (existing behavior)
3. Events are published to NATS subjects (new behavior)
4. Event Worker processes events for webhooks/logs (existing behavior)

### Event Consumption
1. Event Publisher subscribes to all NATS event subjects
2. Events are collected and buffered in memory
3. SSE clients connect to `/api/v1/publish-events`
4. Events are streamed to all connected clients in order

## NATS Subject Design (Updated)

The event naming convention has been updated to a hierarchical, lifecycle-based model. This provides more granular information about the stage and status of an operation.

The format is: `[resource].[operation].[stage].[status]`

- **resource**: `file`, `directory`
- **operation**: `create`, `update`, `delete`, `move`
- **stage**: `authorization`, `validation`, `execution`, `completion`
- **status**: `started`, `succeeded`, `failed`

### Example Events:

```
// File events
file.create.authorization.started
file.create.authorization.succeeded
file.create.validation.succeeded
file.create.completion.succeeded
file.update.completion.succeeded
file.delete.completion.succeeded

// Directory events
directory.create.completion.succeeded
directory.delete.completion.succeeded
```

Wildcards can be used for subscriptions, e.g., `file.create.*` or `file.*.completion.succeeded`.


## Service Modifications

### VFS Service (`services/vfs/`)
**Estimated Changes: Medium (50-100 lines)**
- Add NATS client initialization
- Modify event emission to publish to NATS after database storage
- Add configuration for NATS connection

**Key Functions (Updated):**

The event emission is now handled by an `eventTrigger` service. The key methods for emitting events are:

```go
// In pkg/domain/file_service.go and pkg/domain/directory_service.go

// For synchronous events that can block execution (e.g., for vetoing)
s.eventTrigger.EmitSync(ctx, eventType, payload)

// For asynchronous (fire-and-forget) events
s.eventTrigger.Emit(ctx, eventType, payload)
```

The legacy `emitEvent` function is still used for backward compatibility with older event types.

### Event Worker (`services/event-worker/`)
**Estimated Changes: Small (20-30 lines)**
- Add NATS client for publishing processing events
- Publish webhook delivery status to NATS

**Key Functions:**
```go
func (w *EventWorker) publishEvent(event *models.Event, status string)
```

### Webhook Orchestrator (`services/webhook-orchestrator/`)
**Estimated Changes: Small (20-30 lines)**
- Add NATS client for publishing delivery events
- Publish webhook delivery attempts/results to NATS

**Key Functions:**
```go
func (w *WebhookOrchestrator) publishDeliveryEvent(webhookID string, status string, attempt int)
```

### Scheduler (`services/scheduler/`)
**Estimated Changes: Small (20-30 lines)**
- Add NATS client for publishing scheduled task events
- Publish cron job execution events to NATS

**Key Functions:**
```go
func (s *Scheduler) publishTaskEvent(taskID string, status string)
```

### Event Publisher Service (`services/event-publisher/`) - NEW
**Estimated Size: Medium (200-300 lines)**
- New microservice with NATS subscriber and SSE server
- In-memory event buffer with configurable size
- SSE endpoint with connection management

**Key Components:**
```go
type EventPublisher struct {
    natsConn    *nats.Conn
    eventBuffer chan EventEnvelope
    sseServer   *SSEServer
}

type SSEServer struct {
    clients   map[string]*SSEClient
    broadcast chan EventEnvelope
}

func (p *EventPublisher) Start() error
func (s *SSEServer) HandleConnection(c *app.RequestContext)
func (s *SSEServer) broadcastEvent(event EventEnvelope)
```

## Event Data Structures (Updated)

The event payloads are strongly typed and differ based on the resource type (file or directory). All event-related types are defined in `pkg/events/types.go`.

### Payloads

The two main payload types are `FileEventPayload` and `DirectoryEventPayload`.

```go
// FileEventPayload represents the complete payload for file events
type FileEventPayload struct {
	Event    Event         `json:"event"`
	Resource FileResource  `json:"resource"`
	User     UserContext   `json:"user"`
	Metadata EventMetadata `json:"metadata"`
}

// DirectoryEventPayload represents the complete payload for directory events
type DirectoryEventPayload struct {
	Event    Event             `json:"event"`
	Resource DirectoryResource `json:"resource"`
	User     UserContext       `json:"user"`
	Metadata EventMetadata     `json:"metadata"`
}
```

### Core Types

These are the core building blocks of the event payloads:

```go
// Event represents the core event information
type Event struct {
	ID            string    `json:"id"`
	Type          EventType `json:"type"`
	Timestamp     time.Time `json:"timestamp"`
	DirectoryPath string    `json:"directory_path"`
}

// FileResource represents a file resource in an event
type FileResource struct {
	// ... fields like ID, Name, Path, SizeBytes, etc.
}

// DirectoryResource represents a directory resource in an event
type DirectoryResource struct {
	// ... fields like ID, Name, Path, etc.
}
```

## Event Implementation Details

For developers looking to trace or extend the event system, here are the key files:

- **Event Type Definitions (`pkg/events/types.go`):** This file contains all the core data structures for events, including `EventType`, `FileEventPayload`, and `DirectoryEventPayload`.
- **Event Emitters:**
    - `pkg/domain/file_service.go`: Emits all file-related lifecycle events (e.g., `file.create.*`, `file.update.*`).
    - `pkg/domain/directory_service.go`: Emits all directory-related lifecycle events (e.g., `directory.create.*`, `directory.delete.*`).
- **Event Pattern Matching (`pkg/events/pattern_matcher.go`):** Contains the logic for matching event types against wildcard patterns used in triggers.

## SSE Event Format (Updated)

The SSE event format sends a JSON payload containing the corresponding event payload structure.

### File Event Example

```
event: vfs-event
data: {
  "event": {
    "id": "evt_123",
    "type": "file.create.completion.succeeded",
    "timestamp": "...",
    "directory_path": "/data/"
  },
  "resource": {
    "type": "file",
    "id": "file_456",
    "name": "new_file.txt",
    "path": "/data/new_file.txt",
    ...
  },
  "user": { ... },
  "metadata": { ... }
}
```

### Directory Event Example

```
event: vfs-event
data: {
  "event": {
    "id": "evt_789",
    "type": "directory.create.completion.succeeded",
    "timestamp": "...",
    "directory_path": "/"
  },
  "resource": {
    "type": "directory",
    "id": "dir_abc",
    "name": "new_dir",
    "path": "/new_dir",
    ...
  },
  "user": { ... },
  "metadata": { ... }
}
```


## Configuration

### Environment Variables
```
NATS_URL=nats://nats:4222
EVENT_BUFFER_SIZE=1000
SSE_MAX_CONNECTIONS=100
SSE_CONNECTION_TIMEOUT=30s
```

### Docker Compose Additions
```yaml
services:
  nats:
    image: nats:2.9
    ports:
      - "4222:4222"
      - "8222:8222"

  event-publisher:
    build: ./services/event-publisher
    ports:
      - "8083:8080"
    depends_on:
      - nats
    environment:
      - NATS_URL=nats://nats:4222
```

## Scalability Considerations

### Event Publisher Scaling
- Multiple instances can subscribe to same NATS subjects
- Load balancer distributes SSE connections across instances
- In-memory buffers are per-instance (events may be missed during restarts)

### NATS Clustering
- NATS server can be clustered for high availability
- Event persistence can be configured for delivery guarantees
- Subject-based routing allows flexible event distribution

## Client Integration

### SSE Connection
```javascript
const eventSource = new EventSource('/api/v1/publish-events?filter=file.*');

eventSource.onmessage = function(event) {
    const vfsEvent = JSON.parse(event.data);
    console.log('Received event:', vfsEvent);
};
```

### Event Filtering
Query parameters for filtering events:
- `?filter=file.*` - All file events
- `?filter=directory.create` - Specific event types
- `?source=vfs` - Events from specific service

## Deployment Strategy

### Phase 1: NATS Integration ✅ **COMPLETED**
- ✅ Add NATS dependency to go.mod
- ✅ Extend LifecycleEventTrigger with NATS publishing
- ✅ Update VFS service to initialize NATS connection
- ✅ Add NATS service to docker-compose.yml

### Phase 2: Event Publisher Service ✅ **COMPLETED**
- ✅ Create separate event-publisher service
- ✅ Add NATS subscriber for all VFS events
- ✅ Implement SSE server with connection management
- ✅ Add event filtering and buffering
- ✅ Add event-publisher to docker-compose.yml

### Phase 3: Production Hardening ✅ **COMPLETED**
- ✅ Add JWT authentication/authorization to SSE endpoint
- ✅ Add Prometheus metrics for monitoring
- ✅ Add connection and event throughput metrics
- ⏸️ Implement event persistence and replay (future)
- ⏸️ Configure NATS clustering (future)

## Benefits

1. **Real-time Event Streaming**: Clients receive events instantly via SSE
2. **Unified Event View**: All microservice events combined into single stream
3. **Scalable Architecture**: Independent scaling of event production and consumption
4. **Loose Coupling**: Services communicate via NATS message bus
5. **Client Simplicity**: SSE provides easy browser integration

## Migration Path

1. Deploy NATS server alongside existing services ✅
2. Modify existing services to publish events to NATS (backward compatible) ✅
3. Deploy event-publisher service ✅
4. Update client applications to use SSE instead of polling
5. Gradually phase out polling-based event consumption

---

## Usage Examples

### Starting the Services

```bash
# Start all services including NATS and event-publisher
docker compose up -d

# Check service health
curl http://localhost:18083/health
```

### Connecting to SSE Stream (No Auth)

```bash
# Get all events
curl -N http://localhost:18083/api/v1/publish-events

# Get file events only
curl -N "http://localhost:18083/api/v1/publish-events?filter=file.*"

# Get completion events only
curl -N "http://localhost:18083/api/v1/publish-events?filter=*.completion.*"
```

### Connecting to SSE Stream (With JWT Auth)

First, set `AUTH_ENABLED=true` and `JWT_SECRET=your-secret-key` in docker-compose.yml.

```bash
# Get JWT token from VFS service
TOKEN=$(curl -s -X POST http://localhost:18080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"alice","password":"password"}' | jq -r '.token')

# Connect to SSE with Bearer token
curl -N http://localhost:18083/api/v1/publish-events \
  -H "Authorization: Bearer $TOKEN"

# Or use query parameter (for EventSource which can't set headers)
curl -N "http://localhost:18083/api/v1/publish-events?token=$TOKEN&filter=file.*"
```

### JavaScript Client (Browser)

```javascript
// Without authentication
const eventSource = new EventSource('http://localhost:18083/api/v1/publish-events?filter=file.*');

eventSource.addEventListener('vfs-event', (event) => {
    const data = JSON.parse(event.data);
    console.log('Received event:', data);
    console.log('Event type:', data._event_type);
    console.log('Resource:', data.resource);
});

eventSource.addEventListener('connected', (event) => {
    const data = JSON.parse(event.data);
    console.log('Connected to SSE:', data.client_id);
});

// With authentication (using token query parameter)
const token = 'your-jwt-token';
const eventSource = new EventSource(`http://localhost:18083/api/v1/publish-events?token=${token}&filter=file.*`);
```

### Monitoring with Prometheus

```bash
# View Prometheus metrics
curl http://localhost:18083/metrics

# Key metrics:
# - event_publisher_events_received_total: Events received from NATS
# - event_publisher_events_published_total: Events sent to SSE clients
# - event_publisher_events_dropped_total: Events dropped due to buffer overflow
# - event_publisher_active_connections: Current SSE connections
# - event_publisher_nats_connected: NATS connection status
# - event_publisher_buffer_size: Current buffer utilization
```

### Production Configuration

```yaml
# docker-compose.yml
event-publisher:
  environment:
    NATS_URL: nats://nats:4222
    EVENT_BUFFER_SIZE: 5000        # Larger buffer for high throughput
    SSE_MAX_CONNECTIONS: 1000      # Support more concurrent connections
    PORT: 8083
    JWT_SECRET: ${JWT_SECRET}      # Use secrets management in production
    AUTH_ENABLED: "true"           # Enable authentication
```

### Testing Event Flow

```bash
# In Terminal 1: Start SSE client
curl -N "http://localhost:18083/api/v1/publish-events?filter=file.create.*"

# In Terminal 2: Create a file via VFS API
curl -X POST http://localhost:18080/api/v1/files \
  -H "Content-Type: application/json" \
  -H "X-User-ID: alice" \
  -d '{
    "directory_path": "/data",
    "name": "test.txt",
    "content_type": "text/plain",
    "content": "Hello World"
  }'

# Terminal 1 should receive events:
# - file.create.authorization.started
# - file.create.authorization.succeeded
# - file.create.validation.succeeded
# - file.create.completion.succeeded
```

### Troubleshooting

```bash
# Check NATS connection status
curl http://localhost:18083/health | jq '.nats'

# Check NATS monitoring UI
open http://localhost:8222

# Check active SSE connections
curl http://localhost:18083/health | jq '.connections'

# View event buffer utilization
curl http://localhost:18083/health | jq '.buffer_size, .buffer_cap'

# Check VFS service NATS logs
docker logs cc-vfs-service | grep NATS
```</content>
</xai:function_call
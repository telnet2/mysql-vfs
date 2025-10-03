## Project Overview

This document captures the architectural blueprint for a MySQL-based distributed virtual file system (VFS) composed of microservices. The design emphasizes **strong consistency**, **exactly-once idempotent processing**, **horizontal scalability**, and **robust failure handling**.

### Goals
- Provide a MySQL-backed virtual file system capable of tracking directories, files, and derivative relationships.
    - Store files up to 100MB in S3-like blob storage using gocloud.dev
    - If a file is JSON and less than 16MB, store it as a JSON field in MySQL for efficient querying
    - Each file must have content-type metadata
- Ensure every mutating operation triggers webhook notifications with exactly-once delivery guarantees and callback tracking
- Support cron-executed tasks across distributed runners while maintaining consistency via lease-based coordination
- Offer a REPL-style CLI with import, listing, mutation, and inspection commands, including piping and JSON querying via `jq` semantics
- Deliver a production-ready deployment using Docker Compose with full end-to-end integration tests (Ginkgo v2 + httpexpect) backed by real MySQL
- Design authorization using OPA/REGO policies assigned per directory with fail-closed semantics

### Non-Goals
- Implement advanced authentication systems beyond basic service token validation
- Support files larger than 100MB (hard limit enforced)
- Optimize for low-latency queries; focus is on metadata consistency and correctness

### Key Constraints
- **Maximum file size**: 100MB (enforced at API layer)
- **Maximum directory tree depth**: 100 levels (prevents unbounded recursion)
- **Idempotency window**: 24 hours (cleanup after TTL)
- **Webhook retry limit**: 5 attempts with exponential backoff (2s, 4s, 8s, 16s, 32s)
- **OPA policy timeout**: 200ms (fail-closed on timeout)
- **Cron lease duration**: 5 minutes with 30-second heartbeat requirement
- **No database-level foreign keys, triggers, or stored procedures**: Referential integrity managed entirely in application code (see Phase 6 for validation implementation)

---

## Architecture Summary

### Microservices

1. **VFS Service** (Hertz + Thrift)
   - Manages directories, file metadata, file content storage, and lineage tracking
   - Exposes unified APIs for listing, mutations, uploads, and downloads
   - Uses GORM with MySQL (REPEATABLE READ isolation) and enforces optimistic versioning
   - Stores small JSON files (<16MB) directly in MySQL JSON columns for efficient querying
   - Stores larger files in S3-compatible blob storage via gocloud.dev
   - Emits mutation events transactionally using outbox pattern
   - Enforces idempotency via client-provided `request_id` header (UUIDv4)
   - Validates OPA policies before directory/file operations with timeout protection

2. **Webhook Orchestrator**
   - Synchronizes database webhook configurations with `adnanh/webhook` daemon
   - Consumes events from outbox, dispatches webhooks with idempotency keys
   - Handles callback acknowledgements via Hertz endpoint
   - Implements circuit breaker pattern (5 consecutive failures → open circuit for 1 minute)
   - Routes failed jobs to dead letter queue after retry exhaustion
   - Provides webhook health metrics and circuit breaker status

3. **Scheduler Service**
   - Runs cron jobs using `robfig/cron/v3` with MySQL-backed schedules
   - Multiple instances cooperate via lease-based locking with heartbeats
   - Uses `SELECT ... FOR UPDATE SKIP LOCKED` to claim jobs atomically
   - Writes heartbeat timestamps every 30s for long-running jobs
   - Background reaper recovers stale leases (no heartbeat within 2× lease duration)
   - Supports skip-missed-runs vs catch-up modes per job configuration

4. **Event Worker Pool**
   - Processes queued events from outbox using transactional state machine
   - Drives webhook dispatch, handles retry logic with exponential backoff
   - Performs deferred cleanup and compensation tasks
   - Updates event status atomically (pending → processing → completed/failed)
   - Moves poison messages (3 consecutive failures) to dead letter queue

5. **CLI Gateway**
   - Go-based REPL client using generated Hertz Thrift clients
   - Implements commands: `import`, `ls`, `ls -r`, `mv`, `rm`, `rmdir`, `cat`, `grep`, `jq <path> <expression>`
   - Supports piping between commands with streaming (no full buffering for large files)
   - Generates UUIDv4 `request_id` for all mutation commands
   - Maintains session state (current directory, auth token)
   - Validates local file size before import (<100MB)

---

## Data Model

### Core VFS Tables

**directories**
```sql
id, parent_id, name, path, version, opa_policy_id, created_at, updated_at, deleted_at
UNIQUE INDEX (parent_id, name) WHERE deleted_at IS NULL
INDEX (path) -- for tree queries
```

**files**
```sql
id, directory_id, name, content_type, size_bytes, storage_type (json|s3),
json_content (nullable JSON), s3_key (nullable), checksum_sha256, version,
created_at, updated_at, deleted_at
UNIQUE INDEX (directory_id, name) WHERE deleted_at IS NULL
CHECK (size_bytes <= 104857600) -- 100MB limit
CHECK ((storage_type='json' AND json_content IS NOT NULL) OR (storage_type='s3' AND s3_key IS NOT NULL))
```

**file_versions**
```sql
id, file_id, version_number, content_type, size_bytes, storage_type,
json_content, s3_key, checksum_sha256, created_at
-- immutable audit trail
```

**file_relations**
```sql
id, parent_file_id, derivative_file_id, relation_type, metadata (JSON), created_at
UNIQUE INDEX (parent_file_id, derivative_file_id)
-- prevents circular dependencies via application-level DAG check
```

### Referential Integrity Management

**No Database-Level Foreign Keys**: This system does NOT use database-level foreign key constraints, triggers, or stored procedures. All referential integrity is managed in application code.

**Application-Level Enforcement**:
- **Tree Locking**: Directory operations lock all ancestor paths (root→leaf) using `SELECT ... FOR UPDATE` to prevent parent deletion during child creation
- **Optimistic Locking**: File updates require `expected_version` to prevent lost updates
- **Soft Deletes**: `deleted_at` timestamps prevent hard deletes that could orphan references
- **Explicit Validation**: Service layer validates parent existence before creating children
- **Cascade Logic**: Delete operations explicitly handle cascading (e.g., recursive directory deletion deletes all children)
- **Transaction Boundaries**: All mutations that affect multiple tables use database transactions

**Phase 6 Validation Tasks**:
- Add referential integrity validation checks (e.g., verify all `parent_id` references exist)
- Implement orphan detection and cleanup jobs
- Add foreign key violation detection in integration tests
- Create repair scripts for inconsistent data

**Design Rationale**:
- Enables fine-grained control over cascade behavior
- Avoids database-level locking overhead
- Supports eventual consistency patterns for distributed operations
- Allows flexible schema evolution without migration complexity

### Idempotency & Events

**idempotency_records**
```sql
request_id (UUID, PK), response_hash, response_body, expires_at, created_at
INDEX (expires_at) -- for cleanup job
-- TTL: 24 hours
```

**events** (transactional outbox)
```sql
id, event_type, aggregate_id, payload (JSON), request_id,
status (pending|processing|completed|failed|dead_letter),
visible_at, processing_started_at, completed_at, retry_count,
error_message, created_at
INDEX (status, visible_at) -- for worker polling
-- visibility_delay: created_at + 5s prevents double-processing
```

**webhook_configs**
```sql
id, directory_id (nullable), event_type, target_url, secret,
is_active, circuit_state (closed|open|half_open),
circuit_opened_at, consecutive_failures, created_at, updated_at
INDEX (event_type, is_active)
```

**webhook_jobs**
```sql
id, event_id, webhook_config_id, idempotency_key,
status (pending|sent|acknowledged|failed),
attempt_count, next_retry_at, last_error, created_at, updated_at
INDEX (status, next_retry_at)
```

### Cron & Scheduling

**cron_jobs**
```sql
id, name, cron_expression, timezone, handler_type, payload (JSON),
skip_missed_runs (boolean), is_active, created_at, updated_at
```

**cron_executions**
```sql
id, cron_job_id, execution_key (unique), scheduled_at,
lease_holder_id, lease_expires_at, heartbeat_at,
status (pending|running|completed|failed|recovered),
started_at, completed_at, error_message, created_at
UNIQUE INDEX (execution_key) -- idempotency
INDEX (status, lease_expires_at) -- for reaper
```

### Authorization

**opa_policies**
```sql
id, name, rego_script, compiled_at, is_valid,
compilation_error, timeout_ms (default 200), created_at, updated_at
-- policies pre-compiled and validated on insert/update
```

### Observability

**audit_logs**
```sql
id, request_id, user_id, action, resource_type, resource_id,
ip_address, user_agent, status, duration_ms, created_at
INDEX (created_at, user_id)
INDEX (request_id)
```

**dead_letter_queue**
```sql
id, original_table (events|webhook_jobs), original_id, payload (JSON),
failure_reason, failure_count, moved_at
-- manual intervention required
```

---

## Consistency & Idempotency Strategy

### Strong Consistency Guarantees

1. **Single-Service Transactions**
   - All VFS mutations (directory + file + event outbox) occur in a single GORM transaction
   - Isolation level: `REPEATABLE READ` (prevents phantom reads)
   - Row-level locks using `SELECT ... FOR UPDATE` when needed
   - Optimistic concurrency control via `version` columns

2. **Idempotency Protocol**
   - Client **must** provide `X-Request-ID` header (UUIDv4) for all mutations
   - VFS Service checks `idempotency_records` before processing:
     ```
     IF EXISTS(request_id):
       RETURN cached response
     ELSE:
       BEGIN TRANSACTION
         Perform mutation
         Insert idempotency_record (expires_at = NOW() + 24h)
         Insert event into outbox
       COMMIT
       RETURN new response
     ```
   - Duplicate `request_id` with different parameters returns **400 Bad Request**
   - Background job purges expired idempotency records (expires_at < NOW())

3. **Transactional Outbox Pattern**
   - Events inserted in same transaction as business logic
   - `visible_at = created_at + 5s` prevents workers from seeing uncommitted data
   - Workers poll: `SELECT ... FOR UPDATE SKIP LOCKED WHERE status='pending' AND visible_at <= NOW() LIMIT 10`
   - State machine prevents double-processing:
     ```
     pending → processing (via UPDATE ... WHERE id=X AND status='pending')
     processing → completed|failed (atomic update)
     failed (retry_count < 3) → pending (with new visible_at for backoff)
     failed (retry_count >= 3) → dead_letter
     ```

4. **Tree Lock Protocol** (prevents parent-child race conditions)
   - When mutating directory tree, acquire locks in path order (root → leaf)
   - Example: Creating `/a/b/c/file.txt`
     ```sql
     SELECT id FROM directories WHERE path IN ('/', '/a', '/a/b', '/a/b/c') FOR UPDATE
     -- Then perform file insertion
     ```
   - Prevents concurrent deletion of parent while creating child

### Edge Case Handling

**Concurrent Directory Operations**
- Deleting `/a` while creating `/a/b/c`: Tree lock ensures `/a` locked first → creation fails
- Moving `/a` to `/x` while accessing `/a/b`: Path updates in single transaction, readers retry on serialization error

**File Upload Failures**
- Size validation before accepting upload
- S3 upload wrapped in transaction:
  ```
  BEGIN TRANSACTION
    Upload to S3 (get s3_key)
    IF upload fails: ROLLBACK
    Insert file metadata with s3_key
  COMMIT
  IF commit fails: Schedule async S3 cleanup job for orphaned s3_key
  ```

**Derivative File Cycles**
- DAG validation before creating `file_relations` entry
- BFS traversal from derivative → detect if parent appears in ancestry
- Reject if cycle detected

**Webhook Delivery Guarantees**
- **At-least-once delivery**: Retries on failure (network, 5xx, timeout)
- **Circuit breaker**: 5 consecutive failures → open circuit for 60s → test with single request (half-open) → resume or re-open
- **Idempotency key in payload**: Webhook receivers should deduplicate using `event.request_id`

**Cron Lease Recovery**
- Lease expires if `heartbeat_at + 60s < NOW()` (missed 2 heartbeats)
- Reaper updates: `UPDATE cron_executions SET status='recovered', lease_holder_id=NULL WHERE lease_expires_at < NOW() AND status='running'`
- Recovered executions can be reclaimed by other schedulers

**OPA Policy Failures**
- Policy compilation errors → prevent policy activation (is_valid=false)
- Policy timeout (>200ms) → deny access, log incident
- No policy assigned to directory → inherit parent's policy (traverse to root)
- Invalid policy at runtime → fail-closed (deny access)

---

## Event & Webhook Flow

### Detailed Flow

1. **Client Request**
   ```
   POST /api/v1/files
   Headers: X-Request-ID: 550e8400-e29b-41d4-a716-446655440000
   Body: file upload
   ```

2. **VFS Service Processing**
   ```
   Check idempotency_records[request_id]
   IF exists: RETURN cached response

   Validate OPA policy (timeout 200ms)
   Validate file size <= 100MB

   BEGIN TRANSACTION
     Upload to S3 (if >16MB or not JSON)
     INSERT INTO files
     INSERT INTO file_versions
     INSERT INTO events (type='file.created', visible_at=NOW()+5s, request_id)
     INSERT INTO idempotency_records (request_id, response_body, expires_at=NOW()+24h)
   COMMIT

   RETURN 201 Created
   ```

3. **Event Worker**
   ```
   WHILE true:
     BEGIN TRANSACTION
       SELECT id FROM events WHERE status='pending' AND visible_at <= NOW()
       FOR UPDATE SKIP LOCKED LIMIT 10

       FOR EACH event:
         UPDATE events SET status='processing', processing_started_at=NOW()
         WHERE id=event.id AND status='pending'

         IF updated_rows == 0: SKIP (race condition, another worker got it)

         COMMIT

         -- Process outside transaction
         result = DispatchToWebhookOrchestrator(event)

         BEGIN TRANSACTION
           IF result.success:
             UPDATE events SET status='completed', completed_at=NOW()
           ELIF event.retry_count < 3:
             UPDATE events SET status='pending', retry_count++,
                   visible_at=NOW() + exponential_backoff(retry_count)
           ELSE:
             UPDATE events SET status='dead_letter'
             INSERT INTO dead_letter_queue
         COMMIT
   ```

4. **Webhook Orchestrator**
   ```
   FOR EACH webhook_config matching event.event_type:
     Check circuit_state
     IF open AND circuit_opened_at + 60s > NOW(): SKIP

     INSERT INTO webhook_jobs (event_id, webhook_config_id,
           idempotency_key=event.request_id + webhook_config.id)

     POST webhook_config.target_url
     Headers: X-Idempotency-Key: {idempotency_key}
     Body: event payload

     IF response.status == 2xx:
       UPDATE webhook_jobs SET status='sent'
       UPDATE webhook_configs SET consecutive_failures=0, circuit_state='closed'
       RETURN success
     ELSE:
       UPDATE webhook_configs SET consecutive_failures++
       IF consecutive_failures >= 5:
         UPDATE webhook_configs SET circuit_state='open', circuit_opened_at=NOW()
       RETURN failure
   ```

5. **Webhook Callback** (optional acknowledgment)
   ```
   POST /webhook-callback
   Body: { idempotency_key: "...", status: "acknowledged" }

   UPDATE webhook_jobs SET status='acknowledged' WHERE idempotency_key=...
   ```

---

## Cron Processing

### Lease-Based Execution

1. **Scheduler Polling**
   ```
   WHILE true:
     current_time = NOW()

     SELECT id, cron_expression FROM cron_jobs
     WHERE is_active=true

     FOR EACH job:
       IF cron_expression.IsDue(current_time):
         execution_key = job.id + current_time.truncate(minute) -- idempotency

         BEGIN TRANSACTION
           INSERT INTO cron_executions (cron_job_id, execution_key,
                 scheduled_at, lease_holder_id=scheduler.id,
                 lease_expires_at=NOW()+5min, status='pending')
           ON DUPLICATE KEY UPDATE id=id -- idempotency
         COMMIT

         -- Try to claim lease
         BEGIN TRANSACTION
           UPDATE cron_executions SET status='running', started_at=NOW(),
                 lease_holder_id=scheduler.id, lease_expires_at=NOW()+5min
           WHERE execution_key=execution_key AND status='pending'
           FOR UPDATE SKIP LOCKED

           IF updated_rows == 1:
             COMMIT
             ExecuteJob(job) -- with heartbeat
           ELSE:
             ROLLBACK -- another scheduler claimed it

     SLEEP 10s
   ```

2. **Job Execution with Heartbeat**
   ```
   ExecuteJob(job):
     heartbeat_goroutine = START:
       EVERY 30s:
         UPDATE cron_executions SET heartbeat_at=NOW()
         WHERE execution_key=execution_key

     TRY:
       result = job.handler(job.payload)
       UPDATE cron_executions SET status='completed', completed_at=NOW()
     CATCH error:
       UPDATE cron_executions SET status='failed', error_message=error
     FINALLY:
       STOP heartbeat_goroutine
   ```

3. **Lease Reaper** (background job every 60s)
   ```
   UPDATE cron_executions SET status='recovered', lease_holder_id=NULL
   WHERE status='running'
     AND heartbeat_at + 60s < NOW()
     AND lease_expires_at < NOW()

   -- These can be reclaimed by schedulers
   ```

---

## CLI Behavior

### Session Management
- Maintains in-memory state: `current_directory_path`, `auth_token`
- Generates UUIDv4 for each mutation command
- Validates paths against current working directory

### Command Implementations

**`import <local_path> <vfs_path>`**
```
Validate local file exists and size <= 100MB
Generate request_id = UUIDv4()
Stream upload to VFS Service POST /files (with request_id header)
Handle idempotency: 409 Conflict (duplicate request_id) → report and exit
Display progress bar for uploads >10MB
```

**`ls [-r] [path]`**
```
GET /directories/{path}/entries
If -r flag: Recursive BFS traversal (client-side, respects depth limit 100)
Format output as tree view
```

**`cat <vfs_path>`**
```
GET /files/{path}/content (streaming)
Pipe to stdout (no buffering)
Detect content-type: if binary, warn user before displaying
```

**`jq <vfs_path> <expression>`**
```
GET /files/{path}/content
Validate content-type is application/json
Stream response to local jq process: jq '<expression>'
Pipe jq output to stdout (supports further piping: | grep, etc.)
```

**`mv <src> <dst>`**
```
Generate request_id = UUIDv4()
PUT /files/{src}/move (with dst path, request_id header)
Handle directory mv with server-side transaction (atomic path updates)
```

**`rm <path>`, `rmdir <path>`**
```
Generate request_id = UUIDv4()
DELETE /files/{path} or /directories/{path} (with request_id header)
For rmdir: Server validates directory is empty (or -r flag for recursive)
```

### Piping Behavior
- Commands connected via `io.Pipe()` with streaming
- Example: `cat /large.json | jq '.users[] | select(.active)' | grep "admin"`
  - `cat` streams file from VFS
  - `jq` processes stream incrementally
  - `grep` filters final output
- No intermediate buffering → handles files up to 100MB efficiently

---

## Deployment Plan

### Docker Compose Services

```yaml
services:
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: vfs
    volumes:
      - ./init.sql:/docker-entrypoint-initdb.d/
    healthcheck:
      test: ["CMD", "mysqladmin", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

  localstack:  # S3-compatible storage
    image: localstack/localstack
    environment:
      SERVICES: s3
    ports:
      - "4566:4566"

  vfs-service:
    build: ./services/vfs
    depends_on:
      mysql: { condition: service_healthy }
      localstack: { condition: service_started }
    environment:
      DB_DSN: root:root@tcp(mysql:3306)/vfs
      S3_ENDPOINT: http://localstack:4566
      S3_BUCKET: vfs-storage
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]

  webhook-daemon:
    image: adnanh/webhook
    volumes:
      - ./webhook-configs:/etc/webhook
    command: ["-verbose", "-hooks", "/etc/webhook/hooks.json"]
    ports:
      - "9000:9000"

  webhook-orchestrator:
    build: ./services/webhook-orchestrator
    depends_on:
      vfs-service: { condition: service_healthy }
      webhook-daemon: { condition: service_started }
    environment:
      DB_DSN: root:root@tcp(mysql:3306)/vfs
      WEBHOOK_DAEMON_URL: http://webhook-daemon:9000

  event-worker:
    build: ./services/event-worker
    depends_on:
      vfs-service: { condition: service_healthy }
    environment:
      DB_DSN: root:root@tcp(mysql:3306)/vfs
      WORKER_CONCURRENCY: 10
    deploy:
      replicas: 3  # Horizontal scaling

  scheduler:
    build: ./services/scheduler
    depends_on:
      vfs-service: { condition: service_healthy }
    environment:
      DB_DSN: root:root@tcp(mysql:3306)/vfs
      SCHEDULER_ID: ${HOSTNAME}
    deploy:
      replicas: 2  # HA schedulers

  cli:
    build: ./cli
    depends_on:
      vfs-service: { condition: service_healthy }
    environment:
      VFS_SERVICE_URL: http://vfs-service:8080
    stdin_open: true
    tty: true
```

### Configuration Management
- `.env` file for tunable parameters: DSNs, S3 credentials, retry policies, timeouts
- Secrets injected via Docker secrets or env vars (stubbed auth for initial implementation)
- Health check endpoints for all services

---

## Resource Limits & Quotas

### Per-Request Limits
- **File upload size**: 100MB (enforced at API gateway)
- **Request timeout**: 30s (standard), 300s (file uploads)
- **OPA policy evaluation**: 200ms timeout
- **Recursion depth**: 100 levels for `ls -r`, `rm -r`

### Per-User Quotas (future enhancement)
- **Rate limiting**: 100 req/min per token (token bucket algorithm)
- **Storage quota**: TBD (sum of file sizes per user/organization)

### System-Wide Limits
- **Event queue depth**: 100,000 (alert if exceeded)
- **Webhook retry queue**: 50,000 (backpressure to API)
- **Dead letter queue**: Manual intervention required
- **Connection pools**: 50 max DB connections per service

### Garbage Collection
- **Idempotency records**: Purge after 24h (background job every hour)
- **File versions**: Retain last 10 versions + current (configurable)
- **Audit logs**: Archive to cold storage after 90 days
- **Orphaned S3 objects**: Weekly scan for s3_keys not in `files` table (async cleanup)
- **Event logs**: Archive completed events >7 days old

---

## Observability & Monitoring

### Metrics (Prometheus format)
- **VFS Service**: Request latency, request count by endpoint, error rate, file upload/download throughput
- **Event Workers**: Queue depth, processing rate, retry rate, dead letter rate
- **Webhook Orchestrator**: Circuit breaker state, delivery success rate, callback rate
- **Scheduler**: Job execution count, lease contention rate, heartbeat failures

### Distributed Tracing (OpenTelemetry)
- Trace ID propagation across services (via `X-Trace-ID` header)
- Spans for: API calls, DB queries, S3 operations, webhook dispatch
- Correlation with request_id for debugging

### Audit Logging
- All mutations logged to `audit_logs` table
- Searchable by: request_id, user_id, resource_id, timestamp
- Includes: IP address, user agent, duration, status

### Alerting Rules
- Event queue depth >80% of limit
- Webhook circuit breaker opened
- Cron lease reaper recovering >5 executions/min
- OPA policy timeout rate >1%
- S3 upload failure rate >5%

---

## Testing Strategy

### Unit Tests
- Standard Go testing per package
- Mock database using `go-sqlmock`
- Table-driven tests for business logic

### Integration Tests (Ginkgo v2 + httpexpect)
```go
var _ = Describe("File Lifecycle", func() {
  It("should handle concurrent file creation with idempotency", func() {
    requestID := uuid.New()

    // Two concurrent requests with same request_id
    resp1 := parallel.Run(func() {
      POST("/files").
        WithHeader("X-Request-ID", requestID).
        WithMultipart().WithFile("file", "test.txt").
        Expect().Status(201)
    })

    resp2 := parallel.Run(func() {
      POST("/files").
        WithHeader("X-Request-ID", requestID).
        WithMultipart().WithFile("file", "test.txt").
        Expect().Status(201)
    })

    // Both should succeed, but only one file created
    Expect(resp1.JSON().Object().Value("id")).To(Equal(resp2.JSON().Object().Value("id")))
  })
})
```

### Test Coverage Areas
1. **Directory/File Lifecycle**: CRUD operations, versioning, soft deletes
2. **Idempotency**: Duplicate request_id handling, TTL expiration
3. **Derivative Handling**: Relation creation, cycle detection, DAG validation
4. **Webhook Dispatch**: Event creation, retry logic, circuit breaker, callbacks
5. **Cron Execution**: Lease acquisition, heartbeat, reaper recovery, skip vs catch-up
6. **CLI Commands**: All command variations, piping, error handling
7. **Tree Locking**: Concurrent directory mutations, race condition prevention
8. **OPA Policies**: Policy evaluation, timeout handling, inheritance
9. **Edge Cases**: File size limits, recursion depth, orphaned resource cleanup

### Test Environment
- Docker Compose stack with real MySQL (not mocks)
- Isolated database per test suite (parallel execution safe)
- Automated via `make test-integration`

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1-2)
- [x] Scaffold project structure, Go modules
- [ ] Define Thrift IDL for VFS Service
- [ ] GORM models for all tables with migrations
- [ ] Docker Compose skeleton (MySQL, LocalStack)
- [ ] Basic health check endpoints
- **Deliverable**: Services start successfully, DB migrations apply

### Phase 2: Core VFS Logic (Week 3-4)
- [ ] Implement VFS Service APIs: CRUD for directories/files
- [ ] File upload/download with S3 integration (gocloud.dev)
- [ ] Idempotency layer with request_id tracking
- [ ] OPA policy integration with timeout protection
- [ ] Tree lock protocol for directory operations
- **Deliverable**: All VFS APIs functional, tested with unit tests

### Phase 3: Event & Webhook System (Week 5-6)
- [ ] Transactional outbox implementation
- [ ] Event worker pool with state machine
- [ ] Webhook orchestrator with circuit breaker
- [ ] Integration with adnanh/webhook daemon
- [ ] Dead letter queue handling
- **Deliverable**: End-to-end webhook delivery with retries

### Phase 4: Cron & Scheduling (Week 7)
- [ ] Scheduler service with lease-based locking
- [ ] Heartbeat mechanism for long-running jobs
- [ ] Lease reaper for stale executions
- [ ] Skip vs catch-up mode support
- **Deliverable**: Multiple schedulers running without duplicate executions

### Phase 5: CLI Gateway (Week 8)
- [ ] REPL framework with command parsing
- [ ] Implement all commands: import, ls, cat, jq, mv, rm, rmdir
- [ ] Piping and streaming support
- [ ] Request ID generation and error handling
- **Deliverable**: Fully functional CLI with all commands

### Phase 6: Testing & Hardening (Week 9-10)
- [ ] Integration test suite (Ginkgo + httpexpect)
- [ ] Load testing for concurrency edge cases
- [ ] Circuit breaker and retry scenario tests
- [ ] Referential integrity validation framework
  - [ ] Add validation checks for all foreign key relationships (parent_id, directory_id, file_id, etc.)
  - [ ] Implement orphan detection jobs (find records with missing parent references)
  - [ ] Create data consistency repair scripts
  - [ ] Add integration tests that verify referential integrity after mutations
  - [ ] Implement periodic integrity check cron job
- [ ] Observability: metrics, tracing, audit logs
- **Deliverable**: 80%+ test coverage, all edge cases verified, referential integrity validated

### Phase 7: Documentation & Polish (Week 11)
- [ ] API documentation (OpenAPI/Swagger)
- [ ] Deployment guide and runbook
- [ ] Architecture decision records (ADRs)
- [ ] Performance tuning and optimization
- **Deliverable**: Production-ready system with documentation

---

## Checkpoint Strategy

- Git tag after each phase: `phase-1-complete`, `phase-2-complete`, etc.
- Each phase includes:
  1. Implementation commit(s)
  2. Test commit(s)
  3. Documentation update commit
  4. Phase summary report (docs/phase-N-report.md)
- Enable rollback and incremental review

---

## Security Considerations

### Authentication & Authorization
- **Service tokens**: JWT-based tokens with expiration (initially stubbed, hardcoded secret)
- **OPA policies**: Pre-compiled on create/update, fail-closed on errors
- **Webhook secrets**: HMAC-SHA256 signature verification (stored in webhook_configs)

### Data Protection
- **At-rest**: MySQL encryption (transparent data encryption)
- **In-transit**: TLS for all HTTP endpoints (self-signed certs in dev, Let's Encrypt in prod)
- **S3 credentials**: Stored in environment variables, rotated quarterly

### Input Validation
- **Path traversal**: Reject paths containing `..`, absolute paths outside root
- **File type validation**: Content-type sniffing vs declared type
- **SQL injection**: GORM parameterized queries only
- **REGO injection**: OPA policy compiled in sandbox, timeout enforced

### Rate Limiting
- Token bucket algorithm: 100 req/min per token
- Exempt health check endpoints
- Return 429 with Retry-After header

---

## Failure Modes & Mitigations

| Failure | Detection | Mitigation |
|---------|-----------|------------|
| MySQL down | Health check fails | Return 503, retry with exponential backoff |
| S3 unreachable | Upload timeout | Return 500, orphaned object GC job cleans up |
| Webhook endpoint down | 5 consecutive failures | Open circuit breaker, alert ops |
| Event queue backlog | Queue depth metric | Throttle API (backpressure), scale workers |
| Cron lease deadlock | Heartbeat timeout | Reaper recovers lease, reschedule |
| OPA policy timeout | 200ms timeout exceeded | Fail-closed (deny), log incident |
| Duplicate request_id | Idempotency check | Return cached response (200 OK) |
| Circular derivative | DAG check | Reject with 400 Bad Request |
| Tree lock timeout | 30s lock wait | Return 409 Conflict, suggest retry |

---

## Future Enhancements

- **Multi-tenancy**: Organization/workspace isolation
- **File search**: Full-text search on JSON fields using MySQL JSON functions
- **Versioned reads**: Query file as-of specific version or timestamp
- **Batch operations**: Bulk upload/download APIs
- **Streaming transformations**: Server-side jq, grep without full download
- **Collaborative editing**: Operational transforms for concurrent edits
- **Geo-replication**: Multi-region MySQL replication with S3 cross-region
- **Advanced auth**: OAuth2, SAML integration
- **Quotas & billing**: Track storage/bandwidth per user/org

---

This document represents the complete architectural specification and will be updated as implementation progresses.

## Project Overview

This document captures the architectural blueprint for a distributed virtual file system (VFS) composed of microservices. The design emphasizes **strong consistency**, **effectively-once idempotent processing**, **horizontal scalability**, and **robust failure handling**.

### Goals
- Provide a virtual file system with a pluggable database backend (initially supporting MySQL and SQLite) capable of tracking directories, files, and derivative relationships.
    - Store files up to 100MB in S3-like blob storage using gocloud.dev.
    - If a file is JSON and less than 14MB, store it as a JSON field in the database for efficient querying.
    - Each file must have a server-validated `content-type` metadata.
- Ensure every mutating operation triggers webhook notifications with at-least-once delivery guarantees and callback tracking.
- Support cron-executed tasks across distributed runners while maintaining consistency via lease-based coordination.
- Offer a REPL-style CLI with import, listing, mutation, and inspection commands, including piping and JSON querying via `jq` semantics.
- Deliver a production-ready deployment using Docker Compose. For local development and testing, provide a simplified setup using SQLite and local file storage for blobs, with end-to-end behavioral tests.
- Design authorization using OPA/REGO policies assigned per directory with fail-closed semantics.

### Non-Goals
- Implement advanced authentication systems beyond basic service token validation.
- Support files larger than 100MB (hard limit enforced).
- Optimize for ultra-low-latency queries; focus is on metadata consistency and correctness.

### Key Constraints
- **Maximum file size**: 100MB (enforced at API layer).
- **Maximum in-database file size**: 14MB (provides a safe buffer below MySQL's `max_allowed_packet` limit).
- **Maximum directory tree depth**: 100 levels (prevents unbounded recursion).
- **Idempotency window**: 24 hours (cleanup after TTL).
- **Webhook retry limit**: 5 attempts with exponential backoff (2s, 4s, 8s, 16s, 32s).
- **OPA policy timeout**: 200ms (fail-closed on timeout).
- **Cron lease duration**: 5 minutes with a 30-second heartbeat requirement.

---

## Architecture Summary

### Microservices

1.  **VFS Service** (Hertz + Thrift)
    - Manages directories, file metadata, file content storage, and lineage tracking.
    - Exposes unified APIs for listing, mutations, uploads, and downloads.
    - Uses GORM to abstract database access, supporting both MySQL (for production) and SQLite (for local testing).
    - Implements a robust tree-locking protocol to ensure directory structure integrity during concurrent operations.
    - Stores small JSON files (<14MB) directly in database JSON columns.
    - Stores larger files in S3-compatible blob storage via gocloud.dev.
    - Emits mutation events transactionally using the outbox pattern.
    - Enforces idempotency via a client-provided `request_id` and a hash of request parameters.
    - Validates OPA policies before directory/file operations with timeout protection.

2.  **Event & Webhook Service**
    - Consumes events from the transactional outbox using a polling worker pool.
    - Manages the entire webhook lifecycle: finds matching webhooks, handles dispatch, and processes retries.
    - Implements a circuit breaker pattern (5 consecutive failures → open circuit for 1 minute) per webhook target.
    - Routes failed jobs to a dead-letter queue after retry exhaustion.
    - Synchronizes active webhook configurations with an `adnanh/webhook` daemon by dynamically generating its configuration file.
    - Provides an endpoint for optional callback acknowledgements.

3.  **Scheduler Service**
    - Runs cron jobs using `robfig/cron/v3` with database-backed schedules.
    - Multiple instances cooperate via lease-based locking (`SELECT ... FOR UPDATE SKIP LOCKED`) to claim jobs atomically.
    - Writes heartbeat timestamps every 30s for long-running jobs to maintain the lease.
    - A background reaper process recovers stale leases (no heartbeat within 2× lease duration).
    - Supports skip-missed-runs vs. catch-up modes per job configuration.

4.  **CLI Gateway**
    - Go-based REPL client using generated Hertz Thrift clients.
    - Implements commands: `import`, `ls`, `ls -r`, `mv`, `rm`, `rmdir`, `cat`, `grep`, `jq <path> <expression>`.
    - Leverages efficient server-side recursive listing for `ls -r`.
    - Supports piping between commands with streaming (no full buffering for large files).
    - Generates a UUIDv4 `request_id` for all mutation commands.
    - Maintains session state (current directory, auth token).

### Database Philosophy

The architecture treats the database primarily as a durable, consistent data store, avoiding complex database-level logic. This philosophy keeps business logic centralized within the application services, making the system more scalable, portable, and easier to test.

-   **No Foreign Keys**: All entity relationships (e.g., parent-child directories, files in directories) are enforced by the application layer within transactions. This provides granular control over consistency checks and simplifies potential future database sharding.
-   **No Triggers or Stored Procedures**: All business logic, including data validation, referential integrity, and event generation (via the transactional outbox pattern), is implemented exclusively within the microservices. This ensures that logic is version-controlled with the application code and remains database-agnostic.

---

## Data Model

The following schema is defined for MySQL. GORM will manage compatibility with SQLite, though some database-specific optimizations (like native JSON columns) may be emulated or handled differently.

### Core VFS Tables

**directories**
```sql
id, parent_id, name, path, version, opa_policy_id, created_at, updated_at, deleted_at
-- The 'path' column is denormalized and must be updated transactionally for all descendants on move/rename.
UNIQUE INDEX (parent_id, name) WHERE deleted_at IS NULL
INDEX (path) -- for tree queries
```

**files**
```sql
id, directory_id, name, content_type, size_bytes, storage_type (json|blob),
json_content (nullable JSON), blob_key (nullable), checksum_sha256, version,
created_at, updated_at, deleted_at
UNIQUE INDEX (directory_id, name) WHERE deleted_at IS NULL
CHECK (size_bytes <= 104857600) -- 100MB limit
CHECK ((storage_type='json' AND json_content IS NOT NULL AND size_bytes <= 14680064) OR (storage_type='blob' AND blob_key IS NOT NULL)) -- 14MB limit for JSON
```

**file_versions**
```sql
id, file_id, version_number, content_type, size_bytes, storage_type,
json_content, blob_key, checksum_sha256, created_at
-- immutable audit trail
```

**file_relations**
```sql
id, parent_file_id, derivative_file_id, relation_type, metadata (JSON), created_at
UNIQUE INDEX (parent_file_id, derivative_file_id)
-- circular dependencies prevented via application-level DAG check before insert
```

### Idempotency & Events

**idempotency_records**
```sql
request_id (UUID, PK), request_hash (SHA256), response_hash, response_body, expires_at, created_at
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
-- execution_key is a hash of job_id + exact scheduled_at timestamp for true idempotency
UNIQUE INDEX (execution_key)
INDEX (status, lease_expires_at) -- for reaper
```

---

## Consistency & Idempotency Strategy

### Strong Consistency Guarantees

1.  **Single-Service Transactions**: All VFS mutations (directory + file + event outbox) occur in a single GORM transaction. Isolation levels (e.g., `REPEATABLE READ` for MySQL) are configured per driver.
2.  **Optimistic Concurrency**: `version` columns are used to prevent lost updates on high-contention resources.
3.  **Tree Lock Protocol**: To prevent parent-child race conditions (e.g., deleting a parent while creating a child), mutations acquire locks on the entire directory ancestry path (`SELECT ... FOR UPDATE`). This is driver-specific and will be implemented for MySQL.

### Idempotency Protocol

-   Client **must** provide `X-Request-ID` header (UUIDv4) for all mutations.
-   VFS Service calculates a hash of the request parameters (`request_hash`).
-   The service checks `idempotency_records` before processing:
    ```
    IF EXISTS(request_id):
      IF request_hash matches stored hash:
        RETURN cached response
      ELSE:
        RETURN 400 Bad Request (request_id reused with different params)
    ELSE:
      BEGIN TRANSACTION
        Perform mutation
        Insert idempotency_record (request_id, request_hash, response, expires_at = NOW() + 24h)
        Insert event into outbox
      COMMIT
      RETURN new response
    ```
-   A background job purges expired idempotency records.

### Transactional Outbox Pattern

-   Events are inserted into the `events` table within the same transaction as the business logic.
-   A `visible_at` delay (e.g., `created_at + 5s`) prevents workers from polling data before the transaction is fully committed and visible across replicas.
-   Workers use `SELECT ... FOR UPDATE SKIP LOCKED` to atomically claim a batch of events, preventing double-processing.

### Distributed Operation Safety

-   **External API Calls (e.g., Blob Storage Upload)**: These cannot be part of a DB transaction. The pattern is:
    1.  Perform the external action first (e.g., upload file to blob storage).
    2.  If successful, start the database transaction to commit the result (e.g., the `blob_key`).
    3.  If the database commit fails, schedule a compensating action (e.g., an async job to delete the orphaned blob object).

---

## Event & Webhook Flow

1.  **Client Request**: A mutation request arrives with an `X-Request-ID`.
2.  **VFS Service Processing**:
    -   The service validates the `request_id` and parameters via the idempotency protocol.
    -   It performs the operation (e.g., creating a file) and inserts an `event` into the outbox table within a single transaction.
3.  **Event & Webhook Service**:
    -   A worker polls the `events` table for new records.
    -   It transactionally marks an event as `processing`.
    -   Outside the transaction, it finds all `webhook_configs` matching the event.
    -   For each matching config, it checks the circuit breaker state. If the circuit is closed, it creates a `webhook_jobs` record and dispatches the webhook.
    -   The service updates the `webhook_jobs` status based on the delivery outcome (2xx, 5xx, timeout).
    -   On failure, it increments the `consecutive_failures` counter on the `webhook_config`. If the threshold is met, it opens the circuit.
    -   If all retries for a job fail, the original event is moved to the `dead_letter` queue.

---

## Cron Processing

### Lease-Based Execution

1.  **Scheduler Polling**: Each scheduler instance polls for active jobs that are due.
2.  **Idempotent Execution**: For a due job, an `execution_key` is generated from the `job_id` and the **exact scheduled timestamp** to ensure that jobs running more frequently than once per minute are uniquely identified.
3.  **Lease Claim**: The scheduler attempts to insert a `cron_executions` record. It then immediately tries to acquire a lock on that record using `SELECT ... FOR UPDATE SKIP LOCKED`. If successful, it owns the lease and executes the job. If not, another scheduler has already claimed it.
4.  **Heartbeating**: During job execution, a background goroutine sends a heartbeat update every 30 seconds to the `cron_executions` table, extending the lease.
5.  **Lease Reaper**: A background task periodically scans for `running` executions where the `heartbeat_at` timestamp is stale (e.g., older than 60 seconds), marks them as `recovered`, and nullifies the lease, making them available for other schedulers to claim.

---

## CLI Behavior

### Command Implementations

-   **`import <local_path> <vfs_path>`**: Streams a local file to the VFS. If `vfs_path` does not exist, it creates a new file. If it exists, this command creates a new version of the file, effectively modifying it. The operation is idempotent and safe under concurrency.
-   **`ls [-r] [path]`**: For the `-r` flag, it makes a single API call to a dedicated server-side endpoint that performs an efficient recursive query (e.g., using a Recursive CTE).
-   **`cat <vfs_path>`**: Streams file content directly from the VFS service to `stdout`. Warns the user before printing binary content.
-   **`jq <vfs_path> <expression>`**: Checks if the file's `content-type` is JSON. If the file is stored in-database (`storage_type='json'`), it uses a specialized API endpoint to apply the `jq` expression on the server side. Otherwise, it streams the file content to a local `jq` process.
-   **`mv <src> <dst>`**: The server handles moves atomically, including the transactional update of all denormalized `path` fields for any descendants of a moved directory.

### Piping Behavior
- Commands are connected via `io.Pipe()` to ensure true streaming with minimal memory footprint, allowing large files to be processed efficiently in a pipeline.

---

## Deployment Plan

### Production Deployment
The production environment is defined via `docker-compose.yml` and includes the full set of services for high availability and scalability.

```yaml
services:
  mysql:
    # ... (production-ready MySQL setup)
  localstack: # Or other S3-compatible service
    # ... (production-ready blob storage)
  vfs-service:
    # ... (unchanged)
  webhook-daemon:
    # ... (unchanged)
  event-webhook-service: # Merged service
    build: ./services/event-webhook
    depends_on:
      vfs-service: { condition: service_healthy }
      webhook-daemon: { condition: service_started }
    environment:
      DB_DSN: "gorm:mysql:..." # DSN for MySQL
      BLOB_URL: "s3://..."
      WORKER_CONCURRENCY: 10
    deploy:
      replicas: 3
  scheduler:
    # ... (unchanged)
  cli:
    # ... (unchanged)
```

### Local Development & Testing
For local development, a `docker-compose.override.yml` file simplifies the stack, enabling fast, dependency-free end-to-end testing.

- **Database**: Uses a file-based SQLite database (`/data/vfs.db`).
- **Blob Storage**: Uses a local file directory (`/data/blobs`).
- This removes the need to run `mysql` and `localstack` containers for most development and testing scenarios.

```yaml
# docker-compose.override.yml
services:
  vfs-service:
    environment:
      DB_DSN: "gorm:sqlite:/data/vfs.db"
      BLOB_URL: "file:///data/blobs"
    volumes:
      - .:/data
  # ... other services adjusted similarly
```

---

## Testing Strategy

- **Unit Tests**: Standard Go tests are used for isolated testing of individual packages, functions, and methods to ensure correctness at the lowest level.
- **Local End-to-End Integration Tests**: The primary mode for rapid development and CI. These are behavioral tests written with Ginkgo v2 and httpexpect that run against the entire application stack.
    - **Backend**: Configured to use **SQLite** for the database and a **local filesystem directory** for blob storage.
    - **Benefits**: This approach provides high-fidelity testing of service interactions without the overhead of managing heavier dependencies like MySQL and S3, enabling fast and reliable test execution on developer machines.
- **Production-Fidelity Integration Tests**: Before a release, the test suite is run against a configuration that mirrors production: using **MySQL** and an **S3-compatible blob storage** (like LocalStack or a dedicated test bucket). This ensures that any database-specific or cloud-service-specific logic is validated.

---

## Security Considerations

### Input Validation
- **Path Traversal**: Reject paths containing `..` or attempting to escape the VFS root.
- **File Type Validation**: The client-provided `Content-Type` header is treated as a hint. The VFS service **must** perform its own content sniffing (e.g., using `http.DetectContentType`) and store the server-validated result as the canonical `content_type`.
- **SQL Injection**: GORM parameterized queries are used exclusively.
- **REGO Injection**: OPA policies are compiled in a sandboxed environment with a strict timeout.

---

## Failure Modes & Mitigations

| Failure | Detection | Mitigation |
|---------|-----------|------------|
| Database down | Health check fails | Return 503 Service Unavailable; clients should retry with exponential backoff. |
| Blob Storage unreachable | Upload/download timeout | Return 500 Internal Server Error. For uploads, an async garbage collection job cleans up any orphaned objects. |
| Webhook endpoint down | 5 consecutive delivery failures | Open the circuit breaker for that endpoint; alert operators. |
| Event queue backlog | `events` table `pending` count metric | Throttle incoming API requests (backpressure); scale `event-webhook-service` replicas. |
| Cron lease deadlock | Heartbeat timeout | The lease reaper recovers the lease, allowing another scheduler to claim the job. |
| OPA policy timeout | 200ms evaluation timeout exceeded | Fail-closed (deny access); log an incident for policy performance review. |
| `request_id` reuse | Idempotency check finds matching `request_id` but different `request_hash` | Reject with 400 Bad Request. |
| Tree lock timeout | Lock wait timeout exceeded (e.g., 30s) | Return 409 Conflict; client should retry the operation. |

---

*Sections on Resource Limits, Observability, Roadmap, and Future Enhancements are largely robust and remain as in the original document, with the understanding that the architectural changes noted above will propagate to their implementation and testing.*

This document represents the complete architectural specification and will be updated as implementation progresses.

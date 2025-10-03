## Project Overview

This document captures the architectural blueprint for the MySQL-based distributed virtual file system (VFS) composed of microservices. The design emphasizes strong consistency, idempotent processing, and horizontal scalability.

### Goals
- Provide a MySQL-backed virtual file system capable of tracking directories, files, and derivative relationships.
- Ensure every mutating operation triggers webhook notifications with idempotency guarantees and callback tracking.
- Support cron-executed tasks across distributed runners while maintaining consistency.
- Offer a REPL-style CLI with import, listing, mutation, and inspection commands, including piping and JSON querying via `jq` semantics.
- Deliver a production-like deployment using Docker Compose and a full end-to-end integration test harness (Ginkgo v2 + httpexpect) backed by real MySQL.

### Non-Goals
- Implement advanced authentication or authorization systems beyond basic service tokens.
- Optimize for massive binary storage; focus is on metadata consistency and API behavior.

## Architecture Summary

### Microservices
1. **Metadata Service** – Hertz + Thrift service that manages directories, file metadata, lineage, and exposes listing/mutation APIs. Uses GORM with MySQL and enforces optimistic versioning with row-level locks. Emits mutation events and enqueues webhook jobs transactionally.
2. **Content Service** – Manages chunked file storage and streaming upload/download APIs. Supports copy-on-write semantics for derivatives by reusing chunk references.
3. **Webhook Orchestrator** – Synchronizes database webhook configurations with an `adnanh/webhook` daemon, dispatches events using persisted idempotency keys, handles callback acknowledgements, and performs retries with exponential backoff.
4. **Scheduler Service** – Runs cron jobs using `robfig/cron/v3` with MySQL-backed schedules. Multiple instances cooperate via `SELECT ... FOR UPDATE SKIP LOCKED` to avoid duplicate executions.
5. **Event Worker Pool** – Processes queued events, drives webhook dispatch, handles retry logic, and performs deferred cleanup or compensation tasks.
6. **CLI Gateway** – Go-based REPL client that talks to services through generated Hertz Thrift clients. Implements commands `import`, `ls`, `ls -r`, `mv`, `rm`, `rmdir`, `cat`, `grep`, `jq <path> <expression>`, and supports piping/redirection.

### Data Model (Key Tables)
- `directories`, `files`, `file_versions`, `file_chunks`, `file_relations` for core VFS state and lineage.
- `events`, `webhook_configs`, `webhook_jobs` (with `idempotency_key`), `cron_jobs`, `cron_executions` for coordination and observability.
- Optional `locks` table for advisory locks if MySQL named locks are insufficient.

### Consistency Strategy
- All state mutations occur within GORM-managed SQL transactions using REPEATABLE READ isolation.
- Idempotency enforced via `request_id` stored on mutations, webhook jobs, and cron executions to avoid double-processing.
- Workers consume events with `FOR UPDATE SKIP LOCKED`, enabling concurrency without duplicates.

### Event & Webhook Flow
1. Client requests mutation with `request_id`.
2. Metadata transaction applies change, inserts event + webhook job (same `request_id`), commits.
3. Event worker dequeues event, hands off to webhook orchestrator.
4. Webhook daemon delivers HTTP POST; callback updates job status via orchestrator’s Hertz endpoint.
5. Failed deliveries increment retry count with exponential backoff and may be routed to a dead-letter table after threshold.

### Cron Processing
- Scheduler polls due `cron_jobs`, locking each row before execution.
- Execution metadata stored in `cron_executions` with derived idempotency key.
- Cron tasks may trigger Metadata APIs or enqueue custom events.

### CLI Behavior
- Maintains session state (current directory) and handles piping by streaming between command handlers.
- `jq <path> <expression>` downloads file content, runs jq expression locally, and streams output for further piping or redirection.
- Uses TLS/auth tokens when interacting with services (stubbed initially).

### Deployment Plan
- Docker Compose orchestrates MySQL, services, webhook daemon, scheduler workers, event workers, and optional CLI container.
- Health checks ensure services wait for MySQL readiness.
- Configurable via `.env` for DSNs, webhook secrets, and cron tuning.

## Testing Strategy
- Unit tests per service (standard Go testing).
- Integration tests using Ginkgo v2 and httpexpect against docker-compose stack.
- Tests cover directory/file lifecycle, derivative handling, webhook dispatch/callback, cron execution, CLI command behavior, and idempotency under retries.

## Implementation Roadmap
1. Phase 1 – Scaffold services, Thrift IDLs, GORM models, docker-compose skeleton.
2. Phase 2 – Implement core business logic, CLI commands, webhook orchestration, cron scheduling.
3. Phase 3 – Build integration test harness with real MySQL and docker-compose automation.
4. Phase 4 – Stabilize, execute full test suite, finalize documentation with phase reports.

Checkpoint commits will mark the completion of each phase, enabling progressive review and rollback if needed.

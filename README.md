# MySQL Virtual File System

This repository contains a reference implementation scaffold for a distributed, policy-aware virtual file system backed by MySQL. The design follows the requirements from the product specification and focuses on composable subsystems:

- **Unified node model** with self-contained metadata and Rego policies.
- **Access control** using on-path policy aggregation and compiled OPA policies.
- **Transactional file operations** orchestrated with GORM.
- **Workflow management** driven by template-defined state transitions.
- **Event and webhook dispatching** with compensation support hooks.
- **Cron scheduler** for directory-level background jobs.
- **Automatic inline/blob storage** backed by the Go Cloud blob abstraction layer.

> **Note**: The implementation focuses on scaffolding core components and providing extensible patterns. Application-specific business logic, CLI commands, and workflow orchestration should extend these foundational packages.

## Getting Started

1. Configure the environment variables for database access and blob storage.
2. Run the Hertz-powered API server:

```bash
MYSQL_VFS_ADDR=":8080" go run ./cmd/server
```

3. Interact with the API using HTTP clients or extend the CLI layer to expose file-system semantics.

## Directory Structure

- `cmd/server`: Entrypoint for the Hertz HTTP server.
- `internal/api`: Minimal HTTP handlers exposing node retrieval.
- `internal/access`: Rego-based access control engine with caching.
- `internal/fs`: File system service handling storage orchestration.
- `internal/workflow`: Workflow state and transition helpers.
- `internal/events`: Webhook dispatcher with configurable retry semantics.
- `internal/template`: Template cache loader.
- `internal/cron`: Cron task scheduler leveraging `robfig/cron`.
- `internal/models`: GORM models with descriptive comments.

## Database Schema

The GORM models define column comments and intentionally omit foreign key constraints and stored procedures, aligning with the specification's database design principles.

## Testing

The project currently exposes compile-time guarantees. Run `go test ./...` to ensure the codebase builds correctly once business logic is added.

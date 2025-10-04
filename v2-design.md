# V2 Design Proposal

## Objectives

- Introduce composable service layers (authorization, validation, auditing, eventing) without tightly coupling them to transport handlers.
- Enable consistent policy enforcement across services (metadata, content, scheduler, webhook) and future extensions.
- Improve observability, testing, and extensibility while keeping the current API surface compatible.
- Preserve developer productivity with better CLI UX, automated validations, and richer test coverage.

## Architectural Layers

### 1. Transport Layer

- **Responsibility**: Protocol handling (HTTP via Hertz, future gRPC/WebSocket adapters).
- **Implementation**: Lightweight handlers that translate transport requests into service calls.
- **Enhancements**:
  - Introduce middleware stack for authentication, request tracing, and rate limiting.
  - Use consistent error envelope / status mapping helpers to avoid duplicate logic.

### 2. Authorization & Policy Layer

- **Responsibility**: Enforce identity, tenancy, ACLs, and contextual policies before domain logic executes.
- **Implementation**:
  - Define `Authorizer` interface with pluggable strategies (RBAC, ABAC, tenant isolation).
  - Add middleware to extract principals (e.g., JWT, API keys) and attach context metadata.
  - Domain services receive `authz.Context` to verify operations against policy rules.

### 3. Validation & Guardrail Layer

- **Responsibility**: Schema validation, business invariants, quota checks, and content validation.
- **Implementation**:
  - Use declarative validators (e.g., JSON schema, custom validators) orchestrated by a `Validator` pipeline.
  - Content service: enforce MIME allowlists, size thresholds, checksum verification before storage.
  - Metadata service: validate path norms, version preconditions, and concurrency limits prior to DB interaction.

### 4. Domain Service Layer

- **Responsibility**: Core domain logic (directories, files, cron jobs, webhooks) using transactional repositories.
- **Enhancements**:
  - Extract repositories per aggregate (`DirectoryRepo`, `FileRepo`, `CronRepo`) with interface-first design.
  - Introduce uniform data-mapper layer for shared patterns (soft-delete, versioning, audit columns).
  - Support dependency injection to facilitate testing/mocking.

### 5. Event Layer

- **Responsibility**: Publish domain events, trigger asynchronous workflows, guarantee idempotency.
- **Implementation**:
  - Use event dispatcher abstraction that records events within transactions and asynchronously forwards them (e.g., to webhook jobs, audit sinks).
  - Add event metadata (actor, source IP, correlation IDs) via context propagation from transport layer.
  - Support plug-ins for additional event sinks (Kafka, SNS, metrics).

### 6. Cross-Cutting Concerns

- **Observability**: Unified logging format, distributed tracing (OpenTelemetry), structured audit logs.
- **Configuration**: Central config registry with schema validation and hot-reload support.
- **Caching**: Optional in-memory cache for frequent lookups (directory path resolution) with invalidation hooks in event layer.
- **Resilience**: Circuit breakers/retry policies for external dependencies (S3, database).

## CLI Enhancements

- **Pluggable command architecture**: Register commands via descriptors with metadata (usage, aliases, autocomplete providers).
- **Layered client stack**: HTTP client with request middleware (auth tokens, retries, tracing headers).
- **Context-aware completion**: Extend current path completion to include remote metadata (e.g., `ls` suggestions) with caching and invalidation.
- **Scriptable mode**: Non-interactive commands and JSON output to integrate with CI/CD.

## Testing Strategy

- **Unit Tests**: Cover each layer in isolation (validators, authorizers, repositories, event dispatchers) with mocks/fakes.
- **Integration Tests**: Expand `citest` to include auth scenarios, validation failures, event delivery, and CLI end-to-end flows.
- **Contract Tests**: Validate external integrations (S3, message buses) via testcontainers.
- **Performance Tests**: Benchmarks for path resolution, large directory listings, concurrent mutations.

## Migration Plan

1. **Infrastructure**: Introduce shared packages (`internal/middleware`, `internal/authz`, `internal/validation`, `internal/eventbus`).
2. **Incremental Refactor**:
   - Wrap existing handlers with new middleware chain.
   - Update services to accept context-enriched requests.
   - Replace inline validations with reusable validators.
3. **Feature Rollout**: Enable authorization policies (read-only roles, tenant isolation) and enforce content validation.
4. **Event Enhancements**: Replace current event processing with the new dispatcher/event bus; extend webhook service accordingly.
5. **CLI Update**: Transition to modular commands and enriched completions.

## Open Questions

- Authentication source of truth (internal user db, external IdP, service tokens?).
- Multi-tenant data isolation strategy (separate schemas vs row-level filters).
- Event persistence retention and replay requirements.
- Versioning strategy for API changes (backwards compatibility guarantees).

## Conclusion

The proposed layered architecture decouples concerns, simplifies future feature development (authorization, validation, auditing), and positions the system for enterprise readiness. By addressing cross-cutting needs early and providing a clear migration plan, we can evolve the platform without disrupting existing clients.

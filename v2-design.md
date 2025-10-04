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

### 2a. User & Group Management

- **Identity Model**: Persist users, groups, and memberships in the metadata service; cache membership claims per request for quick authorization checks.
- **Group Roles**: Ship with built-in groups (`admin`, `editor`, `reviewer`, `viewer`). Allow directories and files to reference group ACLs (owner, read, write) that inherit down the tree unless explicitly overridden.
- **Administrative Guardrails**: Only members of `admin` can create, read, or mutate special policy files (see below), manage group membership, or bypass workflow gates.
- **CLI/REST Support**: Expose endpoints and CLI commands for listing groups, adding/removing users, and impersonation for testing (admin-only).

### 2b. Policy Files and Workflow Inheritance
- **Special Files**: Treat files whose names begin with `.` as policy modules:
  - `.rego` – Open Policy Agent rules that evaluate authorization decisions (e.g., who can update metadata, allowed transitions).
  - `.jsonschema` – JSON Schema documents used to validate inline file content or metadata on create/update.
  - `.workflow` – Declarative workflow DSL describing permitted directory transitions (e.g., `draft -> review -> published`) and gating predicates (metadata flags, approvals).
  - `.user` – Declarative user manifests describing principals scoped to a directory subtree.
  - `.group` – Declarative group manifests describing local groups and memberships.

### 2c. User/Group Manifest Specification

- **File Types**: `.user` and `.group` files live alongside other policy manifests and inherit according to the same scope rules.
- **Common Envelope (optional)**
  - `"scope"`: `"tree"` (default), `"directory"`, or `"file"`.
  - `"inheritance"`: `"cascade"` (default), `"override"`, or `"break"`.
- **`.user` Payload**
  - `"users"`: array of user objects (required).
    - `"id"`: required, non-empty, unique within the manifest (case-insensitive).
    - Optional fields: `"display_name"`, `"email"`, `"groups"` (string array), `"attributes"` (object).
    - Group membership lists are trimmed and deduplicated; blank entries are discarded.
- **`.group` Payload**
  - `"groups"`: array of group objects (required).
    - `"id"`: required, non-empty, unique (case-insensitive).
    - Optional fields: `"display_name"`, `"description"`, `"members"` (string array), `"attributes"` (object).
    - Member lists are normalized and cannot contain duplicates per manifest.
- **Storage Requirements**: `.user` and `.group` must use `inline_json` storage so manifests are embedded in metadata records.
- **Validation**: Loader enforces non-empty manifests, valid JSON, required IDs, uniqueness, and supported manifest types. Scope and inheritance overrides are honored when provided.
- **Resolution**: Policy resolver aggregates principals from applicable manifests, returning both manifest metadata and merged `users`/`groups` in the API response.

### 3. Validation & Guardrail Layer
- **Inheritance Rules**: Policies cascade from the closest ancestor directory to descendants; the effective policy for a path is resolved by walking up the tree until a matching `.rego/.jsonschema/.workflow` is found.
- **Evaluation Flow**:
  1. Resolve applicable `.rego` policy and run authorization checks before domain mutation.
  2. Resolve `.workflow` file to ensure requested operations (create, move, delete) obey transition rules and metadata guards.
  3. Resolve `.jsonschema` to validate incoming file metadata/inline payloads; reject requests that fail schema validation.
- **Storage Semantics**: Policy files are versioned like regular files but restricted to admin access; events emit whenever a policy changes to trigger cache invalidation.
- **Runtime Integration**: The metadata service hosts the policy engine, fetching and caching evaluated modules. Other services delegate authorization/validation via RPC/HTTP to the metadata policy endpoint to keep a single source of truth.

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
- Future policy extensions: `.webhook` (subscription rules), `.retention` (TTL/archival), `.transform` (post-processing), `.quota` (storage/version caps).

## Conclusion

The proposed layered architecture decouples concerns, simplifies future feature development (authorization, validation, auditing), and positions the system for enterprise readiness. By addressing cross-cutting needs early and providing a clear migration plan, we can evolve the platform without disrupting existing clients.

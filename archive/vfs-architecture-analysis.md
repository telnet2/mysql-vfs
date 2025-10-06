# Chapter 7 · Inside the MySQL Virtual File System

## 7.1 A System Shaped Like a File Tree
The MySQL Virtual File System (VFS) feels like a familiar filesystem front-end, yet its roots are firmly planted in database tables and object storage. At its heart sits a deceptively simple promise: treat configuration and binary assets as files while layering enterprise controls—authentication, authorization, validation, and auditing—without forcing those concerns into every caller. To understand how it does so, we need to walk the path a request takes as it flows through the service.

## 7.2 The Layered Spine
The service is organised as a four-tier stack (see `docs/DESIGN.md`):

1. **HTTP/API Layer** – Hertz HTTP handlers translate requests into domain calls. The server in `services/vfs/main.go` wires the dependencies, applies middleware, and exposes CRUD routes.
2. **Middleware Corridor** – Hertz middleware mirrors an HTTP “chain of responsibility.” Authentication (`pkg/middleware/auth.go`) extracts the caller, authorization (`pkg/middleware/authorization.go`) loads `.rego` policies, and the idempotency layer (`pkg/idempotency/service.go`) stops duplicate writes.
3. **Domain Core** – Domain services (`pkg/domain`) encapsulate business rules. `FileService` and `DirectoryService` orchestrate validation, repository operations, versioning, and event emission.
4. **Persistence Layer** – Repository abstractions in `pkg/persistence/db` keep MySQL-specific details quarantined. File bytes may live inline (JSON/text) or in S3 depending on size, while metadata and versions use standard GORM models.

This separation pays dividends: HTTP only knows HTTP; domain logic describes business concerns; repositories decide whether data fits best in a row or a bucket.

## 7.3 Walking the File Lifecycle
Creating a file (`pkg/domain/file_service.go`) reads like a chapter outline:

1. **Preparation** – Content is buffered to compute checksum, size, and to support schema validation. The service builds an `events.OperationContext`, giving every downstream observer a structured view of the operation.
2. **Authorization Stage** – When an `EventTrigger` is configured, synchronous handlers fire (`EmitSync`). A handler can veto the operation—much like an HTTP middleware short-circuiting the request pipeline.
3. **Validation Stage** – Built-in constraints (size caps, name rules) execute first. If a `FilesLoader` is present, we proceed to schema enforcement via `.files` configurations; success and failure both fan out lifecycle events.
4. **Execution Stage** – Within a database transaction, the directory is fetched, uniqueness checks are performed, and special-file rules (like `.owner`) are enforced via `GroupLoader` lookups. Storage placement is decided: JSON/text under 16 MB stays in MySQL; larger or binary blobs move to S3, leaving a key behind. Metadata and the initial `FileVersion` record are inserted atomically.
5. **Completion** – Events summarise the outcome, enabling external systems (webhooks, metrics collectors, auditors) to react asynchronously.

By centralising these stages, the base repository remains a simple CRUD component. The domain service is the “decorator” that layers richer behaviour over basic I/O.

## 7.4 Middleware Outside HTTP
While Hertz middleware guards the HTTP boundary, the project embraces a middleware mindset deeper in the stack:

- **Lifecycle Events** (`pkg/domain/event_trigger.go`) look up `.events` files using `EventsLoader` and dispatch handlers registered in `handlers.Registry`. Patterns and filters decide who runs; synchronous handlers can veto, asynchronous handlers run in a worker pool. This is conceptually identical to applying middleware around domain operations.
- **Special File Loaders** (`pkg/domain/*_loader.go`) act as plug-in validators. `FilesLoader` provides schema enforcement. `PolicyLoader` retrieves `.rego` for authorization. `UserLoader` and `GroupLoader` manage authentication bootstrap. Injecting or omitting a loader changes behaviour without touching the repositories.
- **Idempotency Service** (`pkg/idempotency`) wraps handlers as a pre-processing middleware that records request fingerprints in MySQL and rejects duplicates.

In practice, you can layer additional decorators around `FileService` or `DirectoryService` much like you would stack middleware: loggers, quotas, shadow writes, or rate limiters simply wrap the base interface and either veto or extend behaviour before calling the inner implementation.

## 7.5 Building on Top of a Base Filesystem
Suppose the base repository is your “boring filesystem” that only knows how to store bytes. The architecture provides several techniques to build a feature-rich system on top:

- **Decorator Services** – Define thin interfaces (`CreateFile`, `DeleteFile`, etc.) and wrap them. Each decorator can enforce a concern (quota checks, content moderation, audit logging) before delegating. Because the domain already operates on such interfaces, adding a wrapper is low friction.
- **Policy-Driven Access Control** – `.rego` policies live inside the filesystem, loaded via `PolicyLoader`. Middleware evaluates them before the repository sees any change, meaning access control is a layer atop the base store, not baked into it.
- **Schema Validation** – `.files` rules let administrators associate glob/regex patterns with JSON schemas. Validation happens before calling storage, and the same hooks emit events detailing the decision.
- **Lifecycle Hooks** – `.events` enables downstream systems to subscribe to event types (`file.create.*`, `directory.delete.*`) with filters. Handlers can be synchronous (veto) or asynchronous (observe), providing a middleware-like interception model that operates even if the request didn’t originate from HTTP.

Together these pieces transform a rudimentary file store into a policy-aware, schema-enforcing, audit-ready platform—without diluting the single responsibility of the base filesystem code.

## 7.6 Adding Validation or Access Control “Middleware”
The pattern for injecting new validation is straightforward:

1. **Surface Configuration** – Introduce a special file (e.g., `.quota`) or reuse `.files` to declare rules.
2. **Loader** – Implement a loader that reads and caches the configuration using existing repository interfaces.
3. **Decorator or Event Hook** – Either wrap `FileService` to run the loader prior to storage, or create lifecycle handlers that inspect the new configuration. Veto-capable handlers allow you to abort operations cleanly.
4. **Feedback Loop** – Emit outcome events so operators can observe and alert on validation failures.

Because `FileService` already emits structured validation events (e.g., `file.create.validation.schema.failed`), adding another “validation middleware” is as easy as registering a handler or extending the service with another injected dependency.

Access control follows the same blueprint: the authorization middleware fetches `.rego` policies, and the domain layer re-emits authorization events so that external systems or automated tests can verify policy decisions.

## 7.7 Event Triggers as the Nervous System
Events are not hardwired into the base repositories—they are optional components supplied to the domain services. The `LifecycleEventTrigger` pre-resolves directory context, matches patterns, and runs handlers. If absent, the domain silently performs CRUD with no side effects. This design keeps the “base filesystem” lean while allowing operators to opt into rich behaviour by providing `.events` descriptors and registering handlers such as:

- **Webhooks** for real-time notifications (`pkg/events/handlers/webhook_handler.go`).
- **Logging** for audit trails.
- **Metrics** for observability platforms.

The same mechanism can carry new governance features: e.g., a handler that samples payloads for compliance or triggers asynchronous scans.

## 7.8 A Glance at Linux VFS
Linux offers a useful mental model. The kernel’s Virtual File System exposes generic inode and dentry operations; specific filesystems (ext4, XFS, NFS) register their handlers, and stackable layers (OverlayFS, eCryptfs) intercept calls to add features. Caches, journals, and drivers each live in their own layers. The MySQL VFS mirrors this philosophy:

- **VFS API** ↔ Domain service interfaces.
- **File System Modules** ↔ Repositories and storage backends (MySQL metadata, S3 content).
- **Stackable Layers** ↔ Middleware decorators, lifecycle events, loaders.
- **Kernel Events / inotify** ↔ Lifecycle event handlers and webhooks.

Understanding this parallel helps when designing new features: treat each concern as a layer that can intercept, enrich, or veto operations, rather than modifying the core store.

## 7.9 Where to Go Next
- Introduce new decorators (quotas, retention policies) that wrap `FileService` and publish their own events.
- Expand `.events` handlers with business-specific logic—approval workflows, change tickets, or security scans.
- Harden `.rego` policy evaluation by integrating with the OPA runtime and adding policy test suites.

By thinking of the MySQL VFS as a composable stack rather than a monolith, you unlock a scalable path to evolve from basic storage into a governance-rich platform.

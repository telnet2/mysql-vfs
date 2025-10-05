# Performance & Concurrency Enhancement Proposals

This document captures the main hotspots we observed while reviewing MySQL VFS with a focus on runtime concurrency, safety and throughput. Each item highlights the underlying issue, a concrete proposal, expected benefits, and notes on implementation effort or risk.

## 1. Harden Directory Tree Concurrency
**Issue.** Directory operations currently rely on optimistic retries backed by the unique `path_hash`, but transactions do not lock ancestor rows. Concurrent creates/deletes under the same parent can interleave, forcing retries and occasionally surfacing user-visible conflicts. The existing `LockPaths` helpers in the repositories are unused.

**Proposal.** Introduce hierarchical locking for directory transactions:
- Acquire scoped locks on the parent chain (root → leaf) using `FOR UPDATE` before mutating state.
- Provide a lightweight lock manager wrapper so both directory and file services can reuse the strategy.
- Retain optimistic fallbacks for high-contention cases, but prefer deterministic locking first.

**Impact.** Reduces retry churn and eliminates windows where a parent disappears mid-operation. Also tightens safety for cascading deletes. Moderate effort: repository changes plus service-level integration and additional tests.

## 2. Streamline File Ingest / Update Path
**Issue.** `FileService` buffers entire payloads in memory to calculate checksums, run validation, and decide storage tier. With the 100 MB limit this is survivable but expensive under concurrency and blocks GC.

**Proposal.** Move to a streaming ingest pipeline:
- Use `io.TeeReader` to hash while writing to a temporary file or directly into S3.
- Feed schema validation with bounded buffers (chunked JSON validation or temporary files) rather than a single giant byte slice.
- Reuse the buffered content when persisting versions to avoid double reads.

**Impact.** Flattens memory spikes, improves throughput under concurrent large uploads, and reduces tail latency. Effort moderate/high due to refactoring validation and storage interfaces.

## 3. Asynchronous Event Handling Backpressure
**Issue.** Lifecycle events rely on an in-process worker pool per trigger (default 10). Bursts of directory/file activity can overflow the pool and leave async work unobserved. Failure scenarios (handler timeouts, retries) are only logged.

**Proposal.** Introduce a dedicated event queue with visibility into backlog:
- Swap the raw goroutine pool for a bounded channel backed by a simple queue + monitoring metrics (queue depth, oldest age).
- Optionally persist async work to the existing `events` table and drive delivery through a background worker (or existing orchestrator service) for durability.
- Expose configuration knobs (pool size, retry policy, circuit-breaker metrics) via config file / ENV.

**Impact.** Better backpressure semantics, fewer dropped events, clearer operations view. Effort moderate; durability option requires coordination with the event worker services.

## 4. Smarter Cache Invalidation for `.events` / `.files`
**Issue.** Loaders use TTL-based caches (`sync.Map`) but only invalidate the touched directory; children rely on expiry. Inheritance-heavy trees risk serving stale configs for minutes.

**Proposal.** Track simple parent→children relationships while loading configs and fan out invalidation when parents change. For safety, mirror metadata in-memory (e.g., `map[parent][]childID`) guarded by the loader lock.

**Impact.** Ensures handler/schema updates propagate immediately without reducing TTL. Low-to-moderate effort.

## 5. Observability & Chaos Testing
**Issue.** Current logs capture handler failures but we lack structured metrics for contention or async backlog. Concurrency regressions are hard to detect before production.

**Proposal.**
- Emit RED metrics for directory/file operations (rate, errors, duration), async handler queue stats, and DB retry counters.
- Add chaos tests that simulate concurrent create/delete/update bursts and assert bounded retries and latency.
- Bake smoke scenarios into `citest` (using httptest for handlers, SQLite or MySQL with constrained resources) to catch regressions in CI.

**Impact.** Early detection of concurrency regressions, easier capacity planning. Low effort once metrics pipeline is in place.

---

These improvements can be staged. We recommend starting with directory locking (highest correctness win) and cache invalidation (quick impact), then tackling the streaming ingest (higher ROI but larger refactor) and event queueing. Observability work should thread through all phases.

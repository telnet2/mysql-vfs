# Event Trigger & Handler System Design

**Status:** Draft  
**Owner:** Droid (Factory AI)

## 1. Goals

- Enable directories to declare rich, versioned event workflows without code changes.
- Support core lifecycle triggers (create, update, delete) and extension signals (authorization, validation, workflow progression).
- Provide a pluggable action pipeline (emit, enqueue, invoke workflow/webhook) that reuses existing event infrastructure.
- Preserve policy semantics: inheritance, scope, and caching consistent with `.rego` and `.jsonschema` manifests.
- Keep the system observable, testable, and resilient to handler failures.

## 2. Non-Goals

- Implement a full workflow engine or guarantee exactly-once delivery.
- Replace existing `events` table or scheduler delivery loops.
- Provide arbitrary scripting language execution inside manifests in v1.

## 3. Background

Metadata service already persists internal events (e.g., `file.created`) for downstream workers. Extensions (authorization via `.rego`, validation via `.jsonschema`) run inline but cannot emit custom follow-up actions. We need a declarative layer that lets product teams hook domain-specific reactions (audit logs, webhooks, async workflows) per directory.

## 4. Requirements

1. **Declarative triggers** – Directory-scoped manifest describes when to react and which action(s) to run.
2. **Inheritance-aware** – Manifests follow directory hierarchy rules, aligning with existing policy registry.
3. **Context-rich inputs** – Handlers receive actor, file metadata, validation results, etc.
4. **Transactional safety** – Trigger evaluation happens within service transactions; actions are queued atomically.
5. **Replayable** – Events should be idempotent via stable request IDs.
6. **Observability** – Record why triggers fired and handler outcomes.

## 5. Proposed Solution

### 5.1 `.events` Manifest

Introduce a new special file `.events` stored alongside other policy manifests. Payload schema (JSON):

```json
{
  "triggers": [
    {
      "on": "file.created",
      "scope": "directory",            // optional override (directory | tree | file)
      "match": {
        "file_name": "*.profile.json", // glob
        "storage_mode": "inline_json"  // exact match
      },
      "conditions": {
        "rego": "input.metadata.environment == \"prod\""
      },
      "actions": [
        {
          "type": "emit_event",
          "event_type": "profile.created",
          "payload_template": "templates/profile_created.json"
        },
        {
          "type": "invoke_workflow",
          "workflow": "onboard"
        }
      ]
    }
  ]
}
```

Key concepts:

- `on`: Built-in trigger names (`file.created`, `file.updated`, `file.deleted`, `validation.failed`, `authorization.denied`, `workflow.transitioned`, etc.).
- `match`: Simple attribute filters. v1 supports glob patterns for `file_name`, `path`, exact match for `directory_id`, `storage_mode`, boolean flags (e.g., `is_policy_file`).
- `conditions`: Optional Rego expression executed via existing evaluator for advanced checks. Executes with same directory manifests and trigger context.
- `actions`: Ordered list executed sequentially; each action can emit internal events or schedule handlers.

### 5.2 Registry Integration

- Extend `policy.Type` with `TypeEvents` and mark `.events` as special file.
- Update manifest repository to parse `.events` payload into Go structs `TriggerManifest`.
- Registry caches compiled triggers similar to evaluator and validator caches, invalidated on directory changes.

### 5.3 Trigger Evaluation Pipeline

1. **Context assembly** – When core services (e.g., `FileService.Create`) reach commit-ready state, build `TriggerContext` containing request/actor, directory/file metadata, validation/authorization results, diff info, and request ID.
2. **Manifest resolution** – Use registry to fetch `.events` manifests for directory (with inheritance and scope filtering).
3. **Filtering** – Iterate triggers, evaluate `match` arrays first. If `conditions.rego` exists, reuse policy evaluator to run `allow` query with context as input (new query `data.events.match` to avoid clashing with file policies).
4. **Action compilation** – For each triggered action, convert declarative spec into `QueuedAction` struct referencing templates/workflows.

### 5.4 Action Execution Strategy

- **Emit Event**: Create record in `events` table via existing `persistEvent`, using namespaced event types (e.g., `ext.profile.created`) and payload built from templates.
- **Invoke Workflow**: Enqueue `workflow.triggered` event with metadata referencing `.workflow` manifest to evaluate in scheduler.
- **Call Webhook**: Emit event consumed by webhook service, reusing existing router.
- **Run Command (future)**: Placeholder for process orchestration (out of scope for v1).

Actions run asynchronously—service only ensures they are persisted. Scheduler workers pick them based on `event_type` and dispatch to specialized executors (webhook, workflow, audit logger). Each executor is updated to interpret new action metadata.

### 5.5 Persistence & Idempotency

- Use composite key `(request_id, event_type, action_hash)` to deduplicate inserted actions.
- Store additional columns in `events` payload: manifest reference, trigger name, evaluation context digest.
- Provide `source` field (e.g., `"source": "extension"`) for observability.

### 5.6 Failure Handling

- If action enqueue fails, abort transaction; caller receives 500 ensuring consistency.
- Scheduler keeps retry semantics; include backoff policy per action via manifest (optional `retry` block).
- Expose metrics/logging for trigger evaluation time, number of actions queued, and failures.

### 5.7 Security & Permissions

- `.events` manifests are admin-only files like other policy artifacts.
- Rego condition execution sandboxed by OPA; no arbitrary command execution.
- Payload templates stored under same directory and validated to prevent secret leakage (future enhancements: allow referencing `content` service objects by ID).

### 5.8 Performance Considerations

- Cache compiled triggers per directory; reuse existing invalidation path after `.events` updates or parent directory modifications.
- Limit per-request triggers (config with default max) to avoid explosion.
- Lazy load optional templates from content service using cached handles.

## 6. Alternate Approaches Considered

| Approach | Pros | Cons |
| --- | --- | --- |
| Hard-code event hooks in Go | Simple initial implementation | Requires code deploys for every new behavior; violates extensibility goal |
| Store triggers in SQL tables | Easy querying | Breaks versioning and inheritance semantics tied to directory manifests |
| Use pure Rego for actions | Powerful expressiveness | High complexity, harder to secure; less discoverable for operators |

## 7. Rollout Plan

1. Implement manifest parsing and registry caching for `.events`.
2. Add trigger evaluation in `FileService` (create/update/delete) and validation/authorization code paths.
3. Provide action enqueue engine writing to existing `events` table.
4. Update scheduler/webhook/workflow workers to recognize new `ext.*` event types.
5. Add end-to-end tests covering valid/invalid `.events` manifests and trigger execution.
6. Document manifest schema and provide examples for app teams.

## 8. Open Questions

- Should we support synchronous actions (e.g., block request) in v1 or keep everything async?
- Do we need a dedicated payload templating language, or is referencing stored JSON sufficient?
- How to surface trigger execution results back to clients (audit logs/API)?

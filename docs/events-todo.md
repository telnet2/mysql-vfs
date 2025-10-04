# Event System Implementation TODOs

- [ ] **Manifest Infrastructure**
  - [ ] Add `TypeEvents` constant and `.events` entry to manifest registry.
  - [ ] Parse `.events` payload into typed structs with validation (schema parsing, glob validation).
  - [ ] Extend registry caching/invalidation to include event triggers.

- [ ] **Trigger Evaluation Engine**
  - [ ] Define `TriggerContext` structure populated during file/directory operations.
  - [ ] Implement match evaluation (file name/path globs, storage mode, actor filters).
  - [ ] Integrate optional Rego condition execution with existing evaluator sandbox.

- [ ] **Action Enqueue Pipeline**
  - [ ] Implement action compiler that converts manifest actions into queueable items.
  - [ ] Extend `persistEvent` (or new helper) to store extension actions with idempotency hash.
  - [ ] Surface debug logging/metrics for triggered actions.

- [ ] **Service Integration**
  - [ ] Invoke trigger engine from `FileService.Create/Update/Delete` after validation succeeds.
  - [ ] Emit `validation.failed` / `authorization.denied` triggers with relevant context.
  - [ ] Ensure triggers fire for directory operations where applicable.

- [ ] **Worker/Dispatcher Updates**
  - [ ] Teach scheduler workers to route `ext.*` events to appropriate executors.
  - [ ] Define initial executor set (emit core event, invoke workflow, call webhook).
  - [ ] Add retry/backoff strategy configurable via manifest.

- [ ] **Testing & Tooling**
  - [ ] Unit tests for manifest parsing, trigger filtering, and action compilation.
  - [ ] Integration tests covering `.events` inheritance and runtime execution.
  - [ ] CLI tooling or admin endpoint to preview resolved triggers for a directory.

- [ ] **Documentation & Rollout**
  - [ ] Author manifest schema reference with examples.
  - [ ] Provide migration guide for teams adding `.events` files.
  - [ ] Define monitoring/alerting dashboards for trigger failures.

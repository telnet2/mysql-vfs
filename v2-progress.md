# V2 Implementation Progress

> For every subtask below, add focused unit/integration tests and ensure `go test ./...` passes before moving to the next item.

- [ ] Build configurable policy resolver (rego/jsonschema/workflow registry)
  - [x] Define policy module manifest structure (type, scope, inheritance rules)
  - [x] Implement registry and loader that walks directory ancestry
  - [x] Add caching/invalidation tied to policy file events
  - [x] Expose metadata service endpoint for policy resolution
- [ ] Implement user/group management API + CLI commands
  - [x] Support `.user` / `.group` policy manifests with principal parsing
  - [x] Expose resolved principals via metadata policy endpoint
  - [x] Wire CLI commands for inspecting principals and memberships
- [ ] Enforce admin-only access for dot-policy files
  - [x] Restrict metadata service mutations on `.rego`/`.jsonschema`/`.workflow`/`.user`/`.group`
  - [x] Prevent CLI operations on policy files unless actor is admin
  - [x] Integrate policy enforcement into e2e tests
- [x] Integrate `.rego` authorization evaluation
- [x] Integrate `.jsonschema` content validation
- [ ] Implement `.events` trigger system  
  - [x] Parse `.events` manifests via policy registry with caching/invalidation
  - [x] Evaluate triggers (match filters + optional Rego conditions)
  - [x] Enqueue actions into events pipeline with idempotency metadata
  - [x] Update workers to dispatch `ext.*` actions (workflow/webhook emitters)
  - [x] Add unit tests covering trigger execution and failure paths
  - [x] Add e2e tests covering trigger execution and failure paths
<!-- - [ ] Integrate `.workflow` transition enforcement -->
<!-- - [ ] Add `.webhook` policy evaluation (optional future) -->
<!-- - [ ] Add `.retention` / `.transform` / `.quota` policy handlers (optional future) -->
- [ ] Update webhook service to consume resolved policies
<!-- - [ ] Expand citest coverage for policy inheritance and workflows -->

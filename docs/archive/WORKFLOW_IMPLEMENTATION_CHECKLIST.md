# Workflow System - Implementation Checklist

**Status:** Planning
**Architecture:** Directory-as-State with Rego-based Gates
**Last Updated:** 2025-10-06

---

## Quick Reference

- **Design Doc:** `archive/workflow-design.md`
- **Implementation Plan:** `archive/workflow-plan.md`
- **Examples:** `archive/workflow-rego-gates-example.md`
- **Tests:** `citest/e2e_workflow_test.go`

---

## Phase 1: Models & Database (Optional - Audit Only)

### 1.1 Workflow Audit Model
- [ ] Create `pkg/models/workflow_audit.go`
  - [ ] Define `WorkflowAudit` struct with fields:
    - `ID`, `FilePath`, `WorkflowPath`
    - `FromState`, `ToState`, `Operation`
    - `Actor`, `ActorGroups`, `GatesEvaluated`
    - `Success`, `ErrorMessage`, `CreatedAt`
  - [ ] Add GORM tags

### 1.2 Database Migration
- [ ] Create migration in `pkg/persistence/db/mysql/migrate.go`
  - [ ] Add `workflow_audit` table
  - [ ] Add indexes:
    - `idx_file_path (file_path)`
    - `idx_workflow_path (workflow_path)`
    - `idx_actor (actor)`
    - `idx_created_at (created_at)`
  - [ ] Test migration up
  - [ ] Test migration down
  - [ ] Test migration idempotency

---

## Phase 2: Workflow Special File & Loader

### 2.1 Register `.workflow` Special File
**File:** `pkg/domain/special_files.go`

- [ ] Add constant `SpecialFileTypeWorkflow = ".workflow"`
- [ ] Add to `SpecialFileRegistry`:
  ```go
  SpecialFileTypeWorkflow: {
      Name:              SpecialFileTypeWorkflow,
      Description:       "Workflow definition - state machine for files",
      ContentType:       "application/x-yaml",
      AdminOnly:         false,
      ValidateFunc:      validateWorkflowConfig,
      InheritFromParent: false,
  }
  ```

### 2.2 Implement Workflow Validation
**File:** `pkg/domain/special_files.go` (add function)

- [ ] Implement `validateWorkflowConfig(content []byte) error`
  - [ ] **Step 1:** Parse YAML to struct
  - [ ] **Step 2:** Validate against JSON schema (see workflow-plan.md section 2.1.1)
  - [ ] **Step 3:** Validate state names match pattern `^[a-z0-9][a-z0-9_-]*$`
  - [ ] **Step 4:** Validate state directory paths
    - Max 5 levels deep
    - Pattern: `^[a-zA-Z0-9_/-]+$`
    - No `.` or `..` components
  - [ ] **Step 5:** Check reference integrity
    - `initial_state` exists in `states`
    - All `state_directories` keys exist in `states`
    - All transition `to` states exist in `states`
    - No orphaned states
  - [ ] **Step 6:** Validate no nested workflows
    - Check no parent `.workflow` exists
    - Check no child `.workflow` files exist
  - [ ] **Step 7:** Validate state directories exist on filesystem
  - [ ] **Step 8:** Validate gate policy
    - If `gate_policy`: validate Rego syntax
    - If `gate_policy_ref`: check file exists
    - Cannot have both
    - Policy must define package `vfs.workflow.gates`
  - [ ] Return structured errors with error codes

- [ ] Add error constants:
  ```go
  ErrInvalidYAML
  ErrSchemaViolation
  ErrInvalidStateName
  ErrInvalidStatePath
  ErrInitialStateNotFound
  ErrTransitionStateNotFound
  ErrStateDirectoryNotFound
  ErrOrphanedState
  ErrNestedWorkflow
  ErrInvalidGatePolicy
  ErrGatePolicyNotFound
  ErrBothGatePolicies
  ```

### 2.3 Create Workflow Loader
**New File:** `pkg/domain/workflow_loader.go`

- [ ] Define data structures:
  ```go
  type WorkflowDefinition struct {
      WorkflowPath      string
      WorkflowHome      string
      StateDirectories  map[string]string
      InitialState      string
      States            map[string]StateDefinition
      GatePolicy        string
      GatePolicyRef     string
  }

  type StateDefinition struct {
      Transitions []TransitionDefinition
  }

  type TransitionDefinition struct {
      To          string
      Description string
  }
  ```

- [ ] Implement `WorkflowLoader` struct
  - [ ] Use `GenericLoader` pattern (5-min TTL cache)
  - [ ] Add `fileRepo` and `dirRepo` dependencies

- [ ] Implement methods:
  - [ ] `NewWorkflowLoader(fileRepo, dirRepo, cacheTTL) *WorkflowLoader`
  - [ ] `LoadForPath(ctx, filePath) (*WorkflowDefinition, error)` - walk up tree
  - [ ] `GetCurrentState(workflow, filePath) (string, error)` - extract from path
  - [ ] `IsStateDirectory(workflow, dirPath) bool`
  - [ ] `GetStateDirectoryPath(workflow, stateName) (string, error)`
  - [ ] Cache invalidation on `.workflow` file update/delete

### 2.4 Unit Tests
**New File:** `pkg/domain/workflow_loader_test.go`

- [ ] Test YAML parsing (valid/invalid)
- [ ] Test state name validation
- [ ] Test state directory path validation
- [ ] Test reference integrity checks
- [ ] Test nested workflow detection
- [ ] Test state extraction from file paths
- [ ] Test cache behavior (hit/miss/invalidation)
- [ ] Test all error codes

---

## Phase 3: Workflow Engine & Gate Evaluator

### 3.1 Create Workflow Engine
**New File:** `pkg/domain/workflow_engine.go`

- [ ] Define `WorkflowEngine` struct:
  ```go
  type WorkflowEngine struct {
      workflowLoader    *WorkflowLoader
      gateEvaluator     *WorkflowGateEvaluator
      fileRepo          db.FileRepository
      dirRepo           db.DirectoryRepository
      auditRepo         *WorkflowAuditRepository  // optional
  }
  ```

- [ ] Implement validation methods:
  - [ ] `ValidateMoveOperation(ctx, sourcePath, destPath, actor, actorGroups, metadata) error`
    - Extract fromState and toState from paths
    - Check transition exists in workflow definition
    - Call gate evaluator
    - Block if outside workflow scope
    - Allow same-state moves
  - [ ] `ValidateCreateOperation(ctx, filePath, actor, actorGroups) error`
    - Check file is in initial_state directory
  - [ ] `ValidateDeleteOperation(ctx, filePath, actor, actorGroups, metadata) error`
    - Check deletion allowed via gate policy
  - [ ] `ValidateDirectoryOperation(ctx, dirPath, operation) error`
    - Block rename/delete of state directories
  - [ ] `GetValidTransitions(ctx, filePath, actor, actorGroups) ([]string, error)`
    - Return list of states user can transition to

- [ ] Add system-admin bypass:
  - [ ] Check if `system-admin` in `actorGroups`
  - [ ] Return early (allow operation)

- [ ] Add audit logging:
  - [ ] Write `WorkflowAudit` record for each operation
  - [ ] Log success/failure
  - [ ] Log gates evaluated

### 3.2 Create Gate Evaluator (Rego-based)
**New File:** `pkg/domain/workflow_gates.go`

- [ ] Define structures:
  ```go
  type WorkflowGateEvaluator struct {
      workflowLoader *WorkflowLoader
      fileRepo       db.FileRepository
      queryCache     *sync.Map
      cacheTTL       time.Duration
  }

  type WorkflowGateInput struct {
      User       UserInfo
      Transition TransitionInfo
      File       FileInfo
      Workflow   WorkflowInfo
  }

  type CachedRegoQuery struct {
      Query    rego.PreparedEvalQuery
      LoadedAt time.Time
  }
  ```

- [ ] Implement methods:
  - [ ] `NewWorkflowGateEvaluator(loader, fileRepo, cacheTTL)`
  - [ ] `EvaluateGate(ctx, workflow, input) (bool, error)`
    - Get Rego policy (inline or external)
    - Check query cache (5-min TTL)
    - Compile Rego if cache miss
    - Store compiled query in cache
    - Execute query with input
    - Return true/false
  - [ ] `evaluateWithQuery(ctx, query, input) (bool, error)`
    - Execute `data.vfs.workflow.gates.allow`
    - Handle errors (fail closed)
  - [ ] `InvalidateCache(workflowPath)`
    - Remove from cache on `.workflow` or `.workflow.rego` update

- [ ] Build rich input structure:
  - [ ] `user`: id, username, groups
  - [ ] `transition`: from, to, operation
  - [ ] `file`: path, name, metadata, content (parsed JSON), size, mime_type
  - [ ] `workflow`: name, workflow_home, initial_state, available_states

- [ ] Handle inline vs external policies:
  - [ ] If `workflow.GatePolicy` is set → use inline
  - [ ] If `workflow.GatePolicyRef` is set → load from file
  - [ ] Cache both types

### 3.3 Unit Tests
**New File:** `pkg/domain/workflow_engine_test.go`

- [ ] Test `ValidateMoveOperation`:
  - [ ] Valid transition with passing gates
  - [ ] Invalid transition (not in definition)
  - [ ] Valid transition with failing gates
  - [ ] Move outside workflow scope (blocked)
  - [ ] Same-state move (allowed)
  - [ ] system-admin bypass

- [ ] Test `ValidateCreateOperation`:
  - [ ] Create in initial_state (allowed)
  - [ ] Create in non-initial state (blocked)

- [ ] Test `ValidateDeleteOperation`:
  - [ ] Delete with gate approval
  - [ ] Delete with gate denial

- [ ] Test `ValidateDirectoryOperation`:
  - [ ] Rename state directory (blocked)
  - [ ] Delete non-empty state directory (blocked)
  - [ ] Delete empty state directory (allowed)

**New File:** `pkg/domain/workflow_gates_test.go`

- [ ] Test Rego policy compilation
- [ ] Test gate evaluation (allow/deny)
- [ ] Test query caching
- [ ] Test cache invalidation
- [ ] Test inline vs external policies
- [ ] Test input structure building
- [ ] Test JSON content parsing
- [ ] Test error handling

---

## Phase 4: Service Layer Integration

### 4.1 Integrate into FileService
**File:** `pkg/domain/file_service.go`

- [ ] Add `WorkflowEngine` to `FileService` struct
- [ ] Inject via constructor

- [ ] Update `CreateFile()`:
  ```go
  // WORKFLOW VALIDATION (before authorization)
  if err := s.workflowEngine.ValidateCreateOperation(
      ctx, path, user, getUserGroups(user),
  ); err != nil {
      // Emit event: workflow.create.blocked
      return fmt.Errorf("workflow validation failed: %w", err)
  }
  ```

- [ ] Update `MoveFile()`:
  ```go
  // Load file metadata for gates
  file, _ := s.GetFile(ctx, sourcePath)

  // WORKFLOW VALIDATION (before authorization)
  if err := s.workflowEngine.ValidateMoveOperation(
      ctx, sourcePath, destPath, user, getUserGroups(user), file.Metadata,
  ); err != nil {
      // Emit event: workflow.transition.failed
      return fmt.Errorf("workflow validation failed: %w", err)
  }

  // Proceed with move...
  // Emit event: workflow.transition.succeeded
  ```

- [ ] Update `DeleteFile()`:
  ```go
  // Load file metadata
  file, _ := s.GetFile(ctx, path)

  // WORKFLOW VALIDATION
  if err := s.workflowEngine.ValidateDeleteOperation(
      ctx, path, user, getUserGroups(user), file.Metadata,
  ); err != nil {
      // Emit event: workflow.deletion.blocked
      return fmt.Errorf("workflow validation failed: %w", err)
  }
  ```

### 4.2 Integrate into DirectoryService
**File:** `pkg/domain/directory_service.go`

- [ ] Add `WorkflowEngine` to `DirectoryService` struct
- [ ] Inject via constructor

- [ ] Update `RenameDirectory()`:
  ```go
  // WORKFLOW VALIDATION
  if err := s.workflowEngine.ValidateDirectoryOperation(
      ctx, oldPath, "rename",
  ); err != nil {
      // Emit event: workflow.state_dir.rename.blocked
      return err
  }
  ```

- [ ] Update `DeleteDirectory()`:
  ```go
  // WORKFLOW VALIDATION
  if err := s.workflowEngine.ValidateDirectoryOperation(
      ctx, path, "delete",
  ); err != nil {
      // Emit event: workflow.state_dir.delete.blocked
      return err
  }
  ```

### 4.3 Integration Tests
**File:** `citest/e2e_workflow_test.go` (already exists)

- [ ] Add tests for service layer integration
- [ ] Test workflow validation happens before authorization
- [ ] Test events are emitted correctly
- [ ] Test audit logs are created

---

## Phase 5: Event System Integration

### 5.1 Add Workflow Event Types
**File:** `pkg/events/lifecycle_types.go`

- [ ] Add constants:
  ```go
  EventWorkflowTransitionStarted   = "workflow.transition.started"
  EventWorkflowTransitionSucceeded = "workflow.transition.succeeded"
  EventWorkflowTransitionFailed    = "workflow.transition.failed"
  EventWorkflowDeletionBlocked     = "workflow.deletion.blocked"
  EventWorkflowEscapeBlocked       = "workflow.escape.blocked"
  EventWorkflowStateDirProtected   = "workflow.state_dir.protected"
  EventWorkflowCreateBlocked       = "workflow.create.blocked"
  ```

### 5.2 Add `move_file` Action Type (Optional)
**Files:** `pkg/domain/events_loader.go`, `pkg/domain/event_dispatcher.go`

- [ ] Define `MoveFileAction` struct:
  ```go
  type MoveFileAction struct {
      Type             string
      TargetState      string
      PreserveStructure bool  // default: true
  }
  ```

- [ ] Implement action handler:
  - [ ] Load workflow for file
  - [ ] Get current state from file path
  - [ ] Construct destination path from target state
  - [ ] Call `FileService.MoveFile()`
  - [ ] Handle errors (emit workflow.transition.failed)

- [ ] Example `.events` usage:
  ```json
  {
    "handlers": [{
      "name": "auto-approve-small-files",
      "events": ["file.create.succeeded"],
      "type": "move_file",
      "config": {
        "target_state": "approved",
        "preserve_structure": true
      },
      "condition": "input.file.size < 1000000"
    }]
  }
  ```

---

## Phase 6: Authorization Integration (Optional)

### 6.1 Add Workflow Context to Authorization
**File:** `pkg/middleware/authorization.go`

- [ ] Define `WorkflowContext` struct:
  ```go
  type WorkflowContext struct {
      Active       bool
      CurrentState string
      TargetState  string
      ValidStates  []string
  }
  ```

- [ ] Extract workflow info in middleware:
  - [ ] Load workflow for resource path
  - [ ] Extract current state
  - [ ] Get valid transitions for user

- [ ] Add to OPA input:
  ```go
  input := map[string]interface{}{
      // ... existing fields
      "workflow": workflowContext,
  }
  ```

- [ ] Write example Rego policies in docs

---

## Phase 7: API Endpoints

### 7.1 Create Workflow Handlers
**New File:** `services/vfs/handlers/workflow.go`

- [ ] Implement `GET /api/v1/workflows/info/:path`
  - [ ] Load workflow for path
  - [ ] Extract current state
  - [ ] Return workflow metadata

- [ ] Implement `GET /api/v1/workflows/transitions/:path`
  - [ ] Get valid transitions for current user
  - [ ] Return list with gate requirements
  - [ ] Indicate which transitions user can perform

- [ ] Implement `POST /api/v1/workflows/next/:path`
  - [ ] Accept `target_state` in request body
  - [ ] Construct destination path
  - [ ] Call `FileService.MoveFile()`
  - [ ] Return new path

- [ ] Add authentication/authorization
- [ ] Add rate limiting
- [ ] Write OpenAPI spec

### 7.2 API Tests
- [ ] Test info endpoint
- [ ] Test transitions endpoint
- [ ] Test next endpoint
- [ ] Test error responses
- [ ] Test authentication/authorization

---

## Phase 8: Testing

### 8.1 Unit Test Coverage
Target: >90% for all workflow components

- [ ] `workflow_loader_test.go` - 100% coverage
- [ ] `workflow_engine_test.go` - 100% coverage
- [ ] `workflow_gates_test.go` - 100% coverage
- [ ] All error paths tested
- [ ] All edge cases covered

### 8.2 Integration Tests
**File:** `citest/e2e_workflow_test.go`

- [ ] **Test 1:** Document approval workflow
  - [ ] Create in draft (initial_state)
  - [ ] Move to review (editor group)
  - [ ] Move to published (approver group)
  - [ ] Reject invalid draft → published

- [ ] **Test 2:** Escape prevention
  - [ ] Attempt move outside workflow scope → blocked

- [ ] **Test 3:** Deletion gates
  - [ ] Delete with gate approval → success
  - [ ] Delete without gate approval → blocked

- [ ] **Test 4:** State directory protection
  - [ ] Rename state directory → blocked
  - [ ] Delete non-empty state dir → blocked
  - [ ] Delete empty state dir → allowed

- [ ] **Test 5:** Event-triggered transitions
  - [ ] File upload triggers auto-move
  - [ ] Gate failure prevents move

- [ ] **Test 6:** Subdirectory preservation
  - [ ] Move `/drafts/legal/2025/file.pdf` to `review`
  - [ ] Verify: `/review/legal/2025/file.pdf`

- [ ] **Test 7:** system-admin bypass
  - [ ] system-admin can bypass gates
  - [ ] system-admin can rename state dirs

- [ ] **Test 8:** Same-state movement
  - [ ] Move within same state → allowed
  - [ ] No gate validation triggered

- [ ] **Test 9:** Rego policy caching
  - [ ] First evaluation compiles policy
  - [ ] Second evaluation uses cache
  - [ ] Cache invalidation on policy update

- [ ] **Test 10:** Content-based gates
  - [ ] JSON content parsed and available to gates
  - [ ] Metadata-based decisions
  - [ ] Complex Rego rules

### 8.3 Performance Benchmarks
- [ ] Workflow lookup: measure time
- [ ] State extraction: measure time
- [ ] Gate evaluation (cached): < 5ms p95
- [ ] Gate evaluation (uncached): < 50ms p95
- [ ] Full validation flow: < 20ms p95

---

## Phase 9: Documentation

### 9.1 Update DESIGN.md
**File:** `docs/DESIGN.md`

- [ ] Add workflow section to Table of Contents
- [ ] Add "9. Workflow System" section:
  - [ ] Directory-as-state architecture
  - [ ] Rego-based gates
  - [ ] Integration with authorization
  - [ ] Performance characteristics
  - [ ] Caching strategy

### 9.2 Update SECURITY.md
**File:** `docs/SECURITY.md`

- [ ] Add workflow security section:
  - [ ] Workflow as hard constraint
  - [ ] Workflow validates before authorization
  - [ ] system-admin bypass
  - [ ] Defense in depth

### 9.3 Create WORKFLOWS.md
**New File:** `docs/WORKFLOWS.md`

- [ ] Introduction & concepts
- [ ] Workflow YAML reference
- [ ] Rego gate policy guide
- [ ] Example workflows:
  - [ ] Document approval
  - [ ] Content moderation
  - [ ] Build pipeline
  - [ ] Multi-stage approval
- [ ] API reference
- [ ] Troubleshooting guide
- [ ] Migration guide
- [ ] Best practices

### 9.4 Create Examples
**New File:** `examples/workflows/`

- [ ] `document-approval/.workflow`
- [ ] `document-approval/.workflow.rego`
- [ ] `content-moderation/.workflow`
- [ ] `build-pipeline/.workflow`
- [ ] README.md with usage instructions

---

## Phase 10: Advanced Features (Future)

### 10.1 Scheduler Integration
**File:** `services/scheduler/main.go`

- [ ] Add `workflow_timeout_check` handler
- [ ] Query `workflow_audit` for stale files
- [ ] Emit `workflow.state.timeout` events
- [ ] Configure notification webhooks

### 10.2 Workflow Visualization
- [ ] Add `GET /api/v1/workflows/:path/diagram` endpoint
- [ ] Generate Mermaid state machine diagram
- [ ] Support multiple output formats

### 10.3 Workflow Analytics
- [ ] Query `workflow_audit` for metrics
- [ ] Average time in each state
- [ ] Transition success/failure rates
- [ ] Bottleneck identification
- [ ] Dashboard endpoint

---

## Rollout Strategy

### Stage 1: Development (Week 1-2)
- [ ] Complete Phase 1-3 (Models, Loader, Engine)
- [ ] Unit tests passing
- [ ] Basic integration tests passing

### Stage 2: Integration (Week 3)
- [ ] Complete Phase 4 (Service Integration)
- [ ] Complete Phase 5 (Events)
- [ ] All integration tests passing
- [ ] Performance benchmarks acceptable

### Stage 3: API & Docs (Week 4)
- [ ] Complete Phase 7 (API Endpoints)
- [ ] Complete Phase 9 (Documentation)
- [ ] API tests passing
- [ ] Documentation reviewed

### Stage 4: Beta Testing (Week 5)
- [ ] Deploy to development environment
- [ ] Create test workflows
- [ ] Internal testing
- [ ] Gather feedback
- [ ] Fix issues

### Stage 5: Production Rollout (Week 6+)
- [ ] Enable for specific directories
- [ ] Monitor workflow events
- [ ] Monitor performance metrics
- [ ] User training
- [ ] Gradual expansion

---

## Success Criteria

### Functional Requirements
- [ ] All unit tests passing (>90% coverage)
- [ ] All integration tests passing
- [ ] All API tests passing
- [ ] Documentation complete
- [ ] Examples provided

### Performance Requirements
- [ ] Workflow validation < 10ms (p95)
- [ ] Gate evaluation (cached) < 5ms (p95)
- [ ] Gate evaluation (uncached) < 50ms (p95)
- [ ] Cache hit rate > 80%

### Operational Requirements
- [ ] Zero critical bugs in beta
- [ ] Monitoring dashboards created
- [ ] Alerts configured
- [ ] Runbooks written
- [ ] Team trained

---

## Risk Mitigation

### Risk 1: Rego Policy Compilation Performance
**Mitigation:** Aggressive caching with 5-min TTL

### Risk 2: Complex Policies Breaking Gates
**Mitigation:** Fail closed on errors, comprehensive testing

### Risk 3: Cache Invalidation Bugs
**Mitigation:** Clear cache on any `.workflow` or `.workflow.rego` update

### Risk 4: State Directory Conflicts
**Mitigation:** Strict validation on `.workflow` creation

### Risk 5: User Confusion
**Mitigation:** Clear error messages, comprehensive docs, examples

---

## Dependencies

### Code Dependencies
- `github.com/open-policy-agent/opa` - Rego engine ✅ (already in use)
- `gopkg.in/yaml.v3` - YAML parsing (check if in use)

### Team Dependencies
- Backend team: Implementation
- DevOps team: Deployment, monitoring
- Documentation team: User guides
- QA team: Testing

### External Dependencies
- None (all internal to mysql-vfs)

---

## Notes

- Archive contains previous design iterations
- Tests already started in `citest/e2e_workflow_test.go`
- Rego pattern follows existing authorization middleware
- No database schema changes required (audit table is optional)

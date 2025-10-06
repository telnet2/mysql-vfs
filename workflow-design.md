# Workflow and Policy Design

## 1. Introduction

The `.workflow` file is a special policy file within `mysql-vfs` that enables the definition of declarative, state-based workflows using directory structure as the state machine. It provides a powerful mechanism to control file movement between states, ensuring that files follow a predefined lifecycle. This document outlines the design, usage, and DSL for the `.workflow` file.

### 1.1. Key Design Principles

| Principle | Rule | Reason |
|-----------|------|--------|
| **Directory-as-State** | State is determined by file location in directory structure | Physical paths represent logical states |
| **One Workflow Per Tree** | Only one `.workflow` per directory tree (no nesting) | Prevents state ambiguity from overlapping paths |
| **Explicit State Mapping** | States mapped to directories via `state_directories` | Clear, configurable, supports nested state paths |
| **Initial State Only** | Files can only be created in `initial_state` directory | Ensures all files start in defined state |
| **Move = Transition** | Moving files between state dirs = state transition | Natural filesystem operation with validation |
| **Structure Preservation** | Subdirectories preserved during transitions | Maintains organization across states |

### 1.2. Quick Example

```yaml
# /documents/.workflow
state_directories:
  draft: "drafts"
  review: "review"
  published: "published"

initial_state: draft

states:
  draft:
    transitions:
      - to: review
        gates:
          - type: group
            groups: [editor, admin]
  review:
    transitions:
      - to: published
        gates:
          - type: group
            groups: [approver, admin]
  published:
    transitions: []
```

**Resulting structure:**
```
/documents/              # Workflow scope (all files governed by one workflow)
├── .workflow
├── drafts/             # State: draft (initial_state)
│   └── report.pdf      # Can be created here
├── review/             # State: review
│   └── contract.pdf    # Moved from drafts/ after gate validation
└── published/          # State: published
    └── handbook.pdf    # Moved from review/ after gate validation
```

## 2. Core Concepts

### 2.1. Directory-as-State

The fundamental principle of workflows in mysql-vfs is that **directory structure represents the state machine**. Each state corresponds to a subdirectory within the workflow scope.

**Example:**
```
/documents/
├── .workflow           # Workflow definition
├── drafts/            # State: draft
│   └── report.pdf
├── review/            # State: review
│   └── proposal.pdf
└── published/         # State: published
    └── handbook.pdf
```

A file's state is determined by its location within the state directory structure. Moving a file between state directories constitutes a state transition.

### 2.2. One Workflow Per Directory Tree

**Key Design Principle:** Each directory tree can have only **one** `.workflow` file, and it applies to all files within that tree.

**Why:** Workflows bind states to physical directory paths. Allowing nested workflows would create ambiguity when a file's path could match multiple state directories from different workflows.

**Example - Valid:**
```
/
├── documents/
│   ├── .workflow         # Governs ALL files under /documents/
│   ├── drafts/
│   └── review/
│
└── builds/
    ├── .workflow         # Governs ALL files under /builds/
    ├── pending/
    └── deployed/
```

**Example - Invalid:**
```
/documents/
├── .workflow             # Parent workflow
├── drafts/
│   └── legal/
│       └── .workflow    # ❌ FORBIDDEN - nested workflow creates ambiguity
```

See Section 8.1 for detailed explanation of why nesting is forbidden.

### 2.3. State Names

State names must be simple alphanumeric strings with hyphens allowed:
- ✅ Valid: `draft`, `in-review`, `published`, `stage-1`
- ❌ Invalid: `draft/review`, `state_2`, `In Review` (spaces not allowed)

State directory names directly map to state names. For example:
- Directory `/documents/draft/` → State `draft`
- Directory `/documents/in-review/` → State `in-review`

### 2.4. Transitions and Gates

A workflow consists of **states** and **transitions**.

- **States:** Represent a stage in the file's lifecycle. Each state maps to a directory.
- **Transitions:** Define allowed movements from one state to another.
- **Gates:** Conditions that must be satisfied for a transition to be allowed. Gates can check:
  - User group membership
  - File metadata attributes
  - Custom Rego policies
  - External system state

### 2.5. Initial State

Every workflow defines an **initial state**. Files can only be created in the initial state directory. Attempting to create a file in any other state directory will be rejected.

**Example:**
```yaml
initial_state: draft
```

This means:
- ✅ `POST /api/v1/files` with path `/documents/draft/report.pdf` → Allowed
- ❌ `POST /api/v1/files` with path `/documents/review/report.pdf` → Rejected

### 2.6. File Movement as State Transition

State transitions are performed by **moving files** between state directories:

```bash
# Transition from draft to review
POST /api/v1/files/move
{
  "old_path": "/documents/draft/report.pdf",
  "new_path": "/documents/review/report.pdf"
}
```

The workflow engine intercepts this move operation and:
1. Detects that this is a state transition (`draft` → `review`)
2. Loads the workflow definition from `/documents/.workflow`
3. Validates the transition is allowed
4. Evaluates all gates for this transition
5. Allows or denies the move based on gate results

### 2.7. Automatic Transitions via Events

File movements can be triggered automatically using the `.events` system with the `move_file` action:

**Example `.events` file:**
```json
{
  "handlers": [
    {
      "name": "auto-approve-small-files",
      "events": ["file.create.succeeded"],
      "type": "move_file",
      "config": {
        "from_directory": "draft",
        "to_directory": "published"
      },
      "condition": "input.file.size_bytes < 10240"
    }
  ]
}
```

## 3. Workflow Definition (`.workflow` DSL)

### 3.1. File Format

The `.workflow` file uses **YAML** format for human readability and version control compatibility.

### 3.2. Basic Structure

```yaml
# Map states to directory paths (relative to workflow home)
state_directories:
  draft: "drafts"
  review: "review"
  published: "published"

# Initial state where files must be created
initial_state: draft

# State definitions
states:
  draft:
    transitions:
      - to: review
        gates:
          - type: group
            groups: [editor, admin]

  review:
    transitions:
      - to: published
        gates:
          - type: group
            groups: [approver, admin]
      - to: draft
        gates:
          - type: group
            groups: [editor, admin]

  published:
    # Terminal state - no transitions out
    transitions: []
```

### 3.3. Field Definitions

#### `state_directories` (required)
A map of state names to relative directory paths. Defines where files in each state are stored.

**Format:** `state_name: "relative/path/from/workflow/home"`

**Rules:**
- Paths are relative to `$WORKFLOW_HOME` (the directory containing `.workflow`)
- Paths can be nested (e.g., `review/pending`, `archive/2024`)
- Maximum nesting depth: 5 levels
- All directories must exist before workflow activation
- No overlapping paths (one path cannot be a prefix of another)

**Examples:**
```yaml
state_directories:
  draft: "drafts"                    # Simple
  review: "review/pending"           # Nested
  published: "public"                # Different name from state
  archive: "archived/old"            # Nested
```

#### `initial_state` (required)
The state name where files must be created. Must match one of the state names defined in `state_directories`.

#### `states` (required)
A map of state names to state definitions. Each state name must:
- Be defined in `state_directories`
- Be alphanumeric with hyphens only (convention: lowercase with hyphens)
- Have a corresponding `transitions` array

#### `transitions` (required per state)
An array of allowed transitions from this state. Can be empty `[]` for terminal states.

Each transition contains:
- `to` (required): Target state name
- `gates` (optional): Array of conditions that must be satisfied. If empty or omitted, transition is always allowed.

### 3.4. Gate Types

#### Group Gate
Checks if the actor belongs to one of the specified groups.

```yaml
gates:
  - type: group
    groups: [editor, admin]
```

**Evaluation:** Pass if `actor.groups ∩ gate.groups ≠ ∅`

#### Metadata Gate
Checks file metadata attributes.

```yaml
gates:
  - type: metadata
    attribute: priority
    operator: equals
    value: high
```

**Operators:**
- `equals`: Attribute equals value
- `not_equals`: Attribute does not equal value
- `contains`: Attribute contains substring
- `greater_than`: Numeric comparison
- `less_than`: Numeric comparison

#### Rego Gate
Executes a custom Rego policy for complex conditions.

```yaml
gates:
  - type: rego
    policy: |
      package workflow.gate

      default allow = false

      allow {
        input.user.groups[_] == "approver"
        input.file.size_bytes < 1048576
        input.metadata.reviewed == true
      }
```

**Input structure:**
```json
{
  "user": {
    "id": "alice",
    "groups": ["editor", "admin"]
  },
  "file": {
    "id": "file-123",
    "name": "report.pdf",
    "path": "/documents/review/report.pdf",
    "size_bytes": 102400,
    "content_type": "application/pdf"
  },
  "from_state": "review",
  "to_state": "published",
  "metadata": {
    "priority": "high",
    "reviewed": true
  }
}
```

#### Composite Gate (AND/OR)
Combine multiple gates with logical operators.

```yaml
gates:
  - type: and
    gates:
      - type: group
        groups: [approver]
      - type: metadata
        attribute: reviewed
        operator: equals
        value: true

  - type: or
    gates:
      - type: group
        groups: [admin]
      - type: rego
        policy: |
          package workflow.gate
          allow { input.user.id == "emergency-override" }
```

**Evaluation:**
- `and`: All sub-gates must pass
- `or`: At least one sub-gate must pass

### 3.5. Complete Example

```yaml
# Document approval workflow

state_directories:
  draft: "drafts"
  review: "review"
  published: "published"
  archive: "archive"

initial_state: draft

states:
  draft:
    transitions:
      # Editors can submit for review
      - to: review
        gates:
          - type: group
            groups: [editor, admin]

      # Anyone can archive drafts
      - to: archive
        gates: []

  review:
    transitions:
      # Approvers can publish if reviewed
      - to: published
        gates:
          - type: and
            gates:
              - type: group
                groups: [approver, admin]
              - type: metadata
                attribute: reviewed
                operator: equals
                value: true

      # Editors can send back to draft
      - to: draft
        gates:
          - type: group
            groups: [editor, admin]

      # Failed review goes to archive
      - to: archive
        gates:
          - type: group
            groups: [editor, approver, admin]

  published:
    transitions:
      # Only admins can unpublish
      - to: draft
        gates:
          - type: group
            groups: [admin]

      # Anyone can archive published docs
      - to: archive
        gates: []

  archive:
    transitions:
      # Admins can restore from archive
      - to: draft
        gates:
          - type: group
            groups: [admin]
```

## 4. Workflow Enforcement

### 4.1. File Creation Constraints

When a file is created, the system:

1. Checks if the parent directory is within a workflow scope
2. Loads the `.workflow` file via directory inheritance
3. Extracts the state from the parent directory name
4. Validates that the state matches `initial_state`
5. Rejects creation if state ≠ `initial_state`

**Example:**
```yaml
# /documents/.workflow
initial_state: draft
```

```bash
# ✅ Allowed
POST /api/v1/files
{"path": "/documents/draft/report.pdf"}

# ❌ Rejected: "Files can only be created in the initial state 'draft'"
POST /api/v1/files
{"path": "/documents/review/report.pdf"}
```

### 4.2. File Movement Validation

When a file is moved, the system:

1. Extracts `from_state` from old path's parent directory
2. Extracts `to_state` from new path's parent directory
3. Loads the workflow definition
4. Checks if transition `from_state` → `to_state` exists
5. Evaluates all gates for this transition
6. Allows move only if all gates pass

**Example:**
```bash
# Transition: draft → review
POST /api/v1/files/move
{
  "old_path": "/documents/draft/report.pdf",
  "new_path": "/documents/review/report.pdf",
  "actor": "alice",
  "actor_groups": ["editor"]
}
```

**Workflow engine logic:**
```
1. from_state = "draft"
2. to_state = "review"
3. Load /documents/.workflow
4. Find transition: draft.transitions[to=review]
5. Evaluate gates:
   - type: group, groups: [editor, admin]
   - Check: "editor" ∈ ["editor", "admin"] → ✅ PASS
6. Allow move
```

### 4.3. Gate Evaluation Order

Gates are evaluated in the order they appear in the workflow definition. Evaluation stops at the first failure (short-circuit).

For composite gates:
- **AND gates**: Stop at first failure
- **OR gates**: Stop at first success

### 4.4. Error Handling

**Invalid transition:**
```json
{
  "error": "Invalid workflow transition",
  "code": "WORKFLOW_INVALID_TRANSITION",
  "details": {
    "from_state": "published",
    "to_state": "review",
    "allowed_transitions": ["draft", "archive"]
  }
}
```

**Gate failure:**
```json
{
  "error": "Workflow gate evaluation failed",
  "code": "WORKFLOW_GATE_FAILED",
  "details": {
    "gate_type": "group",
    "required_groups": ["approver", "admin"],
    "user_groups": ["editor"]
  }
}
```

## 5. Workflow Transition Commands

### 5.1. Manual Transition (`mv` command)

Use the standard `mv` (move) command to explicitly specify source and target paths:

```bash
# Explicit transition with full path control
mv /documents/drafts/legal/contract.pdf /documents/review/legal/contract.pdf
```

**Behavior:**
- User specifies both source and target paths
- Workflow engine validates the transition
- User controls the exact target location (can reorganize subdirectories)
- Subject to workflow gates and authorization

### 5.2. Automatic Transition (`next` command)

Use the `next` command to transition to a target state while **automatically preserving subdirectory structure**:

```bash
# Transition to specific state, preserving structure
next /documents/drafts/legal/2025/contract.pdf review

# Results in: /documents/review/legal/2025/contract.pdf
```

**Syntax:**
```
next <file-path> <target-state>
```

**Behavior:**
1. Extracts current state from file path
2. Validates transition: `current_state` → `target_state`
3. Evaluates workflow gates
4. Preserves subdirectory structure within state
5. Moves file to target state directory

**Example:**
```bash
# File location
/documents/drafts/legal/high-priority/urgent/contract.pdf

# Command
next /documents/drafts/legal/high-priority/urgent/contract.pdf review

# Workflow engine:
# 1. Current state: draft (from state_directories["draft"] = "drafts")
# 2. Target state: review (from state_directories["review"] = "review")
# 3. Validate transition: draft → review ✅
# 4. Relative path in state: legal/high-priority/urgent/contract.pdf
# 5. New path: /documents/review/legal/high-priority/urgent/contract.pdf
```

**Advantages:**
- ✅ Simple: only specify target state
- ✅ Preserves organization automatically
- ✅ Less error-prone than manual `mv`
- ✅ Clear intent: "move this to the next state"

**API Endpoint:**
```bash
POST /api/v1/workflows/transition
{
  "file_path": "/documents/drafts/legal/contract.pdf",
  "target_state": "review"
}
```

**Response:**
```json
{
  "old_path": "/documents/drafts/legal/contract.pdf",
  "new_path": "/documents/review/legal/contract.pdf",
  "from_state": "draft",
  "to_state": "review",
  "transition": "succeeded"
}
```

### 5.3. Transition Validation

Both `mv` and `next` commands trigger the same validation:

```go
func (e *WorkflowEngine) ValidateTransition(
    oldPath, newPath string,
    actor *Actor,
) error {
    // 1. Load workflow definition
    workflow := e.loader.LoadForPath(oldPath)

    // 2. Extract states
    fromState := workflow.GetState(oldPath)
    toState := workflow.GetState(newPath)

    // 3. Same state = reorganization (allow)
    if fromState == toState {
        return nil
    }

    // 4. Validate transition exists
    transition := workflow.GetTransition(fromState, toState)
    if transition == nil {
        return ErrInvalidTransition
    }

    // 5. Evaluate gates
    for _, gate := range transition.Gates {
        if err := e.evaluateGate(gate, actor); err != nil {
            return err
        }
    }

    return nil
}
```

**System-Admin Bypass:**
- Regular admin: Subject to workflow validation
- System-admin group: Can bypass workflow validation entirely

```bash
# Admin user (groups: [admin])
mv /documents/drafts/file.pdf /documents/undefined-state/file.pdf
# ❌ Error: Invalid transition (undefined-state not in workflow)

# System-admin user (groups: [system-admin])
mv /documents/drafts/file.pdf /documents/undefined-state/file.pdf
# ✅ Allowed (bypasses workflow engine)
```

## 6. Event-Driven Workflows

### 6.1. Automatic State Transitions

The `.events` system can trigger automatic file movements using the `move_file` action.

**Example: Auto-approve small files**

```json
{
  "handlers": [
    {
      "name": "auto-approve-small-docs",
      "events": ["file.create.succeeded"],
      "type": "move_file",
      "config": {
        "from_state": "draft",
        "to_state": "published"
      },
      "condition": "input.file.size_bytes < 10240 && input.file.content_type == 'text/plain'"
    }
  ]
}
```

When a file is created in `/documents/draft/`, if it matches the condition, it's automatically moved to `/documents/published/`.

### 6.2. Delayed Transitions

Use the scheduler to transition files based on time.

**Example: Auto-archive old published files**

```json
{
  "handlers": [
    {
      "name": "archive-old-published",
      "events": ["cron.daily"],
      "type": "move_file",
      "config": {
        "from_state": "published",
        "to_state": "archive",
        "filter": {
          "older_than_days": 90
        }
      }
    }
  ]
}
```

### 6.3. External Trigger Workflows

Workflows can be triggered by external events via webhooks.

**Example: CI/CD pipeline**

```json
{
  "handlers": [
    {
      "name": "deploy-on-ci-success",
      "events": ["webhook.ci.build.succeeded"],
      "type": "move_file",
      "config": {
        "from_state": "testing",
        "to_state": "production"
      },
      "condition": "input.payload.branch == 'main' && input.payload.tests_passed == true"
    }
  ]
}
```

## 7. Integration with Authorization

### 7.1. Workflow-First Authorization

The workflow engine validates transitions **before** the authorization layer. This provides defense in depth:

1. **Workflow Engine:** Is this transition allowed by the workflow?
2. **Authorization Layer (OPA):** Does this user have permission to move files?

If either check fails, the operation is denied.

### 7.2. Injecting Workflow Context into OPA

For fine-grained control, workflow state can be injected into the authorization input:

```json
{
  "user": {
    "id": "alice",
    "groups": ["editor"]
  },
  "resource": {
    "path": "/documents/draft/report.pdf",
    "type": "file"
  },
  "action": "move",
  "workflow": {
    "from_state": "draft",
    "to_state": "review",
    "allowed_transitions": ["review", "archive"]
  }
}
```

**Example OPA policy:**

```rego
package vfs.authz

# Allow file moves for workflow transitions
allow {
    input.action == "move"
    input.workflow.to_state != null

    # User must be in editor or admin group
    input.user.groups[_] == "editor"

    # Additional business logic
    is_valid_transition
}

is_valid_transition {
    input.workflow.to_state == input.workflow.allowed_transitions[_]
}
```

### 7.3. Separation of Concerns

- **Workflow Engine:** Enforces lifecycle and business process rules
- **Authorization Layer:** Enforces access control and permissions

This allows:
- Workflow rules to be managed by product/process owners
- Authorization rules to be managed by security teams

## 8. Advanced Features

### 8.1. Workflow Scope and Nesting Restrictions

Each `.workflow` file defines a workflow scope for its entire directory tree with **no nesting allowed**.

#### 8.1.1. Single Workflow Per Tree

**Rule:** A workflow applies to all files within its directory and all subdirectories. No nested `.workflow` files are permitted.

**Rationale:** Workflows bind states to physical directory paths. Allowing nested workflows would create ambiguity about which state a file belongs to when paths overlap.

**Example - Valid (Multiple Independent Workflows):**
```
/
├── documents/
│   ├── .workflow              # Workflow for all documents
│   ├── drafts/
│   ├── review/
│   └── published/
│
├── builds/
│   ├── .workflow              # Separate workflow for builds
│   ├── pending/
│   ├── testing/
│   └── deployed/
│
└── content/
    ├── .workflow              # Separate workflow for content
    ├── submitted/
    └── approved/
```

**Example - Invalid (Nested Workflows):**
```
/projects/
├── .workflow                  # Parent workflow
│   state_directories:
│     draft: "drafts"
│
├── drafts/                    # Parent's draft state
│   └── alpha/
│       ├── .workflow         # ❌ FORBIDDEN - creates ambiguity
│       └── pending/
│           └── file.pdf      # ❓ State: draft or pending?
```

#### 8.1.2. Why Nesting is Forbidden

**Problem:** State directory path overlap creates ambiguity.

```
File: /projects/drafts/alpha/pending/file.pdf

Parent workflow says:
- Path contains "drafts/"
- State: draft
- Allowed transitions: draft → review

Child workflow says:
- Path contains "pending/"
- State: pending
- Allowed transitions: pending → approved

Which is correct? ❓
```

**Solution:** Only one workflow per directory tree ensures each file has exactly one state.

#### 8.1.3. Validation

When creating a `.workflow` file, the system validates:

1. **No parent workflows:** Check that no `.workflow` file exists in any parent directory
2. **No child workflows:** Check that no `.workflow` files exist in any subdirectories

```go
func validateWorkflowNesting(workflowPath string) error {
    workflowDir := filepath.Dir(workflowPath)

    // Check 1: No parent workflows
    dir := workflowDir
    for dir != "/" {
        dir = filepath.Dir(dir)
        if exists(filepath.Join(dir, ".workflow")) {
            return fmt.Errorf(
                "cannot create workflow at %s: parent workflow exists at %s",
                workflowPath, dir,
            )
        }
    }

    // Check 2: No child workflows
    childWorkflows := findFilesRecursive(workflowDir, ".workflow")
    if len(childWorkflows) > 0 {
        return fmt.Errorf(
            "cannot create workflow at %s: child workflows exist: %v",
            workflowPath, childWorkflows,
        )
    }

    return nil
}
```

#### 8.1.4. Multi-Workflow Organization Pattern

To have different workflows for different purposes, organize them at the top level:

```
/
├── engineering/
│   ├── .workflow              # Engineering workflow
│   │   state_directories:
│   │     draft: "drafts"
│   │     code-review: "review"
│   │     merged: "merged"
│   ├── drafts/
│   ├── review/
│   └── merged/
│
├── legal/
│   ├── .workflow              # Legal workflow
│   │   state_directories:
│   │     pending: "pending"
│   │     approved: "approved"
│   │     archived: "archive"
│   ├── pending/
│   ├── approved/
│   └── archive/
│
└── marketing/
    ├── .workflow              # Marketing workflow
    │   state_directories:
    │     draft: "drafts"
    │     review: "review"
    │     published: "live"
    ├── drafts/
    ├── review/
    └── live/
```

Each top-level directory has its own independent workflow with no conflicts.

### 8.2. Conditional Initial States

Allow files to be created in different initial states based on conditions.

```yaml
initial_state: draft

initial_state_rules:
  - state: review
    condition: |
      input.user.groups[_] == "trusted-partner"
  - state: draft
    condition: |
      # Default for everyone else
      true
```

### 8.3. Transition Hooks

Execute actions when a transition occurs.

**When to use `on_success` vs `.events`:**
- **Use `on_success`**: For actions directly and fundamentally tied to this specific transition (e.g., "when this document is published, clear the CDN cache", "update published_at timestamp")
- **Use `.events`**: For broader, decoupled concerns that apply across multiple operations (e.g., "log all successful file moves to central audit system", "notify Slack on any state change")

`on_success` hooks are workflow-specific and tightly coupled to the transition logic, while `.events` handlers are system-wide and operation-agnostic.

```yaml
states:
  review:
    transitions:
      - to: published
        gates:
          - type: group
            groups: [approver]
        on_success:
          - type: webhook
            url: https://notify.example.com/published
          - type: metadata_update
            set:
              published_at: "{{now}}"
              published_by: "{{actor}}"
          - type: cache_invalidate
            cache_key: "document-{{file.id}}"
```

### 8.4. Parallel States

Support for files that can be in multiple states simultaneously (future enhancement).

```yaml
parallel_states:
  quality:
    - unreviewed
    - reviewed

  security:
    - unchecked
    - approved
    - flagged
```

## 9. Use Cases

### 9.1. Document Approval Process

```yaml
state_directories:
  draft: "drafts"
  review: "review"
  published: "published"

initial_state: draft

states:
  draft:
    transitions:
      - to: review
        gates:
          - type: group
            groups: [editor, admin]

  review:
    transitions:
      - to: published
        gates:
          - type: group
            groups: [approver, admin]
      - to: draft
        gates:
          - type: group
            groups: [editor, admin]

  published:
    transitions: []
```

**Directory structure:**
```
/documents/              # $WORKFLOW_HOME
├── .workflow
├── drafts/             # State: draft
├── review/             # State: review
└── published/          # State: published
```

### 9.2. Software Build Pipeline

```yaml
state_directories:
  pending: "queue"
  building: "building"
  testing: "testing"
  staging: "staging"
  production: "prod"
  failed: "failed"

initial_state: pending

states:
  pending:
    transitions:
      - to: building
        gates: []

  building:
    transitions:
      - to: testing
        gates:
          - type: metadata
            attribute: build_status
            operator: equals
            value: success
      - to: failed
        gates:
          - type: metadata
            attribute: build_status
            operator: equals
            value: failed

  testing:
    transitions:
      - to: staging
        gates:
          - type: metadata
            attribute: test_status
            operator: equals
            value: passed
      - to: failed
        gates:
          - type: metadata
            attribute: test_status
            operator: equals
            value: failed

  staging:
    transitions:
      - to: production
        gates:
          - type: group
            groups: [release-manager, admin]

  production:
    transitions:
      - to: staging
        gates:
          - type: group
            groups: [admin]

  failed:
    transitions:
      - to: pending
        gates:
          - type: group
            groups: [developer, admin]
```

**Directory structure:**
```
/builds/              # $WORKFLOW_HOME
├── .workflow
├── queue/           # State: pending
├── building/        # State: building
├── testing/         # State: testing
├── staging/         # State: staging
├── prod/            # State: production
└── failed/          # State: failed
```

**Event automation:**
```json
{
  "handlers": [
    {
      "name": "trigger-build",
      "events": ["file.create.succeeded"],
      "type": "move_file",
      "config": {
        "from_state": "pending",
        "to_state": "building"
      }
    },
    {
      "name": "build-to-test",
      "events": ["file.metadata.updated"],
      "type": "move_file",
      "config": {
        "from_state": "building",
        "to_state": "testing"
      },
      "condition": "input.metadata.build_status == 'success'"
    }
  ]
}
```

### 9.3. Content Moderation

```yaml
state_directories:
  pending: "moderation-queue"
  approved: "approved"
  rejected: "rejected"

initial_state: pending

states:
  pending:
    transitions:
      - to: approved
        gates:
          - type: group
            groups: [moderator, admin]
      - to: rejected
        gates:
          - type: group
            groups: [moderator, admin]

  approved:
    transitions:
      - to: rejected
        gates:
          - type: group
            groups: [senior-moderator, admin]

  rejected:
    transitions:
      - to: pending
        gates:
          - type: group
            groups: [admin]
```

**Directory structure:**
```
/user-content/           # $WORKFLOW_HOME
├── .workflow
├── moderation-queue/   # State: pending
├── approved/           # State: approved
└── rejected/           # State: rejected
```

**Event automation:**
```json
{
  "handlers": [
    {
      "name": "auto-approve-safe-content",
      "events": ["file.create.succeeded"],
      "type": "move_file",
      "config": {
        "from_state": "pending",
        "to_state": "approved"
      },
      "condition": "input.file.size_bytes < 1024 && input.metadata.ai_safety_score > 0.95"
    }
  ]
}
```

## 10. Implementation Details

### 10.1. State Extraction Algorithm

```go
func extractState(filePath string, workflowDef *WorkflowDefinition) (string, error) {
    // Get immediate parent directory
    parentDir := filepath.Base(filepath.Dir(filePath))

    // Check if this is a valid state
    if _, exists := workflowDef.States[parentDir]; exists {
        return parentDir, nil
    }

    return "", ErrNotInWorkflowState
}
```

**Example:**
- Path: `/documents/review/report.pdf`
- Parent: `review`
- State: `review` ✅

### 10.2. Workflow Loader

The workflow loader searches up the directory tree to find the applicable `.workflow` file for a given path:

```go
func (l *WorkflowLoader) LoadForPath(filePath string) (*WorkflowDefinition, error) {
    // Start from file's directory and walk up to find the workflow
    dir := filepath.Dir(filePath)

    for {
        workflowPath := filepath.Join(dir, ".workflow")

        if exists(workflowPath) {
            // Found the workflow that governs this file
            return l.loadAndParse(workflowPath)
        }

        if dir == "/" {
            break
        }

        dir = filepath.Dir(dir)
    }

    return nil, ErrNoWorkflowFound
}
```

**Note:** Since nested workflows are forbidden (Section 8.1), there will be at most one `.workflow` file in the directory hierarchy. This search simply finds which workflow scope (if any) the file belongs to.

**Example:**
```
/
├── documents/
│   ├── .workflow           # Governs all files under /documents/
│   └── legal/
│       └── contract.pdf    # Uses /documents/.workflow
│
└── builds/
    ├── .workflow           # Governs all files under /builds/
    └── app.tar.gz          # Uses /builds/.workflow
```

**Cache:** Workflow definitions are cached with a 5-minute TTL and automatically invalidated when the `.workflow` file is updated.

### 10.3. Transition Validation Flow

```go
func (e *WorkflowEngine) ValidateTransition(
    ctx context.Context,
    oldPath string,
    newPath string,
    actor *Actor,
) error {
    // 1. Load workflow
    workflow, err := e.loader.LoadForPath(oldPath)
    if err != nil {
        return nil // No workflow, allow move
    }

    // 2. Extract states
    fromState, _ := extractState(oldPath, workflow)
    toState, _ := extractState(newPath, workflow)

    // 3. Check if moving between states
    if fromState == toState {
        return nil // Same state, allow
    }

    // 4. Validate transition exists
    transition := workflow.GetTransition(fromState, toState)
    if transition == nil {
        return ErrInvalidTransition
    }

    // 5. Evaluate gates
    for _, gate := range transition.Gates {
        if err := e.evaluateGate(ctx, gate, actor); err != nil {
            return err
        }
    }

    return nil
}
```

### 10.4. Gate Evaluation

```go
func (e *WorkflowEngine) evaluateGate(
    ctx context.Context,
    gate *Gate,
    actor *Actor,
) error {
    switch gate.Type {
    case "group":
        return e.evaluateGroupGate(gate, actor)
    case "metadata":
        return e.evaluateMetadataGate(gate, actor)
    case "rego":
        return e.evaluateRegoGate(ctx, gate, actor)
    case "and":
        return e.evaluateAndGate(ctx, gate, actor)
    case "or":
        return e.evaluateOrGate(ctx, gate, actor)
    default:
        return ErrUnknownGateType
    }
}

func (e *WorkflowEngine) evaluateGroupGate(gate *Gate, actor *Actor) error {
    requiredGroups := gate.Groups

    for _, group := range actor.Groups {
        for _, required := range requiredGroups {
            if group == required {
                return nil // Found matching group
            }
        }
    }

    return ErrGroupGateFailed
}
```

## 11. Directory Structure Requirements

### 11.1. State Directory Creation

When a `.workflow` file is created or updated, the system should validate that all referenced state directories exist:

```yaml
initial_state: draft

states:
  draft: {}
  review: {}
  published: {}
```

**Validation:**
```
✅ /documents/draft/ exists
✅ /documents/review/ exists
✅ /documents/published/ exists
```

If any state directory is missing, the workflow is considered invalid.

### 11.2. State Directory Naming

State directories must:
- Match exactly the state name (case-sensitive)
- Contain only alphanumeric characters and hyphens
- Not contain any special files that would interfere with workflow operation

### 11.3. State Directory Mapping

Each state must be explicitly mapped to a directory path relative to the workflow home (`$WORKFLOW_HOME`):

```yaml
# /documents/.workflow

state_directories:
  draft: "drafts"                    # $WORKFLOW_HOME/drafts/
  review: "review/pending"           # $WORKFLOW_HOME/review/pending/
  published: "public"                # $WORKFLOW_HOME/public/
  archive: "archived/old-docs"       # $WORKFLOW_HOME/archived/old-docs/

initial_state: draft

states:
  draft:
    transitions:
      - to: review
        gates:
          - type: group
            groups: [editor, admin]
  review:
    transitions:
      - to: published
        gates:
          - type: group
            groups: [approver, admin]
      - to: draft
        gates:
          - type: group
            groups: [editor, admin]
  published:
    transitions:
      - to: archive
        gates: []
  archive:
    transitions:
      - to: draft
        gates:
          - type: group
            groups: [admin]
```

**Resulting directory structure:**
```
/documents/                      # $WORKFLOW_HOME
├── .workflow
├── drafts/                      # State: draft
│   └── report.pdf
├── review/
│   └── pending/                 # State: review
│       └── contract.pdf
├── public/                      # State: published
│   └── handbook.pdf
└── archived/
    └── old-docs/                # State: archive
        └── memo.pdf
```

**Rules:**
1. **Explicit mapping required:** Every state must have a corresponding entry in `state_directories`
2. **Relative paths:** State directory paths are relative to `$WORKFLOW_HOME`
3. **Nested state paths allowed:** State directories can be nested (e.g., `review/pending`, `archive/2024/q1`)
4. **Maximum depth:** State directory paths can be up to 5 levels deep
5. **No overlaps:** State directories cannot be prefixes of other state directories
6. **Must exist:** All state directories must exist before workflow activation

### 11.4. Subdirectories within States

Files within state directories **can** be organized in subdirectories for better organization:

```
/documents/                      # $WORKFLOW_HOME
├── .workflow
└── drafts/                      # State: draft (from state_directories)
    ├── legal/                   # Organization subdirectory
    │   ├── 2025/               # Organization subdirectory
    │   │   └── contract-a.pdf  # State: draft
    │   └── contract-b.pdf      # State: draft
    ├── marketing/              # Organization subdirectory
    │   └── brochure.pdf        # State: draft
    └── engineering/            # Organization subdirectory
        └── spec.pdf            # State: draft
```

**Rules:**

#### 1. Nesting Depth Limit
- **Maximum depth:** 5 levels below the state root directory
- Counting starts from the state directory, not `$WORKFLOW_HOME`

**Example:**
```
/documents/drafts/a/b/c/d/e/file.pdf  ✅ Depth 5: OK
/documents/drafts/a/b/c/d/e/f/file.pdf  ❌ Depth 6: FORBIDDEN
```

#### 2. State Determination
State is determined by matching the file path against `state_directories`:

```
File: /documents/review/pending/legal/urgent/contract.pdf

Lookup:
- Check state_directories for "review/pending" → Found: state = "review" ✅
```

**Algorithm:**
```go
func extractState(filePath string, workflowHome string, stateDirs map[string]string) string {
    relativePath := strings.TrimPrefix(filePath, workflowHome+"/")

    // Try matching from longest to shortest path (up to 5 levels)
    for depth := 5; depth >= 1; depth-- {
        stateRoot := getPathPrefix(relativePath, depth)

        for state, dirPath := range stateDirs {
            if strings.HasPrefix(relativePath, dirPath+"/") {
                return state
            }
        }
    }

    return ""
}
```

#### 3. Subdirectory Naming
- **Any valid filesystem name allowed** (no restrictions on state name conflicts since state directories are explicitly defined)
- **Cannot start with `.`** (reserved for special files)
- **Standard filesystem restrictions apply** (no `/`, null bytes, etc.)

**Examples:**
```
/documents/drafts/
├── review/          ✅ OK (not ambiguous - "review" state is at review/pending)
├── legal/           ✅ OK
├── 2025/            ✅ OK
├── high-priority/   ✅ OK
└── .workflow/       ❌ FORBIDDEN (special file prefix)
```

#### 4. Transition Behavior - Preserve Structure

When transitioning between states, subdirectory structure is **preserved** by default.

**Manual transition (mv command):**
```bash
# User explicitly specifies target path
mv /documents/drafts/legal/2025/contract.pdf /documents/public/legal/2025/contract.pdf
```

**Automatic transition (next command):**
```bash
# System preserves structure automatically
next /documents/drafts/legal/2025/contract.pdf review

# Results in:
# /documents/review/pending/legal/2025/contract.pdf
```

**Implementation:**
```go
func preserveStructure(oldPath, fromStateDir, toStateDir string) string {
    // Extract relative path within state
    relativePath := strings.TrimPrefix(oldPath, fromStateDir+"/")

    // Preserve structure in new state
    return filepath.Join(toStateDir, relativePath)
}

// Example:
// oldPath: /documents/drafts/legal/2025/contract.pdf
// fromStateDir: /documents/drafts
// toStateDir: /documents/review/pending
// Result: /documents/review/pending/legal/2025/contract.pdf
```

#### 5. Same-State Movement

Moving files within the same state (reorganization) does **NOT** trigger workflow validation:

```bash
# Reorganization within draft state - no workflow check
mv /documents/drafts/legal/contract.pdf /documents/drafts/marketing/contract.pdf

# Both paths resolve to state "draft", so this is allowed without gate evaluation
```

#### 6. Special File Restrictions

- **`.` prefix directories forbidden:** Cannot create directories starting with `.` (reserved for special files)
- **Nested `.workflow` forbidden:** Cannot create a `.workflow` file under another workflow's scope to prevent state ambiguity

**Rationale:** Since workflows bind states to physical directory paths, nested workflows would create ambiguity about which state a file belongs to. See Section 8.1 for detailed explanation.

```
/documents/
├── .workflow                    ✅ OK (workflow home)
├── drafts/
│   ├── .workflow               ❌ FORBIDDEN (nested workflow - creates state ambiguity)
│   ├── .config/                ❌ FORBIDDEN (. prefix directory)
│   └── legal/                  ✅ OK
└── .rego                        ✅ OK (special file at workflow home)
```

**Validation:** When creating a `.workflow` file:
1. System checks no `.workflow` exists in any parent directory
2. System checks no `.workflow` files exist in any subdirectories
3. If either check fails, creation is rejected

#### 7. System-Admin Bypass

- **Regular admin:** Cannot bypass workflow rules (even with `admin` group)
- **System-admin group:** Can bypass all domain layer operations including workflow validation
- **Move restrictions:** Even admins cannot move files to states not defined in the workflow (only system-admin can)

**Example:**
```bash
# Admin user (alice, groups: [admin])
mv /documents/drafts/file.pdf /documents/invalid-state/file.pdf
# ❌ FORBIDDEN: "invalid-state" not in workflow definition

# System-admin user (system, groups: [system-admin])
mv /documents/drafts/file.pdf /documents/invalid-state/file.pdf
# ✅ ALLOWED: system-admin bypasses workflow engine
```

## 12. Security Considerations

### 12.1. Workflow Bypass Prevention

- Workflow validation occurs at the **service layer**, not just API layer
- Direct database modifications bypass workflows (administrative operations only)
- System admin tokens can bypass workflow validation if needed

### 12.2. Gate Security

- **Group gates:** Rely on authentication system's group assignments
- **Metadata gates:** Metadata must be validated and trusted
- **Rego gates:** Sandboxed execution, no file system or network access

### 12.3. Audit Trail

All workflow transitions should be logged:

```json
{
  "event": "workflow.transition.succeeded",
  "timestamp": "2025-01-05T10:30:00Z",
  "actor": "alice",
  "file_id": "file-123",
  "from_state": "draft",
  "to_state": "review",
  "workflow": "document-approval",
  "gates_evaluated": [
    {"type": "group", "result": "pass"}
  ]
}
```

## 13. Future Enhancements

### 13.1. Workflow Versioning

Support for versioning workflow definitions:

```yaml
version: 2
initial_state: draft

migration_from_v1:
  state_mappings:
    old-draft: draft
    old-review: review
```

### 13.2. Workflow Analytics

Track metrics:
- Average time in each state
- Transition frequencies
- Bottleneck identification
- Gate failure analysis

### 13.3. Visual Workflow Editor

Web UI for:
- Creating/editing workflows visually
- Viewing workflow as state diagram
- Testing transitions

### 13.4. Workflow Templates

Pre-built workflows:
- Document approval
- Content moderation
- CI/CD pipeline
- Issue tracking

### 13.5. State Callbacks

Execute custom code when entering/exiting states:

```yaml
states:
  published:
    on_enter:
      - type: webhook
        url: https://cdn.example.com/invalidate
      - type: metadata
        set:
          published_at: "{{now}}"
```

## 14. References

- **System Design:** `docs/DESIGN.md`
- **Security Model:** `docs/SECURITY.md`
- **Event System:** `pkg/events/lifecycle_types.go`
- **Authorization:** `pkg/middleware/authorization.go`
- **Implementation Plan:** `workflow-plan.md`

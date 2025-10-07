# Workflows - Complete Guide

**Last Updated**: October 7, 2025  
**Status**: Production Ready ✅

---

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Architecture](#architecture)
4. [Configuration](#configuration)
5. [API Reference](#api-reference)
6. [Authorization](#authorization)
7. [Events & Audit](#events--audit)
8. [Examples](#examples)
9. [Best Practices](#best-practices)

---

## Overview

The VFS Workflow system provides **directory-as-state** file lifecycle management with automatic validation gates and audit trails.

### Key Features

- ✅ **Directory-Based States** - Each state is a subdirectory
- ✅ **Rego Policy Gates** - Validate transitions with OPA policies
- ✅ **Automatic Audit Trail** - All transitions logged to database
- ✅ **Event Emission** - Real-time notifications via event system
- ✅ **Inheritance** - Workflows inherit from parent directories
- ✅ **REST API** - Query workflow info and available transitions

### Use Cases

- **Document Approval** - draft → review → approved → published
- **Content Moderation** - submitted → pending → approved/rejected
- **Order Processing** - created → paid → shipped → delivered
- **Ticket Management** - new → in_progress → resolved → closed

---

## Core Concepts

### State Directories

States are represented as subdirectories. Moving a file between state directories triggers workflow transitions.

```
/projects/alpha/
  .workflow              # Workflow definition
  draft/                 # Initial state
    document1.txt
  review/                # Review state
  final/                 # Final state
```

### Transitions

Transitions define allowed movements between states:

```yaml
states:
  draft:
    transitions:
      - to: review
        description: "Submit for review"
  review:
    transitions:
      - to: final
        description: "Approve document"
      - to: draft
        description: "Request changes"
```

### Gates

Gates are Rego policies that validate whether a transition is allowed:

```yaml
gate_policy: |
  package workflow.gates
  
  # Allow editors to submit for review
  allow_transition["draft"]["review"] {
    input.user.groups[_] == "editors"
  }
```

---

## Architecture

### Components

```
┌─────────────────────────────────────────────┐
│           File Operation (Move)             │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│         WorkflowEngine.Validate()           │
│  - Load workflow definition                 │
│  - Detect state change                      │
│  - Evaluate gate policy                     │
│  - Record audit log                         │
│  - Emit events                              │
└──────────────────┬──────────────────────────┘
                   │
       ┌───────────┼───────────┐
       ▼           ▼           ▼
┌──────────┐ ┌─────────┐ ┌─────────┐
│ Database │ │  Rego   │ │ Events  │
│  Audit   │ │  Gate   │ │ System  │
└──────────┘ └─────────┘ └─────────┘
```

### Database Schema

```sql
CREATE TABLE workflow_audits (
    id VARCHAR(36) PRIMARY KEY,
    directory_id VARCHAR(36) NOT NULL,
    file_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    from_state VARCHAR(100) NOT NULL,
    to_state VARCHAR(100) NOT NULL,
    transition_allowed BOOLEAN NOT NULL,
    policy_result TEXT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_directory (directory_id),
    INDEX idx_file (file_id),
    INDEX idx_user (user_id)
);
```

### Workflow Loader

```go
type WorkflowLoader struct {
    fileRepo db.FileRepository
    dirRepo  db.DirectoryRepository
    cache    *sync.Map
    cacheTTL time.Duration
}

// Load workflow with caching and inheritance
func (l *WorkflowLoader) Load(ctx context.Context, directoryPath string) (*WorkflowDefinition, error)
```

### Workflow Engine

```go
type WorkflowEngine struct {
    loader         *WorkflowLoader
    gates          *WorkflowGates
    auditRepo      db.WorkflowAuditRepository
    eventDispatcher events.EventTrigger
}

// Validate file move against workflow rules
func (e *WorkflowEngine) ValidateFileMove(
    ctx context.Context,
    file *models.File,
    sourceDir, targetDir *models.Directory,
    user *User,
) error
```

---

## Configuration

### .workflow File Format

```yaml
# State directory mapping
state_directories:
  draft: "draft"
  review: "review"
  final: "final"

# Initial state for new files
initial_state: "draft"

# State definitions with transitions
states:
  draft:
    transitions:
      - to: review
        description: "Submit for review"
  
  review:
    transitions:
      - to: final
        description: "Approve"
      - to: draft
        description: "Request changes"
  
  final:
    transitions: []  # Terminal state

# Gate policy (inline)
gate_policy: |
  package workflow.gates
  
  # Editors can submit to review
  allow_transition["draft"]["review"] {
    input.user.groups[_] == "editors"
  }
  
  # Admins can approve
  allow_transition["review"]["final"] {
    input.user.groups[_] == "admins"
  }

# Or reference external policy
gate_policy_ref: "workflow-policy.rego"
```

### Schema Validation

The `.workflow` file is validated against JSON schema:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["state_directories", "initial_state", "states"],
  "properties": {
    "state_directories": {
      "type": "object",
      "minProperties": 2,
      "patternProperties": {
        "^[a-z0-9][a-z0-9_-]*$": {
          "type": "string",
          "pattern": "^[a-zA-Z0-9_/-]+$"
        }
      }
    },
    "initial_state": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9_-]*$"
    },
    "states": {
      "type": "object",
      "minProperties": 2
    },
    "gate_policy": {"type": "string"},
    "gate_policy_ref": {"type": "string"}
  }
}
```

### Rego Policy Input

Gate policies receive this input:

```json
{
  "user": {
    "id": "alice",
    "groups": ["editors", "users"],
    "role": "editor"
  },
  "file": {
    "id": "file-123",
    "name": "document.txt",
    "directory_id": "dir-456"
  },
  "transition": {
    "from": "draft",
    "to": "review"
  },
  "directory": {
    "id": "dir-456",
    "path": "/projects/alpha"
  }
}
```

---

## API Reference

### GET /api/v1/workflow/info

Get workflow information for a directory.

**Request**:
```bash
GET /api/v1/workflow/info?directory_id=dir-123
Authorization: Bearer <token>
```

**Response**:
```json
{
  "workflow": {
    "directory_id": "dir-123",
    "directory_path": "/projects/alpha",
    "state_directories": {
      "draft": "draft",
      "review": "review",
      "final": "final"
    },
    "initial_state": "draft",
    "states": {
      "draft": {
        "transitions": [
          {"to": "review", "description": "Submit for review"}
        ]
      }
    },
    "has_gate_policy": true
  }
}
```

### GET /api/v1/workflow/transitions

Get available transitions for a file.

**Request**:
```bash
GET /api/v1/workflow/transitions?file_id=file-123
Authorization: Bearer <token>
```

**Response**:
```json
{
  "file": {
    "id": "file-123",
    "name": "document.txt",
    "directory_path": "/projects/alpha/draft"
  },
  "current_state": "draft",
  "available_transitions": [
    {
      "to_state": "review",
      "to_directory": "/projects/alpha/review",
      "description": "Submit for review",
      "allowed": true
    }
  ]
}
```

### GET /api/v1/workflow/next

Get the next workflow state for a file.

**Request**:
```bash
GET /api/v1/workflow/next?file_id=file-123
Authorization: Bearer <token>
```

**Response**:
```json
{
  "file": {
    "id": "file-123",
    "name": "document.txt"
  },
  "current_state": "draft",
  "next_state": "review",
  "next_directory": "/projects/alpha/review",
  "allowed": true
}
```

**Error Response** (403):
```json
{
  "error": "workflow transition denied",
  "details": {
    "from": "draft",
    "to": "review",
    "reason": "user not in required group 'editors'"
  }
}
```

---

## Authorization

### System Admin Bypass

Users with `system-admin` role bypass ALL workflow checks:

```go
if IsSystemAdmin(user.Role) {
    return nil // Allow transition
}
```

### OPA Integration

Workflows can access user context in policies:

```rego
package vfs.authz

# Allow file move if workflow permits
allow {
    input.action == "move_file"
    
    # Get workflow context
    workflow_allowed := workflow_check(
        input.user,
        input.file,
        input.source_directory,
        input.target_directory
    )
    
    workflow_allowed == true
}
```

### Authorization Middleware

The authorization middleware injects workflow context:

```go
// Check workflow constraints
if targetDir != sourceDir {
    // This is a move - check workflow
    workflowAllowed := checkWorkflow(ctx, file, sourceDir, targetDir, user)
    authzInput["workflow_allowed"] = workflowAllowed
}
```

---

## Events & Audit

### Event Types

The workflow system emits several event types:

```go
const (
    // State transition events
    EventWorkflowTransitionStarted   = "workflow.transition.started"
    EventWorkflowTransitionSucceeded = "workflow.transition.succeeded"
    EventWorkflowTransitionFailed    = "workflow.transition.failed"

    // Protection events (when workflows block operations)
    EventWorkflowDeletionBlocked   = "workflow.deletion.blocked"
    EventWorkflowEscapeBlocked     = "workflow.escape.blocked"
    EventWorkflowStateDirProtected = "workflow.state_dir.protected"
    EventWorkflowCreateBlocked     = "workflow.create.blocked"
)
```

**Event Descriptions:**

| Event Type | When Emitted | Description |
|------------|--------------|-------------|
| `workflow.transition.started` | Move begins | File move operation between states initiated |
| `workflow.transition.succeeded` | Move succeeds | File successfully transitioned to new state |
| `workflow.transition.failed` | Move denied | Transition blocked by gate policy |
| `workflow.deletion.blocked` | Delete attempt | Attempt to delete file in workflow-managed directory |
| `workflow.escape.blocked` | Move out | Attempt to move file outside workflow directory tree |
| `workflow.state_dir.protected` | Direct modify | Attempt to directly create/delete state directories |
| `workflow.create.blocked` | Create in state | Attempt to create file directly in state directory |

### Event Payloads

**Transition Events** (`workflow.transition.*`):

```json
{
  "event_type": "workflow.transition.succeeded",
  "timestamp": "2025-10-07T12:34:56Z",
  "data": {
    "file_id": "file-123",
    "file_name": "document.txt",
    "user_id": "alice",
    "from_state": "draft",
    "to_state": "review",
    "directory_path": "/projects/alpha",
    "allowed": true,
    "policy_result": "transition allowed by policy"
  }
}
```

**Protection Events** (`workflow.*.blocked`):

```json
{
  "event_type": "workflow.deletion.blocked",
  "timestamp": "2025-10-07T12:34:56Z",
  "data": {
    "file_path": "/projects/alpha/draft/document.txt",
    "workflow_path": "/projects/alpha",
    "operation": "delete",
    "actor": {
      "id": "alice",
      "groups": ["users"]
    },
    "error_message": "cannot delete files in workflow-managed directories",
    "metadata": {}
  }
}
```

### Audit Trail

Every transition attempt is logged to the database:

```go
type WorkflowAudit struct {
    ID                string
    DirectoryID       string
    FileID            string
    UserID            string
    FromState         string
    ToState           string
    TransitionAllowed bool
    PolicyResult      string
    ErrorMessage      string
    CreatedAt         time.Time
}
```

### Event Handlers

Register handlers in `.events` file:

```json
{
  "handlers": [
    {
      "name": "workflow-transitions",
      "events": [
        "workflow.transition.succeeded",
        "workflow.transition.failed"
      ],
      "type": "webhook",
      "config": {
        "url": "https://notify.example.com/workflow/transitions",
        "method": "POST"
      }
    },
    {
      "name": "workflow-protection-alerts",
      "events": [
        "workflow.deletion.blocked",
        "workflow.escape.blocked",
        "workflow.create.blocked"
      ],
      "type": "webhook",
      "config": {
        "url": "https://notify.example.com/workflow/security",
        "method": "POST"
      }
    },
    {
      "name": "all-workflow-events",
      "events": ["workflow.>"],
      "type": "log",
      "config": {
        "level": "info",
        "message": "Workflow event: {{.event.type}}"
      }
    }
  ]
}
```

---

## Examples

### Example 1: Simple Document Approval

**Structure**:
```
/docs/
  .workflow
  draft/
    proposal.txt
  review/
  approved/
```

**Workflow**:
```yaml
state_directories:
  draft: "draft"
  review: "review"
  approved: "approved"

initial_state: "draft"

states:
  draft:
    transitions:
      - to: review
  review:
    transitions:
      - to: approved
      - to: draft
  approved:
    transitions: []

gate_policy: |
  package workflow.gates
  
  # Anyone can submit for review
  allow_transition["draft"]["review"] {
    input.user.authenticated
  }
  
  # Only admins can approve
  allow_transition["review"]["approved"] {
    input.user.groups[_] == "admins"
  }
  
  # Reviewers can request changes
  allow_transition["review"]["draft"] {
    input.user.groups[_] == "reviewers"
  }
```

**Usage**:
```bash
# Upload to draft
vfs-cli upload proposal.txt /docs/draft/

# Submit for review (anyone can do this)
vfs-cli mv /docs/draft/proposal.txt /docs/review/
# ✅ Transition recorded, event emitted

# Try to approve (fails if not admin)
vfs-cli mv /docs/review/proposal.txt /docs/approved/
# ❌ Error: transition denied - user not in 'admins' group
```

### Example 2: Content Moderation

**Structure**:
```
/uploads/
  .workflow
  pending/
  approved/
  rejected/
```

**Workflow**:
```yaml
state_directories:
  pending: "pending"
  approved: "approved"
  rejected: "rejected"

initial_state: "pending"

states:
  pending:
    transitions:
      - to: approved
        description: "Approve content"
      - to: rejected
        description: "Reject content"
  
  approved:
    transitions: []
  
  rejected:
    transitions:
      - to: pending
        description: "Resubmit for review"

gate_policy: |
  package workflow.gates
  
  # Moderators can approve or reject
  allow_transition["pending"]["approved"] {
    input.user.groups[_] == "moderators"
  }
  
  allow_transition["pending"]["rejected"] {
    input.user.groups[_] == "moderators"
  }
  
  # Users can resubmit rejected content
  allow_transition["rejected"]["pending"] {
    true  # Anyone can resubmit
  }
```

### Example 3: Complex Order Processing

**Workflow**:
```yaml
state_directories:
  created: "created"
  paid: "paid"
  processing: "processing"
  shipped: "shipped"
  delivered: "delivered"
  cancelled: "cancelled"

initial_state: "created"

states:
  created:
    transitions:
      - to: paid
      - to: cancelled
  
  paid:
    transitions:
      - to: processing
      - to: cancelled
  
  processing:
    transitions:
      - to: shipped
      - to: cancelled
  
  shipped:
    transitions:
      - to: delivered
  
  delivered:
    transitions: []
  
  cancelled:
    transitions: []

gate_policy: |
  package workflow.gates
  
  # Payment system can mark as paid
  allow_transition["created"]["paid"] {
    input.user.groups[_] == "payment-system"
  }
  
  # Warehouse can process
  allow_transition["paid"]["processing"] {
    input.user.groups[_] == "warehouse"
  }
  
  # Shipping can mark as shipped
  allow_transition["processing"]["shipped"] {
    input.user.groups[_] == "shipping"
  }
  
  # System can mark as delivered
  allow_transition["shipped"]["delivered"] {
    input.user.groups[_] == "system"
  }
  
  # Support can cancel
  allow_transition[from]["cancelled"] {
    from != "delivered"
    input.user.groups[_] == "support"
  }
```

---

## Best Practices

### 1. **Design Clear State Machines**

- ✅ Keep states simple and meaningful
- ✅ Avoid too many states (5-7 is ideal)
- ✅ Define clear terminal states
- ✅ Consider bi-directional transitions where needed

### 2. **Write Explicit Gate Policies**

```rego
# ❌ BAD: Too permissive
allow_transition[from][to] {
    true
}

# ✅ GOOD: Explicit rules
allow_transition["draft"]["review"] {
    input.user.groups[_] == "editors"
    input.file.size < 10485760  # 10MB limit
}
```

### 3. **Use Workflow Events**

Set up event handlers to:
- Send notifications on state changes
- Trigger external systems
- Update dashboards
- Send emails/slack messages

### 4. **Leverage Audit Trail**

Query the audit log for:
- Compliance reports
- User activity tracking
- Debugging transition failures
- Performance analytics

```sql
-- Find failed transitions
SELECT * FROM workflow_audits
WHERE transition_allowed = FALSE
ORDER BY created_at DESC;

-- User activity report
SELECT user_id, 
       COUNT(*) as total_transitions,
       SUM(CASE WHEN transition_allowed THEN 1 ELSE 0 END) as successful
FROM workflow_audits
GROUP BY user_id;
```

### 5. **Test Gate Policies**

Use OPA's testing framework:

```rego
# workflow_test.rego
test_editors_can_submit {
    allow_transition["draft"]["review"] with input as {
        "user": {"groups": ["editors"]},
        "transition": {"from": "draft", "to": "review"}
    }
}

test_users_cannot_approve {
    not allow_transition["review"]["approved"] with input as {
        "user": {"groups": ["users"]},
        "transition": {"from": "review", "to": "approved"}
    }
}
```

### 6. **Use Inheritance Wisely**

Workflows inherit from parent directories:

```
/projects/
  .workflow                 # Global workflow
  alpha/
    .workflow              # Overrides parent
    draft/
  beta/                     # Uses parent workflow
    draft/
```

### 7. **Monitor Performance**

Watch for:
- Slow gate policy evaluations
- High audit log growth
- Cache hit rates
- Transition failure rates

---

## Troubleshooting

### Transition Denied

**Symptom**: File move fails with "workflow transition denied"

**Debugging**:
1. Check audit log: `SELECT * FROM workflow_audits WHERE file_id = ?`
2. Examine `policy_result` and `error_message`
3. Test policy with OPA: `opa eval -d policy.rego -i input.json`
4. Verify user groups: Check `.user` and `.group` files

### Workflow Not Found

**Symptom**: "workflow not found for directory"

**Solutions**:
- Create `.workflow` file in directory or parent
- Check file permissions
- Verify YAML syntax
- Check cache invalidation

### Gate Policy Errors

**Symptom**: "gate policy evaluation failed"

**Solutions**:
- Validate Rego syntax: `opa check policy.rego`
- Check for undefined variables
- Ensure policy package matches expected name
- Review input structure

---

## Related Documentation

- [Design Document](./DESIGN.md) - Overall architecture
- [API Reference](./README.md) - Complete API docs
- [Security](./SECURITY.md) - Security model
- [Special Files](./SPECIAL_FILES_FRAMEWORK.md) - Framework details

---

*Last Updated: October 7, 2025*  
*Status: Production Ready ✅*

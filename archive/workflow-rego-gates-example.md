# Workflow Rego Gates - Complete Example

## Overview

Workflow gates use **Rego policies** (same as authorization) for maximum flexibility and power. This document shows complete examples of `.workflow` files with Rego-based gates.

---

## Example 1: Inline Rego Policy

### Directory Structure
```
/documents/
├── .workflow          # Workflow definition with inline policy
├── drafts/           # initial_state
├── review/
└── published/
```

### .workflow File (with inline Rego)

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
        description: "Submit for review"

  review:
    transitions:
      - to: published
        description: "Approve and publish"
      - to: draft
        description: "Send back for revision"

  published:
    transitions: []

# Inline Rego policy
gate_policy: |
  package vfs.workflow.gates

  # Allow editors to submit drafts for review
  allow {
      input.transition.from == "draft"
      input.transition.to == "review"
      input.user.groups[_] == "editor"
  }

  # Allow approvers to publish from review
  allow {
      input.transition.from == "review"
      input.transition.to == "published"
      input.user.groups[_] == "approver"
  }

  # Allow editors to send back to draft
  allow {
      input.transition.from == "review"
      input.transition.to == "draft"
      input.user.groups[_] == "editor"
  }

  # Allow deletion of small draft files
  allow {
      input.transition.operation == "delete"
      input.transition.from == "draft"
      input.file.size < 10485760  # < 10MB
  }

  # Allow admins to delete anything
  allow {
      input.transition.operation == "delete"
      input.user.groups[_] == "admin"
  }
```

---

## Example 2: External Rego Policy File

### Directory Structure
```
/documents/
├── .workflow          # Workflow definition (references external policy)
├── .workflow.rego     # Rego policy file
├── drafts/
├── review/
└── published/
```

### .workflow File

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

  review:
    transitions:
      - to: published
      - to: draft

  published:
    transitions: []

# Reference external policy file
gate_policy_ref: ".workflow.rego"
```

### .workflow.rego File

```rego
package vfs.workflow.gates

# Import helper functions (optional)
import future.keywords.if
import future.keywords.in

# Allow editors to submit for review
allow if {
    input.transition.from == "draft"
    input.transition.to == "review"
    "editor" in input.user.groups
}

# Allow approvers to publish
allow if {
    input.transition.from == "review"
    input.transition.to == "published"
    "approver" in input.user.groups
}

# Allow editors to send back to draft
allow if {
    input.transition.from == "review"
    input.transition.to == "draft"
    "editor" in input.user.groups
}

# Deletion rules
allow if {
    input.transition.operation == "delete"
    can_delete
}

can_delete if {
    # Small files in draft
    input.transition.from == "draft"
    input.file.size < 10485760
}

can_delete if {
    # Admins can delete anything
    "admin" in input.user.groups
}
```

---

## Example 3: Advanced - Metadata and Content Validation

### .workflow.rego

```rego
package vfs.workflow.gates

# High-priority documents need senior approvers
allow if {
    input.transition.from == "review"
    input.transition.to == "published"
    input.file.metadata.priority == "high"
    "senior-approver" in input.user.groups
}

# Normal priority can be approved by regular approvers
allow if {
    input.transition.from == "review"
    input.transition.to == "published"
    input.file.metadata.priority != "high"
    "approver" in input.user.groups
}

# Block publishing if JSON content has errors
deny if {
    input.transition.to == "published"
    input.file.content.errors
    count(input.file.content.errors) > 0
}

# Block publishing if required fields missing
deny if {
    input.transition.to == "published"
    input.file.mime_type == "application/json"
    not input.file.content.title
}

# Allow draft → review only if document has author
allow if {
    input.transition.from == "draft"
    input.transition.to == "review"
    input.file.metadata.author
    input.file.metadata.author != ""
}

# Time-based restriction (business hours only)
allow if {
    input.transition.to == "published"
    is_business_hours
    "approver" in input.user.groups
}

is_business_hours if {
    # Check if current hour is between 9 AM and 5 PM
    # Note: Would need to pass current time in input
    hour := input.workflow.current_hour
    hour >= 9
    hour < 17
}
```

---

## Example 4: Multi-Stage Approval

### .workflow File

```yaml
state_directories:
  draft: "drafts"
  legal_review: "legal"
  technical_review: "technical"
  final_approval: "final"
  published: "published"

initial_state: draft

states:
  draft:
    transitions:
      - to: legal_review
      - to: technical_review

  legal_review:
    transitions:
      - to: final_approval
      - to: draft

  technical_review:
    transitions:
      - to: final_approval
      - to: draft

  final_approval:
    transitions:
      - to: published
      - to: draft

  published:
    transitions: []

gate_policy_ref: ".workflow.rego"
```

### .workflow.rego

```rego
package vfs.workflow.gates

# Allow submission to either review track
allow if {
    input.transition.from == "draft"
    input.transition.to in ["legal_review", "technical_review"]
    "editor" in input.user.groups
}

# Legal review approval
allow if {
    input.transition.from == "legal_review"
    input.transition.to == "final_approval"
    "legal-reviewer" in input.user.groups
    input.file.metadata.legal_approved == true
}

# Technical review approval
allow if {
    input.transition.from == "technical_review"
    input.transition.to == "final_approval"
    "technical-reviewer" in input.user.groups
    input.file.metadata.technical_approved == true
}

# Final approval requires both reviews completed
allow if {
    input.transition.from == "final_approval"
    input.transition.to == "published"
    "senior-approver" in input.user.groups
    input.file.metadata.legal_approved == true
    input.file.metadata.technical_approved == true
}

# Allow sending back to draft from any review state
allow if {
    input.transition.from in ["legal_review", "technical_review", "final_approval"]
    input.transition.to == "draft"
    "editor" in input.user.groups
}
```

---

## Input Structure Reference

When evaluating Rego policies, the workflow engine provides this input:

```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "groups": ["editor", "approver"]
  },
  "transition": {
    "from": "draft",
    "to": "review",
    "operation": "move"
  },
  "file": {
    "path": "/documents/drafts/legal/contract.pdf",
    "name": "contract.pdf",
    "metadata": {
      "author": "Alice",
      "priority": "high",
      "legal_approved": false
    },
    "content": {
      "title": "Software License Agreement",
      "version": "2.0"
    },
    "size": 524288,
    "mime_type": "application/pdf"
  },
  "workflow": {
    "name": "document-approval",
    "workflow_home": "/documents",
    "initial_state": "draft",
    "available_states": ["draft", "review", "published"]
  }
}
```

---

## Cache Invalidation

When `.workflow` or `.workflow.rego` files are updated:

1. **File update event** triggers cache invalidation
2. **WorkflowGateEvaluator.InvalidateCache()** removes compiled query
3. **Next transition** recompiles Rego policy
4. **New query cached** for 5 minutes (configurable)

This matches the existing pattern used for `.rego` authorization policies.

---

## Testing Rego Policies

Use OPA's test framework to test workflow policies:

```rego
# .workflow_test.rego
package vfs.workflow.gates

test_editor_can_submit_for_review {
    allow with input as {
        "user": {"groups": ["editor"]},
        "transition": {"from": "draft", "to": "review"}
    }
}

test_non_editor_cannot_submit {
    not allow with input as {
        "user": {"groups": ["viewer"]},
        "transition": {"from": "draft", "to": "review"}
    }
}

test_approver_can_publish {
    allow with input as {
        "user": {"groups": ["approver"]},
        "transition": {"from": "review", "to": "published"}
    }
}

test_high_priority_needs_senior_approver {
    not allow with input as {
        "user": {"groups": ["approver"]},
        "transition": {"from": "review", "to": "published"},
        "file": {"metadata": {"priority": "high"}}
    }

    allow with input as {
        "user": {"groups": ["senior-approver"]},
        "transition": {"from": "review", "to": "published"},
        "file": {"metadata": {"priority": "high"}}
    }
}
```

Run tests:
```bash
opa test .workflow.rego .workflow_test.rego
```

---

## Benefits of Rego-Based Gates

1. **Unified Language**: Same Rego used for authorization and workflow gates
2. **Maximum Flexibility**: Complex logic, content inspection, time-based rules
3. **Testable**: Use OPA test framework
4. **Cacheable**: Compiled queries cached for performance
5. **Composable**: Combine multiple rules with AND/OR logic naturally
6. **Familiar**: Team already knows Rego from authorization policies
7. **External Tools**: Use OPA CLI for policy development and testing

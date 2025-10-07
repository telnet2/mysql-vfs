# Workflow Authorization Integration

This document explains how workflow state information is integrated into the authorization system, allowing OPA policies to make decisions based on workflow state.

## Overview

When a workflow is active for a resource, the authorization middleware automatically extracts workflow context and makes it available to Rego policies. This enables fine-grained access control based on workflow state.

## Workflow Context Structure

The workflow context is added to the OPA input under the `workflow` key:

```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "groups": ["editors", "authors"]
  },
  "resource": {
    "path": "/documents/draft/proposal.txt",
    "type": "file",
    "owners": ["legal-team"]
  },
  "action": "read",
  "workflow": {
    "active": true,
    "current_state": "draft",
    "target_state": "",
    "valid_states": ["draft", "review", "published"]
  }
}
```

### Fields

- **active** (bool): Whether a workflow is active for this resource
- **current_state** (string): The current workflow state (e.g., "draft", "review", "published")
- **target_state** (string): The target state for move operations (populated during move)
- **valid_states** ([]string): List of all valid states defined in the workflow

## Example Policies

### Example 1: State-Based Read Access

Allow users to read files only in certain states:

```rego
package vfs.authz

default allow = false

# Editors can read files in draft and review states
allow {
    input.action == "read"
    input.user.groups[_] == "editors"
    input.workflow.active == true
    input.workflow.current_state == "draft"
}

allow {
    input.action == "read"
    input.user.groups[_] == "editors"
    input.workflow.current_state == "review"
}

# Published files are readable by everyone
allow {
    input.action == "read"
    input.workflow.current_state == "published"
}
```

### Example 2: State-Based Write Access

Restrict modifications based on workflow state:

```rego
package vfs.authz

default allow = false

# Only authors can write to draft state
allow {
    input.action == "write"
    input.user.groups[_] == "authors"
    input.workflow.active == true
    input.workflow.current_state == "draft"
}

# Reviewers can write to review state
allow {
    input.action == "write"
    input.user.groups[_] == "reviewers"
    input.workflow.current_state == "review"
}

# Published files are read-only (no write access)
```

### Example 3: State-Based Deletion

Control deletion based on workflow state:

```rego
package vfs.authz

default allow = false

# Files in draft can be deleted by authors
allow {
    input.action == "delete"
    input.user.groups[_] == "authors"
    input.workflow.active == true
    input.workflow.current_state == "draft"
}

# Files in review or published states require admin approval
allow {
    input.action == "delete"
    input.user.groups[_] == "admins"
    input.workflow.current_state != "draft"
}
```

### Example 4: Combined Workflow and Ownership

Combine workflow state with resource ownership:

```rego
package vfs.authz

default allow = false

# Resource owners can read their files in any state
allow {
    input.action == "read"
    input.user.groups[_] == input.resource.owners[_]
}

# Non-owners can only read published files
allow {
    input.action == "read"
    input.workflow.current_state == "published"
    not is_owner
}

is_owner {
    input.user.groups[_] == input.resource.owners[_]
}

# Only owners can modify files in draft
allow {
    input.action == "write"
    input.workflow.current_state == "draft"
    is_owner
}
```

### Example 5: System Admin Override

Ensure system admins can bypass workflow restrictions:

```rego
package vfs.authz

default allow = false

# System admins bypass all checks
allow {
    input.user.groups[_] == "system-admin"
}

# Regular workflow-based rules below...
allow {
    input.action == "read"
    input.workflow.active == true
    check_workflow_read_permission
}

check_workflow_read_permission {
    input.workflow.current_state == "published"
}

check_workflow_read_permission {
    input.user.groups[_] == "editors"
    input.workflow.current_state == "draft"
}
```

### Example 6: State Transition Authorization

Control who can move files between states:

```rego
package vfs.authz

default allow = false

# Allow read for everyone
allow {
    input.action == "read"
}

# Draft to review transition requires editor group
allow {
    input.action == "move"
    input.workflow.current_state == "draft"
    input.workflow.target_state == "review"
    input.user.groups[_] == "editors"
}

# Review to published requires approver group
allow {
    input.action == "move"
    input.workflow.current_state == "review"
    input.workflow.target_state == "published"
    input.user.groups[_] == "approvers"
}

# Review back to draft is allowed for editors
allow {
    input.action == "move"
    input.workflow.current_state == "review"
    input.workflow.target_state == "draft"
    input.user.groups[_] == "editors"
}
```

### Example 7: Complex Business Logic

Implement complex authorization rules based on multiple factors:

```rego
package vfs.authz

import future.keywords.if
import future.keywords.in

default allow = false

# System admins always allowed
allow if input.user.groups[_] == "system-admin"

# Read access rules
allow if {
    input.action == "read"
    can_read_in_state(input.workflow.current_state, input.user.groups)
}

# Write access rules
allow if {
    input.action == "write"
    can_write_in_state(input.workflow.current_state, input.user.groups)
    not is_locked(input.resource.path)
}

# Helper functions
can_read_in_state("draft", groups) if {
    "authors" in groups
}

can_read_in_state("review", groups) if {
    {"editors", "reviewers", "authors"}[_] in groups
}

can_read_in_state("published", _) = true

can_write_in_state("draft", groups) if {
    "authors" in groups
}

can_write_in_state("review", groups) if {
    "reviewers" in groups
}

can_write_in_state("published", _) = false

is_locked(path) if {
    # Check if file is locked (would need additional metadata)
    # This is a placeholder for custom logic
    false
}
```

## Best Practices

### 1. Always Check Workflow Active

Before using workflow state, check if a workflow is active:

```rego
allow {
    input.workflow.active == true
    input.workflow.current_state == "draft"
    # ... other conditions
}
```

### 2. Provide Fallback for Non-Workflow Resources

Ensure policies work for resources without workflows:

```rego
# Allow read for non-workflow resources
allow {
    input.action == "read"
    not input.workflow.active
}

# Or use a default rule
default allow = true  # Only if appropriate for your security model!
```

### 3. Use Helper Functions

Create reusable helper functions for complex logic:

```rego
can_access_state(state, required_groups) {
    input.workflow.current_state == state
    input.user.groups[_] == required_groups[_]
}

allow {
    input.action == "read"
    can_access_state("draft", ["authors", "editors"])
}
```

### 4. Combine with Workflow Gates

Remember that authorization and workflow gates serve different purposes:

- **Authorization**: Controls whether a user can perform an action
- **Workflow Gates**: Controls whether a transition is allowed based on business rules

Both layers provide defense in depth.

### 5. Test Your Policies

Always test your policies with various scenarios:

```bash
# Test with OPA CLI
opa eval -d policy.rego -i input.json 'data.vfs.authz.allow'
```

## Integration with Workflow System

The workflow context is automatically extracted by the authorization middleware when:

1. A `WorkflowLoader` is configured in the middleware
2. A workflow definition exists for the resource path
3. The resource is within the workflow's scope

The workflow context is read-only in authorization policies. Actual workflow transitions are still controlled by the workflow engine and its gates.

## Debugging

To debug workflow context in authorization:

1. Enable logging in the authorization middleware
2. Check the input being passed to OPA
3. Use OPA's built-in tracing: `opa eval --trace ...`
4. Verify workflow is properly loaded: check `input.workflow.active`

## Related Documentation

- [Workflow System Design](./WORKFLOWS.md)
- [Authorization System](./SECURITY.md)
- [OPA Policy Guide](./OPA_POLICIES.md)

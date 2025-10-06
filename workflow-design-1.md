# Directory-Based Workflow Design

## 1. Introduction

This document outlines a redesigned, directory-based workflow system for `mysql-vfs`. In this model, a file's location within the directory structure explicitly determines its state in a workflow. State transitions are accomplished by moving files between these designated directories.

The `.workflow` file acts as the central policy document, defining the valid paths of movement (transitions) and the conditions (gates) that must be met to allow a file to move from one state to another. This design provides a powerful, intuitive, and enforceable mechanism for managing a resource's lifecycle directly through filesystem operations.

## 2. Core Concepts

### 2.1. State as Directory

The fundamental principle of this design is that **a directory represents a workflow state**. A file's current state is determined by the name of its parent directory.

*   **Workflow Root:** A directory containing a `.workflow` file becomes the root of that workflow.
*   **State Directories:** Subdirectories within the workflow root represent the different states (e.g., `draft`, `review`, `published`).
*   **Naming Convention:** State directory names must be simple, consisting of alphanumeric characters and hyphens (`-`).

### 2.2. Transition as Movement

A state transition is a **`move` operation**. To change a file's state, a user moves it from one state directory to another. The workflow system intercepts this move operation to enforce the rules defined in the `.workflow` file.

### 2.3. The `.workflow` File: The Rulebook

The `.workflow` file, located at the root of the workflow directories, defines the entire state machine. Its key responsibilities are:

1.  **Declare the Initial State:** Specify the *only* directory where new files can be created.
2.  **Define Valid Transitions:** Explicitly list the allowed movements between state directories.
3.  **Set Conditions (Gates):** Define the conditions that must be satisfied for a move to be permitted.

### 2.4. Policy Inheritance

The `.workflow` file applies to the directory where it is located and its immediate subdirectories (the state directories). It does not inherit from parent directories, as each workflow is a self-contained system.

## 3. Workflow Enforcement

The workflow engine enforces the rules at two critical points: file creation and file movement.

### 3.1. On File Creation (`create`)

When a user attempts to create a new file within a workflow-enabled directory:

1.  The system checks for a `.workflow` file in the parent directory.
2.  It verifies that the file is being created in the directory specified as the `initial_state`.
3.  If the target directory is not the `initial_state` directory, the `create` operation is **rejected**.

This ensures that all new resources enter the workflow at the correct starting point.

### 3.2. On File Movement (`move`)

When a user attempts to move a file between two state directories:

1.  The system identifies the source directory as the `from_state` and the destination directory as the `to_state`.
2.  It consults the `.workflow` file to see if a transition from `from_state` to `to_state` is defined.
3.  If the transition is defined, the system evaluates the associated `gates`.
4.  If all gates pass, the `move` operation is **allowed**.
5.  If the transition is not defined or any gate fails, the `move` operation is **rejected**.

## 4. The `.workflow` DSL

The `.workflow` file uses a simple YAML structure to define the state machine.

**Hypothetical `.workflow` Example:**

```yaml
# The only directory where new files can be created.
initial_state: draft

# Defines the transitions allowed from each state directory.
states:
  draft:
    transitions:
      - to: review
        # To move a file from 'draft' to 'review', the user must be in the 'editor' group.
        gates:
          - type: group
            group: editor

  review:
    transitions:
      - to: published
        # To move from 'review' to 'published', the file must have 'approved: true' in its metadata
        # AND the user must be in the 'approver' group.
        gates:
          - type: metadata
            key: approval_status
            value: "approved"
          - type: group
            group: approver

      - to: draft
        # A reviewer can send a file back to 'draft'.
        gates:
          - type: group
            group: reviewer

  published:
    # Files in 'published' cannot be moved.
    transitions: []

  archived:
    # No transitions out of 'archived'.
    transitions: []
```

## 5. Gate Evaluation

Gates are the conditions that unlock a transition. The system will support several gate types.

*   **`group`:** Checks if the user performing the move is a member of the specified group.
    ```yaml
    - type: group
      group: editor
    ```
*   **`metadata`:** Checks if the file's metadata contains a specific key-value pair.
    ```yaml
    - type: metadata
      key: approval_status
      value: "approved"
    ```
*   **`rego`:** For complex conditions, a small, embedded Rego script can be executed. This provides maximum flexibility.
    ```yaml
    - type: rego
      policy: |
        package workflow.gate
default allow = false
allow {
  # Allow if the file name contains 'urgent'.
  contains(input.file.name, "urgent")
  # And the user is an admin.
  input.user.groups[_] == "admin"
}
    ```

## 6. Automated Transitions via Events

File movements can be automated by using the existing `.events` infrastructure to trigger a move.

**Example `.events` file:**

```json
{
  "triggers": [
    {
      "name": "auto-archive-published-docs",
      "on": "file.updated",
      "actions": [
        {
          "type": "move_file",
          "to": "../archived/"
        }
      ],
      "conditions": {
        "rego": "input.file.metadata.status == \"final\""
      }
    }
  ]
}
```

In this example, when a file in the `published` directory is updated and its metadata `status` is set to `
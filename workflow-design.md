# Workflow and Policy Design

## 1. Introduction

The `.workflow` file is a special policy file within `mysql-vfs` that enables the definition of declarative, state-based workflows for directories and files. It provides a powerful mechanism to control the lifecycle of resources, ensuring that they follow a predefined set of states and transitions. This document outlines the design, usage, and a hypothetical DSL for the `.workflow` file, as well as a proposal for enhanced authorization using a new `.files` special file.

## 2. Core Concepts

### 2.1. Declarative Workflows

At its core, a `.workflow` file defines a state machine for the resources within its scope. This allows administrators to model complex lifecycles, such as a document moving from `draft` to `review` and finally to `published`. The workflow is declarative, meaning that the desired states and transitions are defined in a simple, human-readable format, without the need for custom code.

### 2.2. Transitions and Gates

A workflow is composed of states and transitions.

*   **States:** A state represents a specific stage in the lifecycle of a resource. For example, a document might have the states `draft`, `review`, and `published`.
*   **Transitions:** A transition defines the allowed movement from one state to another. For example, a transition could be defined to allow a document to move from `draft` to `review`.
*   **Gates:** Gates are conditions that must be met for a transition to be allowed. Gates can be based on various factors, such as the user's role, metadata attributes, or the successful completion of an external process. For example, a gate could require that a user has the `reviewer` role to transition a document from `draft` to `review`.

### 2.3. Policy Inheritance

Like other policy files in `mysql-vfs`, `.workflow` files follow a cascading inheritance model. When a workflow is triggered for a resource, the system searches for a `.workflow` file in the resource's directory. If one is not found, it continues to search up the directory hierarchy until a `.workflow` file is found. This allows for the definition of global workflows at the root level, with the option to override or extend them in subdirectories.

## 3. Workflow-Enforced Transitions

To create a robust and layered security model, the workflow engine itself will be the primary enforcer of workflow transitions. This simplifies the authorization layer and provides a clear separation of concerns.

### 3.1. Workflow Engine as the Gatekeeper

*   Before any operation that could change the state of a resource (e.g., an update or move that corresponds to a state transition), the metadata service will first consult the workflow engine.
*   The workflow engine will look up the relevant `.workflow` file and check if the requested transition is valid from the resource's current state.
*   If the transition is **not** allowed by the `.workflow` file, the operation will be rejected immediately, without ever invoking the authorization layer for that specific check.

### 3.2. Simplified Authorization Layer

*   The primary role of the `.rego` policies will be to enforce access control (e.g., who can do what), rather than workflow logic.
*   This makes the `.rego` policies simpler, more focused, and easier to maintain.

### 3.3. Defense in Depth (Optional Granular Control)

*   For more advanced scenarios, we can still inject the workflow state into the `AuthorizationInput`.
*   This allows for policies that combine workflow state with other factors, such as user roles or resource attributes. For example, a `.rego` policy could enforce that only users with the `approver` role can transition a document to the `published` state, even if the workflow itself allows the transition.

This approach provides a clear separation of concerns:

*   **`.workflow` file:** Defines the valid lifecycle of a resource.
*   **Workflow Engine:** Enforces the lifecycle defined in the `.workflow` file.
*   **`.rego` file:** Enforces who can perform actions, with the option to add more granular, context-aware checks.

## 4. Triggering Workflows

Workflows are not self-triggering. They are initiated by events that are defined in `.events` files.

### 4.1. `.events` File

The `.events` file is used to define event-driven triggers for various actions, including invoking workflows. An `.events` file can be configured to listen for specific events, such as `file.created`, `file.updated`, or `file.deleted`.

### 4.2. `invoke_workflow` Action

Within an `.events` file, the `invoke_workflow` action is used to trigger a workflow. This action specifies the name of the workflow to be invoked.

**Example `.events` file:**

```json
{
  "scope": "file",
  "triggers": [
    {
      "name": "on-delete",
      "on": "file.deleted",
      "scope": "file",
      "actions": [
        {
          "type": "invoke_workflow",
          "workflow": "cleanup-workflow"
        }
      ]
    }
  ]
}
```

### 4.3. `ext.workflow.triggered` Event

When the `invoke_workflow` action is executed, it generates an `ext.workflow.triggered` event. This event contains information about the triggered workflow and the resource that triggered it. The event is then placed on a queue for the scheduler to process.

## 5. Workflow Execution

### 5.1. Scheduler's Role

The scheduler is responsible for processing `ext.workflow.triggered` events. It has a dedicated worker that claims and processes these events.

### 5.2. Workflow Processing (Hypothetical)

The following steps outline the hypothetical process the scheduler would take to execute a workflow:

1.  **Event Dequeue:** The scheduler dequeues an `ext.workflow.triggered` event.
2.  **`.workflow` Lookup:** The scheduler looks for the corresponding `.workflow` file, starting from the directory of the resource that triggered the event and moving up the hierarchy.
3.  **Workflow Definition Parsing:** The scheduler parses the `.workflow` file to understand the defined states, transitions, and gates.
4.  **Transition Evaluation:** The scheduler evaluates the current state of the resource and the requested transition.
5.  **Gate Evaluation:** The scheduler evaluates the gates for the requested transition. This may involve checking user permissions, metadata attributes, or the status of external systems.
6.  **State Transition:** If the transition is allowed and all gates are passed, the scheduler updates the state of the resource.
7.  **Post-Transition Actions:** The scheduler can be configured to perform actions after a successful transition, such as sending a notification or triggering another event.

## 6. `.workflow` DSL (Hypothetical Example)

The following is a hypothetical example of a `.workflow` file in YAML format. This example defines a simple document approval workflow.

```yaml
# .workflow
#
# Defines a simple document approval workflow.
#

# The initial state of a new document.
initial_state: draft

# The states of the workflow.
states:
  draft:
    transitions:
      - to: review
        gates:
          - type: role
            role: editor
  review:
    transitions:
      - to: published
        gates:
          - type: role
            role: approver
      - to: draft
        gates:
          - type: role
            role: editor
  published:
    # No transitions out of the published state.
    transitions: []

```

**Explanation:**

*   **`initial_state`:** Specifies the state that a new resource will be in when it is created.
*   **`states`:** Defines the different states in the workflow.
*   **`transitions`:** For each state, a list of allowed transitions is defined.
*   **`gates`:** For each transition, a list of gates is defined. In this example, the gates are based on the user's role.

## 7. Introduce `.files` Special File

A `.files` special file can be introduced to restrict file patterns and other file-related policies.

### Proposed Design:

1.  **`.files` File DSL:** The `.files` file would be a declarative file, likely in YAML or JSON format, that defines file-related policies.

    **Hypothetical `.files` example:**

    ```yaml
    # .files
    #
    # Defines file-related policies for this directory.

    # Restrict file names using glob patterns.
    allowed_patterns:
      - "*.md"
      - "*.txt"
    denied_patterns:
      - "private_*.md"

    # Restrict file sizes (in bytes).
    max_size: 1048576 # 1MB

    # Rego policy for more complex validation.
    rego: |
      package files.policy

      default allow = true

      # Deny creating a file with 'secret' in the name.
      allow = false {
          input.action == "create"
          contains(input.file_name, "secret")
      }
    ```

2.  **Policy Evaluation Logic:** The policy evaluator would be updated to:
    *   Resolve the active `.files` file for the directory.
    *   Evaluate the policies defined in the `.files` file. This would involve checking the file name against the `allowed_patterns` and `denied_patterns`, checking the file size, and executing the embedded Rego policy.

3.  **Integration with `AuthorizationInput`:** The `AuthorizationInput` can be used as the input for the embedded Rego policy in the `.files` file.

This design would provide a powerful and flexible way to enforce file-related policies, allowing for fine-grained control over file creation, deletion, and modification based on naming conventions, size, and other attributes.

## 8. Rego Input Context

There are two distinct Rego input contexts in `mysql-vfs`, depending on whether the policy is for authorization or for an event trigger condition.

### 8.1. Authorization Input (`AuthorizationInput`)

This context is used for authorizing `create`, `update`, and `delete` operations. The input object is a JSON representation of the `AuthorizationInput` struct.

**Definition (`services/metadata/internal/policy/evaluator.go`):**

```go
type AuthorizationInput struct {
    Actor       string         `json:"actor"`
    Action      Action         `json:"action"` // "create", "update", or "delete"
    DirectoryID string         `json:"directory_id"`
    FileID      string         `json:"file_id,omitempty"`
    FileName    string         `json:"file_name,omitempty"`
    Path        string         `json:"path,omitempty"`
    Principals  PrincipalSet   `json:"principals"`
    Attributes  map[string]any `json:"attributes,omitempty"`
    Workflow    *WorkflowState `json:"workflow,omitempty"` // New field
}

// WorkflowState represents the current state of the resource in a workflow.
type WorkflowState struct {
    State      string   `json:"state"`
    NextStates []string `json:"next_states"`
}
```

### 8.2. Event Trigger Input (`TriggerContext`)

This context is used for evaluating conditions within `.events` manifests. The input object is a JSON map constructed from the `TriggerContext` struct.

**Definition (`services/metadata/internal/policy/events_engine.go`):**

The `toRegoInput()` method constructs the following JSON object:

```json
{
    "actor": "...",
    "event": {
        "type": "...",
        "scope": "...",
        "directory_id": "..."
    },
    "file": {
        "id": "...",
        "name": "...",
        "path": "...",
        "storage_mode": "..."
    },
    "metadata": { ... },
    "attributes": { ... },
    "payload": { ... },
    "request_id": "...",
    "directory_id": "..."
}
```

## 9. Use Cases

### 9.1. Document Approval Process

A common use case for `.workflow` files is to manage a document approval process. A document can move through states such as `draft`, `review`, and `published`, with gates at each stage to ensure that the correct people approve the document.

### 9.2. Software Build Pipeline

`.workflow` files can also be used to model a software build pipeline. A build could move through states such as `building`, `testing`, and `deployed`, with gates to ensure that tests pass before a build is deployed.

### 9.3. Content Moderation

A content moderation workflow can be implemented using `.workflow` files. User-generated content can be in states such as `pending`, `approved`, or `rejected`, with moderators controlling the transitions.

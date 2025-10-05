# .workflow File Design

## 1. Introduction

The `.workflow` file is a special policy file within the `mysql-vfs` that enables the definition of declarative, state-based workflows for directories and files. It provides a powerful mechanism to control the lifecycle of resources, ensuring that they follow a predefined set of states and transitions. This document outlines the design, usage, and a hypothetical DSL for the `.workflow` file.

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

## 3. Triggering Workflows

Workflows are not self-triggering. They are initiated by events that are defined in `.events` files.

### 3.1. `.events` File

The `.events` file is used to define event-driven triggers for various actions, including invoking workflows. An `.events` file can be configured to listen for specific events, such as `file.created`, `file.updated`, or `file.deleted`.

### 3.2. `invoke_workflow` Action

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

### 3.3. `ext.workflow.triggered` Event

When the `invoke_workflow` action is executed, it generates an `ext.workflow.triggered` event. This event contains information about the triggered workflow and the resource that triggered it. The event is then placed on a queue for the scheduler to process.

## 4. Workflow Execution

### 4.1. Scheduler's Role

The scheduler is responsible for processing `ext.workflow.triggered` events. It has a dedicated worker that claims and processes these events.

### 4.2. Workflow Processing (Hypothetical)

The following steps outline the hypothetical process the scheduler would take to execute a workflow:

1.  **Event Dequeue:** The scheduler dequeues an `ext.workflow.triggered` event.
2.  **`.workflow` Lookup:** The scheduler looks for the corresponding `.workflow` file, starting from the directory of the resource that triggered the event and moving up the hierarchy.
3.  **Workflow Definition Parsing:** The scheduler parses the `.workflow` file to understand the defined states, transitions, and gates.
4.  **Transition Evaluation:** The scheduler evaluates the current state of the resource and the requested transition.
5.  **Gate Evaluation:** The scheduler evaluates the gates for the requested transition. This may involve checking user permissions, metadata attributes, or the status of external systems.
6.  **State Transition:** If the transition is allowed and all gates are passed, the scheduler updates the state of the resource.
7.  **Post-Transition Actions:** The scheduler can be configured to perform actions after a successful transition, such as sending a notification or triggering another event.

## 5. `.workflow` DSL (Hypothetical Example)

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

## 6. Use Cases

### 6.1. Document Approval Process

A common use case for `.workflow` files is to manage a document approval process. A document can move through states such as `draft`, `review`, and `published`, with gates at each stage to ensure that the correct people approve the document.

### 6.2. Software Build Pipeline

`.workflow` files can also be used to model a software build pipeline. A build could move through states such as `building`, `testing`, and `deployed`, with gates to ensure that tests pass before a build is deployed.

### 6.3. Content Moderation

A content moderation workflow can be implemented using `.workflow` files. User-generated content can be in states such as `pending`, `approved`, or `rejected`, with moderators controlling the transitions.

## 7. Rego Input Context

There are two distinct Rego input contexts in `mysql-vfs`, depending on whether the policy is for authorization or for an event trigger condition.

### 7.1. Authorization Input (`AuthorizationInput`)

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
}
```

### 7.2. Event Trigger Input (`TriggerContext`)

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
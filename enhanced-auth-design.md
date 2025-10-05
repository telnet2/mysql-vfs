# Design Proposal: Enhanced Authorization with .workflow and .files

This proposal aims to enhance the authorization mechanism in `mysql-vfs` by incorporating information from `.workflow` files and introducing a new `.files` special file. This will provide a more granular and expressive way to control access and mutations within the VFS.

## 1. Workflow-Enforced Transitions

To create a robust and layered security model, the workflow engine itself will be the primary enforcer of workflow transitions. This simplifies the authorization layer and provides a clear separation of concerns.

### 1.1. Workflow Engine as the Gatekeeper

*   Before any operation that could change the state of a resource (e.g., an update or move that corresponds to a state transition), the metadata service will first consult the workflow engine.
*   The workflow engine will look up the relevant `.workflow` file and check if the requested transition is valid from the resource's current state.
*   If the transition is **not** allowed by the `.workflow` file, the operation will be rejected immediately, without ever invoking the authorization layer for that specific check.

### 1.2. Simplified Authorization Layer

*   The primary role of the `.rego` policies will be to enforce access control (e.g., who can do what), rather than workflow logic.
*   This makes the `.rego` policies simpler, more focused, and easier to maintain.

### 1.3. Defense in Depth (Optional Granular Control)

*   For more advanced scenarios, we can still inject the workflow state into the `AuthorizationInput`.
*   This allows for policies that combine workflow state with other factors, such as user roles or resource attributes. For example, a `.rego` policy could enforce that only users with the `approver` role can transition a document to the `published` state, even if the workflow itself allows the transition.

This approach provides a clear separation of concerns:

*   **`.workflow` file:** Defines the valid lifecycle of a resource.
*   **Workflow Engine:** Enforces the lifecycle defined in the `.workflow` file.
*   **`.rego` file:** Enforces who can perform actions, with the option to add more granular, context-aware checks.

## 2. Introduce `.files` Special File

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
# Workflow API Documentation

This document describes the REST API endpoints for querying and managing workflow state transitions.

## Overview

The Workflow API provides three endpoints:
1. **GET /api/v1/workflows/{filepath}/info** - Get workflow information for a file
2. **GET /api/v1/workflows/{filepath}/transitions** - Get valid transitions for a file
3. **POST /api/v1/workflows/{filepath}/next** - Transition a file to a new state

All endpoints require authentication and are subject to authorization policies.

## Authentication

All workflow endpoints require a valid authentication token. Include it in the request:

```http
Authorization: Bearer <token>
X-User-ID: <user-id>
X-User-Groups: <comma-separated-groups>
```

## Endpoints

### 1. Get Workflow Info

Returns workflow information for a specific file path.

**Endpoint:** `GET /api/v1/workflows/{filepath}/info`

**Parameters:**
- `filepath` (path parameter): The file path to query (e.g., `/documents/draft/proposal.txt`)

**Response (200 OK):**

```json
{
  "active": true,
  "workflow_path": "/documents/.workflow",
  "workflow_home": "/documents",
  "current_state": "draft",
  "initial_state": "draft",
  "states": {
    "draft": {
      "name": "draft",
      "directory": "/documents/draft",
      "transitions": ["review"]
    },
    "review": {
      "name": "review",
      "directory": "/documents/review",
      "transitions": ["published", "draft"]
    },
    "published": {
      "name": "published",
      "directory": "/documents/published",
      "transitions": []
    }
  }
}
```

**Response when no workflow exists:**

```json
{
  "active": false
}
```

**Error Responses:**
- `400 Bad Request` - Invalid file path or cannot determine state
- `500 Internal Server Error` - Failed to load workflow

**Example:**

```bash
curl -X GET \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: editors" \
  http://localhost:8080/api/v1/workflows/documents/draft/proposal.txt/info
```

---

### 2. Get Valid Transitions

Returns the list of valid state transitions available to the current user for a specific file.

**Endpoint:** `GET /api/v1/workflows/{filepath}/transitions`

**Parameters:**
- `filepath` (path parameter): The file path to query

**Response (200 OK):**

```json
{
  "current_state": "draft",
  "available_states": ["draft", "review", "published"],
  "valid_transitions": [
    {
      "to_state": "review",
      "target_path": "/documents/review/proposal.txt",
      "requires_gates": true,
      "gate_policies": []
    }
  ]
}
```

**Fields:**
- `current_state`: The file's current workflow state
- `available_states`: All states defined in the workflow
- `valid_transitions`: Array of transitions the user can perform
  - `to_state`: The destination state name
  - `target_path`: The file's path after transition
  - `requires_gates`: Whether gates need to be evaluated
  - `gate_policies`: Array of gate policy references (internal detail)

**Error Responses:**
- `400 Bad Request` - Invalid file path
- `401 Unauthorized` - User context not found
- `404 Not Found` - No workflow found for path
- `500 Internal Server Error` - Failed to load workflow or get transitions

**Example:**

```bash
curl -X GET \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: editors" \
  http://localhost:8080/api/v1/workflows/documents/draft/proposal.txt/transitions
```

---

### 3. Transition to State

Transitions a file to a new workflow state by moving it to the appropriate directory.

**Endpoint:** `POST /api/v1/workflows/{filepath}/next`

**Parameters:**
- `filepath` (path parameter): The file path to transition

**Request Body:**

```json
{
  "target_state": "review",
  "preserve_structure": true
}
```

**Fields:**
- `target_state` (required): The destination state name
- `preserve_structure` (optional): Whether to preserve subdirectory structure (default: `true`)

**Response (200 OK):**

```json
{
  "success": true,
  "from_state": "draft",
  "to_state": "review",
  "old_path": "/documents/draft/proposal.txt",
  "new_path": "/documents/review/proposal.txt",
  "message": "Successfully transitioned from draft to review"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid request body, invalid target state, or cannot determine current state
- `401 Unauthorized` - User context not found
- `403 Forbidden` - Transition denied by workflow gates
- `404 Not Found` - No workflow found for path or file not found
- `500 Internal Server Error` - Failed to perform transition

**Example (with structure preservation):**

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: editors" \
  -H "Content-Type: application/json" \
  -d '{"target_state": "review", "preserve_structure": true}' \
  http://localhost:8080/api/v1/workflows/documents/draft/legal/2024/proposal.txt/next
```

This moves `/documents/draft/legal/2024/proposal.txt` to `/documents/review/legal/2024/proposal.txt`.

**Example (without structure preservation):**

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: alice" \
  -H "X-User-Groups: editors" \
  -H "Content-Type: application/json" \
  -d '{"target_state": "review", "preserve_structure": false}' \
  http://localhost:8080/api/v1/workflows/documents/draft/legal/2024/proposal.txt/next
```

This moves `/documents/draft/legal/2024/proposal.txt` to `/documents/review/proposal.txt` (no subdirectories).

---

## Common Use Cases

### 1. Check if a File is in a Workflow

```bash
# Get workflow info
response=$(curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/workflows/path/to/file.txt/info)

# Check if active
active=$(echo $response | jq -r '.active')
if [ "$active" = "true" ]; then
  echo "File is in a workflow"
fi
```

### 2. List Available Actions for a File

```bash
# Get valid transitions
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/workflows/documents/draft/file.txt/transitions | jq '.valid_transitions'
```

### 3. Move a File Through Workflow States

```bash
# Transition from draft to review
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_state": "review"}' \
  http://localhost:8080/api/v1/workflows/documents/draft/file.txt/next

# Then from review to published
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_state": "published"}' \
  http://localhost:8080/api/v1/workflows/documents/review/file.txt/next
```

### 4. Build a UI Workflow Widget

```javascript
async function loadWorkflowWidget(filePath) {
  // Get workflow info
  const infoResponse = await fetch(`/api/v1/workflows${filePath}/info`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  const info = await infoResponse.json();
  
  if (!info.active) {
    return "No workflow";
  }
  
  // Get available transitions
  const transResponse = await fetch(`/api/v1/workflows${filePath}/transitions`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  const transitions = await transResponse.json();
  
  // Display current state and buttons for each valid transition
  return {
    currentState: transitions.current_state,
    actions: transitions.valid_transitions.map(t => ({
      label: `Move to ${t.to_state}`,
      state: t.to_state,
      requiresApproval: t.requires_gates
    }))
  };
}

async function transitionFile(filePath, targetState) {
  const response = await fetch(`/api/v1/workflows${filePath}/next`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ target_state: targetState })
  });
  
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error);
  }
  
  return await response.json();
}
```

## Error Handling

All endpoints return errors in the following format:

```json
{
  "error": "Error message describing what went wrong"
}
```

Common error scenarios:

1. **File not in workflow**: `/info` returns `{"active": false}`, `/transitions` returns 404
2. **Invalid transition**: `/next` returns 403 with workflow gate error
3. **No permission**: Standard 403 Forbidden from authorization middleware
4. **File not found**: 404 with appropriate error message

## Integration with Workflow System

These API endpoints are thin wrappers around the workflow system:

- **Workflow validation** still occurs in the service layer during file moves
- **Gates are evaluated** by the workflow engine when transitions are attempted
- **Audit logs** are created for all transition attempts
- **Events are emitted** for workflow state changes

The API provides a convenient way to query and trigger transitions without directly calling the file move endpoint.

## Security Considerations

1. **Authentication required**: All endpoints require valid user credentials
2. **Authorization applies**: OPA policies can restrict access to workflow operations
3. **Workflow gates enforced**: Gates are evaluated during transitions
4. **Audit trail**: All attempts (successful and failed) are logged
5. **No policy exposure**: Gate policies are not exposed through the API

## Rate Limiting

Workflow API endpoints are subject to the same rate limiting as other VFS API endpoints. Configure rate limits in the service configuration.

## Related Documentation

- [Workflow System Overview](./WORKFLOWS.md)
- [Workflow Authorization Integration](./WORKFLOW_AUTHORIZATION.md)
- [VFS API Documentation](./API.md)

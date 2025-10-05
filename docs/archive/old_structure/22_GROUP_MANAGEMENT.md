# 22. Group Management

**Type:** Global, File-Based
**Status:** Implemented

---

## Overview

Mysql VFS v2 uses a single, root-level `.group` file to define all user groups in the system. This file-based approach allows for simple, centralized management of group memberships.

**Implementation:** `pkg/domain/group_loader.go`

---

## How It Works

### 1. The `.group` File

- **Location:** The `.group` file **must** be located in the root directory (`/`).
- **Format:** It is a JSON file containing a list of groups and their members.
- **Global:** This file defines all groups for the entire VFS instance. There is no per-directory or inherited group configuration.

**Example `.group` file:**

```json
{
  "groups": [
    {
      "group_id": "admins",
      "description": "System administrators with full access.",
      "members": ["user-alice", "user-bob"]
    },
    {
      "group_id": "engineering",
      "description": "Engineering team members.",
      "members": ["user-alice", "user-charlie"]
    },
    {
      "group_id": "qa",
      "description": "Quality assurance team.",
      "members": ["user-dave"]
    }
  ]
}
```

### 2. Group Resolution

When a user authenticates, the `GroupLoader` reads the `/.group` file and determines which groups the user belongs to. This list of groups is then passed to the OPA authorization engine as the `input.user.groups` field.

**Example Flow:**

1.  User `user-alice` makes a request.
2.  The `GroupLoader` reads `/.group`.
3.  It finds that `user-alice` is a member of the "admins" and "engineering" groups.
4.  The authorization middleware receives the group list: `["admins", "engineering"]`.
5.  This list is passed to the OPA policy in the `input.user.groups` field.

---

## Managing Groups

Since the `.group` file is a regular file in the VFS, you can manage it using the standard file API endpoints. You must have appropriate permissions to create, update, or delete the `/.group` file (typically, this is restricted to administrators).

### Create or Update the `.group` File

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer <admin-token>" \
  -d '{
    "directory_path": "/",
    "name": ".group",
    "content_type": "application/json",
    "content": "{\"groups\": [{\"group_id\": \"admins\", \"members\": [\"user-alice\"]}]}"
  }'
```

To update the file, you can use the `PUT /api/v1/files/{file_id}` endpoint.

### Reading the `.group` File

```bash
curl http://localhost:8080/api/v1/files?directory_path=/&name=.group \
  -H "Authorization: Bearer <admin-token>"
```

---

## Integration with Authorization

The primary purpose of groups is to be used in OPA authorization policies (`.rego` files). You can create flexible, role-based access control rules by checking a user's groups.

**Example `.rego` policy:**

```rego
package vfs.authz

# Allow members of the "engineering" group to write to this directory.
allow {
    "engineering" in input.user.groups
    input.action == "write"
}

# Allow members of the "admins" group to do anything.
allow {
    "admins" in input.user.groups
}
```

---

## Caching

The `/.group` file is cached in memory to improve performance.

- **TTL:** The cache has a time-to-live (TTL) of 5 minutes (configurable via `GROUP_CACHE_TTL_SECONDS`).
- **Invalidation:** The cache is automatically invalidated when the `/.group` file is updated.

---

## Deprecated System

The previous design for group management involved storing groups in a database. This system has been **deprecated** and is no longer in use. The current file-based approach provides a simpler and more transparent way to manage groups.

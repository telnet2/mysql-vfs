package middleware

// DefaultRegoPolicy is the built-in fallback policy when no .rego file is found
//
// IMPORTANT: This is only used as a fallback if /.rego doesn't exist.
// Run the bootstrap script to create /.rego as an actual file that can be edited.
//
// This provides basic role-based access control:
// - admin role: full read+write+delete access
// - user role: read-only access
// - owners (users in owner groups): read+write+delete access
// - all other roles: denied
const DefaultRegoPolicy = `package vfs.authz

# Default policy: admin has full access, user has read-only access, owners have write access
# This policy is used when no .rego file is found in the directory hierarchy

# Admin role: full access to all actions
allow {
    input.user.role == "admin"
}

# User role: read-only access
allow {
    input.user.role == "user"
    input.action == "read"
}

# Owners: Users in owner groups can write
allow {
    input.user.role == "user"
    input.action == "write"
    is_owner
}

# Owners: Users in owner groups can delete
allow {
    input.user.role == "user"
    input.action == "delete"
    is_owner
}

# Helper rule: Check if user is in any owner group
is_owner {
    # Get user's groups
    user_group := input.user.groups[_]
    # Check if any owner group matches
    owner_group := input.resource.owners[_]
    user_group == owner_group
}
`

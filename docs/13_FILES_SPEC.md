# .files Special File Specification

## Overview

The `.files` special file defines allowed file patterns and their validation schemas for a directory.

**Replaces:** `.jsonschema` (more flexible, supports multiple patterns)

## Format

```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email", "name"]
      },
      "description": "User JSON files"
    },
    {
      "pattern": "config-.*\\.yaml",
      "type": "regex",
      "schema": {
        "type": "object",
        "required": ["version"]
      },
      "description": "Config YAML files"
    },
    {
      "pattern": "*.txt",
      "type": "glob",
      "schema": null,
      "description": "Plain text files (no validation)"
    }
  ],
  "default_action": "deny"
}
```

## Fields

### Root Level

- `rules` (array, required) - List of file pattern rules
- `default_action` (string, optional) - Action when no pattern matches: "allow" or "deny" (default: "allow")

### Rule Object

- `pattern` (string, required) - File name pattern
- `type` (string, required) - Pattern type: "glob" or "regex"
- `schema` (object, optional) - JSON schema for validation (null = no validation)
- `description` (string, optional) - Human-readable description

## Pattern Types

### Glob Pattern

Unix-style wildcards:
- `*` - Match any characters
- `?` - Match single character
- `[abc]` - Match any of a, b, c
- `[a-z]` - Match any character in range

**Examples:**
```json
{
  "pattern": "*.json",
  "type": "glob"
}
{
  "pattern": "user-*.json",
  "type": "glob"
}
{
  "pattern": "config.??",
  "type": "glob"
}
```

### Regex Pattern

Full regular expressions:

**Examples:**
```json
{
  "pattern": "^[a-z0-9-]+\\.json$",
  "type": "regex"
}
{
  "pattern": "config-[0-9]{4}\\.yaml",
  "type": "regex"
}
```

## Validation Order

Rules are tested **in declaration order**. First match wins.

**Example:**
```json
{
  "rules": [
    {
      "pattern": "admin-*.json",
      "type": "glob",
      "schema": {"required": ["role", "permissions"]}
    },
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {"required": ["name"]}
    }
  ]
}
```

- `admin-user.json` → Matched by rule 1 (requires role, permissions)
- `regular-user.json` → Matched by rule 2 (requires name)

## Default Action

### Allow (default)

```json
{
  "rules": [...],
  "default_action": "allow"
}
```

Files not matching any pattern are **allowed** without validation.

### Deny

```json
{
  "rules": [...],
  "default_action": "deny"
}
```

Files not matching any pattern are **rejected**.

**Use case:** Whitelist-only mode

## Schema Validation

### With Schema

```json
{
  "pattern": "*.json",
  "type": "glob",
  "schema": {
    "type": "object",
    "required": ["email"],
    "properties": {
      "email": {"type": "string", "format": "email"}
    }
  }
}
```

File content is validated against JSON schema.

### Without Schema (null)

```json
{
  "pattern": "*.txt",
  "type": "glob",
  "schema": null
}
```

File is allowed but **not validated**.

## Examples

### Example 1: User Directory

```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email", "name", "created_at"],
        "properties": {
          "email": {"type": "string", "format": "email"},
          "name": {"type": "string", "minLength": 1},
          "created_at": {"type": "string", "format": "date-time"}
        }
      },
      "description": "User profile JSON"
    }
  ],
  "default_action": "deny"
}
```

**Result:**
- `alice.json` with valid schema → ✅ Allowed
- `alice.json` missing email → ❌ Rejected (schema)
- `alice.txt` → ❌ Rejected (no matching pattern, default deny)

### Example 2: Mixed Content

```json
{
  "rules": [
    {
      "pattern": "config-*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["version", "settings"]
      },
      "description": "Configuration files"
    },
    {
      "pattern": "*.md",
      "type": "glob",
      "schema": null,
      "description": "Markdown documentation"
    },
    {
      "pattern": "data-[0-9]{8}\\.json",
      "type": "regex",
      "schema": {
        "type": "array",
        "items": {"type": "object"}
      },
      "description": "Daily data exports"
    }
  ],
  "default_action": "allow"
}
```

**Results:**
- `config-prod.json` → Validated against config schema
- `README.md` → Allowed, no validation
- `data-20251004.json` → Validated as array
- `random-file.txt` → Allowed (default action)

### Example 3: Strict Mode

```json
{
  "rules": [
    {
      "pattern": "user-[a-z0-9]+\\.json",
      "type": "regex",
      "schema": {
        "type": "object",
        "required": ["user_id", "email"]
      }
    }
  ],
  "default_action": "deny"
}
```

**Result:** Only files matching `user-[a-z0-9]+\.json` are allowed.

## Special Cases

### Allow All Files

```json
{
  "rules": [],
  "default_action": "allow"
}
```

or

```json
{
  "rules": [
    {
      "pattern": "*",
      "type": "glob",
      "schema": null
    }
  ]
}
```

### Deny All Files

```json
{
  "rules": [],
  "default_action": "deny"
}
```

### Validate All JSON

```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object"
      }
    }
  ],
  "default_action": "allow"
}
```

## Inheritance

Like other special files, `.files` rules **inherit from parent directories**:

```
/
├── .files (*.json must have "type" field)
└── data/
    ├── .files (*.json must have "email" field)
    └── users/
        └── alice.json
```

**Resolution:**
1. Check `/data/.files` → Match `*.json`, validate email
2. If no `/data/.files`, check `/.files` → Validate type

**Child overrides parent** (no merging).

## Migration from .jsonschema

**Old (.jsonschema):**
```json
{
  "type": "object",
  "required": ["email"]
}
```

**New (.files):**
```json
{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "required": ["email"]
      }
    }
  ]
}
```

**Benefit:** Can now have different schemas for different file patterns!

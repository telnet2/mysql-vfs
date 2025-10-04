# Special Files Inheritance Behavior

**Feature:** .jsonschema and .rego Inheritance
**Date:** 2025-10-03
**Status:** Specification

---

## Summary

**Yes, `.xxx` files ARE inherited by subdirectories and files.**

Special files (`.jsonschema` and `.rego`) follow a **parent-to-child inheritance** model where:
- If a directory doesn't have a special file, it inherits from its parent
- Child directories can override parent special files
- Files validate/authorize based on their directory's effective special file (including inherited ones)

---

## Inheritance Rules

### Rule 1: Child Inherits from Parent

```
/data/
├── .jsonschema          ← Schema A
└── users/               (no .jsonschema)
    ├── john.json        ← Validates against Schema A (inherited)
    └── jane.json        ← Validates against Schema A (inherited)
```

**Behavior:**
- `/data/users/` has NO `.jsonschema`
- When validating `john.json`, system looks for `/data/users/.jsonschema`
- Not found → Check parent `/data/.jsonschema`
- Found → Use Schema A for validation

---

### Rule 2: Child Can Override Parent

```
/data/
├── .jsonschema          ← Schema A (base schema)
└── users/
    ├── .jsonschema      ← Schema B (user-specific schema)
    ├── john.json        ← Validates against Schema B (override)
    └── admins/          (no .jsonschema)
        └── root.json    ← Validates against Schema B (inherited from parent)
```

**Behavior:**
- `/data/users/john.json` validates against Schema B (direct)
- `/data/users/admins/root.json` validates against Schema B (inherited from `/data/users/`)
- Schema B completely replaces Schema A (no merging)

---

### Rule 3: Inheritance Chains Upward

```
/
├── .jsonschema          ← Schema ROOT
└── projects/            (no .jsonschema)
    └── secret/          (no .jsonschema)
        └── data.json    ← Validates against Schema ROOT
```

**Lookup chain for `/projects/secret/data.json`:**
1. Check `/projects/secret/.jsonschema` → Not found
2. Check `/projects/.jsonschema` → Not found
3. Check `/.jsonschema` → Found (Schema ROOT)
4. Use Schema ROOT

---

### Rule 4: No Schema = No Validation

```
/data/
└── unvalidated/         (no .jsonschema, no parent .jsonschema)
    └── anything.json    ← No validation (any JSON is valid)
```

**Behavior:**
- No schema in current directory
- No schema in parent directories
- File upload succeeds without validation

---

## Examples by File Type

### .jsonschema Inheritance

```
/
├── .jsonschema                    ← Requires: {id, type}
└── api/
    ├── v1/
    │   ├── .jsonschema           ← Requires: {id, type, version}
    │   ├── users.json            ← Must have: id, type, version
    │   └── posts/
    │       └── latest.json       ← Must have: id, type, version (inherited)
    └── v2/                        (no .jsonschema)
        └── users.json            ← Must have: id, type (inherited from /)
```

**Validation for each file:**
- `/api/v1/users.json` → Uses `/api/v1/.jsonschema`
- `/api/v1/posts/latest.json` → Uses `/api/v1/.jsonschema` (inherited)
- `/api/v2/users.json` → Uses `/.jsonschema` (inherited, skipping `/api`)

---

### .rego Inheritance

```
/
├── .rego                          ← Default: authenticated users only
└── projects/
    ├── public/                    (no .rego)
    │   └── readme.md              ← Auth: authenticated users (inherited)
    └── secret/
        ├── .rego                  ← Override: admin-only
        └── keys.json              ← Auth: admin-only
```

**Authorization for each file:**
- `/projects/public/readme.md` → Uses `/.rego` (authenticated users)
- `/projects/secret/keys.json` → Uses `/projects/secret/.rego` (admin-only)

---

## Inheritance Algorithm

### Pseudocode

```python
def get_effective_special_file(directory_path, special_filename):
    """
    Find effective special file by checking current directory
    and walking up the tree to parents.
    """
    current_path = directory_path

    while current_path != "/":
        # Try to load special file from current directory
        file = load_file(current_path + "/" + special_filename)
        if file exists:
            return file.content

        # Move to parent directory
        current_path = parent_directory(current_path)

    # Check root directory
    file = load_file("/" + special_filename)
    if file exists:
        return file.content

    # No special file found in entire path
    return None
```

### Go Implementation

```go
// pkg/domain/schema_loader.go

func (s *SchemaLoader) LoadSchemaForDirectory(ctx context.Context, dirPath string) (string, error) {
    // Normalize path
    dirPath = filepath.Clean(dirPath)

    // Check cache first
    if cached, ok := s.getFromCache(dirPath); ok {
        return cached.content, nil
    }

    // Walk up directory tree
    currentPath := dirPath
    for {
        // Try to load .jsonschema from current directory
        content, err := s.loadSchemaFile(ctx, currentPath)
        if err == nil {
            // Found schema - cache it for this directory
            s.cacheSchema(dirPath, content) // Cache for original path
            return content, nil
        }

        // Check if we've reached root
        if currentPath == "/" {
            break
        }

        // Move to parent
        currentPath = filepath.Dir(currentPath)
        if currentPath == "." {
            currentPath = "/"
        }
    }

    // No schema found in entire path
    return "", ErrSchemaNotFound
}
```

---

## Caching Strategy

### Cache Key = Original Directory Path

```go
type SchemaLoader struct {
    cache map[string]*cachedSchema
}

type cachedSchema struct {
    content    string  // Schema content (or path to inherited schema)
    sourcePath string  // Where the schema actually lives
    loadedAt   time.Time
}

// Example cache after loading /api/v1/posts/latest.json:
cache = {
    "/api/v1/posts": {
        content: "{...schema from /api/v1/.jsonschema...}",
        sourcePath: "/api/v1",
        loadedAt: 2025-10-03T10:00:00Z
    }
}
```

### Cache Invalidation

When a special file is created/updated/deleted, invalidate:
1. **Direct cache entry** for that directory
2. **All child directory cache entries** (they might have inherited from this file)

```go
func (s *SchemaLoader) InvalidateCache(dirPath string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Invalidate this directory
    delete(s.cache, dirPath)

    // Invalidate all children
    for cachedPath, cached := range s.cache {
        // If cached path is under dirPath, invalidate it
        if strings.HasPrefix(cachedPath, dirPath + "/") {
            delete(s.cache, cachedPath)
        }
    }
}
```

---

## Real-World Examples

### Example 1: Multi-Tenant SaaS

```
/tenants/
├── .jsonschema                    ← Base tenant schema: {tenant_id, name}
├── tenant-a/
│   └── data.json                  ← Must have: tenant_id, name
└── tenant-b/
    ├── .jsonschema                ← Extended schema: {tenant_id, name, plan, limits}
    └── data.json                  ← Must have: tenant_id, name, plan, limits
```

**Validation:**
- `tenant-a` inherits base schema → simpler validation
- `tenant-b` overrides with extended schema → stricter validation

---

### Example 2: API Versioning

```
/api/
├── v1/
│   ├── .jsonschema               ← v1 schema (loose)
│   ├── users.json
│   └── posts.json
├── v2/
│   ├── .jsonschema               ← v2 schema (stricter, more fields)
│   ├── users.json
│   └── posts.json
└── experimental/                  (no .jsonschema)
    └── new-feature.json          ← No validation (experimental)
```

---

### Example 3: Progressive Security Hardening

```
/
├── .rego                          ← Default: authenticated users
└── data/
    ├── public/                    (no .rego)
    │   └── stats.json             ← Auth: authenticated (inherited)
    └── sensitive/
        ├── .rego                  ← Override: admin + MFA required
        ├── secrets.json           ← Auth: admin + MFA
        └── backups/               (no .rego)
            └── daily.json         ← Auth: admin + MFA (inherited)
```

---

## Special Cases

### Case 1: Root Directory Schema

```
/
├── .jsonschema                    ← ALL files inherit this (unless overridden)
├── file.json                      ← Validates against /.jsonschema
└── deep/
    └── nested/
        └── file.json              ← Validates against /.jsonschema
```

**Use case:** Global schema for entire VFS

---

### Case 2: Multiple Levels of Overrides

```
/
├── .jsonschema                    ← Level 0: {id}
└── a/
    ├── .jsonschema                ← Level 1: {id, type}
    └── b/
        ├── .jsonschema            ← Level 2: {id, type, version}
        └── c/                     (no .jsonschema)
            └── file.json          ← Uses Level 2 schema
```

**Inheritance chain:**
- `file.json` validates with Level 2 schema (most specific ancestor)

---

### Case 3: Deleting a Schema

```
Before:
/data/
├── .jsonschema                    ← Schema A
└── users/
    └── john.json                  ← Validates against Schema A

After deleting /data/.jsonschema:
/data/                             (no .jsonschema)
└── users/
    └── john.json                  ← No validation (no schema found)
```

**Impact:**
- Deleting a schema affects all children that were inheriting it
- Cache invalidation removes all child entries
- Next upload to children will not validate

---

## API Behavior

### Creating a File

```http
POST /api/v1/files
{
  "directory_path": "/data/users/admins",
  "name": "root.json",
  "content": "{\"email\":\"root@example.com\"}"
}

Internal flow:
1. Find directory: /data/users/admins
2. Look for schema:
   a. /data/users/admins/.jsonschema → Not found
   b. /data/users/.jsonschema → Found!
3. Validate content against /data/users/.jsonschema
4. If valid → Create file
5. If invalid → Return 400 with errors
```

### Reading Schema Information

```http
# Get effective schema for a directory
GET /api/v1/directories/data/users/admins?include_schema=true

Response:
{
  "id": "dir-123",
  "path": "/data/users/admins",
  "effective_schema": {
    "path": "/data/users/.jsonschema",
    "inherited": true
  }
}
```

---

## Performance Considerations

### Optimization 1: Cache Effective Schemas

Cache the resolved schema for each directory (not just the source):

```go
cache["/data/users/admins"] = {
    content: "...",
    sourcePath: "/data/users",  // Where it actually came from
    inherited: true
}
```

### Optimization 2: Bulk Lookup

When listing files in a directory, lookup schema once for all files:

```go
// Instead of:
for file in files {
    schema = LoadSchemaForDirectory(file.directoryPath)
    validate(file, schema)
}

// Do:
schema = LoadSchemaForDirectory(directoryPath)
for file in files {
    validate(file, schema)
}
```

### Optimization 3: Path-based Cache Key

Use normalized paths as cache keys to avoid duplicate lookups:

```go
"/data/users/" -> cache entry
"/data/users"  -> same cache entry (normalized)
```

---

## Testing Inheritance

### Test Case 1: Basic Inheritance

```go
func TestSchemaInheritance_Basic(t *testing.T) {
    // Create schema in /data
    createSchema("/data/.jsonschema", basicSchema)

    // Upload file to /data/users (no schema there)
    file, err := uploadFile("/data/users", "test.json", validData)

    assert.NoError(t, err)
    assert.NotNil(t, file)
    // Validates against /data/.jsonschema
}
```

### Test Case 2: Override

```go
func TestSchemaInheritance_Override(t *testing.T) {
    // Create schema in /data
    createSchema("/data/.jsonschema", looseSchema)

    // Create stricter schema in /data/users
    createSchema("/data/users/.jsonschema", strictSchema)

    // Upload to /data/users
    file, err := uploadFile("/data/users", "test.json", validLooseData)

    // Should fail because /data/users/.jsonschema is stricter
    assert.Error(t, err)
}
```

### Test Case 3: Cache Invalidation

```go
func TestSchemaInheritance_CacheInvalidation(t *testing.T) {
    // Create schema
    createSchema("/data/.jsonschema", schemaV1)

    // Upload file (cached)
    uploadFile("/data/users", "test.json", validV1Data)

    // Update parent schema
    updateSchema("/data/.jsonschema", schemaV2)

    // Upload another file - should use V2
    err := uploadFile("/data/users", "test2.json", invalidV2Data)
    assert.Error(t, err) // V2 is stricter
}
```

---

## Documentation for Users

### For Admins

```markdown
# How Schema Inheritance Works

1. **Create a schema in a parent directory**
   - All child directories inherit it automatically
   - Example: Schema in `/data` applies to `/data/users`, `/data/products`, etc.

2. **Override with a child schema**
   - Create `.jsonschema` in child directory
   - Completely replaces parent schema (no merging)

3. **Delete a schema**
   - Removes validation for that directory
   - Children may still inherit from grandparent

4. **Check effective schema**
   - View directory details to see which schema applies
   - Shows if inherited or direct
```

### For Developers

```markdown
# Schema Inheritance API

GET /api/v1/directories/{path}/effective-schema
→ Returns the actual schema that will be used for validation

Response:
{
  "schema_content": "{...}",
  "source_path": "/data/.jsonschema",
  "is_inherited": true
}
```

---

**Status:** Specification Complete
**Applies to:** `.jsonschema` and `.rego` files
**Inheritance:** Yes - Child directories inherit from parents
**Override:** Yes - Child can override parent
**Cache:** Yes - With invalidation on updates

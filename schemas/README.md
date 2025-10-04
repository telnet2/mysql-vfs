# VFS JSON Schemas

This directory contains JSON Schema definitions for validating API requests to the VFS service.

## Schemas

| Schema | Endpoint | Description |
|--------|----------|-------------|
| `create_directory_request.json` | `POST /api/v1/directories` | Create a new directory |
| `create_file_request.json` | `POST /api/v1/files` | Create a new file |
| `move_file_request.json` | `POST /api/v1/files/move` | Move or rename a file |

## Schema Structure

All schemas follow JSON Schema Draft 07 specification and include:

- **Required fields** - Fields that must be present
- **Type validation** - Correct data types
- **Pattern validation** - Regex patterns for strings
- **Length constraints** - Min/max length for strings
- **Examples** - Sample valid payloads

## Validation Rules

### Common Rules

- **Paths** - Must start with `/` and not exceed 1024 characters
- **Names** - Cannot contain `/`, `\`, or control characters (1-255 chars)
- **UUIDs** - Must match UUID v4 format

### Directory Names

```
Pattern: ^[^/\\\\\\x00-\\x1f\\x7f]+$
```

**Valid:**
- `documents`
- `my-project`
- `data_2024`

**Invalid:**
- `parent/child` (contains `/`)
- `folder\name` (contains `\`)
- `.` or `..` (reserved)

### File Names

```
Pattern: ^[^/\\\\\\x00-\\x1f\\x7f]+$
```

**Valid:**
- `document.txt`
- `report-2024.pdf`
- `config.json`

**Invalid:**
- `path/to/file.txt` (contains `/`)
- `file\name.txt` (contains `\`)
- Files with control characters

### Content Types

```
Pattern: ^[a-z]+/[a-z0-9.+-]+$
```

**Valid:**
- `text/plain`
- `application/json`
- `application/pdf`
- `image/png`
- `application/vnd.api+json`

**Invalid:**
- `TEXT/PLAIN` (uppercase)
- `json` (missing subtype)
- `text-plain` (invalid separator)

## Usage

### Manual Validation

Use a JSON Schema validator to test request payloads:

```bash
# Using ajv-cli
npm install -g ajv-cli
ajv validate -s create_directory_request.json -d request.json
```

### Programmatic Validation

The `ValidationMiddleware` automatically validates requests:

```go
// Load schemas
validator := middleware.NewValidationMiddleware()
validator.LoadSchemasFromDirectory("./schemas")

// Apply to routes
h.POST("/api/v1/directories",
    validator.Handler(),
    createDirectoryHandler)
```

## Adding New Schemas

1. Create schema file: `schemas/<request_name>.json`
2. Follow JSON Schema Draft 07 specification
3. Include required fields, types, and patterns
4. Add examples for documentation
5. Update route mapping in `middleware/validation.go`

## Example Requests

### Create Directory

```json
{
  "parent_path": "/",
  "name": "projects",
  "opa_policy_id": null
}
```

### Create File

```json
{
  "directory_path": "/documents",
  "name": "readme.txt",
  "content_type": "text/plain",
  "content": "Hello, World!",
  "storage_type": "mysql"
}
```

### Move File

```json
{
  "source_path": "/temp/file.txt",
  "destination_path": "/archive/file.txt"
}
```

## Validation Errors

When validation fails, the API returns:

```json
{
  "error": "validation failed",
  "details": [
    "name: Does not match pattern '^[^/\\\\\\x00-\\x1f\\x7f]+$'",
    "parent_path: String length must be greater than or equal to 1"
  ]
}
```

## References

- [JSON Schema Specification](https://json-schema.org/specification.html)
- [JSON Schema Validator](https://www.jsonschemavalidator.net/)
- [Understanding JSON Schema](https://json-schema.org/understanding-json-schema/)

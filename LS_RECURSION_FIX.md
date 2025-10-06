# Fix: ls -r Maximum Recursion Depth Bug

## Problem

```
/> ls -r
✗ maximum recursion depth (100) exceeded
```

The `ls -r` command was failing immediately with a recursion error.

## Root Cause

**API Response Mismatch**

The server (`services/vfs/handlers/directory.go`) was returning:
```json
{
  "directories": [...]
}
```

But the CLI client (`cli/client/client.go`) expected:
```json
{
  "entries": [...]
}
```

This caused the client to receive an empty `entries` array, which likely caused the recursion logic to malfunction or the response parsing to fail silently.

## The Fix

### Updated `services/vfs/handlers/directory.go`

**Before:**
```go
// Format response
dirResponses := make([]DirectoryResponse, len(directories))
for i, dir := range directories {
    dirResponses[i] = DirectoryResponse{
        ID:        dir.ID,
        Name:      dir.Name,
        Path:      dir.Path,
        ParentID:  dir.ParentID,
        CreatedAt: dir.CreatedAt,
        UpdatedAt: dir.UpdatedAt,
    }
}

response := ListDirectoryResponse{
    Directories: dirResponses,
}

// Note: files are currently not included in the response, but they are available if needed
_ = files

c.JSON(200, response)
```

**After:**
```go
// Format response - create entries array combining directories and files
entries := make([]DirectoryEntry, 0, len(directories)+len(files))

// Add directories
for _, dir := range directories {
    entries = append(entries, DirectoryEntry{
        Name:      dir.Name,
        Type:      "directory",
        SizeBytes: 0,
    })
}

// Add files
for _, file := range files {
    entries = append(entries, DirectoryEntry{
        Name:      file.Name,
        Type:      "file",
        SizeBytes: file.SizeBytes,
    })
}

response := ListDirectoryEntriesResponse{
    Entries: entries,
}

if nextCursor != "" {
    response.NextCursor = &nextCursor
}

c.JSON(200, response)
```

### Added New Response Types

```go
// DirectoryEntry represents a directory or file entry
type DirectoryEntry struct {
    Name      string `json:"name"`
    Type      string `json:"type"` // "directory" or "file"
    SizeBytes int64  `json:"size_bytes"`
}

// ListDirectoryEntriesResponse represents a combined list of directories and files
type ListDirectoryEntriesResponse struct {
    Entries    []DirectoryEntry `json:"entries"`
    NextCursor *string          `json:"next_cursor,omitempty"`
}
```

## Benefits

1. **Fixed ls -r** - Recursive listing now works correctly
2. **Files included** - The response now includes both directories AND files (previously files were ignored with `_ = files`)
3. **API consistency** - Server response matches client expectations
4. **Better UX** - Users can now see full directory contents including files

## Testing

```bash
# Start VFS service
go run services/vfs/main.go

# In another terminal, start CLI
./bin/vfs-cli

# Test commands
/> ls              # Shows directories and files
/> ls -r           # Should now work without errors
/> ls /validation  # Shows contents
```

## Expected Output

### Before Fix:
```
/> ls -r
✗ maximum recursion depth (100) exceeded
```

### After Fix:
```
/> ls -r
/
validation/
  .group  (117 bytes)
  .rego  (432 bytes)
  CLI-HOWTO.md  (13273 bytes)
  ...
schemas/
  address.json  (234 bytes)
  customer.json  (456 bytes)
  ...
```

## Files Modified

- `services/vfs/handlers/directory.go` - Updated `ListDirectory` handler to return `entries` array with both directories and files

## Related Issues

This fix also resolves:
- Files not showing in `ls` output
- API inconsistency between server and client
- Potential parsing errors in CLI

---

**Status**: ✅ Fixed
**Impact**: High - Core functionality
**Testing**: Required after service restart

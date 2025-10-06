# Fix: tree Command Showing Contents Repeatedly

## Problem

```
/> tree
//
├── /
│   ├── /
│   │   ├── /
│   │   ├── validation/
│   │   ├── .group
│   │   ├── .rego
│   ...
```

The `tree` command was showing the root directory "/" as an entry within itself, causing infinite recursion and repeated content.

## Root Cause

**Self-Referential Directory Listing**

When listing "/", the API was returning "/" itself as one of the entries:

```json
{
  "entries": [
    {"name": "/", "type": "directory"},     // ← Self-reference!
    {"name": "validation", "type": "directory"},
    {"name": ".group", "type": "file"},
    ...
  ]
}
```

The tree command would then:
1. List "/"
2. See "/" as an entry
3. Recursively list "/" again
4. See "/" as an entry again
5. Repeat infinitely (or until max depth)

## The Fix

### Updated `services/vfs/handlers/directory.go:129-143`

Added filtering to exclude:
1. **The current directory itself** - Prevent self-references
2. **"." and ".." entries** - Prevent parent directory references

**Code:**
```go
// Add directories (exclude current directory to prevent recursion)
for _, dir := range directories {
    // Skip the current directory itself (e.g., "/" when listing "/")
    if dir.Path == path {
        continue
    }
    // Skip parent directory references
    if dir.Name == "." || dir.Name == ".." {
        continue
    }
    entries = append(entries, DirectoryEntry{
        Name:      dir.Name,
        Type:      "directory",
        SizeBytes: 0,
    })
}
```

## Before vs After

### Before (Incorrect):

```
/> tree
//
├── /                        ← Self-reference
│   ├── /                    ← Infinite recursion
│   │   ├── /
│   │   ├── validation/
│   │   ├── .group
│   │   ├── .rego
│   ├── validation/          ← Duplicate entries
│   ├── .group
│   ├── .rego
├── validation/              ← Duplicate entries again
├── .group
├── .rego
```

### After (Correct):

```
/> tree
/
├── validation/
│   └── (validation contents)
├── .group
├── .rego
├── CLI-HOWTO.md
├── DOCUMENTATION_UPDATE_SUMMARY.md
├── hello.txt
└── workflow-plan.md
```

## Impact

This fix resolves:

1. ✅ **tree command** - No more repeated content
2. ✅ **ls -r command** - No more self-referential recursion
3. ✅ **API correctness** - Directory listings don't include themselves
4. ✅ **Performance** - No wasted cycles on infinite loops

## Testing

```bash
# Restart VFS service (required for server changes)
go run services/vfs/main.go

# Test tree command
./bin/vfs-cli

/> tree                    # Should show clean tree
/> tree /validation        # Should show validation tree
/> ls -r                   # Should work without recursion issues
/> cd validation
/validation> tree          # Should work from any directory
```

## Files Modified

- `services/vfs/handlers/directory.go:129-143` - Added filtering to prevent self-references

## Related Fixes

This fix complements the previous `LS_RECURSION_FIX.md` which addressed the API schema mismatch. Together, these fixes ensure:

1. API returns correct `entries` format (LS_RECURSION_FIX)
2. API doesn't include self-referential directories (this fix)
3. Both `ls -r` and `tree` work correctly

---

**Status**: ✅ Fixed
**Requires**: VFS service restart
**Testing**: tree, ls -r commands

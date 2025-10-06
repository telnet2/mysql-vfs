# MUST READ
The format of this doc follows:
```
# YYYY-MM-DD
- [added]  ... one line description ...
- [removed] ... one line description ...
- [fixed] ... one line description ...
- [enhanced] ... one line description ...
- [chore] ... one line description ...
```

# 2025-10-06
- [added] metadata JSON field to directories, files, and file_versions tables for ownership and audit tracking
- [added] /etc read-only system directory with embedded JSON schema files (owner, files, events, file.metadata, directory.metadata)
- [added] pkg/seed package with embedded schema files using go:embed for version-controlled system schemas
- [added] IsSystemProtectedPath() function to prevent modification of /etc directory and contents
- [added] bootstrapSystemFiles() function to automatically seed /etc directory on service startup
- [added] ErrProtectedSystemDirectory error for /etc modification attempts
- [added] system metadata population for /etc directory and schema files (owner: system-admin, system: true, readonly: true)
- [added] comprehensive documentation for system files (docs/SYSTEM_FILES.md)
- [added] comprehensive documentation for metadata structure and implementation plan (docs/METADATA.md)
- [added] comprehensive documentation for on-behalf-of delegation with security model (docs/ON_BEHALF_OF.md)
- [added] integration tests for system files protection (citest/system_files_test.go)
- [enhanced] directory and file service layers with /etc protection checks in CreateFile, UpdateFile, DeleteFile, CreateDirectory, DeleteDirectory
- [enhanced] database migration to add metadata column with NULL default for backward compatibility
- [chore] archived 13 obsolete markdown files to archive/ directory with cross-references updated
- [chore] created archive/README.md to document historical files and current documentation structure

# 2025-10-05
- [added] create-sample-files CLI command for generating sample schema validation configurations
- [enhanced] wildcard glob support added to ls, rm, mv, and import commands for pattern matching
- [enhanced] ls -l long listing format with detailed table display including Name, Type, Size, Version, Modified columns
- [removed] json command (superseded by jq command for JSON pretty printing and syntax coloring)
- [fixed] version field display in ls -l command to show correct latest version numbers
- [enhanced] ls -l and ls -lr commands updated to Linux-like format with Modified, Size, Version, Type, Name columns
- [fixed] cat command now adds trailing newline to ensure CLI prompt appears on new line
- [enhanced] jq command now defaults to "." expression when no expression provided for easier JSON file viewing

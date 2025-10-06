# 2025-10-05
- [added] create-sample-files CLI command for generating sample schema validation configurations
- [enhanced] wildcard glob support added to ls, rm, mv, and import commands for pattern matching
- [enhanced] ls -l long listing format with detailed table display including Name, Type, Size, Version, Modified columns
- [removed] json command (superseded by jq command for JSON pretty printing and syntax coloring)
- [fixed] version field display in ls -l command to show correct latest version numbers
- [enhanced] ls -l and ls -lr commands updated to Linux-like format with Modified, Size, Version, Type, Name columns
- [fixed] cat command now adds trailing newline to ensure CLI prompt appears on new line
- [enhanced] jq command now defaults to "." expression when no expression provided for easier JSON file viewing
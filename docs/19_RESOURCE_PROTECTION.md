# Resource Protection System

The VFS includes a **pluggable protection system** that allows you to define hard-coded rules to protect critical resources. This is separate from the `.rego` authorization policy and provides an additional layer of security.

## Implementation

**Location**: `pkg/domain/protection.go`

The protection system provides a layered security approach:
- Resource Protection (this system) - Hard-coded, cannot be misconfigured
- `.rego` Authorization - Flexible, policy-based access control

## Why Protection Rules?

**Authorization (`.rego`)** is flexible but can be misconfigured. **Protection rules** are hard-coded to ensure critical system files remain secure even if authorization policies are broken.

## Core Interface

```go
// ResourceProtection defines protection rules (line 10)
type ResourceProtection interface {
    CanModify(ctx context.Context, req ProtectionRequest) error
    CanDelete(ctx context.Context, req ProtectionRequest) error
}
```

**Source**: `pkg/domain/protection.go` lines 8-16

## Default Protection Rules

**Implementation**: `pkg/domain/protection.go` lines 32-102

The system comes with `DefaultProtectionRules` that protect:

### Protected Resources at Root (`/`)

- **`/.rego`** - Authorization policy (only system-admin can modify/delete)
- **`/.group`** - Group definitions (only system-admin can modify/delete)
- **`/.user`** - User credentials (only system-admin can modify/delete)
- **`/` directory** - Root directory (only system-admin can delete)

**Source**: `pkg/domain/protection.go` line 44

### Subdirectory Restrictions

- **`/.group`** - Cannot be created in subdirectories (only at root)
- **`/.user`** - Cannot be created in subdirectories (only at root)

**Implementation**: `pkg/domain/protection.go` lines 62-69

### Allowed in Subdirectories

- ✅ `/.rego` - Can override parent policy
- ✅ `/.owner` - Can set directory ownership
- ✅ `/.files` - Can set schema validation
- ✅ `/.events` - Can set event handlers

## Usage

### 1. Using Default Protection (Recommended)

```go
import "github.com/telnet2/mysql-vfs/pkg/domain"

// Initialize with default protection
protection := domain.NewDefaultProtectionRules()

// Check if user can modify a file
err := protection.CanModify(ctx, domain.ProtectionRequest{
    DirectoryPath: "/",
    FileName:      ".rego",
    UserRole:      "admin", // Not system-admin
})
// Returns error: "only system-admin can modify .rego"

// System admin can modify
err = protection.CanModify(ctx, domain.ProtectionRequest{
    DirectoryPath: "/",
    FileName:      ".rego",
    UserRole:      "system-admin",
})
// Returns nil (allowed)
```

**Source**: `pkg/domain/protection.go` lines 42-46 (NewDefaultProtectionRules)

### 2. Custom Protection Rules

You can customize which files are protected:

```go
protection := &domain.DefaultProtectionRules{
    ProtectedRootFiles: []string{".rego", ".config", ".secrets"},
    SystemAdminRole:    "super-admin",
}

// Now .config and .secrets are also protected
// And "super-admin" bypasses protection instead of "system-admin"
```

**Customizable Fields**: `pkg/domain/protection.go` lines 35-38

### 3. No Protection (Disable)

For testing or if you want full `.rego` control:

```go
protection := domain.NoProtection{}

// Everything is allowed
err := protection.CanModify(ctx, request)
// Always returns nil
```

**Implementation**: `pkg/domain/protection.go` lines 104-114

### 4. Custom Protection Functions

Create ad-hoc rules:

```go
protection := &domain.CustomProtection{
    ModifyFunc: func(ctx context.Context, req domain.ProtectionRequest) error {
        // Protect files ending with .lock
        if strings.HasSuffix(req.FileName, ".lock") {
            return fmt.Errorf("lock files cannot be modified")
        }

        // Only allow modifications during business hours
        hour := time.Now().Hour()
        if hour < 9 || hour > 17 {
            return fmt.Errorf("modifications only allowed 9AM-5PM")
        }

        return nil
    },
    DeleteFunc: func(ctx context.Context, req domain.ProtectionRequest) error {
        // Protect directories with specific prefix
        if strings.HasPrefix(req.DirectoryPath, "/archive/") {
            return fmt.Errorf("archived directories cannot be deleted")
        }
        return nil
    },
}
```

**Implementation**: `pkg/domain/protection.go` lines 116-136

### 5. Chained Protection

Combine multiple protection rules:

```go
defaultRules := domain.NewDefaultProtectionRules()
customRules := &domain.CustomProtection{
    ModifyFunc: func(ctx context.Context, req domain.ProtectionRequest) error {
        // Your custom logic
        if req.FileName == "readonly.txt" {
            return fmt.Errorf("file is read-only")
        }
        return nil
    },
}

// Both rules must pass
protection := &domain.ChainedProtection{
    Rules: []domain.ResourceProtection{
        defaultRules,
        customRules,
    },
}

// Will be checked against both rules
err := protection.CanModify(ctx, request)
```

**Implementation**: `pkg/domain/protection.go` lines 138-160

## Integration with Services

### File Service Integration

**Location**: `pkg/domain/file_service.go`

```go
type FileService struct {
    db         *gorm.DB
    storage    storage.Storage
    protection domain.ResourceProtection
}

func NewFileServiceWithProtection(
    db *gorm.DB,
    storage storage.Storage,
    protection domain.ResourceProtection,
) *FileService {
    return &FileService{
        db:         db,
        storage:    storage,
        protection: protection,
    }
}

func (s *FileService) CreateFile(ctx context.Context, dirPath, name string, ...) error {
    // Get user from context
    user := auth.GetUserFromContext(ctx)

    // Check protection rules
    err := s.protection.CanModify(ctx, domain.ProtectionRequest{
        DirectoryPath: dirPath,
        FileName:      name,
        UserRole:      user.Role,
    })
    if err != nil {
        return err // Blocked by protection rules
    }

    // Proceed with creation
    // ...
}
```

### Directory Service Integration

**Location**: `pkg/domain/directory_service.go`

```go
func (s *DirectoryService) DeleteDirectory(ctx context.Context, path string) error {
    user := auth.GetUserFromContext(ctx)

    // Check protection rules
    err := s.protection.CanDelete(ctx, domain.ProtectionRequest{
        ResourcePath: path,
        ResourceType: "directory",
        UserRole:     user.Role,
    })
    if err != nil {
        return err
    }

    // Proceed with deletion
    // ...
}
```

## Protection Request Structure

**Source**: `pkg/domain/protection.go` lines 18-30

```go
type ProtectionRequest struct {
    // Resource information
    DirectoryPath string  // Parent directory path
    FileName      string  // File name
    ResourcePath  string  // Full path: directoryPath + fileName
    ResourceType  string  // "file" or "directory"

    // User information
    UserID   string   // User ID
    UserRole string   // User role
    Groups   []string // User groups (deprecated in role-only refactor)
}
```

## Protection vs Authorization

| Feature | Protection Rules | `.rego` Authorization |
|---------|------------------|----------------------|
| **Location** | Hard-coded in application | Stored in `/.rego` file |
| **Flexibility** | Plugin/interface based | Fully dynamic |
| **Can be broken** | No (code-level) | Yes (misconfiguration) |
| **Scope** | System-critical resources | All resources |
| **Bypass** | Only system-admin | Based on policy rules |
| **When to use** | Protect system integrity | Business logic authorization |
| **Implementation** | `pkg/domain/protection.go` | `pkg/domain/policy_loader.go` |

## Best Practices

1. **Use DefaultProtectionRules** for production systems
2. **Don't disable protection** unless you have a specific reason
3. **Keep protected files list minimal** - only truly critical files
4. **Use ChainedProtection** to add business-specific rules on top of defaults
5. **System-admin should be environment-based** (token), not user-based

## Examples

### Example 1: Production Setup

```go
// Recommended for production
protection := domain.NewDefaultProtectionRules()

fileService := domain.NewFileServiceWithProtection(fileRepo, protection)
dirService := domain.NewDirectoryServiceWithProtection(dirRepo, protection)
```

### Example 2: Add Business Rules

```go
defaultRules := domain.NewDefaultProtectionRules()
businessRules := &domain.CustomProtection{
    ModifyFunc: func(ctx context.Context, req domain.ProtectionRequest) error {
        // Compliance requirement: audit log files cannot be modified
        if strings.Contains(req.DirectoryPath, "/audit/") {
            if req.UserRole != "compliance-officer" {
                return fmt.Errorf("only compliance officers can modify audit files")
            }
        }
        return nil
    },
}

protection := &domain.ChainedProtection{
    Rules: []domain.ResourceProtection{defaultRules, businessRules},
}
```

### Example 3: Testing Environment

```go
// Disable protection for integration tests
protection := domain.NoProtection{}

// Or keep defaults but with test-specific admin role
protection := &domain.DefaultProtectionRules{
    ProtectedRootFiles: []string{".rego", ".group", ".user"},
    SystemAdminRole:    "test-admin", // Match your test setup
}
```

## Security Considerations

1. **System-admin token** should be:
   - Generated randomly
   - Stored in environment variables
   - Never committed to version control
   - Rotated regularly

2. **Protection rules should**:
   - Be reviewed in security audits
   - Be tested thoroughly
   - Fail closed (deny by default)
   - Log all denials

3. **Don't rely solely on protection**:
   - Use in combination with `.rego` policies
   - Implement least privilege
   - Monitor access patterns

## Configuration

**Location**: `pkg/config/config.go`

System admin configuration:

```bash
# Environment variables
SYSTEM_ADMIN_TOKEN=your-secure-random-token
SYSTEM_ADMIN_ID=system-admin
SYSTEM_ADMIN_ROLE=system-admin
```

**Default Values**:
- `SystemAdminRole`: "system-admin"
- `ProtectedRootFiles`: [".rego", ".group", ".user"]

## Code References

### Core Implementation

- **Main protection logic**: `pkg/domain/protection.go`
  - ResourceProtection interface: lines 8-16
  - ProtectionRequest struct: lines 18-30
  - DefaultProtectionRules: lines 32-102
  - NoProtection: lines 104-114
  - CustomProtection: lines 116-136
  - ChainedProtection: lines 138-160

### Integration Points

- **File service**: `pkg/domain/file_service.go`
  - Protection checks in CreateFile, UpdateFile, DeleteFile

- **Directory service**: `pkg/domain/directory_service.go`
  - Protection checks in CreateDirectory, DeleteDirectory

- **Main service setup**: `services/vfs/main.go`
  - Protection initialization and wiring

### Related Components

- **Authorization**: `pkg/middleware/authorization.go`
- **System admin config**: `pkg/config/config.go`
- **Special files**: `pkg/domain/special_files.go`

## Testing

**Location**: `pkg/domain/protection_test.go`

Test coverage includes:
- Default protection rules
- System admin bypass
- Protected file checks
- Subdirectory restrictions
- Custom protection functions
- Chained protection

## See Also

- [Authorization Guide](6_AUTHORIZATION.md) - OPA/Rego policies
- [Authentication](5_AUTHENTICATION.md) - User authentication
- [Bootstrap Guide](18_BOOTSTRAP.md) - Setting up default files
- [Special Files](4_SPECIAL_FILES.md) - File types

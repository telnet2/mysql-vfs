# User & Group Management Design

**Feature:** Basic Admin/User/Group Management
**Date:** 2025-10-03
**Status:** Design

---

## Overview

Implement a basic user and group management system to support:
- User authentication and authorization
- Role-based access control (RBAC)
- Group memberships
- Special file permissions (super-user only)

---

## User Model

### Database Schema

```sql
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(255),
    role VARCHAR(50) NOT NULL DEFAULT 'user',  -- 'admin', 'user'
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL,

    INDEX idx_username (username),
    INDEX idx_email (email),
    INDEX idx_role (role)
);
```

### Go Model

```go
// pkg/models/user.go
package models

import (
    "time"
    "gorm.io/gorm"
)

type User struct {
    ID           string         `gorm:"type:char(36);primaryKey"`
    Username     string         `gorm:"type:varchar(255);not null;uniqueIndex"`
    Email        string         `gorm:"type:varchar(255);not null;uniqueIndex"`
    PasswordHash string         `gorm:"type:varchar(255);not null"`
    FullName     string         `gorm:"type:varchar(255)"`
    Role         UserRole       `gorm:"type:varchar(50);not null;default:'user';index"`
    IsActive     bool           `gorm:"not null;default:true"`
    CreatedAt    time.Time      `gorm:"not null"`
    UpdatedAt    time.Time      `gorm:"not null"`
    DeletedAt    gorm.DeletedAt `gorm:"index"`

    // Relations
    Groups []Group `gorm:"many2many:user_groups;"`
}

type UserRole string

const (
    RoleAdmin     UserRole = "admin"      // Super-user, can manage everything
    RoleUser      UserRole = "user"       // Regular user
    RoleReadOnly  UserRole = "readonly"   // Read-only access
)

func (User) TableName() string {
    return "users"
}

// IsSuperUser checks if user has super-user privileges
func (u *User) IsSuperUser() bool {
    return u.Role == RoleAdmin
}

// HasRole checks if user has a specific role
func (u *User) HasRole(role UserRole) bool {
    return u.Role == role
}
```

---

## Group Model

### Database Schema

```sql
CREATE TABLE groups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP NULL,

    INDEX idx_name (name)
);

CREATE TABLE user_groups (
    user_id VARCHAR(36) NOT NULL,
    group_id VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL,

    PRIMARY KEY (user_id, group_id),
    INDEX idx_user_id (user_id),
    INDEX idx_group_id (group_id)
);
```

### Go Model

```go
// pkg/models/group.go
package models

import (
    "time"
    "gorm.io/gorm"
)

type Group struct {
    ID          string         `gorm:"type:char(36);primaryKey"`
    Name        string         `gorm:"type:varchar(255);not null;uniqueIndex"`
    Description string         `gorm:"type:text"`
    CreatedAt   time.Time      `gorm:"not null"`
    UpdatedAt   time.Time      `gorm:"not null"`
    DeletedAt   gorm.DeletedAt `gorm:"index"`

    // Relations
    Users []User `gorm:"many2many:user_groups;"`
}

func (Group) TableName() string {
    return "groups"
}

type UserGroup struct {
    UserID    string    `gorm:"type:char(36);primaryKey"`
    GroupID   string    `gorm:"type:char(36);primaryKey"`
    CreatedAt time.Time `gorm:"not null"`
}

func (UserGroup) TableName() string {
    return "user_groups"
}
```

---

## Repository Layer

### User Repository

```go
// pkg/repository/interfaces.go

type UserRepository interface {
    // Create creates a new user
    Create(ctx context.Context, user *models.User) error

    // FindByID finds a user by ID
    FindByID(ctx context.Context, id string) (*models.User, error)

    // FindByUsername finds a user by username
    FindByUsername(ctx context.Context, username string) (*models.User, error)

    // FindByEmail finds a user by email
    FindByEmail(ctx context.Context, email string) (*models.User, error)

    // List lists all users with pagination
    List(ctx context.Context, limit int, cursor string) ([]*models.User, string, error)

    // Update updates a user
    Update(ctx context.Context, user *models.User) error

    // Delete soft deletes a user
    Delete(ctx context.Context, id string) error

    // AddToGroup adds a user to a group
    AddToGroup(ctx context.Context, userID, groupID string) error

    // RemoveFromGroup removes a user from a group
    RemoveFromGroup(ctx context.Context, userID, groupID string) error

    // GetGroups gets all groups for a user
    GetGroups(ctx context.Context, userID string) ([]*models.Group, error)
}

type GroupRepository interface {
    // Create creates a new group
    Create(ctx context.Context, group *models.Group) error

    // FindByID finds a group by ID
    FindByID(ctx context.Context, id string) (*models.Group, error)

    // FindByName finds a group by name
    FindByName(ctx context.Context, name string) (*models.Group, error)

    // List lists all groups with pagination
    List(ctx context.Context, limit int, cursor string) ([]*models.Group, string, error)

    // Update updates a group
    Update(ctx context.Context, group *models.Group) error

    // Delete soft deletes a group
    Delete(ctx context.Context, id string) error

    // GetMembers gets all users in a group
    GetMembers(ctx context.Context, groupID string) ([]*models.User, error)
}
```

### Add to UnitOfWork

```go
type UnitOfWork interface {
    // BeginTransaction starts a new transaction
    BeginTransaction(ctx context.Context) (Transaction, error)

    // Directories returns the directory repository
    Directories() DirectoryRepository

    // Files returns the file repository
    Files() FileRepository

    // Events returns the event repository
    Events() EventRepository

    // Users returns the user repository
    Users() UserRepository

    // Groups returns the group repository
    Groups() GroupRepository
}
```

---

## Authentication & Authorization

### JWT Token Structure

```go
// pkg/auth/token.go

type Claims struct {
    UserID   string   `json:"user_id"`
    Username string   `json:"username"`
    Email    string   `json:"email"`
    Role     string   `json:"role"`
    Groups   []string `json:"groups"`
    jwt.RegisteredClaims
}

func GenerateToken(user *models.User, groups []string) (string, error) {
    claims := Claims{
        UserID:   user.ID,
        Username: user.Username,
        Email:    user.Email,
        Role:     string(user.Role),
        Groups:   groups,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}
```

### Authentication Middleware

```go
// pkg/middleware/auth.go

type AuthMiddleware struct {
    jwtSecret []byte
    userRepo  repository.UserRepository
}

func NewAuthMiddleware(jwtSecret string, userRepo repository.UserRepository) *AuthMiddleware {
    return &AuthMiddleware{
        jwtSecret: []byte(jwtSecret),
        userRepo:  userRepo,
    }
}

func (m *AuthMiddleware) Handler() app.HandlerFunc {
    return func(ctx context.Context, c *app.RequestContext) {
        // Extract token from Authorization header
        authHeader := c.GetHeader("Authorization")
        if len(authHeader) == 0 {
            c.JSON(401, map[string]string{"error": "unauthorized"})
            c.Abort()
            return
        }

        // Parse "Bearer {token}"
        parts := strings.Split(string(authHeader), " ")
        if len(parts) != 2 || parts[0] != "Bearer" {
            c.JSON(401, map[string]string{"error": "invalid authorization header"})
            c.Abort()
            return
        }

        tokenString := parts[1]

        // Parse and validate token
        claims := &auth.Claims{}
        token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
            return m.jwtSecret, nil
        })

        if err != nil || !token.Valid {
            c.JSON(401, map[string]string{"error": "invalid token"})
            c.Abort()
            return
        }

        // Store user info in context
        userContext := map[string]interface{}{
            "user_id":  claims.UserID,
            "username": claims.Username,
            "email":    claims.Email,
            "role":     claims.Role,
            "groups":   claims.Groups,
        }

        c.Set("user", userContext)
        c.Next(ctx)
    }
}
```

### Super-User Check (Updated)

```go
// pkg/domain/special_files.go

func IsSuperUser(ctx context.Context) bool {
    user, ok := ctx.Value("user").(map[string]interface{})
    if !ok {
        return false
    }

    role, ok := user["role"].(string)
    if !ok {
        return false
    }

    return role == string(models.RoleAdmin)
}

func GetUserFromContext(ctx context.Context) (map[string]interface{}, bool) {
    user, ok := ctx.Value("user").(map[string]interface{})
    return user, ok
}

func GetUserID(ctx context.Context) string {
    user, ok := GetUserFromContext(ctx)
    if !ok {
        return ""
    }

    userID, _ := user["user_id"].(string)
    return userID
}
```

---

## API Endpoints

### User Management (Admin Only)

```http
# Create user
POST /api/v1/admin/users
Authorization: Bearer {admin-token}
{
  "username": "john",
  "email": "john@example.com",
  "password": "secure-password",
  "full_name": "John Doe",
  "role": "user"
}

# List users
GET /api/v1/admin/users?limit=20&cursor=abc

# Get user
GET /api/v1/admin/users/{id}

# Update user
PATCH /api/v1/admin/users/{id}
{
  "full_name": "John Smith",
  "role": "admin"
}

# Delete user
DELETE /api/v1/admin/users/{id}

# Add user to group
POST /api/v1/admin/users/{id}/groups
{
  "group_id": "group-uuid"
}

# Remove user from group
DELETE /api/v1/admin/users/{id}/groups/{group_id}
```

### Group Management (Admin Only)

```http
# Create group
POST /api/v1/admin/groups
Authorization: Bearer {admin-token}
{
  "name": "engineering",
  "description": "Engineering team"
}

# List groups
GET /api/v1/admin/groups?limit=20&cursor=abc

# Get group
GET /api/v1/admin/groups/{id}

# Update group
PATCH /api/v1/admin/groups/{id}
{
  "description": "Engineering and DevOps team"
}

# Delete group
DELETE /api/v1/admin/groups/{id}

# Get group members
GET /api/v1/admin/groups/{id}/members
```

### Authentication

```http
# Login
POST /api/v1/auth/login
{
  "username": "john",
  "password": "secure-password"
}

Response (200):
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "user-uuid",
    "username": "john",
    "email": "john@example.com",
    "role": "user",
    "groups": ["engineering"]
  }
}

# Get current user
GET /api/v1/auth/me
Authorization: Bearer {token}

Response (200):
{
  "id": "user-uuid",
  "username": "john",
  "email": "john@example.com",
  "role": "user",
  "groups": ["engineering"]
}

# Change password
POST /api/v1/auth/change-password
Authorization: Bearer {token}
{
  "old_password": "secure-password",
  "new_password": "new-secure-password"
}
```

---

## Integration with Special Files

### Updated Special File Authorization

```go
// In FileService.CreateFile()

// Check if special file
if domain.IsSpecialFile(name) {
    if !domain.IsSuperUser(ctx) {
        return nil, fmt.Errorf("only administrators can create special files (files starting with '.')")
    }

    // Validate special file content
    if err := s.validateSpecialFileContent(name, contentBytes); err != nil {
        return nil, fmt.Errorf("invalid special file content: %w", err)
    }

    // Log admin action
    s.auditLog(ctx, "special_file_created", map[string]interface{}{
        "file_name": name,
        "directory": directoryPath,
        "user_id":   domain.GetUserID(ctx),
    })
}
```

---

## Bootstrap Process

### Initial Admin Creation

```go
// pkg/bootstrap/admin.go

func CreateInitialAdmin(db *gorm.DB) error {
    // Check if admin already exists
    var count int64
    if err := db.Model(&models.User{}).
        Where("role = ?", models.RoleAdmin).
        Count(&count).Error; err != nil {
        return err
    }

    if count > 0 {
        log.Info("Admin user already exists, skipping bootstrap")
        return nil
    }

    // Create initial admin from environment variables
    username := os.Getenv("ADMIN_USERNAME")
    if username == "" {
        username = "admin"
    }

    password := os.Getenv("ADMIN_PASSWORD")
    if password == "" {
        password = "admin" // WARN: Change this!
        log.Warn("Using default admin password. Please change it!")
    }

    email := os.Getenv("ADMIN_EMAIL")
    if email == "" {
        email = "admin@example.com"
    }

    passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return err
    }

    admin := &models.User{
        ID:           uuid.New().String(),
        Username:     username,
        Email:        email,
        PasswordHash: string(passwordHash),
        FullName:     "System Administrator",
        Role:         models.RoleAdmin,
        IsActive:     true,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }

    if err := db.Create(admin).Error; err != nil {
        return err
    }

    log.Info("Initial admin user created", "username", username)
    return nil
}
```

### Call in main.go

```go
// services/vfs/main.go

func main() {
    // ... database connection

    // Bootstrap initial admin
    if err := bootstrap.CreateInitialAdmin(db); err != nil {
        log.Fatal("Failed to create initial admin", "error", err)
    }

    // ... rest of initialization
}
```

---

## Environment Variables

```bash
# .env
JWT_SECRET=your-super-secret-jwt-key-change-this
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-this-secure-password
ADMIN_EMAIL=admin@example.com
```

---

## Migration

```go
// pkg/db/migrate.go

func AutoMigrate(db *gorm.DB) error {
    modelsToMigrate := []interface{}{
        &models.User{},          // Add
        &models.Group{},         // Add
        &models.UserGroup{},     // Add
        &models.OPAPolicy{},
        &models.Directory{},
        &models.File{},
        &models.FileVersion{},
        &models.FileRelation{},
        &models.Event{},
        &models.WebhookConfig{},
        &models.WebhookJob{},
        &models.CronJob{},
        &models.CronExecution{},
        &models.IdempotencyRecord{},
        &models.AuditLog{},
        &models.DeadLetterQueue{},
    }

    if err := db.AutoMigrate(modelsToMigrate...); err != nil {
        return fmt.Errorf("failed to run migrations: %w", err)
    }

    return nil
}
```

---

## Example Workflow

### 1. Initial Setup

```bash
# Start service with admin credentials
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD=SecureP@ssw0rd
export ADMIN_EMAIL=admin@company.com
export JWT_SECRET=your-super-secret-key

docker-compose up
```

### 2. Admin Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "SecureP@ssw0rd"
  }'

# Response:
{
  "token": "eyJhbGci...",
  "user": {
    "username": "admin",
    "role": "admin"
  }
}
```

### 3. Create Regular User

```bash
curl -X POST http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john",
    "email": "john@company.com",
    "password": "UserP@ssw0rd",
    "full_name": "John Doe",
    "role": "user"
  }'
```

### 4. Create Group

```bash
curl -X POST http://localhost:8080/api/v1/admin/groups \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "engineering",
    "description": "Engineering team"
  }'
```

### 5. Add User to Group

```bash
curl -X POST http://localhost:8080/api/v1/admin/users/{user-id}/groups \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "group_id": "{group-id}"
  }'
```

### 6. Admin Creates Schema

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data/users",
    "name": ".jsonschema",
    "content_type": "application/json",
    "content": "{\"type\":\"object\",\"required\":[\"email\"]}"
  }'
```

### 7. Regular User Uploads File (Validated)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {user-token}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data/users",
    "name": "profile.json",
    "content_type": "application/json",
    "content": "{\"email\":\"john@company.com\",\"name\":\"John\"}"
  }'
```

### 8. Regular User Tries to Create Special File (Rejected)

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer {user-token}" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": ".custom",
    "content": "anything"
  }'

# Response: 403 Forbidden
{
  "error": "only administrators can create special files"
}
```

---

## Security Considerations

### Password Security
- Use bcrypt for password hashing
- Minimum password length: 8 characters
- Require password complexity (optional, configurable)
- Implement password reset flow

### Token Security
- Use strong JWT secret (256-bit minimum)
- Token expiration: 24 hours (configurable)
- Implement token refresh mechanism
- Store tokens securely on client side

### Role Separation
- Admin: Full access, can create special files
- User: Regular access, subject to OPA policies
- ReadOnly: Read-only access

---

## Implementation Checklist

### Phase 1: Models & Database
- [ ] Create User model
- [ ] Create Group model
- [ ] Create UserGroup model
- [ ] Add to AutoMigrate
- [ ] Create repositories

### Phase 2: Authentication
- [ ] JWT token generation
- [ ] Login endpoint
- [ ] Auth middleware
- [ ] Password hashing utilities

### Phase 3: User Management
- [ ] User CRUD endpoints (admin only)
- [ ] Group CRUD endpoints (admin only)
- [ ] User-group association endpoints
- [ ] Bootstrap initial admin

### Phase 4: Integration
- [ ] Update IsSuperUser check
- [ ] Update special file authorization
- [ ] Add audit logging
- [ ] Update file service

### Phase 5: Testing
- [ ] Unit tests for auth
- [ ] Integration tests for user management
- [ ] E2E tests for special file permissions

---

**Status:** Design Complete
**Next:** Implementation
**Estimated Effort:** 2-3 days

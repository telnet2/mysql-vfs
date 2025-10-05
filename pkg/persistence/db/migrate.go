package db

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds database configuration
type Config struct {
	DSN      string
	LogLevel logger.LogLevel
}

// Connect establishes a connection to the database
func Connect(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(cfg.LogLevel),
		// Disable foreign key constraints - we manage referential integrity in application code
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// AutoMigrate runs automatic migrations for all models
func AutoMigrate(db *gorm.DB) error {
	// Define all models in dependency order
	modelsToMigrate := []interface{}{
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

	// Run auto migration
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Add additional constraints and indexes
	if err := addCustomConstraints(db); err != nil {
		return fmt.Errorf("failed to add custom constraints: %w", err)
	}

	// Run manual migrations for schema changes
	if err := runManualMigrations(db); err != nil {
		return fmt.Errorf("failed to run manual migrations: %w", err)
	}

	// Bootstrap default files
	if err := bootstrapDefaultFiles(db); err != nil {
		return fmt.Errorf("failed to bootstrap default files: %w", err)
	}

	return nil
}

// addCustomConstraints adds CHECK constraints and other custom database constraints
func addCustomConstraints(db *gorm.DB) error {
	// Get database name to determine which constraints to apply
	var dbName string
	db.Raw("SELECT DATABASE()").Scan(&dbName)

	// Only add constraints for MySQL (skip for SQLite in tests)
	if dbName == "" {
		// Likely SQLite or other DB, skip constraints
		return nil
	}

	// Add CHECK constraint for files.size_bytes <= 100MB
	if err := db.Exec(`
		ALTER TABLE files
		ADD CONSTRAINT chk_file_size
		CHECK (size_bytes <= 104857600)
	`).Error; err != nil {
		// Ignore if constraint already exists
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add file size constraint: %w", err)
		}
	}

	// Add CHECK constraint for files storage type consistency
	if err := db.Exec(`
		ALTER TABLE files
		ADD CONSTRAINT chk_storage_type
		CHECK (
			(storage_type = 'json' AND json_content IS NOT NULL) OR
			(storage_type = 'text' AND text_content IS NOT NULL) OR
			(storage_type = 's3' AND s3_key IS NOT NULL)
		)
	`).Error; err != nil {
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add storage type constraint: %w", err)
		}
	}

	// Add CHECK constraint for text content size (100MB limit, same as JSON)
	if err := db.Exec(`
		ALTER TABLE files
		ADD CONSTRAINT chk_text_size
		CHECK (storage_type != 'text' OR size_bytes <= 104857600)
	`).Error; err != nil {
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add text size constraint: %w", err)
		}
	}

	// Add unique index for directories (parent_id, name) where deleted_at IS NULL
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dir_parent_name_active
		ON directories(parent_id, name)
		WHERE deleted_at IS NULL
	`).Error; err != nil {
		// For MySQL, use a different approach since it doesn't support WHERE in unique indexes
		// We'll rely on GORM's soft delete handling instead
	}

	return nil
}

// isConstraintExistsError checks if the error is due to constraint already existing
func isConstraintExistsError(err error) bool {
	// MySQL error code 1061: Duplicate key name
	// MySQL error code 3822: Duplicate check constraint
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "Error 1061") ||
		strings.Contains(errMsg, "Error 3822") ||
		strings.Contains(errMsg, "Duplicate check constraint")
}

// runManualMigrations runs manual schema migrations that AutoMigrate can't handle
func runManualMigrations(db *gorm.DB) error {
	// Get database name to determine which migrations to apply
	var dbName string
	db.Raw("SELECT DATABASE()").Scan(&dbName)

	// Only run migrations for MySQL (skip for SQLite in tests)
	if dbName == "" {
		// Likely SQLite or other DB, skip migrations
		return nil
	}

	// Migrate text_content from TEXT to MEDIUMTEXT to support files up to 16MB
	// This migration is idempotent (safe to run multiple times)
	if err := db.Exec(`
		ALTER TABLE files
		MODIFY COLUMN text_content MEDIUMTEXT
	`).Error; err != nil {
		return fmt.Errorf("failed to migrate files.text_content to MEDIUMTEXT: %w", err)
	}

	if err := db.Exec(`
		ALTER TABLE file_versions
		MODIFY COLUMN text_content MEDIUMTEXT
	`).Error; err != nil {
		return fmt.Errorf("failed to migrate file_versions.text_content to MEDIUMTEXT: %w", err)
	}

	return nil
}

// HealthCheck performs a database health check
func HealthCheck(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// bootstrapDefaultFiles creates default /.rego and /.group files during migration
func bootstrapDefaultFiles(db *gorm.DB) error {
	// Ensure root directory exists
	var rootDir models.Directory
	if err := db.Where("path = ?", "/").First(&rootDir).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create root directory
			rootDir = models.Directory{
				ID:       "root",
				Name:     "",
				Path:     "/",
				ParentID: nil,
			}
			if err := db.Create(&rootDir).Error; err != nil {
				return fmt.Errorf("failed to create root directory: %w", err)
			}
			fmt.Println("✓ Created root directory")
		} else {
			return fmt.Errorf("failed to query root directory: %w", err)
		}
	}

	// Create /.rego file if it doesn't exist
	if err := createDefaultRegoFile(db, rootDir.ID); err != nil {
		return err
	}

	// Create /.group file if it doesn't exist
	if err := createDefaultGroupFile(db, rootDir.ID); err != nil {
		return err
	}

	return nil
}

// createDefaultRegoFile creates the default /.rego authorization policy
func createDefaultRegoFile(db *gorm.DB, rootDirID string) error {
	// Check if .rego already exists
	var existing models.File
	err := db.Where("directory_id = ? AND name = ?", rootDirID, ".rego").First(&existing).Error
	if err == nil {
		// File already exists, skip
		fmt.Println("✓ /.rego already exists")
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to query .rego file: %w", err)
	}

	// Default policy: admin group has full access, user group has read-only
	defaultPolicy := `package vfs.authz

# Simple default policy for flexible authorization
# Customize this policy to match your security requirements

# Admin group: full access to all operations
allow {
    input.user.groups[_] == "admin"
}

# System admin group: full access to all operations
allow {
    input.user.groups[_] == "system-admin"
}

# User group: read-only access
allow {
    input.user.groups[_] == "user"
    input.action == "read"
}
`

	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(defaultPolicy)))

	// Create the file
	regoFile := models.File{
		ID:             uuid.New().String(),
		DirectoryID:    rootDirID,
		Name:           ".rego",
		ContentType:    "text/plain",
		SizeBytes:      int64(len(defaultPolicy)),
		StorageType:    "text",
		TextContent:    &defaultPolicy,
		ChecksumSHA256: checksum,
		Version:        1,
	}

	if err := db.Create(&regoFile).Error; err != nil {
		return fmt.Errorf("failed to create .rego file: %w", err)
	}

	fmt.Println("✓ Created /.rego with default authorization policy (admin: full access, user: read-only)")
	return nil
}

// createDefaultGroupFile creates the default /.group file
func createDefaultGroupFile(db *gorm.DB, rootDirID string) error {
	// Check if .group already exists
	var existing models.File
	err := db.Where("directory_id = ? AND name = ?", rootDirID, ".group").First(&existing).Error
	if err == nil {
		// File already exists, skip
		fmt.Println("✓ /.group already exists")
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to query .group file: %w", err)
	}

	// Default groups: admin and user (empty members)
	defaultGroups := `{
	"groups": [
		{
			"group_id": "admin",
			"members": []
		},
		{
			"group_id": "user",
			"members": []
		}
	]
}`

	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(defaultGroups)))

	// Create the file
	groupFile := models.File{
		ID:             uuid.New().String(),
		DirectoryID:    rootDirID,
		Name:           ".group",
		ContentType:    "application/json",
		SizeBytes:      int64(len(defaultGroups)),
		StorageType:    "json",
		JSONContent:    &defaultGroups,
		ChecksumSHA256: checksum,
		Version:        1,
	}

	if err := db.Create(&groupFile).Error; err != nil {
		return fmt.Errorf("failed to create .group file: %w", err)
	}

	fmt.Println("✓ Created /.group with default groups (admin, user)")
	return nil
}

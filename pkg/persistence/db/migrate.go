package db

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/telnet2/mysql-vfs/pkg/defaults"
	"github.com/telnet2/mysql-vfs/pkg/etc"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// customNamingStrategy implements GORM's NamingStrategy with table prefix support
type customNamingStrategy struct {
	schema.NamingStrategy
	TablePrefix string
}

// TableName adds prefix to table names
func (ns *customNamingStrategy) TableName(table string) string {
	// First call the embedded strategy to get the proper table name (pluralized, etc.)
	baseName := ns.NamingStrategy.TableName(table)
	// Then add our prefix
	return ns.TablePrefix + baseName
}

// Config holds database configuration
type Config struct {
	DSN         string
	TablePrefix string
	LogLevel    logger.LogLevel
}

// Connect establishes a connection to the database
func Connect(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(cfg.LogLevel),
		// Disable foreign key constraints - we manage referential integrity in application code
		DisableForeignKeyConstraintWhenMigrating: true,
		NamingStrategy: &customNamingStrategy{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false, // use plural table names
			},
			TablePrefix: cfg.TablePrefix,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// getTableName returns the table name for a model using GORM's naming strategy
func getTableName(db *gorm.DB, model interface{}) string {
	// Get the model's type name
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return ""
	}
	// Use the naming strategy to get the table name
	if db.NamingStrategy != nil {
		return db.NamingStrategy.TableName(stmt.Schema.ModelType.Name())
	}
	return stmt.Schema.Table
}

// AutoMigrate runs automatic migrations for all models
func AutoMigrate(db *gorm.DB) error {
	// Define all models in dependency order with explicit table names
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
		&models.WorkflowAudit{},
		&models.DeadLetterQueue{},
	}

	// Run auto migration with explicit table names
	for _, model := range modelsToMigrate {
		tableName := getTableName(db, model)
		if err := db.Table(tableName).AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T to table %s: %w", model, tableName, err)
		}
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

	// Bootstrap system files in /etc
	if err := bootstrapSystemFiles(db); err != nil {
		return fmt.Errorf("failed to bootstrap system files: %w", err)
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

	// Get table names with prefix
	filesTable := getTableName(db, &models.File{})
	directoriesTable := getTableName(db, &models.Directory{})

	// Add CHECK constraint for files.size_bytes <= 100MB
	query := fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT chk_file_size
		CHECK (size_bytes <= 104857600)
	`, filesTable)
	if err := db.Exec(query).Error; err != nil {
		// Ignore if constraint already exists
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add file size constraint: %w", err)
		}
	}

	// Add CHECK constraint for files storage type consistency
	query = fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT chk_storage_type
		CHECK (
			(storage_type = 'json' AND json_content IS NOT NULL) OR
			(storage_type = 'text' AND text_content IS NOT NULL) OR
			(storage_type = 's3' AND s3_key IS NOT NULL)
		)
	`, filesTable)
	if err := db.Exec(query).Error; err != nil {
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add storage type constraint: %w", err)
		}
	}

	// Add CHECK constraint for text content size (100MB limit, same as JSON)
	query = fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT chk_text_size
		CHECK (storage_type != 'text' OR size_bytes <= 104857600)
	`, filesTable)
	if err := db.Exec(query).Error; err != nil {
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add text size constraint: %w", err)
		}
	}

	// Add unique index for directories (parent_id, name) where deleted_at IS NULL
	query = fmt.Sprintf(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dir_parent_name_active
		ON %s(parent_id, name)
		WHERE deleted_at IS NULL
	`, directoriesTable)
	if err := db.Exec(query).Error; err != nil {
		// For MySQL, use a different approach since it doesn't support WHERE in unique indexes
		// We'll rely on GORM's soft delete handling instead
	}

	if err := addWorkflowAuditIndexes(db); err != nil {
		return err
	}

	return nil
}

func addWorkflowAuditIndexes(db *gorm.DB) error {
	workflowAuditTable := getTableName(db, &models.WorkflowAudit{})

	indexes := []struct {
		Name  string
		Field string
	}{
		{"idx_file_path", "file_path"},
		{"idx_workflow_path", "workflow_path"},
		{"idx_actor", "actor"},
		{"idx_created_at", "created_at"},
	}

	for _, idx := range indexes {
		query := fmt.Sprintf("CREATE INDEX %s ON %s(%s)", idx.Name, workflowAuditTable, idx.Field)
		if err := db.Exec(query).Error; err != nil {
			if !isConstraintExistsError(err) {
				return fmt.Errorf("failed to create workflow_audit index %s: %w", idx.Name, err)
			}
		}
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

	// Get table names with prefix
	filesTable := getTableName(db, &models.File{})
	fileVersionsTable := getTableName(db, &models.FileVersion{})

	// Migrate text_content from TEXT to MEDIUMTEXT to support files up to 16MB
	// This migration is idempotent (safe to run multiple times)
	query := fmt.Sprintf(`
		ALTER TABLE %s
		MODIFY COLUMN text_content MEDIUMTEXT
	`, filesTable)
	if err := db.Exec(query).Error; err != nil {
		return fmt.Errorf("failed to migrate files.text_content to MEDIUMTEXT: %w", err)
	}

	query = fmt.Sprintf(`
		ALTER TABLE %s
		MODIFY COLUMN text_content MEDIUMTEXT
	`, fileVersionsTable)
	if err := db.Exec(query).Error; err != nil {
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

	// Load default policy from embedded file
	defaultPolicy := string(defaults.DefaultRego())

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

	// Load default groups from embedded file
	defaultGroups := string(defaults.DefaultGroup())

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

// bootstrapSystemFiles creates /etc/schemas directory and seeds schema files (always overwrites)
func bootstrapSystemFiles(db *gorm.DB) error {
	// Find or create /etc directory
	var etcDir models.Directory
	etcPath := "/etc"
	etcPathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(etcPath)))

	err := db.Where("path_hash = ?", etcPathHash).First(&etcDir).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to query /etc directory: %w", err)
	}

	if err == gorm.ErrRecordNotFound {
		// Create /etc directory with system metadata
		etcMetadata := `{"owner":"system-admin","creator":"system-admin","system":true,"readonly":true}`
		etcDir = models.Directory{
			ID:       "etc",
			ParentID: stringPtr("root"),
			Name:     "etc",
			Path:     etcPath,
			PathHash: etcPathHash,
			Version:  1,
			Metadata: &etcMetadata,
		}

		if err := db.Create(&etcDir).Error; err != nil {
			return fmt.Errorf("failed to create /etc directory: %w", err)
		}
		fmt.Println("✓ Created /etc directory")
	} else {
		fmt.Println("✓ /etc directory exists")
	}

	// Find or create /etc/schemas directory
	var schemasDir models.Directory
	schemasPath := "/etc/schemas"
	schemasPathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(schemasPath)))

	err = db.Where("path_hash = ?", schemasPathHash).First(&schemasDir).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to query /etc/schemas directory: %w", err)
	}

	if err == gorm.ErrRecordNotFound {
		// Create /etc/schemas directory with system metadata
		schemasMetadata := `{"owner":"system-admin","creator":"system-admin","system":true,"readonly":true}`
		schemasDir = models.Directory{
			ID:       "etc-schemas",
			ParentID: stringPtr("etc"),
			Name:     "schemas",
			Path:     schemasPath,
			PathHash: schemasPathHash,
			Version:  1,
			Metadata: &schemasMetadata,
		}

		if err := db.Create(&schemasDir).Error; err != nil {
			return fmt.Errorf("failed to create /etc/schemas directory: %w", err)
		}
		fmt.Println("✓ Created /etc/schemas directory")
	} else {
		fmt.Println("✓ /etc/schemas directory exists")
	}

	// Delete all existing files in /etc/schemas (we always overwrite)
	if err := db.Where("directory_id = ?", schemasDir.ID).Delete(&models.File{}).Error; err != nil {
		return fmt.Errorf("failed to clean /etc/schemas directory: %w", err)
	}

	// Seed schema files from embedded FS
	schemaFiles := []string{
		"owner.schema.json",
		"files.schema.json",
		"events.schema.json",
		"file.metadata.schema.json",
		"directory.metadata.schema.json",
	}

	systemMetadata := `{"owner":"system-admin","creator":"system-admin","system":true}`

	for _, filename := range schemaFiles {
		// Read embedded content
		content, err := etc.GetSchemaContent(filename)
		if err != nil {
			return fmt.Errorf("failed to read embedded schema %s: %w", filename, err)
		}

		// Calculate checksum
		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		contentStr := string(content)

		// Create file
		schemaFile := models.File{
			ID:             uuid.New().String(),
			DirectoryID:    schemasDir.ID,
			Name:           filename,
			ContentType:    "application/schema+json",
			SizeBytes:      int64(len(content)),
			StorageType:    "json",
			JSONContent:    &contentStr,
			ChecksumSHA256: checksum,
			Version:        1,
			Metadata:       &systemMetadata,
		}

		if err := db.Create(&schemaFile).Error; err != nil {
			return fmt.Errorf("failed to create /etc/schemas/%s: %w", filename, err)
		}

		fmt.Printf("✓ Seeded /etc/schemas/%s\n", filename)
	}

	fmt.Println("✓ System files bootstrap completed")
	return nil
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}

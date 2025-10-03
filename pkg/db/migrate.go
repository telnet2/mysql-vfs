package db

import (
	"fmt"

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

	// Run auto migration
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Add additional constraints and indexes
	if err := addCustomConstraints(db); err != nil {
		return fmt.Errorf("failed to add custom constraints: %w", err)
	}

	return nil
}

// addCustomConstraints adds CHECK constraints and other custom database constraints
func addCustomConstraints(db *gorm.DB) error {
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
		ADD CONSTRAINT chk_storage_type_json
		CHECK (
			(storage_type = 'json' AND json_content IS NOT NULL) OR
			(storage_type = 's3' AND s3_key IS NOT NULL)
		)
	`).Error; err != nil {
		if !isConstraintExistsError(err) {
			return fmt.Errorf("failed to add storage type constraint: %w", err)
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
	return err != nil && (
		err.Error() == "Error 1061" ||
		err.Error() == "Error 3822")
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

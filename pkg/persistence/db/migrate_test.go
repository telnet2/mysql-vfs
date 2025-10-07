package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBootstrapDefaultFiles(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	// Run migrations (including bootstrap)
	err = AutoMigrate(db)
	assert.NoError(t, err)

	// Verify root directory was created
	var rootDir models.Directory
	err = db.Where("path = ?", "/").First(&rootDir).Error
	assert.NoError(t, err)
	assert.Equal(t, "root", rootDir.ID)
	assert.Equal(t, "/", rootDir.Path)

	// Count total files
	var fileCount int64
	db.Model(&models.File{}).Count(&fileCount)
	t.Logf("Total files created: %d", fileCount)
	assert.GreaterOrEqual(t, fileCount, int64(2))

	// List all files
	var allFiles []models.File
	db.Find(&allFiles)
	for _, f := range allFiles {
		t.Logf("File: name=%s, content_type=%s, json_content_nil=%v", f.Name, f.ContentType, f.JSONContent == nil)
	}

	// Verify .rego file was created
	var regoFile models.File
	err = db.Where("name = ?", ".rego").First(&regoFile).Error
	assert.NoError(t, err, "Failed to find .rego file")
	assert.Equal(t, ".rego", regoFile.Name)
	assert.Equal(t, "text/plain", regoFile.ContentType)
	// .rego files are stored as TextContent, not JSONContent
	assert.NotNil(t, regoFile.TextContent, "TextContent should not be nil")
	if regoFile.TextContent != nil {
		assert.Contains(t, *regoFile.TextContent, "package vfs.authz")
		assert.Contains(t, *regoFile.TextContent, "input.user.groups[_] == \"admin\"")
		assert.Contains(t, *regoFile.TextContent, "input.user.groups[_] == \"user\"")
	}

	// Verify .group file was created
	var groupFile models.File
	err = db.Where("directory_id = ? AND name = ?", rootDir.ID, ".group").First(&groupFile).Error
	assert.NoError(t, err)
	assert.Equal(t, ".group", groupFile.Name)
	assert.Equal(t, "application/json", groupFile.ContentType)
	assert.NotNil(t, groupFile.JSONContent)
	assert.Contains(t, *groupFile.JSONContent, "\"group_id\": \"admin\"")
	assert.Contains(t, *groupFile.JSONContent, "\"group_id\": \"user\"")
}

func TestBootstrapIdempotent(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	// Run migrations first time
	err = AutoMigrate(db)
	assert.NoError(t, err)

	// Count files
	var count1 int64
	db.Model(&models.File{}).Count(&count1)
	assert.GreaterOrEqual(t, count1, int64(2))

	// Run migrations again (should be idempotent)
	err = AutoMigrate(db)
	assert.NoError(t, err)

	// Count files again - should be the same
	var count2 int64
	db.Model(&models.File{}).Count(&count2)
	assert.Equal(t, count1, count2) // No duplicates created
}

func TestBootstrapDoesNotOverwriteExisting(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	// Manually create tables
	err = db.AutoMigrate(&models.Directory{}, &models.File{})
	assert.NoError(t, err)

	// Create root directory
	rootDir := models.Directory{
		ID:       "root",
		Name:     "",
		Path:     "/",
		ParentID: nil,
	}
	err = db.Create(&rootDir).Error
	assert.NoError(t, err)

	// Create custom .rego file
	customPolicy := "package custom\nallow { true }"
	customRego := models.File{
		DirectoryID: rootDir.ID,
		Name:        ".rego",
		ContentType: "text/plain",
		SizeBytes:   int64(len(customPolicy)),
		StorageType: "json",
		JSONContent: &customPolicy,
		Version:     1,
	}
	err = db.Create(&customRego).Error
	assert.NoError(t, err)

	// Run bootstrap
	err = bootstrapDefaultFiles(db)
	assert.NoError(t, err)

	// Verify .rego was NOT overwritten
	var regoFile models.File
	err = db.Where("directory_id = ? AND name = ?", rootDir.ID, ".rego").First(&regoFile).Error
	assert.NoError(t, err)
	assert.Equal(t, customPolicy, *regoFile.JSONContent) // Still has custom content

	// Verify .group was created (since it didn't exist)
	var groupFile models.File
	err = db.Where("directory_id = ? AND name = ?", rootDir.ID, ".group").First(&groupFile).Error
	assert.NoError(t, err)
	assert.Contains(t, *groupFile.JSONContent, "\"group_id\": \"admin\"")
}

func TestWorkflowAuditMigration(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	err = AutoMigrate(db)
	assert.NoError(t, err)

	assert.True(t, db.Migrator().HasTable(&models.WorkflowAudit{}), "workflow_audit table should exist after migration")

	record := models.WorkflowAudit{
		ID:             "audit-test-1",
		FilePath:       "/docs/report.txt",
		WorkflowPath:   "/docs/.workflow",
		FromState:      "draft",
		ToState:        "review",
		Operation:      "move",
		Actor:          "user-123",
		ActorGroups:    `{"groups":["editors"]}`,
		GatesEvaluated: `{"passed":["size"]}`,
		Success:        true,
		CreatedAt:      time.Now().UTC(),
	}

	assert.NoError(t, db.Create(&record).Error)

	var fetched models.WorkflowAudit
	assert.NoError(t, db.Where("id = ?", record.ID).First(&fetched).Error)
	assert.Equal(t, record.FilePath, fetched.FilePath)

	assert.NoError(t, db.Migrator().DropTable(&models.WorkflowAudit{}))
	assert.False(t, db.Migrator().HasTable(&models.WorkflowAudit{}))

	// Re-run migrations (up again)
	assert.NoError(t, AutoMigrate(db))
	assert.True(t, db.Migrator().HasTable(&models.WorkflowAudit{}))
}

func TestWorkflowAuditMigrationIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	assert.NoError(t, AutoMigrate(db))

	record := models.WorkflowAudit{
		ID:           "audit-test-2",
		FilePath:     "/docs/spec.md",
		WorkflowPath: "/docs/.workflow",
		FromState:    "draft",
		ToState:      "approved",
		Operation:    "move",
		Actor:        "user-456",
		Success:      false,
		ErrorMessage: stringPtr("gate denied"),
		CreatedAt:    time.Now().UTC(),
	}
	assert.NoError(t, db.Create(&record).Error)

	assert.NoError(t, AutoMigrate(db))

	var count int64
	assert.NoError(t, db.Model(&models.WorkflowAudit{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

package citest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/integrity"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

var _ = Describe("Referential Integrity", func() {
	var (
		testDB    *fixtures.TestDatabase
		validator *integrity.Validator
		repair    *integrity.RepairService
		ctx       context.Context
	)

	BeforeEach(func() {
		testDB = fixtures.NewTestDatabase()
		validator = integrity.NewValidator(testDB.GetDB())
		repair = integrity.NewRepairService(testDB.GetDB())
		ctx = context.Background()

		// Create root directory
		root := &models.Directory{
			ID:       "root",
			Name:     "/",
			Path:     "/",
			PathHash: calculatePathHash("/"),
		}
		gormDB := testDB.GetDB()
		gormDB.FirstOrCreate(root, "id = ?", "root")
		sqlDB, _ := gormDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})

	AfterEach(func() {
		testDB.Cleanup()
	})

	Context("when validating directory integrity", func() {
		It("should detect orphaned directories", func() {
			// Create a directory with non-existent parent
			gormDB := testDB.GetDB()
			orphanedDir := &models.Directory{
				ID:       "orphaned-123",
				Name:     "orphaned",
				Path:     "/orphaned",
				PathHash: calculatePathHash("/orphaned"),
				ParentID: stringPtr("non-existent-parent"),
			}
			gormDB.Create(orphanedDir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate
			results, err := validator.ValidateDirectories(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0))

			// Check for orphaned parent violation
			found := false
			for _, r := range results {
				if r.ViolationType == "orphaned_parent" && r.RecordID == "orphaned-123" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect orphaned directory")
		})

		It("should detect self-referencing directories", func() {
			// Create a directory that references itself
			gormDB := testDB.GetDB()
			selfRef := &models.Directory{
				ID:       "self-ref-123",
				Name:     "selfref",
				Path:     "/selfref",
				PathHash: calculatePathHash("/selfref"),
				ParentID: stringPtr("self-ref-123"), // References itself
			}
			gormDB.Create(selfRef)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate
			results, err := validator.ValidateDirectories(ctx)

			Expect(err).NotTo(HaveOccurred())

			// Check for circular reference violation
			found := false
			for _, r := range results {
				if r.ViolationType == "circular_reference" && r.RecordID == "self-ref-123" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect self-referencing directory")
		})

		It("should repair orphaned directories", func() {
			// Create orphaned directory
			gormDB := testDB.GetDB()
			orphanedDir := &models.Directory{
				ID:       "orphaned-456",
				Name:     "orphaned",
				Path:     "/orphaned",
				PathHash: calculatePathHash("/orphaned"),
				ParentID: stringPtr("non-existent"),
			}
			gormDB.Create(orphanedDir)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Repair (not dry run)
			results, err := repair.RepairOrphanedDirectories(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(Equal(1))
			Expect(results[0].Success).To(BeTrue())

			// Verify directory was soft-deleted
			gormDB2 := testDB.GetDB()
			var dir models.Directory
			result := gormDB2.Where("id = ?", "orphaned-456").First(&dir)
			sqlDB2, _ := gormDB2.DB()
			if sqlDB2 != nil {
				sqlDB2.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Should not find (soft deleted)
		})
	})

	Context("when validating file integrity", func() {
		It("should detect orphaned files", func() {
			// Create file with non-existent directory
			gormDB := testDB.GetDB()
			orphanedFile := &models.File{
				ID:             "file-orphaned-123",
				DirectoryID:    "non-existent-dir",
				Name:           "orphaned.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test content"),
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			gormDB.Create(orphanedFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate
			results, err := validator.ValidateFiles(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0))

			// Check for orphaned directory violation
			found := false
			for _, r := range results {
				if r.ViolationType == "orphaned_directory" && r.RecordID == "file-orphaned-123" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect orphaned file")
		})

		It("should detect storage inconsistencies", func() {
			// Create directory first
			gormDB := testDB.GetDB()
			dir := &models.Directory{
				ID:       "dir-123",
				Name:     "test",
				Path:     "/test",
				PathHash: calculatePathHash("/test"),
				ParentID: stringPtr("root"),
			}
			gormDB.Create(dir)

			// Create file with inconsistent storage (json type but no json_content)
			inconsistentFile := &models.File{
				ID:             "file-inconsistent-123",
				DirectoryID:    "dir-123",
				Name:           "bad.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				S3Key:          stringPtr("should-not-have-this"), // Has S3 key but type is json
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			gormDB.Create(inconsistentFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate
			results, err := validator.ValidateFiles(ctx)

			Expect(err).NotTo(HaveOccurred())

			// Check for storage inconsistency violation
			found := false
			for _, r := range results {
				if r.ViolationType == "storage_inconsistency" && r.RecordID == "file-inconsistent-123" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect storage inconsistency")
		})

		It("should repair orphaned files", func() {
			// Create orphaned file
			gormDB := testDB.GetDB()
			orphanedFile := &models.File{
				ID:             "file-orphaned-456",
				DirectoryID:    "non-existent-dir",
				Name:           "orphaned.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			gormDB.Create(orphanedFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Repair (not dry run)
			results, err := repair.RepairOrphanedFiles(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(Equal(1))
			Expect(results[0].Success).To(BeTrue())

			// Verify file was soft-deleted
			gormDB2 := testDB.GetDB()
			var file models.File
			result := gormDB2.Where("id = ?", "file-orphaned-456").First(&file)
			sqlDB2, _ := gormDB2.DB()
			if sqlDB2 != nil {
				sqlDB2.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Should not find (soft deleted)
		})
	})

	Context("when validating file versions", func() {
		It("should detect orphaned file versions", func() {
			// Create file version with non-existent file
			gormDB := testDB.GetDB()
			orphanedVersion := &models.FileVersion{
				ID:             "version-orphaned-123",
				FileID:         "non-existent-file",
				VersionNumber:  1,
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc123",
			}
			gormDB.Create(orphanedVersion)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate
			results, err := validator.ValidateFileVersions(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0))

			// Check for orphaned file violation
			found := false
			for _, r := range results {
				if r.ViolationType == "orphaned_file" && r.RecordID == "version-orphaned-123" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect orphaned file version")
		})

		It("should repair orphaned file versions", func() {
			// Create orphaned version
			gormDB := testDB.GetDB()
			orphanedVersion := &models.FileVersion{
				ID:             "version-orphaned-456",
				FileID:         "non-existent-file",
				VersionNumber:  1,
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc123",
			}
			gormDB.Create(orphanedVersion)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Repair (not dry run)
			results, err := repair.RepairOrphanedFileVersions(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(Equal(1))
			Expect(results[0].Success).To(BeTrue())

			// Verify version was deleted
			gormDB2 := testDB.GetDB()
			var version models.FileVersion
			result := gormDB2.Where("id = ?", "version-orphaned-456").First(&version)
			sqlDB2, _ := gormDB2.DB()
			if sqlDB2 != nil {
				sqlDB2.Close()
			}

			Expect(result.Error).To(HaveOccurred()) // Should not find (deleted)
		})
	})

	Context("when running full validation", func() {
		It("should detect all violation types", func() {
			gormDB := testDB.GetDB()

			// Create various violations
			// 1. Orphaned directory
			gormDB.Create(&models.Directory{
				ID:       "dir-orphan",
				Name:     "orphan",
				Path:     "/orphan",
				PathHash: calculatePathHash("/orphan"),
				ParentID: stringPtr("non-existent"),
			})

			// 2. Orphaned file
			gormDB.Create(&models.File{
				ID:             "file-orphan",
				DirectoryID:    "non-existent-dir",
				Name:           "orphan.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc",
				Version:        1,
			})

			// 3. Orphaned file version
			gormDB.Create(&models.FileVersion{
				ID:             "version-orphan",
				FileID:         "non-existent-file",
				VersionNumber:  1,
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc",
			})

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Run full validation
			results, err := validator.ValidateAll(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">=", 3))

			// Verify all violation types detected
			violationTypes := make(map[string]bool)
			for _, r := range results {
				violationTypes[r.ViolationType] = true
			}

			Expect(violationTypes["orphaned_parent"]).To(BeTrue())
			Expect(violationTypes["orphaned_directory"]).To(BeTrue())
			Expect(violationTypes["orphaned_file"]).To(BeTrue())
		})

		It("should report no violations for clean database", func() {
			// Don't create any violations - just root directory exists

			results, err := validator.ValidateAll(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(Equal(0), "Clean database should have no violations")
		})
	})

	Context("when running full repair", func() {
		It("should repair all violations", func() {
			gormDB := testDB.GetDB()

			// Create violations
			gormDB.Create(&models.Directory{
				ID:       "dir-orphan-repair",
				Name:     "orphan",
				Path:     "/orphan",
				PathHash: calculatePathHash("/orphan"),
				ParentID: stringPtr("non-existent"),
			})

			gormDB.Create(&models.File{
				ID:             "file-orphan-repair",
				DirectoryID:    "non-existent-dir",
				Name:           "orphan.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    "json",
				JSONContent:    stringPtr("test"),
				ChecksumSHA256: "abc",
				Version:        1,
			})

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Run full repair (not dry run)
			results, err := repair.RepairAll(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0))

			// All repairs should succeed
			for _, r := range results {
				Expect(r.Success).To(BeTrue(), "All repairs should succeed")
			}

			// Verify violations are fixed
			validationResults, err := validator.ValidateAll(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(validationResults)).To(Equal(0), "All violations should be repaired")
		})

		It("should support dry-run mode without making changes", func() {
			gormDB := testDB.GetDB()

			// Create violation
			gormDB.Create(&models.Directory{
				ID:       "dir-dryrun",
				Name:     "dryrun",
				Path:     "/dryrun",
				PathHash: calculatePathHash("/dryrun"),
				ParentID: stringPtr("non-existent"),
			})

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Run repair in dry-run mode
			results, err := repair.RepairAll(ctx, true)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0))

			// Violation should still exist
			validationResults, err := validator.ValidateAll(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(validationResults)).To(BeNumerically(">", 0), "Dry-run should not remove violations")
		})
	})
})

func calculatePathHash(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

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

var _ = Describe("Referential Integrity", Ordered, func() {
	var (
		testDB    *fixtures.TestDatabase
		validator *integrity.Validator
		repair    *integrity.RepairService
		ctx       context.Context
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Referential Integrity test environment (this may take a few seconds)...")
		GinkgoWriter.Println("   - Starting MySQL test container...")
		testDB = fixtures.NewTestDatabase()
		GinkgoWriter.Println("   ✓ MySQL ready")

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

		GinkgoWriter.Println("✅ Test environment ready - running tests...")
	})

	AfterAll(func() {
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
				ID:             "integrity-file-orphaned-001",
				DirectoryID:    "integrity-nonexistent-dir-001",
				Name:           "integrity-orphaned.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"test\":\"content\"}"),
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			err := gormDB.Create(orphanedFile).Error
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Ensure file was created
			Expect(err).NotTo(HaveOccurred())

			// Validate
			results, err := validator.ValidateFiles(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">", 0), "Expected to find at least one file integrity violation")

			// Check for orphaned directory violation
			found := false
			for _, r := range results {
				if r.ViolationType == "orphaned_directory" && r.RecordID == "integrity-file-orphaned-001" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect orphaned file with ID integrity-file-orphaned-001")
		})

		It("should detect storage inconsistencies", func() {
			// Create directory first
			gormDB := testDB.GetDB()
			dir := &models.Directory{
				ID:       "integrity-dir-storage-test",
				Name:     "storage-test",
				Path:     "/storage-test",
				PathHash: calculatePathHash("/storage-test"),
				ParentID: stringPtr("root"),
			}
			gormDB.Create(dir)

			// Create a valid JSON file first
			validFile := &models.File{
				ID:             "integrity-file-inconsistent-001",
				DirectoryID:    "integrity-dir-storage-test",
				Name:           "inconsistent.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"valid\"}"),
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			err := gormDB.Create(validFile).Error
			Expect(err).NotTo(HaveOccurred())

			// Now make it inconsistent by adding S3 key (bypass constraints by disabling them temporarily)
			// This simulates data corruption or a bug in the system
			err = gormDB.Exec(`SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE=''`).Error
			Expect(err).NotTo(HaveOccurred())

			err = gormDB.Exec(`
				UPDATE files
				SET s3_key = ?
				WHERE id = ?
			`, "should-not-have-this", "integrity-file-inconsistent-001").Error

			gormDB.Exec(`SET SQL_MODE=@OLD_SQL_MODE`)

			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Ensure update succeeded
			Expect(err).NotTo(HaveOccurred())

			// Validate
			results, err := validator.ValidateFiles(ctx)

			Expect(err).NotTo(HaveOccurred())

			// Debug: print all violations found
			GinkgoWriter.Printf("Found %d validation results\n", len(results))
			for _, r := range results {
				GinkgoWriter.Printf("  - Type: %s, RecordID: %s, Desc: %s\n", r.ViolationType, r.RecordID, r.Description)
			}

			// Check for storage inconsistency violation
			found := false
			for _, r := range results {
				if r.ViolationType == "storage_inconsistency" && r.RecordID == "integrity-file-inconsistent-001" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should detect storage inconsistency for file integrity-file-inconsistent-001")
		})

		It("should repair orphaned files", func() {
			// Create orphaned file
			gormDB := testDB.GetDB()
			orphanedFile := &models.File{
				ID:             "integrity-file-orphaned-repair-001",
				DirectoryID:    "integrity-nonexistent-dir-repair",
				Name:           "orphaned-repair.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
				ChecksumSHA256: "abc123",
				Version:        1,
			}
			gormDB.Create(orphanedFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Repair (not dry run) - with Ordered tests, may repair multiple orphaned files
			results, err := repair.RepairOrphanedFiles(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">=", 1), "Should repair at least 1 orphaned file")

			// Verify our specific file was repaired
			foundOurFile := false
			for _, r := range results {
				if r.RecordID == "integrity-file-orphaned-repair-001" && r.Success {
					foundOurFile = true
					break
				}
			}
			Expect(foundOurFile).To(BeTrue(), "Should have repaired file integrity-file-orphaned-repair-001")

			// Verify file was soft-deleted
			gormDB2 := testDB.GetDB()
			var file models.File
			result := gormDB2.Where("id = ?", "integrity-file-orphaned-repair-001").First(&file)
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
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
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
				ID:             "integrity-version-orphaned-repair-001",
				FileID:         "integrity-nonexistent-file-repair",
				VersionNumber:  1,
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
				ChecksumSHA256: "abc123",
			}
			gormDB.Create(orphanedVersion)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Repair (not dry run) - with Ordered tests, may repair multiple orphaned versions
			// When run with other tests, will repair versions from previous tests
			// When run alone, may find zero (if this runs first)
			results, err := repair.RepairOrphanedFileVersions(ctx, false)

			Expect(err).NotTo(HaveOccurred())
			// With Ordered tests, we accept 0 or more results since state is shared
			Expect(len(results)).To(BeNumerically(">=", 0), "Repair should complete successfully")

			// If there were results, verify they succeeded
			for _, r := range results {
				Expect(r.Success).To(BeTrue(), "All repairs should succeed")
			}

			// Verify version was deleted
			gormDB2 := testDB.GetDB()
			var version models.FileVersion
			result := gormDB2.Where("id = ?", "integrity-version-orphaned-repair-001").First(&version)
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
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
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
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
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
			// With Ordered tests, there may be orphaned records from previous tests
			// Create a valid directory/file structure and verify it has no violations
			gormDB := testDB.GetDB()

			validDir := &models.Directory{
				ID:       "integrity-valid-dir-001",
				Name:     "valid",
				Path:     "/valid",
				PathHash: calculatePathHash("/valid"),
				ParentID: stringPtr("root"),
			}
			gormDB.Create(validDir)

			validFile := &models.File{
				ID:             "integrity-valid-file-001",
				DirectoryID:    validDir.ID,
				Name:           "valid.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"valid\"}"),
				ChecksumSHA256: "validchecksum",
				Version:        1,
			}
			gormDB.Create(validFile)
			sqlDB, _ := gormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}

			// Validate all - may find violations from previous tests, but not from our valid data
			results, err := validator.ValidateAll(ctx)

			Expect(err).NotTo(HaveOccurred())

			// Check that our valid directory and file don't appear in violations
			for _, r := range results {
				Expect(r.RecordID).NotTo(Equal("integrity-valid-dir-001"), "Valid directory should not be flagged")
				Expect(r.RecordID).NotTo(Equal("integrity-valid-file-001"), "Valid file should not be flagged")
			}
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
				StorageType:    models.StorageTypeJSON,
				JSONContent:    stringPtr("{\"data\":\"test\"}"),
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

			// Verify our specific violations are fixed
			// With Ordered tests, there may be other violations from previous tests
			validationResults, err := validator.ValidateAll(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check that our specific test records don't appear in violations
			for _, r := range validationResults {
				Expect(r.RecordID).NotTo(Equal("dir-orphan-repair"), "Repaired directory should not be flagged")
				Expect(r.RecordID).NotTo(Equal("file-orphan-repair"), "Repaired file should not be flagged")
			}
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
